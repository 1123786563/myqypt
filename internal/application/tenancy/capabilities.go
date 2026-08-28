package tenancy

import (
	"context"
	"slices"
	"sort"

	"github.com/1123786563/myqypt/internal/application/identity"
)

// RoleCapabilities is the visible capability set of one active
// membership (Issue #7, T06): the tenant, the actor's Platform Role, and
// that role's capabilities served sorted ascending (design ruling 6:
// replay-deterministic bodies).
type RoleCapabilities struct {
	TenantID     string
	Role         string
	Capabilities []string
}

// roleCapabilities is the Platform Role capability matrix (design ruling
// 1): every capability name traces to a CONTEXT.md Platform access
// sentence, verbatim:
//
//   - Owner — "The sole Platform role accountable for a Tenant's
//     ownership, deletion, billing authority, and complete access
//     policy" — holds the full superset: ownership.manage and
//     billing.manage plus every other role's authority.
//   - Admin — "manages Tenant membership, Product purchases,
//     configuration, and Product Access" — membership.manage,
//     purchases.manage, configuration.manage, product_access.manage.
//   - Billing Member — "manages payments and can inspect subscriptions,
//     usage, and bills" — payments.manage, subscriptions.read,
//     usage.read, bills.read.
//   - Member — "can use Products" — product.use.
//
// The four sets are pairwise distinct (AC1) and their union is exactly
// the closed vocabulary; memberships.role (migration 000003 CHECK) is
// the persisted closed role set the matrix must match (design ruling
// 10). Baseline membership operations stay out of the matrix (design
// ruling 4) so the later OpenFGA projection maps 1:1.
var roleCapabilities = map[string][]string{
	"owner": {
		"ownership.manage",
		"billing.manage",
		"membership.manage",
		"configuration.manage",
		"product_access.manage",
		"purchases.manage",
		"payments.manage",
		"subscriptions.read",
		"usage.read",
		"bills.read",
		"product.use",
	},
	"admin": {
		"membership.manage",
		"purchases.manage",
		"configuration.manage",
		"product_access.manage",
	},
	"billing_member": {
		"payments.manage",
		"subscriptions.read",
		"usage.read",
		"bills.read",
	},
	"member": {
		"product.use",
	},
}

// Capabilities returns the visible capability set of the verified
// identity's active membership in tenantID (AC1: each Platform Role gets
// its own operations). Rejections are classified before any port call
// (the T03/T05 precedent): an identity that was never verified end to
// end with ErrUserRequired and a missing tenant with ErrTenantRequired.
// The port resolves the active-membership role — ErrNotAnActiveMember
// (no existence oracle) and ErrUserNotBound flow out unchanged — and a
// persisted role outside the matrix classifies as ErrRoleNotSupported
// (design ruling 5: defensive, never a 500). The served list is a sorted
// copy: the matrix itself is never handed out.
func (s *Service) Capabilities(ctx context.Context, verified identity.VerifiedIdentity, tenantID string) (RoleCapabilities, error) {
	if verified.Issuer == "" || verified.Subject == "" {
		return RoleCapabilities{}, ErrUserRequired
	}
	if tenantID == "" {
		return RoleCapabilities{}, ErrTenantRequired
	}
	role, err := s.repository.ActiveMembershipRole(ctx, verified, tenantID)
	if err != nil {
		return RoleCapabilities{}, err
	}
	capabilities, ok := roleCapabilities[role]
	if !ok {
		return RoleCapabilities{}, ErrRoleNotSupported
	}
	sorted := slices.Clone(capabilities)
	sort.Strings(sorted)
	return RoleCapabilities{TenantID: tenantID, Role: role, Capabilities: sorted}, nil
}
