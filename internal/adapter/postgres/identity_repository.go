package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IdentityRepository binds verified identities to platform users in
// Postgres, implementing the application identity Repository port.
type IdentityRepository struct {
	pool *pgxpool.Pool
}

var _ identity.Repository = (*IdentityRepository)(nil)

// NewIdentityRepository wraps an existing pool. Opening no connection here
// keeps startup independent of database availability.
func NewIdentityRepository(pool *pgxpool.Pool) *IdentityRepository {
	return &IdentityRepository{pool: pool}
}

// BindOrLoad returns the platform user bound to (identityProvider,
// subject), creating both the user and the binding when none exists yet.
// The created flag reports which path ran: true only on the insert path
// (the load path returns the existing user with false).
//
// Concurrency: a transaction-scoped advisory lock on
// hashtextextended(identity_provider || ':' || subject, 0) serializes
// concurrent deliveries of the same identity, so exactly one transaction
// inserts and the rest load the same user. The user and its binding are
// written in one transaction, leaving no orphan platform_users rows.
func (r *IdentityRepository) BindOrLoad(ctx context.Context, identityProvider, subject string) (identity.User, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: begin identity bind: %w", err)
	}
	defer tx.Rollback(ctx)

	lockKey := identityProvider + ":" + subject
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: lock identity bind: %w", err)
	}

	var userID string
	err = tx.QueryRow(ctx, `
		SELECT platform_user_id::text
		FROM identity_bindings
		WHERE identity_provider = $1 AND subject = $2`,
		identityProvider, subject).Scan(&userID)
	if err == nil {
		return identity.User{ID: userID}, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, false, fmt.Errorf("postgres: load identity binding: %w", err)
	}

	if err := tx.QueryRow(ctx, `
		INSERT INTO platform_users (id) VALUES (gen_random_uuid())
		RETURNING id::text`).Scan(&userID); err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: insert platform user: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO identity_bindings (identity_provider, subject, platform_user_id)
		VALUES ($1, $2, $3)`,
		identityProvider, subject, userID); err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: insert identity binding: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: commit identity bind: %w", err)
	}
	return identity.User{ID: userID}, true, nil
}
