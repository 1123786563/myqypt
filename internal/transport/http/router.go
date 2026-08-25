package httptransport

import (
	"net/http"

	"github.com/1123786563/myqypt/internal/transport/http/api"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
)

// Dependencies carries the transport-level configuration for NewRouter.
type Dependencies struct {
	Version string
}

// NewRouter builds the platform HTTP transport. The middleware order is:
// request-ID middleware (engine-wide, so every response is correlatable),
// OpenAPI request validation and the strict contract handlers (scoped to the
// contract paths), and finally Problem Details mappings for unmatched routes
// and methods. /livez stays outside the OpenAPI contract and is served
// verbatim.
func NewRouter(deps Dependencies) http.Handler {
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(RequestID())

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
		api.NewStrictHandlerWithOptions(&StatusHandler{Version: deps.Version}, nil, strictServerOptions()),
		api.GinServerOptions{},
	)

	router.GET("/livez", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"status":"alive"}`)
	})

	router.NoRoute(func(c *gin.Context) {
		WriteProblem(c, newProblem(http.StatusNotFound, CodeNotFound))
	})
	router.NoMethod(func(c *gin.Context) {
		WriteProblem(c, newProblem(http.StatusMethodNotAllowed, CodeMethodNotAllowed))
	})
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
