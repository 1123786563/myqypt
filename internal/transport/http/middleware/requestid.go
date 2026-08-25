// Package middleware provides the HTTP transport middleware assembled by
// httptransport.NewRouter: request-ID correlation, security headers with
// restricted CORS, access logging, and panic recovery.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
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

// validRequestID gates which inbound X-Request-ID values are reused: after
// trimming surrounding whitespace, 1-64 characters from the letters, digits,
// '-' and '_'. Anything else — including oversized or malformed values — is
// replaced by a freshly generated ID, so the platform never propagates a
// caller-controlled identifier that other systems would reject.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9-_]{1,64}$`)

// RequestID returns middleware that resolves a request ID per request: an
// inbound X-Request-ID header that survives validation is reused verbatim,
// otherwise a random hex ID is generated. The ID is stored in the gin
// context and echoed on the response header. It deliberately does no logging
// or metrics.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(HeaderRequestID))
		if !validRequestID.MatchString(id) {
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
