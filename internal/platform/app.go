package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

type ReadinessDependency interface {
	Name() string
	CheckReadiness(context.Context) error
}

type Dependencies struct {
	ReadinessDependencies []ReadinessDependency
}

type healthResponse struct {
	Status string `json:"status"`
}

type readinessFailure struct {
	Dependency string `json:"dependency"`
	Message    string `json:"message"`
}

type readinessResponse struct {
	Status   string             `json:"status"`
	Failures []readinessFailure `json:"failures,omitempty"`
}

func New(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "alive"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		failures := checkReadiness(r.Context(), deps.ReadinessDependencies)
		if len(failures) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, readinessResponse{
				Status:   "not_ready",
				Failures: failures,
			})
			return
		}

		writeJSON(w, http.StatusOK, readinessResponse{Status: "ready"})
	})

	return mux
}

func checkReadiness(ctx context.Context, dependencies []ReadinessDependency) []readinessFailure {
	if len(dependencies) == 0 {
		return nil
	}

	failures := make([]readinessFailure, 0, len(dependencies))
	for _, dependency := range dependencies {
		if err := dependency.CheckReadiness(ctx); err != nil {
			failures = append(failures, readinessFailure{
				Dependency: dependency.Name(),
				Message:    err.Error(),
			})
		}
	}

	return failures
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, errors.New("encode response").Error(), http.StatusInternalServerError)
	}
}
