package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

// capturingLogger returns a logger writing text records into a buffer, so
// tests can assert on exactly what the access-log middleware emitted.
func capturingLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func TestAccessLogRecordsAllowlistedFields(t *testing.T) {
	useTestGinMode(t)
	var buf bytes.Buffer
	router := httptransport.NewRouter(httptransport.Dependencies{
		Version: "test",
		Logger:  capturingLogger(&buf),
	})

	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	request.Header.Set("X-Request-ID", "log-trace-7")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	logged := buf.String()
	for _, want := range []string{
		"method=GET",
		"path=/livez",
		"status=200",
		"duration_ms=",
		"request_id=log-trace-7",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("access log=%q missing %q", logged, want)
		}
	}
}

func TestAccessLogNeverLogsCredentialHeaderValues(t *testing.T) {
	useTestGinMode(t)
	var buf bytes.Buffer
	router := httptransport.NewRouter(httptransport.Dependencies{
		Version: "test",
		Logger:  capturingLogger(&buf),
	})

	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	request.Header.Set("Authorization", "Bearer secret-bearer-token-9")
	request.Header.Set("Cookie", "session=secret-cookie-value-9")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	logged := buf.String()
	for _, leaked := range []string{"secret-bearer-token-9", "secret-cookie-value-9", "Authorization", "Cookie"} {
		if strings.Contains(logged, leaked) {
			t.Fatalf("access log=%q leaked %q", logged, leaked)
		}
	}
}

func TestAccessLogAbsentWhenLoggerNil(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}
