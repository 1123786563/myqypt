package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// HeaderRequestID carries the correlation ID inbound and outbound.
	HeaderRequestID = "X-Request-ID"

	// requestIDContextKey is the unexported gin context key under which the
	// middleware stores the resolved request ID.
	requestIDContextKey = "myqypt.request_id"

	// requestIDHexLength is the length of generated request IDs in hex chars.
	requestIDHexLength = 16
)

// RequestID returns middleware that resolves a request ID per request: a
// non-empty inbound X-Request-ID header is reused verbatim, otherwise a
// random hex ID is generated. The ID is stored in the gin context and echoed
// on the response header. It deliberately does no logging or metrics.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(HeaderRequestID))
		if id == "" {
			id = newRequestID()
		}
		c.Set(requestIDContextKey, id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// RequestIDFromContext returns the request ID stored by the middleware, or
// an empty string when it is absent.
func RequestIDFromContext(c *gin.Context) string {
	if value, ok := c.Get(requestIDContextKey); ok {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}

func newRequestID() string {
	buf := make([]byte, requestIDHexLength/2)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
