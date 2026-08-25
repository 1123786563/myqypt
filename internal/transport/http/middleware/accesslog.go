package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog returns middleware that writes one Info-level record per request
// carrying exactly the allowlisted fields: method, path, status, duration_ms
// and request_id. Durations are whole milliseconds. Headers, query strings
// and bodies are never recorded, so credential values and customer content
// cannot reach the logs. The trace_id field is deliberately absent until
// tracing exists; it arrives with the observability wiring.
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("request_id", RequestIDFromContext(c)),
		)
	}
}
