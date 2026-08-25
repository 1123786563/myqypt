package httptransport

import (
	"net/http"

	"github.com/1123786563/myqypt/internal/transport/http/api"
	"github.com/gin-gonic/gin"
)

// Dependencies carries the transport-level configuration for NewRouter.
type Dependencies struct {
	Version string
}

func NewRouter(deps Dependencies) http.Handler {
	router := gin.New()
	api.RegisterHandlers(router, api.NewStrictHandler(&StatusHandler{Version: deps.Version}, nil))
	router.GET("/livez", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"status":"alive"}`)
	})
	return router
}
