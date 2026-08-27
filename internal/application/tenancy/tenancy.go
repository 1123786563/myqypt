// Package tenancy carries the tenant-context domain: the tenants a user
// can select, the selection itself, and the rules that govern choosing
// and switching (Issue #4, T03).
//
// The vocabulary follows CONTEXT.md: a Tenant Context is the tenant the
// user explicitly selected; the client-submitted tenant id is only a
// selection request — the server validates an active membership before
// persisting and re-validates on every read, so a revocation invalidates
// a persisted selection immediately (ADR 0013: global users and tenant
// lifecycle stay separate; ADR 0009: no cross-tenant sharing).
package tenancy

import (
	"context"
	"errors"
	"time"

	"github.com/1123786563/myqypt/internal/application/identity"
)

// TenantSummary is one selectable tenant of a user: the tenant id, its
// kind, and the membership role the user holds.
type TenantSummary struct {
	TenantID string
	Kind     string
	Role     string
}

// TenantContext is the user's current selection: the selected tenant and
// when the selection was persisted or last switched.
type TenantContext struct {
	TenantID   string
	SelectedAt time.Time
}

// BusinessTenant is one explicitly created business tenant: the tenant id,
// its display name, and when it was created (Issue #5, T04).
type BusinessTenant struct {
	TenantID    string
	DisplayName string
	CreatedAt   time.Time
}

// ErrUserRequired reports an operation delivered without a verified
// identity (empty issuer or subject): the platform user cannot be known.
var ErrUserRequired = errors.New("tenancy: verified user required")

// ErrTenantRequired reports a selection delivered without a tenant id.
var ErrTenantRequired = errors.New("tenancy: tenant required")

// ErrNotAnActiveMember reports a selection of a tenant the user holds no
// active membership in — nonexistent, revoked, or someone else's tenant,
// indistinguishable by design (no existence oracle).
var ErrNotAnActiveMember = errors.New("tenancy: not an active member of the tenant")

// ErrNoTenantContext reports the absence of a valid current selection:
// none was ever persisted, or the persisted one lost its active
// membership to a revocation.
var ErrNoTenantContext = errors.New("tenancy: no tenant context")

// ErrDisplayNameRequired reports a business tenant creation delivered
// without a usable display name (empty or only whitespace).
var ErrDisplayNameRequired = errors.New("tenancy: display name required")

// ErrIdempotencyKeyRequired reports a business tenant creation delivered
// without the retry key header.
var ErrIdempotencyKeyRequired = errors.New("tenancy: idempotency key required")

// ErrUserNotBound reports a business tenant creation delivered by a
// verified identity that was never bound: no platform user exists to
// become the owner. Transport maps it onto the same 404 as any other
// unknown principal — no existence oracle.
var ErrUserNotBound = errors.New("tenancy: verified identity is not bound to a platform user")

// Repository is the persistence port for the tenant-context domain:
// the active-membership tenant list, the re-validated current selection,
// and the validated, transactional selection write.
type Repository interface {
	// ListMembershipTenants returns the tenants the verified identity's
	// platform user holds an active membership in.
	ListMembershipTenants(ctx context.Context, verified identity.VerifiedIdentity) ([]TenantSummary, error)

	// SelectedTenant returns the persisted selection of the verified
	// identity's platform user, re-validated against an active
	// membership: a revoked membership reads as ErrNoTenantContext.
	SelectedTenant(ctx context.Context, verified identity.VerifiedIdentity) (TenantContext, error)

	// SaveSelection persists the explicit selection of tenantID for the
	// verified identity's platform user after validating an active
	// membership, rejecting non-members with ErrNotAnActiveMember and
	// leaving zero rows behind.
	SaveSelection(ctx context.Context, verified identity.VerifiedIdentity, tenantID string) (TenantContext, error)

	// CreateBusinessTenant creates a business tenant owned by the
	// verified identity's platform user — tenant, 1:1 billing customer,
	// and the single active owner membership in one transaction. The
	// idempotencyKey converges retries of the same delivery onto the
	// same tenant: created is true exactly when this call inserted,
	// false on the replay path that loads the existing tenant. A never
	// bound identity is rejected with ErrUserNotBound before any write.
	CreateBusinessTenant(ctx context.Context, verified identity.VerifiedIdentity, displayName, idempotencyKey string) (BusinessTenant, bool, error)
}
