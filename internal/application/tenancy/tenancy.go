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

// MembershipInvitation is one pending membership invitation (Issue #6,
// T05): the tenant, the non-owner role the invitation grants, the
// invited status, and when the invitation was created. The membership
// row IS the invitation record (design ruling 1), so an invitation that
// is accepted becomes an active membership — never a second row.
type MembershipInvitation struct {
	TenantID  string
	Role      string
	Status    string
	InvitedAt time.Time
}

// ActivatedMembership is the membership after the invitee accepted the
// invitation: the single-row transition invited -> active is complete.
type ActivatedMembership struct {
	TenantID string
	Role     string
	Status   string
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

// ErrInviteeSubjectRequired reports a membership invitation delivered
// without the invitee's external subject: there is no principal to
// address the invitation to.
var ErrInviteeSubjectRequired = errors.New("tenancy: invitee subject required")

// ErrRoleNotSupported reports a membership invitation asking for a role
// that cannot be granted by invitation: owner is not invitable (the
// partial one-active-owner index backs ownership changes being out of
// scope) and unknown role strings are rejected outright.
var ErrRoleNotSupported = errors.New("tenancy: role is not invitable")

// ErrInviterNotAuthorized reports an invitation delivered by an actor
// holding no active owner or admin membership in the tenant — a member,
// a billing_member, a non-member, or a revoked member is
// indistinguishable from an unknown tenant (no existence oracle).
var ErrInviterNotAuthorized = errors.New("tenancy: inviter is not authorized to invite into the tenant")

// ErrInvitationNotFound reports the absence of a usable invitation:
// none was ever delivered, the membership already exists in another
// state, or it belongs to someone else — indistinguishable by design.
var ErrInvitationNotFound = errors.New("tenancy: membership invitation not found")

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

	// InviteMember delivers one membership invitation: the verified
	// inviter names the invitee by inviteeSubject (the invitee's
	// external subject, resolved through identity_bindings by issuer
	// and subject — design ruling 3) for role in tenantID. The inviter
	// must hold an active owner or admin membership in the tenant
	// (design ruling 5); a never-bound inviter or invitee resolves to
	// ErrUserNotBound. Replay convergence rides the natural key
	// (tenant, invitee) — the repository never sees the idempotency
	// key (design ruling 2): the first invitation answers created=true,
	// any repeat delivery of a still-pending invitation answers the
	// same facts with created=false, and an existing membership in any
	// other state classifies as ErrInvitationNotFound (no oracle).
	InviteMember(ctx context.Context, verified identity.VerifiedIdentity, tenantID, inviteeSubject, role string) (MembershipInvitation, bool, error)

	// AcceptInvitation delivers the invitee-only acceptance of the
	// pending invitation of tenantID: the adapter matches the verified
	// identity's platform user against the pending invited row of that
	// tenant (design ruling 6). Not invited, already resolved, or
	// someone else's row classifies as ErrInvitationNotFound with zero
	// writes; success is the single-row transition invited -> active,
	// and replays converge onto the same activation without a second
	// transition.
	AcceptInvitation(ctx context.Context, verified identity.VerifiedIdentity, tenantID string) (ActivatedMembership, error)

	// ActiveMembershipRole resolves the role of the verified identity's
	// platform user's active membership in tenantID (Issue #7, T06): the
	// actor's Platform Role, the key the capability matrix is derived
	// from. A principal with no active membership in the tenant — never
	// a member, revoked, invited-not-accepted, a stranger, or an unknown
	// tenant — classifies as ErrNotAnActiveMember (no existence
	// oracle); a never-bound identity classifies as ErrUserNotBound.
	// The persisted role is one of the four-role memberships CHECK
	// vocabulary; anything else is a contract breach the service
	// classifies as ErrRoleNotSupported (design ruling 5).
	ActiveMembershipRole(ctx context.Context, verified identity.VerifiedIdentity, tenantID string) (string, error)
}
