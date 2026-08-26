package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/1123786563/myqypt/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// panicMarker is a recognizable secret-like value carried by the panicking
// handler: it must never reach a response or a log line.
const panicMarker = "secret-marker-xyz"

// newPanicRouter builds a router with a /panic route registered through the
// Dependencies.Routes seam, panicking with the marker value.
func newPanicRouter(t *testing.T, deps httptransport.Dependencies) http.Handler {
	t.Helper()
	deps.Routes = func(r *gin.Engine) {
		r.GET("/panic", func(*gin.Context) { panic("boom " + panicMarker) })
	}
	return httptransport.NewRouter(deps)
}

func TestRecoveryReturnsCorrelatedProblemWithoutLeakingPanicValue(t *testing.T) {
	useTestGinMode(t)
	router := newPanicRouter(t, httptransport.Dependencies{Version: "test"})

	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	request.Header.Set("X-Request-ID", "018f4f70-7c40-7c7e-9f0b-8c7a10b65211")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content-type=%q want application/problem+json", got)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"code":"internal_error"`) {
		t.Fatalf("problem body=%q missing code internal_error", body)
	}
	if !strings.Contains(body, `"trace_id":"018f4f70-7c40-7c7e-9f0b-8c7a10b65211"`) {
		t.Fatalf("problem body=%q missing the request's trace id", body)
	}
	if got := response.Header().Get(middleware.HeaderRequestID); got != "018f4f70-7c40-7c7e-9f0b-8c7a10b65211" {
		t.Fatalf("X-Request-ID=%q want echoed inbound id", got)
	}
	if strings.Contains(body, panicMarker) {
		t.Fatalf("problem body=%q leaks panic value marker", body)
	}
}

// TestRecoveryNeverLogsPanicValue pins the minimization ruling: with the
// access log installed, a recovered panic yields a status-500 record that
// still correlates via request_id, while the panic value never reaches the
// log.
func TestRecoveryNeverLogsPanicValue(t *testing.T) {
	useTestGinMode(t)
	var buf bytes.Buffer
	router := newPanicRouter(t, httptransport.Dependencies{
		Version: "test",
		Logger:  slog.New(slog.NewTextHandler(&buf, nil)),
	})

	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	request.Header.Set("X-Request-ID", "trace-panic-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	logged := buf.String()
	if !strings.Contains(logged, "status=500") {
		t.Fatalf("access log=%q missing status=500", logged)
	}
	if !strings.Contains(logged, "request_id=trace-panic-1") {
		t.Fatalf("access log=%q missing request_id", logged)
	}
	if strings.Contains(logged, panicMarker) {
		t.Fatalf("access log=%q leaks panic value marker", logged)
	}
}
