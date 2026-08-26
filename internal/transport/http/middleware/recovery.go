package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ProblemWriter renders a stable application/problem+json response for the
// given status and problem code. It is the middleware package's minimal
// rendering contract: the transport injects its own Problem writer, so this
// package never imports the transport that assembles it.
type ProblemWriter func(c *gin.Context, status int, code string)

// Recovery returns middleware that converts a panicking handler into a
// stable 500 internal_error problem via writeProblem. It never re-panics and
// never logs the panic value: the value may carry secrets, and the access
// log already records the status and the correlation ID.
func Recovery(writeProblem ProblemWriter) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				writeProblem(c, http.StatusInternalServerError, "internal_error")
			}
		}()
		c.Next()
	}
}
