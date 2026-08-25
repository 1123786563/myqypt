package httptransport_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

func TestSystemStatusReportsAvailableWithVersion(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "be4cc10"})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type=%q", got)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"status":"available"`) {
		t.Fatalf("body=%q missing %q", body, `"status":"available"`)
	}
	if !strings.Contains(body, `"version":"be4cc10"`) {
		t.Fatalf("body=%q missing %q", body, `"version":"be4cc10"`)
	}
}

// TestNewRouterDefaultsEmptyVersionToDev guards the contract floor: an empty
// Version dependency must default to "dev" (mirroring cmd/platform-api's
// var version = "dev") instead of serving a contract-violating empty string,
// which the SystemStatus schema rejects via version minLength 1.
func TestNewRouterDefaultsEmptyVersionToDev(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !strings.Contains(body, `"version":"dev"`) {
		t.Fatalf("body=%q missing default %q", body, `"version":"dev"`)
	}
}
