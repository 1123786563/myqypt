package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/1123786563/myqypt/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// useTestGinMode pins gin to TestMode for one test so route registration
// does not print [GIN-debug] warnings. It mirrors the same-named helpers in
// the transport test files: every test package needs its own copy.
func useTestGinMode(t *testing.T) {
	t.Helper()
	original := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(original)
	})
}

// validRequestIDPattern is the inbound request-ID contract: after trimming
// surrounding whitespace, 1-64 characters from the URL-safe set.
var validRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9-_]{1,64}$`)

func TestRequestIDGeneratedWhenInboundHeaderAbsent(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	got := response.Header().Get(middleware.HeaderRequestID)
	if got == "" {
		t.Fatalf("X-Request-ID header absent on response")
	}
	if !validRequestIDPattern.MatchString(got) {
		t.Fatalf("generated request id=%q violates format %q", got, validRequestIDPattern.String())
	}
}

func TestRequestIDPreservesValidInboundIDs(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	cases := []struct {
		name    string
		inbound string
		want    string
	}{
		// A UUIDv7-shaped inbound ID, as ecosystem callers send.
		{"uuid v7 form", "018f4f70-7c40-7c7e-9f0b-8c7a10b65211", "018f4f70-7c40-7c7e-9f0b-8c7a10b65211"},
		// The platform's own generated form: 16 hex characters.
		{"generated hex form", "0123456789abcdef", "0123456789abcdef"},
		// Whitespace is trimmed before validation, so padding around an
		// otherwise valid ID still preserves the ID.
		{"padded valid form", "  0123456789abcdef  ", "0123456789abcdef"},
		{"dash and underscore allowed", "req_42-Zz", "req_42-Zz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/livez", nil)
			request.Header.Set(middleware.HeaderRequestID, tc.inbound)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			if got := response.Header().Get(middleware.HeaderRequestID); got != tc.want {
				t.Fatalf("X-Request-ID=%q want preserved inbound %q", got, tc.want)
			}
		})
	}
}

func TestRequestIDReplacesInvalidInboundIDs(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	cases := []struct {
		name    string
		inbound string
	}{
		{"too long", strings.Repeat("a", 200)},
		{"characters outside the allowed set", "drop/me?id=1"},
		// Inner whitespace survives the trim, so the ID is invalid.
		{"whitespace-padded garbage", "  ab cd  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/livez", nil)
			request.Header.Set(middleware.HeaderRequestID, tc.inbound)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			got := response.Header().Get(middleware.HeaderRequestID)
			if got == "" {
				t.Fatalf("X-Request-ID header absent on response")
			}
			if got == tc.inbound {
				t.Fatalf("X-Request-ID=%q echoes invalid inbound value", got)
			}
			if !validRequestIDPattern.MatchString(got) {
				t.Fatalf("replacement request id=%q violates format %q", got, validRequestIDPattern.String())
			}
		})
	}
}
