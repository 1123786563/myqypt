package httptransport

import (
	"github.com/1123786563/myqypt/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// Stable machine-readable problem codes returned by the HTTP transport.
const (
	CodeInvalidRequest   = "invalid_request"
	CodeNotFound         = "not_found"
	CodeMethodNotAllowed = "method_not_allowed"
	CodeInternalError    = "internal_error"
)

// problemTypePrefix is the stable URI namespace for problem types.
const problemTypePrefix = "https://api.myqypt.dev/problems/"

var problemTitles = map[string]string{
	CodeInvalidRequest:   "The request could not be understood.",
	CodeNotFound:         "The requested resource was not found.",
	CodeMethodNotAllowed: "The HTTP method is not allowed for this resource.",
	CodeInternalError:    "An internal server error occurred.",
}

// Problem is the transport's RFC 9457 application/problem+json payload. It
// carries stable, machine-readable codes and never includes internal error
// details: the fields are limited to identity (type, code), presentation
// (title), status, and correlation (trace_id).
type Problem struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Status  int    `json:"status"`
	Code    string `json:"code"`
	TraceID string `json:"trace_id"`
}

// newProblem builds a Problem for a stable code, deriving the type URI and
// the displayable title. TraceID is filled in by WriteProblem.
func newProblem(status int, code string) Problem {
	return Problem{
		Type:   problemTypePrefix + code,
		Title:  problemTitles[code],
		Status: status,
		Code:   code,
	}
}

// WriteProblem emits p as an application/problem+json response and aborts the
// handler chain. Empty Type and TraceID fields are derived: Type from the
// problem code, TraceID from the request-ID middleware.
func WriteProblem(c *gin.Context, p Problem) {
	if p.Type == "" {
		p.Type = problemTypePrefix + p.Code
	}
	if p.TraceID == "" {
		p.TraceID = middleware.RequestIDFromContext(c)
	}
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(p.Status, p)
}
