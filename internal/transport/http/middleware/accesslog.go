package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

// AccessLog returns middleware that writes one Info-level record per request
// carrying exactly the allowlisted fields: method, path, status, duration_ms,
// request_id and — when the tracing middleware left a valid span in the
// request context — trace_id. The trace ID (not the span ID) is recorded
// because the access log's correlation role is joining every record and span
// of the same request. Durations are whole milliseconds. Headers, query
// strings and bodies are never recorded, so credential values and customer
// content cannot reach the logs.
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("request_id", RequestIDFromContext(c)),
		}
		if spanContext := trace.SpanContextFromContext(c.Request.Context()); spanContext.IsValid() {
			attrs = append(attrs, slog.String("trace_id", spanContext.TraceID().String()))
		}
		logger.LogAttrs(context.Background(), slog.LevelInfo, "http request", attrs...)
	}
}
