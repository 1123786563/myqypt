package httptransport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/1123786563/myqypt/internal/application/readiness"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

type fixedChecker struct {
	err error
}

func (c fixedChecker) Check(context.Context) error {
	return c.err
}

func readyService(err error) *readiness.Service {
	return &readiness.Service{
		Checks: map[string]readiness.Checker{
			"database": fixedChecker{err: err},
		},
		Timeout: time.Second,
	}
}

func TestReadyzReportsDependencyStates(t *testing.T) {
	// The exact-body assertions double as the no-error-text guarantee: the
	// failing checker's error carries marker text that must never appear.
	tests := []struct {
		name       string
		readiness  *readiness.Service
		wantStatus int
		wantBody   string
	}{
		{
			name:       "all checks ok",
			readiness:  readyService(nil),
			wantStatus: http.StatusOK,
			wantBody:   "{\"checks\":{\"database\":\"ok\"}}",
		},
		{
			name:       "failed check is not ready and never echoes the error",
			readiness:  readyService(errors.New("secret-dial-failure user=postgres password=secret-connection-refused")),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "{\"checks\":{\"database\":\"failed\"}}",
		},
		{
			name:       "no readiness service fails closed",
			readiness:  nil,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "{\"checks\":{}}",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useTestGinMode(t)
			router := httptransport.NewRouter(httptransport.Dependencies{Version: "test", Readiness: test.readiness})

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if got := response.Body.String(); got != test.wantBody {
				t.Fatalf("body = %q, want %q", got, test.wantBody)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
		})
	}
}

func TestLivezStaysAliveWhileReadinessFails(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{
		Version:   "test",
		Readiness: readyService(errors.New("database unreachable")),
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.String(); got != "{\"status\":\"alive\"}" {
		t.Fatalf("body = %q, want %q", got, "{\"status\":\"alive\"}")
	}
}
