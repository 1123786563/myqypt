package tenancy

import (
	"context"
	"strings"

	"github.com/1123786563/myqypt/internal/application/identity"
)

// Service carries the tenant-context use cases through the Repository
// port.
type Service struct {
	repository Repository
}

// NewService returns a Service that persists through r.
func NewService(r Repository) *Service {
	return &Service{repository: r}
}

// List returns the tenants the verified identity's platform user holds
// an active membership in. An identity that was never verified end to
// end (empty issuer or subject) is rejected with ErrUserRequired
// without touching the repository.
func (s *Service) List(ctx context.Context, verified identity.VerifiedIdentity) ([]TenantSummary, error) {
	if verified.Issuer == "" || verified.Subject == "" {
		return nil, ErrUserRequired
	}
	return s.repository.ListMembershipTenants(ctx, verified)
}

// Current returns the server-side re-validated selection of the
// verified identity's platform user: absent or invalidated selections
// classify as ErrNoTenantContext. An unverified identity is rejected
// with ErrUserRequired without touching the repository.
func (s *Service) Current(ctx context.Context, verified identity.VerifiedIdentity) (TenantContext, error) {
	if verified.Issuer == "" || verified.Subject == "" {
		return TenantContext{}, ErrUserRequired
	}
	return s.repository.SelectedTenant(ctx, verified)
}

// Select persists the explicit selection of tenantID for the verified
// identity's platform user. The repository validates an active
// membership inside the write transaction; an unverified identity is
// rejected with ErrUserRequired and a missing tenant with
// ErrTenantRequired, both before a single repository call (design
// ruling 4: reject before write).
func (s *Service) Select(ctx context.Context, verified identity.VerifiedIdentity, tenantID string) (TenantContext, error) {
	if verified.Issuer == "" || verified.Subject == "" {
		return TenantContext{}, ErrUserRequired
	}
	if tenantID == "" {
		return TenantContext{}, ErrTenantRequired
	}
	return s.repository.SaveSelection(ctx, verified, tenantID)
}

// CreateBusinessTenant delivers the explicit creation of a business
// tenant for the verified identity's platform user. Every rejection is
// classified before a single repository call (design ruling 2): an
// identity that was never verified end to end with ErrUserRequired, a
// display name that is empty or only whitespace with
// ErrDisplayNameRequired, and a missing idempotency key with
// ErrIdempotencyKeyRequired; the repository's ErrUserNotBound (a
// verified identity with no platform user) flows out unchanged.
func (s *Service) CreateBusinessTenant(ctx context.Context, verified identity.VerifiedIdentity, displayName, idempotencyKey string) (BusinessTenant, bool, error) {
	if verified.Issuer == "" || verified.Subject == "" {
		return BusinessTenant{}, false, ErrUserRequired
	}
	if strings.TrimSpace(displayName) == "" {
		return BusinessTenant{}, false, ErrDisplayNameRequired
	}
	if idempotencyKey == "" {
		return BusinessTenant{}, false, ErrIdempotencyKeyRequired
	}
	return s.repository.CreateBusinessTenant(ctx, verified, displayName, idempotencyKey)
}
