package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
	"github.com/jackc/pgx/v5"
)

// CreateBusinessTenant delivers the explicit creation of a business
// tenant owned by the verified identity's platform user (Issue #5, T04).
//
// The whole operation runs in one transaction (design ruling 4): the
// identity is resolved to its platform user first — a never-bound
// identity has no user to become the owner and is rejected with
// ErrUserNotBound before any write — then a transaction-scoped advisory
// lock on hashtextextended(user_id || ':' || idempotency_key, 0)
// serializes concurrent deliveries of the same (user, key), and the
// creation mapping is checked: a hit returns the existing tenant with
// created=false; a miss provisions the bundle — the business tenant with
// its display name, the 1:1 billing customer (ADR 0004), and the single
// active owner membership (the partial unique index of migration 000003
// backs the single-owner invariant) — records the creation mapping, and
// commits with created=true. Any failure anywhere rolls the whole bundle
// back and leaves zero rows behind. A user may hold any number of
// business tenants; accidental duplicates are prevented by the
// idempotency key, not by a per-user cap.
func (r *TenancyRepository) CreateBusinessTenant(ctx context.Context, verified identity.VerifiedIdentity, displayName, idempotencyKey string) (tenancy.BusinessTenant, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tenancy.BusinessTenant{}, false, fmt.Errorf("postgres: begin business tenant creation: %w", err)
	}
	defer tx.Rollback(ctx)

	userID, err := r.boundUserID(ctx, tx, verified)
	if err != nil {
		return tenancy.BusinessTenant{}, false, err
	}
	if userID == "" {
		return tenancy.BusinessTenant{}, false, tenancy.ErrUserNotBound
	}

	lockKey := userID + ":" + idempotencyKey
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return tenancy.BusinessTenant{}, false, fmt.Errorf("postgres: lock business tenant creation: %w", err)
	}

	var replayed tenancy.BusinessTenant
	err = tx.QueryRow(ctx, `
		SELECT t.id::text, t.display_name, t.created_at
		FROM business_tenant_creations c
		JOIN tenants t ON t.id = c.tenant_id
		WHERE c.actor_user_id = $1::uuid AND c.idempotency_key = $2`,
		userID, idempotencyKey).Scan(&replayed.TenantID, &replayed.DisplayName, &replayed.CreatedAt)
	if err == nil {
		return replayed, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return tenancy.BusinessTenant{}, false, fmt.Errorf("postgres: load business tenant creation: %w", err)
	}

	var created tenancy.BusinessTenant
	if err := tx.QueryRow(ctx, `
		INSERT INTO tenants (id, owner_user_id, kind, display_name)
		VALUES (gen_random_uuid(), $1::uuid, 'business', $2)
		RETURNING id::text, display_name, created_at`,
		userID, displayName).Scan(&created.TenantID, &created.DisplayName, &created.CreatedAt); err != nil {
		return tenancy.BusinessTenant{}, false, fmt.Errorf("postgres: insert business tenant: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing_customers (id, tenant_id)
		VALUES (gen_random_uuid(), $1::uuid)`, created.TenantID); err != nil {
		return tenancy.BusinessTenant{}, false, fmt.Errorf("postgres: insert billing customer: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'owner')`,
		created.TenantID, userID); err != nil {
		return tenancy.BusinessTenant{}, false, fmt.Errorf("postgres: insert owner membership: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO business_tenant_creations (actor_user_id, idempotency_key, tenant_id)
		VALUES ($1::uuid, $2, $3::uuid)`,
		userID, idempotencyKey, created.TenantID); err != nil {
		return tenancy.BusinessTenant{}, false, fmt.Errorf("postgres: insert business tenant creation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return tenancy.BusinessTenant{}, false, fmt.Errorf("postgres: commit business tenant creation: %w", err)
	}
	return created, true, nil
}
