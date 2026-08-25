package httptransport

import (
	"log/slog"
	"net/http"

	"github.com/1123786563/myqypt/internal/application/readiness"
	"github.com/1123786563/myqypt/internal/transport/http/api"
	"github.com/1123786563/myqypt/internal/transport/http/middleware"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
)

// Dependencies carries the transport-level configuration for NewRouter.
// Readiness is optional: when absent, /readyz fails closed (503). Routes is
// a test seam for registering extra routes after the middleware assembly
// (nil keeps production wiring unchanged). Logger enables the access-log
// middleware when non-nil. Security configures CORS; when nil, the security
// headers stay on with CORS disabled (an empty origin allowlist).
type Dependencies struct {
	Version   string
	Readiness *readiness.Service
	Routes    func(*gin.Engine)
	Logger    *slog.Logger
	Security  *middleware.SecurityConfig
}

// defaultVersion mirrors cmd/platform-api's `var version = "dev"` default so
// an unconfigured Dependencies can never serve a contract-violating empty
// version (the SystemStatus schema requires version minLength 1).
const defaultVersion = "dev"

// NewRouter builds the platform HTTP transport. The middleware order is
// fixed: request ID → security headers/CORS → access log (when a Logger is
// injected) → panic recovery, all engine-wide so every response is
// correlatable, hardened and never crashes the connection; the tracing slot
// between security and the access log is reserved for the observability
// wiring. Then come OpenAPI request validation and the strict contract
// handlers (scoped to the contract paths), Problem Details mappings for
// unmatched routes and methods, and finally the Routes seam. /livez and
// /readyz stay outside the OpenAPI contract as operational endpoints:
// /livez is served verbatim, /readyz reports dependency states only.
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
	router.Use(
		middleware.RequestID(),
		middleware.Security(securityConfig),
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
		api.NewStrictHandlerWithOptions(&StatusHandler{Version: version}, nil, strictServerOptions()),
		api.GinServerOptions{},
	)

	router.GET("/livez", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"status":"alive"}`)
	})

	router.GET("/readyz", readinessHandler(deps.Readiness))

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
// they are not part of the stable public contract.
func openAPIValidatorMiddleware(swagger *openapi3.T) gin.HandlerFunc {
	return ginmiddleware.OapiRequestValidatorWithOptions(swagger, &ginmiddleware.Options{
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
