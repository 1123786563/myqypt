package httptransport

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter() http.Handler {
	router := gin.New()
	router.GET("/livez", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"status":"alive"}`)
	})
	return router
}
