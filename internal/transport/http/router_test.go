package httptransport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/gin-gonic/gin"
)

// useTestGinMode pins gin to TestMode for one test so route registration
// does not print [GIN-debug] warnings. It mirrors the same-named helper in
// problem_test.go: the internal and external test packages compile
// separately, so each needs its own copy.
func useTestGinMode(t *testing.T) {
	t.Helper()
	original := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(original)
	})
}

func TestLivezReportsOnlyProcessLiveness(t *testing.T) {
	useTestGinMode(t)
	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	response := httptest.NewRecorder()
	httptransport.NewRouter(httptransport.Dependencies{Version: "test"}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	if got := response.Body.String(); got != "{\"status\":\"alive\"}" {
		t.Fatalf("body=%q", got)
	}
}

func TestNewRouterDoesNotChangeGinMode(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(originalMode)
	})

	httptransport.NewRouter(httptransport.Dependencies{Version: "test"})

	if got := gin.Mode(); got != gin.TestMode {
		t.Fatalf("gin mode=%q, want %q", got, gin.TestMode)
	}
}
