package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/1123786563/myqypt/internal/transport/http/middleware"
)

// assertSecurityHeaders checks the four fixed security response headers on
// any response that passed through the transport middleware.
func assertSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	want := map[string]string{
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "no-referrer",
	}
	for name, value := range want {
		if got := response.Header().Get(name); got != value {
			t.Fatalf("%s=%q want %q", name, got, value)
		}
	}
}

func TestSecurityHeadersPresentOnLivez(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	assertSecurityHeaders(t, response)
}

func TestSecurityHeadersPresentOnNotFoundProblem(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content-type=%q want application/problem+json", got)
	}
	assertSecurityHeaders(t, response)
}

func TestSecurityHeadersPresentOnMethodNotAllowedProblem(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/system/status", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content-type=%q want application/problem+json", got)
	}
	assertSecurityHeaders(t, response)
}

func TestSecurityHeadersPresentOnRecoveredPanic(t *testing.T) {
	useTestGinMode(t)
	router := newPanicRouter(t, httptransport.Dependencies{Version: "test"})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	assertSecurityHeaders(t, response)
}

// corsRouter builds a router whose CORS allowlist admits exactly
// https://app.example.com (and, optionally, carries credentials mode).
func corsRouter(t *testing.T, config middleware.SecurityConfig) http.Handler {
	t.Helper()
	useTestGinMode(t)
	return httptransport.NewRouter(httptransport.Dependencies{
		Version:  "test",
		Security: &config,
	})
}

func preflightRequest(path, origin string) *http.Request {
	request := httptest.NewRequest(http.MethodOptions, path, nil)
	request.Header.Set("Origin", origin)
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	return request
}

func TestCORSPreflightAllowedOriginShortCircuitsWithNoContent(t *testing.T) {
	router := corsRouter(t, middleware.SecurityConfig{
		AllowedOrigins: []string{"https://app.example.com"},
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, preflightRequest("/api/v1/system/status", "https://app.example.com"))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%q want 204", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Access-Control-Allow-Origin=%q want echoed origin", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Fatalf("Access-Control-Allow-Methods=%q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, X-Request-ID" {
		t.Fatalf("Access-Control-Allow-Headers=%q", got)
	}
	if got := response.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary=%q want Origin", got)
	}
	// Credentials mode is off in this configuration.
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Access-Control-Allow-Credentials=%q want absent", got)
	}
	assertSecurityHeaders(t, response)
}

func TestCORSPreflightDeniedOriginGetsNoAllowOriginHeader(t *testing.T) {
	router := corsRouter(t, middleware.SecurityConfig{
		AllowedOrigins: []string{"https://app.example.com"},
	})

	responses := []*httptest.ResponseRecorder{
		func() *httptest.ResponseRecorder {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, preflightRequest("/api/v1/system/status", "https://evil.example"))
			return response
		}(),
		func() *httptest.ResponseRecorder {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/livez", nil)
			request.Header.Set("Origin", "https://evil.example")
			router.ServeHTTP(response, request)
			return response
		}(),
	}
	for i, response := range responses {
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("response %d: Access-Control-Allow-Origin=%q want absent for denied origin", i, got)
		}
		if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("response %d: Access-Control-Allow-Credentials=%q want absent for denied origin", i, got)
		}
	}
}

func TestCORSSimpleAllowedRequestEchoesOrigin(t *testing.T) {
	router := corsRouter(t, middleware.SecurityConfig{
		AllowedOrigins: []string{"https://app.example.com"},
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	request.Header.Set("Origin", "https://app.example.com")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Access-Control-Allow-Origin=%q want echoed origin", got)
	}
	if got := response.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary=%q want Origin", got)
	}
}

func TestCORSCredentialsModeEchoesOriginNeverWildcard(t *testing.T) {
	router := corsRouter(t, middleware.SecurityConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowCredentials: true,
	})

	preflight := httptest.NewRecorder()
	router.ServeHTTP(preflight, preflightRequest("/api/v1/system/status", "https://app.example.com"))
	simple := httptest.NewRecorder()
	simpleRequest := httptest.NewRequest(http.MethodGet, "/livez", nil)
	simpleRequest.Header.Set("Origin", "https://app.example.com")
	router.ServeHTTP(simple, simpleRequest)

	for name, response := range map[string]*httptest.ResponseRecorder{"preflight": preflight, "simple": simple} {
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" || got == "*" {
			t.Fatalf("%s: Access-Control-Allow-Origin=%q want exact origin, never * in credentials mode", name, got)
		}
		if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Fatalf("%s: Access-Control-Allow-Credentials=%q want true", name, got)
		}
	}
}

func TestCORSWildcardWithoutCredentialsAllowed(t *testing.T) {
	router := corsRouter(t, middleware.SecurityConfig{
		AllowedOrigins: []string{"*"},
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, preflightRequest("/api/v1/system/status", "https://any.example"))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%q want 204", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin=%q want *", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Access-Control-Allow-Credentials=%q want absent", got)
	}
}

// TestCORSWildcardWithCredentialsRejectedAtConstruction pins the fail-fast
// contract: the wildcard + credentials combination can never serve, because
// credentialed responses must name origins exactly.
func TestCORSWildcardWithCredentialsRejectedAtConstruction(t *testing.T) {
	useTestGinMode(t)
	defer func() {
		if recover() == nil {
			t.Fatal("expected router construction to reject wildcard origins combined with credentials")
		}
	}()
	httptransport.NewRouter(httptransport.Dependencies{
		Version: "test",
		Security: &middleware.SecurityConfig{
			AllowedOrigins:   []string{"*"},
			AllowCredentials: true,
		},
	})
}

// TestCORSDisabledWhenSecurityConfigNil pins the default wiring: without a
// Security dependency the security headers stay on while CORS stays off.
func TestCORSDisabledWhenSecurityConfigNil(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, preflightRequest("/api/v1/system/status", "https://any.example"))

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin=%q want absent with nil Security config", got)
	}
	assertSecurityHeaders(t, response)
}
