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
// inserts and the rest load the same user.
//
// Provisioning: the insert path also creates the new user's personal
// tenant bundle — one personal tenant, its 1:1 billing customer, and the
// active owner membership — inside the same transaction, so a failure
// anywhere in provisioning rolls back the user and binding with it and a
// committed create always leaves the complete bundle behind.
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

	// Provision the personal tenant bundle (T02): the first delivery of
	// a new identity gains exactly one personal tenant, its 1:1 billing
	// customer, and the owner membership. The uniqueness of the bundle
	// per user is enforced by the schema (partial unique indexes), and
	// replays never reach this code: they load the binding above.
	var tenantID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO tenants (id, owner_user_id, kind)
		VALUES (gen_random_uuid(), $1, 'personal')
		RETURNING id::text`, userID).Scan(&tenantID); err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: insert personal tenant: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing_customers (id, tenant_id)
		VALUES (gen_random_uuid(), $1)`, tenantID); err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: insert billing customer: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role)
		VALUES (gen_random_uuid(), $1, $2, 'owner')`, tenantID, userID); err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: insert owner membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return identity.User{}, false, fmt.Errorf("postgres: commit identity bind: %w", err)
	}
	return identity.User{ID: userID}, true, nil
}
