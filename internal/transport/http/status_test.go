package httptransport_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

func TestSystemStatusReportsAvailableWithVersion(t *testing.T) {
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "be4cc10"})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
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
