package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SecurityConfig scopes cross-origin access to an explicit allowlist of
// origins. Exact (byte-for-byte) membership is required after trimming
// surrounding whitespace; a "*" entry admits every origin but can never be
// combined with credentials, because credentialed responses must name the
// origin exactly rather than via the wildcard.
type SecurityConfig struct {
	AllowedOrigins   []string
	AllowCredentials bool
}

// Fixed security response headers applied to every response that passes
// through the transport, independent of CORS configuration.
const (
	hstsHeaderValue = "max-age=31536000; includeSubDomains"
	cspHeaderValue  = "default-src 'none'; frame-ancestors 'none'"
)

// corsAllowMethodsValue and corsAllowHeadersValue are the fixed capability
// answers returned to allowed CORS preflights.
const (
	corsAllowMethodsValue = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowHeadersValue = "Content-Type, X-Request-ID"
)

// Security returns middleware that stamps the fixed security response
// headers on every response and enforces the configured CORS allowlist. A
// request from an allowed origin gets its origin echoed (never a wildcard in
// credentials mode); an allowed preflight is short-circuited with 204. A
// request from any other origin — or any router built with an empty
// allowlist — receives no CORS grant at all: the security headers stay on,
// the cross-origin response stays unreadable. Configuring the wildcard
// allowlist together with credentials is rejected here, at construction,
// rather than silently weakening responses.
func Security(config SecurityConfig) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(config.AllowedOrigins))
	wildcard := false
	for _, origin := range config.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			wildcard = true
			continue
		}
		allowed[origin] = struct{}{}
	}
	if wildcard && config.AllowCredentials {
		panic(`middleware: SecurityConfig "*" AllowedOrigins cannot be combined with AllowCredentials`)
	}

	return func(c *gin.Context) {
		c.Header("Strict-Transport-Security", hstsHeaderValue)
		c.Header("Content-Security-Policy", cspHeaderValue)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")

		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin == "" {
			c.Next()
			return
		}
		// Every origin-bearing response varies by Origin, allowed or not,
		// so shared caches cannot reuse one origin's grant for another.
		c.Header("Vary", "Origin")

		allowOrigin := ""
		if wildcard {
			allowOrigin = "*"
		} else if _, ok := allowed[origin]; ok {
			allowOrigin = origin
		}
		if allowOrigin == "" {
			c.Next()
			return
		}
		c.Header("Access-Control-Allow-Origin", allowOrigin)
		if config.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if isPreflight(c) {
			c.Header("Access-Control-Allow-Methods", corsAllowMethodsValue)
			c.Header("Access-Control-Allow-Headers", corsAllowHeadersValue)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// isPreflight reports whether the request is a CORS preflight: an OPTIONS
// request announcing the intended method.
func isPreflight(c *gin.Context) bool {
	return c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""
}
