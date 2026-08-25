package readiness_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1123786563/myqypt/internal/application/readiness"
)

// stubChecker returns a fixed outcome. The error carries a marker so tests
// can prove the error text never reaches a Result.
type stubChecker struct {
	err error
}

func (s stubChecker) Check(context.Context) error {
	return s.err
}

// blockingChecker blocks until its release channel is closed, ignoring the
// context, so it models a checker stuck past its timeout deterministically.
type blockingChecker struct {
	release chan struct{}
}

func (b blockingChecker) Check(context.Context) error {
	<-b.release
	return nil
}

func TestServiceCheckReportsAllDependencies(t *testing.T) {
	tests := []struct {
		name       string
		checks     map[string]readiness.Checker
		wantReady  bool
		wantChecks map[string]string
	}{
		{
			name: "all healthy",
			checks: map[string]readiness.Checker{
				"cache":    stubChecker{},
				"database": stubChecker{},
			},
			wantReady: true,
			wantChecks: map[string]string{
				"cache":    "ok",
				"database": "ok",
			},
		},
		{
			name: "one failed dependency is not ready",
			checks: map[string]readiness.Checker{
				"cache":    stubChecker{},
				"database": stubChecker{err: errors.New("dial 10.0.0.9:5432: secret-connection-refused")},
			},
			wantReady: false,
			wantChecks: map[string]string{
				"cache":    "ok",
				"database": "failed",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &readiness.Service{Checks: test.checks, Timeout: time.Second}

			got := service.Check(context.Background())

			if got.Ready != test.wantReady {
				t.Fatalf("Ready = %v, want %v", got.Ready, test.wantReady)
			}
			if len(got.Checks) != len(test.wantChecks) {
				t.Fatalf("Checks = %v, want %v", got.Checks, test.wantChecks)
			}
			for name, wantState := range test.wantChecks {
				if got.Checks[name] != wantState {
					t.Fatalf("Checks[%q] = %q, want %q (full: %v)", name, got.Checks[name], wantState, got.Checks)
				}
			}
			for name, state := range got.Checks {
				if state != "ok" && state != "failed" {
					t.Fatalf("Checks[%q] = %q, want state ok or failed (error text must not leak)", name, state)
				}
			}
		})
	}
}

func TestServiceCheckTimesOutBlockedChecker(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	service := &readiness.Service{
		Checks: map[string]readiness.Checker{
			"database": blockingChecker{release: release},
		},
		Timeout: 25 * time.Millisecond,
	}

	started := time.Now()
	got := service.Check(context.Background())
	elapsed := time.Since(started)

	if got.Ready {
		t.Fatal("Ready = true, want false for a timed-out check")
	}
	if state := got.Checks["database"]; state != "failed" {
		t.Fatalf("Checks[database] = %q, want %q", state, "failed")
	}
	// The per-check timeout must bound the whole call: a blocked checker may
	// never hold /readyz hostage.
	if elapsed > 5*time.Second {
		t.Fatalf("Check took %v, want it bounded by the Timeout", elapsed)
	}
}
