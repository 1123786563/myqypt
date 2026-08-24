package platform_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1123786563/myqypt/internal/platform"
)

func TestPlatformRoutes(t *testing.T) {
	handler := platform.New(platform.Dependencies{})

	tests := []struct {
		name string
		path string
		want int
	}{
		{
			name: "livez responds ok",
			path: "/livez",
			want: http.StatusOK,
		},
		{
			name: "readyz responds ok without dependency failures",
			path: "/readyz",
			want: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != tc.want {
				t.Fatalf("path=%s status=%d want=%d", tc.path, response.Code, tc.want)
			}
		})
	}
}

func TestPlatformReadinessFailsWhenDependencyIsUnreachable(t *testing.T) {
	handler := platform.New(platform.Dependencies{
		ReadinessDependencies: []platform.ReadinessDependency{
			stubDependency{
				name: "temporal",
				err:  errors.New("dial timeout"),
			},
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusServiceUnavailable)
	}
}

type stubDependency struct {
	name string
	err  error
}

func (s stubDependency) Name() string {
	return s.name
}

func (s stubDependency) CheckReadiness(context.Context) error {
	return s.err
}
