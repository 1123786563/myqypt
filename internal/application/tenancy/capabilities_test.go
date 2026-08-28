package tenancy_test

import (
	"context"
	"errors"
	"slices"
	"sort"
	"testing"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
)

// capabilitiesTestTenant is the tenant the capabilities tests deliver; a
// fixed canonical uuid keeps the pass-through assertions concrete.
const capabilitiesTestTenant = "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e"

// capabilityVocabulary is the closed capability vocabulary (design ruling
// 1): every name traces to a CONTEXT.md Platform access sentence. The
// union of the four roles' sets is exactly this list — nothing else can
// ever be served.
var capabilityVocabulary = []string{
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
}

// wantCapabilitiesByRole pins the exact per-role capability sets derived
// verbatim from the CONTEXT.md Platform access definitions (design ruling
// 1): Owner holds the full superset (the sole accountable role), Admin
// holds the four administration domains, Billing Member the four billing
// domains, Member the single product.use domain. Every list is already
// sorted the way the endpoint must serve it.
var wantCapabilitiesByRole = map[string][]string{
	"owner":          sortedCopy(capabilityVocabulary),
	"admin":          sortedCopy([]string{"membership.manage", "purchases.manage", "configuration.manage", "product_access.manage"}),
	"billing_member": sortedCopy([]string{"payments.manage", "subscriptions.read", "usage.read", "bills.read"}),
	"member":         sortedCopy([]string{"product.use"}),
}

// sortedCopy returns a sorted copy so the pin tables above stay order
// insensitive while the served lists stay order sensitive.
func sortedCopy(values []string) []string {
	copied := slices.Clone(values)
	sort.Strings(copied)
	return copied
}

// equalExactSet reports whether got equals want element by element (the
// served list is an exact ordered pin, not a subset check).
func equalExactSet(got, want []string) bool {
	return slices.Equal(got, want)
}

// TestServiceCapabilitiesMatrixPerRole pins the matrix (design ruling 1):
// for each of the four Platform Roles the service returns exactly the
// role's CONTEXT.md capability set — sorted, with the tenant and role
// echoed — after exactly one port call carrying the verified identity and
// the tenant.
func TestServiceCapabilitiesMatrixPerRole(t *testing.T) {
	for role, want := range wantCapabilitiesByRole {
		t.Run(role, func(t *testing.T) {
			fake := &fakeRepository{roleResult: role}
			service := tenancy.NewService(fake)

			result, err := service.Capabilities(context.Background(), selectTestIdentity, capabilitiesTestTenant)
			if err != nil {
				t.Fatalf("Capabilities: %v", err)
			}
			if result.TenantID != capabilitiesTestTenant || result.Role != role {
				t.Fatalf("Capabilities = {%s %s}, want {%s %s}", result.TenantID, result.Role, capabilitiesTestTenant, role)
			}
			if !equalExactSet(result.Capabilities, want) {
				t.Fatalf("Capabilities(%s) = %v, want exactly %v", role, result.Capabilities, want)
			}
			if fake.roleCalls != 1 {
				t.Fatalf("port role calls = %d, want 1", fake.roleCalls)
			}
			if fake.lastRoleVerified != selectTestIdentity || fake.lastRoleTenant != capabilitiesTestTenant {
				t.Fatalf("port saw (%+v, %q), want (%+v, %q)",
					fake.lastRoleVerified, fake.lastRoleTenant, selectTestIdentity, capabilitiesTestTenant)
			}
		})
	}
}

// TestServiceCapabilitiesSetsPairwiseDistinct proves the four visible
// sets are pairwise distinct (AC1: each role gets its own operations).
func TestServiceCapabilitiesSetsPairwiseDistinct(t *testing.T) {
	roles := []string{"owner", "admin", "billing_member", "member"}
	for i, first := range roles {
		for _, second := range roles[i+1:] {
			if equalExactSet(wantCapabilitiesByRole[first], wantCapabilitiesByRole[second]) {
				t.Fatalf("roles %s and %s hold the same capability set", first, second)
			}
		}
	}
	// The served behavior matches the pin table per role, so the
	// pairwise distinctness holds on the wire too.
	for _, role := range roles {
		fake := &fakeRepository{roleResult: role}
		service := tenancy.NewService(fake)
		served, err := service.Capabilities(context.Background(), selectTestIdentity, capabilitiesTestTenant)
		if err != nil {
			t.Fatalf("Capabilities(%s): %v", role, err)
		}
		if !equalExactSet(served.Capabilities, wantCapabilitiesByRole[role]) {
			t.Fatalf("served set for %s drifted from the pin", role)
		}
	}
}

// TestServiceCapabilitiesClosedVocabulary proves the vocabulary is
// closed: every served name is one of the eleven CONTEXT.md capabilities,
// and every vocabulary name is served to some role (no dead entries).
func TestServiceCapabilitiesClosedVocabulary(t *testing.T) {
	served := map[string]bool{}
	for role := range wantCapabilitiesByRole {
		fake := &fakeRepository{roleResult: role}
		service := tenancy.NewService(fake)
		result, err := service.Capabilities(context.Background(), selectTestIdentity, capabilitiesTestTenant)
		if err != nil {
			t.Fatalf("Capabilities(%s): %v", role, err)
		}
		for _, capability := range result.Capabilities {
			if !slices.Contains(capabilityVocabulary, capability) {
				t.Fatalf("role %s served unknown capability %q", role, capability)
			}
			served[capability] = true
		}
	}
	for _, capability := range capabilityVocabulary {
		if !served[capability] {
			t.Fatalf("vocabulary entry %q is served to no role", capability)
		}
	}
}

// TestServiceCapabilitiesOwnerIsSuperset pins the Owner position (CONTEXT
// .md: "sole Platform role accountable for ... complete access policy"):
// the owner set is exactly the full vocabulary — a strict superset of
// every other role's set.
func TestServiceCapabilitiesOwnerIsSuperset(t *testing.T) {
	owner := wantCapabilitiesByRole["owner"]
	if !equalExactSet(owner, sortedCopy(capabilityVocabulary)) {
		t.Fatal("owner set is not the full capability vocabulary")
	}
	for _, role := range []string{"admin", "billing_member", "member"} {
		for _, capability := range wantCapabilitiesByRole[role] {
			if !slices.Contains(owner, capability) {
				t.Fatalf("owner set lacks %q held by %s", capability, role)
			}
		}
	}
}

// TestServiceCapabilitiesSortedOutput proves the deterministic ordering
// (design ruling 6): the served list is sorted ascending, so identical
// requests produce byte-identical bodies.
func TestServiceCapabilitiesSortedOutput(t *testing.T) {
	for role := range wantCapabilitiesByRole {
		fake := &fakeRepository{roleResult: role}
		service := tenancy.NewService(fake)
		result, err := service.Capabilities(context.Background(), selectTestIdentity, capabilitiesTestTenant)
		if err != nil {
			t.Fatalf("Capabilities(%s): %v", role, err)
		}
		if !sort.StringsAreSorted(result.Capabilities) {
			t.Fatalf("capabilities for %s are not sorted: %v", role, result.Capabilities)
		}
	}
}

// TestServiceCapabilitiesRejectsBeforePort proves the front door (T03/T05
// precedent): an unverified identity is rejected with ErrUserRequired and
// a missing tenant with ErrTenantRequired — both before a single port
// call.
func TestServiceCapabilitiesRejectsBeforePort(t *testing.T) {
	cases := []struct {
		name     string
		identity identity.VerifiedIdentity
		tenantID string
		wantErr  error
	}{
		{"unverified identity", identity.VerifiedIdentity{}, capabilitiesTestTenant, tenancy.ErrUserRequired},
		{"missing tenant", selectTestIdentity, "", tenancy.ErrTenantRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRepository{}
			service := tenancy.NewService(fake)

			if _, err := service.Capabilities(context.Background(), tc.identity, tc.tenantID); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Capabilities error = %v, want %v", err, tc.wantErr)
			}
			assertZeroPortCalls(t, fake)
		})
	}
}

// TestServiceCapabilitiesPropagatesClassifiedPortRejections classifies
// the port rejections: ErrNotAnActiveMember (no active membership in the
// tenant — never a member, revoked, or someone else's tenant) and
// ErrUserNotBound (never-bound identity) flow out unchanged, each after
// exactly one port call (classification happens in the port).
func TestServiceCapabilitiesPropagatesClassifiedPortRejections(t *testing.T) {
	for _, err := range []error{tenancy.ErrNotAnActiveMember, tenancy.ErrUserNotBound} {
		fake := &fakeRepository{roleErr: err}
		service := tenancy.NewService(fake)

		if _, got := service.Capabilities(context.Background(), selectTestIdentity, capabilitiesTestTenant); !errors.Is(got, err) {
			t.Fatalf("Capabilities error = %v, want %v", got, err)
		}
		if fake.roleCalls != 1 {
			t.Fatalf("port role calls = %d, want 1 (classification happens in the port)", fake.roleCalls)
		}
	}
}

// TestServiceCapabilitiesUnknownRoleRejected proves the defensive
// classification (design ruling 5): a role string outside the four-role
// matrix — impossible behind the memberships.role CHECK but a contract
// breach if it ever leaks — answers ErrRoleNotSupported, never a panic
// and never a 500-shaped surprise.
func TestServiceCapabilitiesUnknownRoleRejected(t *testing.T) {
	for _, role := range []string{"", "emperor", "Owner"} {
		fake := &fakeRepository{roleResult: role}
		service := tenancy.NewService(fake)

		if _, err := service.Capabilities(context.Background(), selectTestIdentity, capabilitiesTestTenant); !errors.Is(err, tenancy.ErrRoleNotSupported) {
			t.Fatalf("Capabilities(role=%q) error = %v, want ErrRoleNotSupported", role, err)
		}
		if fake.roleCalls != 1 {
			t.Fatalf("port role calls = %d, want 1 (the role arrives from persistence)", fake.roleCalls)
		}
	}
}

// TestMembershipManageHoldersAreOwnerAndAdmin pins the 不可越权 lockstep
// contract (design ruling 3): membership.manage is held by exactly owner
// and admin — the same role pair the T05 adapter's transactional invite
// guard enforces (role IN ('owner','admin') AND status='active'). If the
// matrix ever grants membership.manage to billing_member or member, or
// takes it from owner or admin, this pin fails.
func TestMembershipManageHoldersAreOwnerAndAdmin(t *testing.T) {
	wantHolders := map[string]bool{"owner": true, "admin": true}
	for role := range wantCapabilitiesByRole {
		holds := slices.Contains(wantCapabilitiesByRole[role], "membership.manage")
		if holds != wantHolders[role] {
			t.Fatalf("role %s holds membership.manage=%t, want %t (the invite guard's role set)", role, holds, wantHolders[role])
		}
	}
}
