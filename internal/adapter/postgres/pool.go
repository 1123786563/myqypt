package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pool tuning applied on top of any URL-provided configuration.
const (
	maxConnLifetime   = 30 * time.Minute
	maxConnIdleTime   = 5 * time.Minute
	healthCheckPeriod = 1 * time.Minute
)

// Open parses databaseURL and returns a connection pool tuned for the
// platform workload. The pool is built lazily: no connection attempt is made
// here, so an unreachable database is not an Open error. Callers that need to
// fail fast on connectivity must ping explicitly.
//
// A databaseURL that cannot be parsed yields an error that never echoes the
// URL itself (it may contain credentials).
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("postgres: invalid database URL")
	}
	config.MaxConnLifetime = maxConnLifetime
	config.MaxConnIdleTime = maxConnIdleTime
	config.HealthCheckPeriod = healthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	return pool, nil
}
