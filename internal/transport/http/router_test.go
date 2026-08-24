package httptransport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/gin-gonic/gin"
)

func TestLivezReportsOnlyProcessLiveness(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	response := httptest.NewRecorder()
	httptransport.NewRouter().ServeHTTP(response, request)
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

	httptransport.NewRouter()

	if got := gin.Mode(); got != gin.TestMode {
		t.Fatalf("gin mode=%q, want %q", got, gin.TestMode)
	}
}
