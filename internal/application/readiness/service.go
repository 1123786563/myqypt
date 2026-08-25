// Package readiness evaluates dependency health for the /readyz endpoint.
// A service is ready only when every configured check passes; any failure,
// timeout, or blocking checker fails closed.
package readiness

import (
	"context"
	"maps"
	"slices"
	"time"
)

// Checker reports whether one named dependency is currently usable. The
// returned error is consumed as a boolean signal only and its text is
// discarded before it can reach a Result or an HTTP response.
type Checker interface {
	Check(ctx context.Context) error
}

// Service runs a fixed set of named dependency checks.
//
// Timeout bounds each individual check; when zero a package default is
// used. A Service with no checks is vacuously ready.
type Service struct {
	Checks  map[string]Checker
	Timeout time.Duration
}

// Result is the readiness outcome. Ready drives the HTTP status; Checks
// maps each check name to exactly "ok" or "failed" and never contains
// error text, DSNs, or hostnames.
type Result struct {
	Ready  bool
	Checks map[string]string
}

const (
	stateOK     = "ok"
	stateFailed = "failed"

	// defaultTimeout bounds one check when Service.Timeout is unset.
	defaultTimeout = 5 * time.Second
)

// Check evaluates every configured check, each under its own timeout, and
// reports their states. Check names are evaluated and reported in sorted
// order, so the outcome is deterministic. The overall result is ready only
// when every check reports ok.
func (s *Service) Check(ctx context.Context) Result {
	names := slices.Sorted(maps.Keys(s.Checks))
	result := Result{
		Ready:  true,
		Checks: make(map[string]string, len(names)),
	}
	for _, name := range names {
		state := s.checkOne(ctx, s.Checks[name])
		result.Checks[name] = state
		if state != stateOK {
			result.Ready = false
		}
	}
	return result
}

// checkOne runs a single check under a fresh timeout. A checker that fails,
// cancels, or simply never returns before its deadline is failed — a stuck
// checker can never hold readiness hostage.
func (s *Service) checkOne(ctx context.Context, checker Checker) string {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- checker.Check(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			return stateFailed
		}
		return stateOK
	case <-ctx.Done():
		return stateFailed
	}
}
