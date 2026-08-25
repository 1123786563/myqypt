package postgres

import (
	"context"
	"errors"

	"github.com/1123786563/myqypt/internal/application/readiness"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthChecker is the readiness checker for the platform database. It
// fails unless the database answers a ping AND the schema_health migration
// marker table exists: a reachable but un-migrated database is not ready.
//
// The returned error is a boolean signal for the readiness service; its
// text stays inside the process and never reaches an HTTP response.
type HealthChecker struct {
	pool *pgxpool.Pool
}

var _ readiness.Checker = (*HealthChecker)(nil)

// NewHealthChecker wraps a lazily-connecting pool. Opening no connection
// here keeps serve startup independent of database availability.
func NewHealthChecker(pool *pgxpool.Pool) *HealthChecker {
	return &HealthChecker{pool: pool}
}

// Check pings the pool and probes the schema_health migration marker.
// count(*) returns exactly one row whenever the relation exists, so the
// marker is the table itself — an empty-but-migrated schema is healthy —
// while a missing table (SQLSTATE 42P01) or any connection failure fails
// the check.
func (h *HealthChecker) Check(ctx context.Context) error {
	if err := h.pool.Ping(ctx); err != nil {
		return err
	}

	var rows int
	return h.pool.QueryRow(ctx, `SELECT count(*) FROM schema_health`).Scan(&rows)
}

// UnconfiguredChecker is the fail-closed database checker used when
// DATABASE_URL is not set: the process keeps serving, but readiness stays
// failed until it is configured.
type UnconfiguredChecker struct{}

var _ readiness.Checker = UnconfiguredChecker{}

// Check always fails: an unconfigured dependency is never ready.
func (UnconfiguredChecker) Check(context.Context) error {
	return errors.New("postgres: database is not configured")
}
