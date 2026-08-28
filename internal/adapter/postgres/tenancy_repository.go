package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenancyRepository persists tenant context selections in Postgres,
// implementing the application tenancy Repository port. Every operation
// resolves the verified identity (issuer, subject) to its platform user
// through identity_bindings first — the ADR 0024 identity key — because a
// bearer token is the only principal the public endpoints ever see.
type TenancyRepository struct {
	pool *pgxpool.Pool
}

var _ tenancy.Repository = (*TenancyRepository)(nil)

// NewTenancyRepository wraps an existing pool. Opening no connection here
// keeps startup independent of database availability.
func NewTenancyRepository(pool *pgxpool.Pool) *TenancyRepository {
	return &TenancyRepository{pool: pool}
}

// tenancyQueryer is the subset both a pool and a transaction share, so
// the identity resolution helper runs inside and outside transactions.
type tenancyQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// boundUserID resolves the verified identity to its platform user id
// inside q. A never-bound identity resolves to the empty string with no
// error: it simply holds no active membership anywhere, so callers treat
// it as "no rows" instead of inventing an unknown-identity oracle.
func (r *TenancyRepository) boundUserID(ctx context.Context, q tenancyQueryer, verified identity.VerifiedIdentity) (string, error) {
	var userID string
	if err := q.QueryRow(ctx, `
		SELECT platform_user_id::text
		FROM identity_bindings
		WHERE identity_provider = $1 AND subject = $2`,
		verified.Issuer, verified.Subject).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("postgres: resolve identity binding: %w", err)
	}
	return userID, nil
}

// ActiveMembershipRole resolves the role of the verified identity's
// platform user's active membership in tenantID (Issue #7, T06) with one
// parameterized SELECT: the platform user is resolved through
// identity_bindings by (issuer, subject) — the ADR 0024 identity key —
// and the LEFT JOIN keeps the two rejection classes apart. A never-bound
// identity answers ErrUserNotBound (no binding row at all); a bound
// principal with no active membership in the tenant — never a member,
// invited but not accepted, revoked, a stranger, or an unknown tenant —
// answers ErrNotAnActiveMember, all indistinguishable from each other
// (no existence oracle). memberships' UNIQUE (tenant_id, user_id) keeps
// the join single-row by construction.
func (r *TenancyRepository) ActiveMembershipRole(ctx context.Context, verified identity.VerifiedIdentity, tenantID string) (string, error) {
	var role sql.NullString
	err := r.pool.QueryRow(ctx, `
		SELECT m.role
		FROM identity_bindings b
		LEFT JOIN memberships m
		       ON m.tenant_id = $3::uuid
		      AND m.user_id = b.platform_user_id
		      AND m.status = 'active'
		WHERE b.identity_provider = $1 AND b.subject = $2`,
		verified.Issuer, verified.Subject, tenantID).Scan(&role)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", tenancy.ErrUserNotBound
	case err != nil:
		return "", fmt.Errorf("postgres: resolve active membership role: %w", err)
	case !role.Valid:
		return "", tenancy.ErrNotAnActiveMember
	}
	return role.String, nil
}

// ListMembershipTenants returns the tenants the verified identity's
// platform user holds an active membership in, with the tenant kind and
// the membership role. Revoked memberships are filtered by the status
// predicate, and a never-bound identity yields an empty list.
func (r *TenancyRepository) ListMembershipTenants(ctx context.Context, verified identity.VerifiedIdentity) ([]tenancy.TenantSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id::text, t.kind, m.role
		FROM identity_bindings b
		JOIN memberships m ON m.user_id = b.platform_user_id
		JOIN tenants t ON t.id = m.tenant_id
		WHERE b.identity_provider = $1 AND b.subject = $2
		  AND m.status = 'active'
		ORDER BY t.id`,
		verified.Issuer, verified.Subject)
	if err != nil {
		return nil, fmt.Errorf("postgres: list membership tenants: %w", err)
	}
	defer rows.Close()

	tenants := make([]tenancy.TenantSummary, 0)
	for rows.Next() {
		var summary tenancy.TenantSummary
		if err := rows.Scan(&summary.TenantID, &summary.Kind, &summary.Role); err != nil {
			return nil, fmt.Errorf("postgres: scan membership tenant: %w", err)
		}
		tenants = append(tenants, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate membership tenants: %w", err)
	}
	return tenants, nil
}

// SelectedTenant returns the persisted selection for the verified
// identity's platform user, re-validated against an active membership
// with a LEFT JOIN: a revoked membership nulls the join and the selection
// reads as absent. No selection row, a never-bound identity, or a
// selection whose membership was revoked all classify as
// ErrNoTenantContext — the persisted row is never deleted on revocation.
func (r *TenancyRepository) SelectedTenant(ctx context.Context, verified identity.VerifiedIdentity) (tenancy.TenantContext, error) {
	var selected tenancy.TenantContext
	err := r.pool.QueryRow(ctx, `
		SELECT s.tenant_id::text, s.selected_at
		FROM identity_bindings b
		JOIN tenant_context_selections s ON s.platform_user_id = b.platform_user_id
		LEFT JOIN memberships m
		       ON m.tenant_id = s.tenant_id
		      AND m.user_id = s.platform_user_id
		      AND m.status = 'active'
		WHERE b.identity_provider = $1 AND b.subject = $2
		  AND m.id IS NOT NULL`,
		verified.Issuer, verified.Subject).Scan(&selected.TenantID, &selected.SelectedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenancy.TenantContext{}, tenancy.ErrNoTenantContext
	}
	if err != nil {
		return tenancy.TenantContext{}, fmt.Errorf("postgres: load tenant context selection: %w", err)
	}
	return selected, nil
}

// SaveSelection persists the explicit selection of tenantID for the
// verified identity's platform user. The whole operation runs in one
// transaction: the user is resolved, a transaction-scoped advisory lock on
// hashtextextended(user_id, 0) serializes concurrent selections of the
// same user, the active membership is validated before any write, and the
// selection is upserted (one row per user; a switch is last-write-wins).
// A user with no active membership in tenantID — including a never-bound
// identity or a nonexistent tenant — is rejected with
// ErrNotAnActiveMember and leaves zero rows behind.
func (r *TenancyRepository) SaveSelection(ctx context.Context, verified identity.VerifiedIdentity, tenantID string) (tenancy.TenantContext, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tenancy.TenantContext{}, fmt.Errorf("postgres: begin tenant context selection: %w", err)
	}
	defer tx.Rollback(ctx)

	userID, err := r.boundUserID(ctx, tx, verified)
	if err != nil {
		return tenancy.TenantContext{}, err
	}
	if userID == "" {
		return tenancy.TenantContext{}, tenancy.ErrNotAnActiveMember
	}

	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return tenancy.TenantContext{}, fmt.Errorf("postgres: lock tenant context selection: %w", err)
	}

	var isActiveMember bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM memberships
			WHERE user_id = $1::uuid AND tenant_id = $2::uuid AND status = 'active'
		)`,
		userID, tenantID).Scan(&isActiveMember); err != nil {
		return tenancy.TenantContext{}, fmt.Errorf("postgres: validate active membership: %w", err)
	}
	if !isActiveMember {
		return tenancy.TenantContext{}, tenancy.ErrNotAnActiveMember
	}

	var selected tenancy.TenantContext
	if err := tx.QueryRow(ctx, `
		INSERT INTO tenant_context_selections (platform_user_id, tenant_id)
		VALUES ($1::uuid, $2::uuid)
		ON CONFLICT (platform_user_id) DO UPDATE
		SET tenant_id = EXCLUDED.tenant_id, selected_at = now()
		RETURNING tenant_id::text, selected_at`,
		userID, tenantID).Scan(&selected.TenantID, &selected.SelectedAt); err != nil {
		return tenancy.TenantContext{}, fmt.Errorf("postgres: upsert tenant context selection: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return tenancy.TenantContext{}, fmt.Errorf("postgres: commit tenant context selection: %w", err)
	}
	return selected, nil
}
