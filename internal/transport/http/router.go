package httptransport

import (
	"log/slog"
	"net/http"

	"github.com/1123786563/myqypt/internal/application/readiness"
	"github.com/1123786563/myqypt/internal/transport/http/api"
	"github.com/1123786563/myqypt/internal/transport/http/middleware"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
	otelgin "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Dependencies carries the transport-level configuration for NewRouter.
// Readiness is optional: when absent, /readyz fails closed (503). Routes is
// a test seam for registering extra routes after the middleware assembly
// (nil keeps production wiring unchanged). Logger enables the access-log
// middleware when non-nil. Security configures CORS; when nil, the security
// headers stay on with CORS disabled (an empty origin allowlist).
// TracerProvider feeds the otelgin tracing middleware; nil selects an
// explicit noop provider (the otel global is never read). Identity
// optionally wires the internal identity callback endpoint: nil leaves the
// route unregistered (unmatched paths get the 404 problem), while a
// non-nil assembly with a missing port fails closed per request. Tenancy
// optionally wires the tenant-context contract endpoints: their paths are
// always registered (the contract surface stays complete), and a nil
// assembly answers every request with the fail-closed 503 problem.
type Dependencies struct {
	Version        string
	Readiness      *readiness.Service
	Routes         func(*gin.Engine)
	Logger         *slog.Logger
	Security       *middleware.SecurityConfig
	TracerProvider trace.TracerProvider
	Identity       *IdentityDependencies
	Tenancy        *TenancyDependencies
}

// contractAPI aggregates every strict handler of the OpenAPI contract
// into the one api.ServerInterface implementation handed to the generated
// registration: the system status endpoint and the tenant-context
// endpoints. The flat registration registers every contract path in one
// group, so per-route handler assemblies are impossible — the aggregate
// is the composition point (design ruling 3).
type contractAPI struct {
	*StatusHandler
	TenancyHandler
}

var _ api.StrictServerInterface = (*contractAPI)(nil)

// defaultVersion mirrors cmd/platform-api's `var version = "dev"` default so
// an unconfigured Dependencies can never serve a contract-violating empty
// version (the SystemStatus schema requires version minLength 1).
const defaultVersion = "dev"

// NewRouter builds the platform HTTP transport. The middleware order is
// fixed: request ID → security headers/CORS → tracing → access log (when a
// Logger is injected) → panic recovery, all engine-wide so every response is
// correlatable, traced, hardened and never crashes the connection. Then come
// OpenAPI request validation and the strict contract handlers (scoped to the
// contract paths), Problem Details mappings for unmatched routes and
// methods, and finally the Routes seam. /livez and /readyz stay outside the
// OpenAPI contract as operational endpoints: /livez is served verbatim,
// /readyz reports dependency states only.
func NewRouter(deps Dependencies) http.Handler {
	version := deps.Version
	if version == "" {
		version = defaultVersion
	}
	router := gin.New()
	router.HandleMethodNotAllowed = true

	securityConfig := middleware.SecurityConfig{}
	if deps.Security != nil {
		securityConfig = *deps.Security
	}
	tracerProvider := deps.TracerProvider
	if tracerProvider == nil {
		tracerProvider = tracenoop.NewTracerProvider()
	}
	router.Use(
		middleware.RequestID(),
		middleware.Security(securityConfig),
		otelgin.Middleware("platform-api",
			// Everything otelgin needs is injected: the tracer provider
			// (an explicit noop instance when Dependencies carries none),
			// a noop meter provider (this issue wires metrics assembly
			// only — no HTTP metric instrumentation), and fixed W3C
			// propagators. The otel globals are never consulted.
			otelgin.WithTracerProvider(tracerProvider),
			otelgin.WithMeterProvider(metricnoop.NewMeterProvider()),
			otelgin.WithPropagators(propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			)),
		),
	)
	if deps.Logger != nil {
		router.Use(middleware.AccessLog(deps.Logger))
	}
	router.Use(middleware.Recovery(func(c *gin.Context, status int, code string) {
		WriteProblem(c, newProblem(status, code))
	}))

	swagger, err := api.GetSpec()
	if err != nil {
		panic(err)
	}

	// The validator is scoped to the contract routes: gin-middleware aborts
	// any request whose path is absent from the spec, so an engine-wide
	// placement would swallow /livez and the NoRoute problem mapping.
	contractRoutes := router.Group("/", openAPIValidatorMiddleware(swagger))
	api.RegisterHandlersWithOptions(
		contractRoutes,
		api.NewStrictHandlerWithOptions(&contractAPI{
			StatusHandler:  &StatusHandler{Version: version},
			TenancyHandler: TenancyHandler{Dependencies: deps.Tenancy},
		}, nil, strictServerOptions()),
		api.GinServerOptions{},
	)

	router.GET("/livez", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"status":"alive"}`)
	})

	router.GET("/readyz", readinessHandler(deps.Readiness))

	// The identity callback is an internal route outside the OpenAPI
	// contract, registered (like /livez) before the NoRoute/NoMethod
	// problem mappings; it stays unregistered entirely when no identity
	// assembly is wired.
	registerIdentityRoutes(router, deps.Identity)

	router.NoRoute(func(c *gin.Context) {
		WriteProblem(c, newProblem(http.StatusNotFound, CodeNotFound))
	})
	router.NoMethod(func(c *gin.Context) {
		WriteProblem(c, newProblem(http.StatusMethodNotAllowed, CodeMethodNotAllowed))
	})

	// The Routes seam registers additional routes after the middleware
	// assembly so black-box tests can exercise the full chain (e.g. a
	// panicking handler). Production wiring leaves it nil.
	if deps.Routes != nil {
		deps.Routes(router)
	}
	return router
}

// openAPIValidatorMiddleware returns the production request-validation
// middleware for the given OpenAPI document. Validation failures map to a
// 400 invalid_request Problem; validator messages are never echoed because
// they are not part of the stable public contract. AuthenticationFunc is
// pinned to kin-openapi's explicit no-op: the contract documents security
// on the tenant-context operations, and without an authentication hook
// kin-openapi would reject every such request with
// ErrAuthenticationServiceMissing — the no-op keeps security declarative
// only, with real authentication running inside the handlers.
func openAPIValidatorMiddleware(swagger *openapi3.T) gin.HandlerFunc {
	return ginmiddleware.OapiRequestValidatorWithOptions(swagger, &ginmiddleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		ErrorHandler: func(c *gin.Context, _ string, _ int) {
			WriteProblem(c, newProblem(http.StatusBadRequest, CodeInvalidRequest))
		},
	})
}

// strictServerOptions wires the generated strict handler's error hooks onto
// Problem Details responses so raw internal error text can never escape:
// request decode failures become 400 invalid_request, handler and response
// failures become 500 internal_error.
func strictServerOptions() api.StrictGinServerOptions {
	return api.StrictGinServerOptions{
		RequestErrorHandlerFunc:  strictRequestErrorHandler,
		HandlerErrorFunc:         strictInternalErrorHandler,
		ResponseErrorHandlerFunc: strictInternalErrorHandler,
	}
}

func strictRequestErrorHandler(c *gin.Context, _ error) {
	WriteProblem(c, newProblem(http.StatusBadRequest, CodeInvalidRequest))
}

func strictInternalErrorHandler(c *gin.Context, _ error) {
	WriteProblem(c, newProblem(http.StatusInternalServerError, CodeInternalError))
}
