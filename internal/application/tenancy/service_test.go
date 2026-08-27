package tenancy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
)

// fakeRepository is the in-memory tenancy.Repository port fake: it records
// every port call and serves canned membership/selection state. It never
// touches a database, so the service contract here is the focused no-DB
// evidence (design ruling 7).
type fakeRepository struct {
	// memberships is the active-membership set the fake serves for the
	// verified identity (tenant summaries only; membership revocation is
	// an adapter concern, expressed here by serving a shorter list).
	memberships []tenancy.TenantSummary
	// selection is the current persisted selection; hasSelection reports
	// whether one exists at all.
	selection    tenancy.TenantContext
	hasSelection bool
	// saveErr is returned by SaveSelection when non-nil (classification
	// pass-through, e.g. ErrNotAnActiveMember).
	saveErr error

	listCalls    int
	currentCalls int
	saveCalls    int

	lastListVerified    identity.VerifiedIdentity
	lastCurrentVerified identity.VerifiedIdentity
	lastSaveVerified    identity.VerifiedIdentity
	lastSavedTenantID   string
}

func (f *fakeRepository) ListMembershipTenants(_ context.Context, verified identity.VerifiedIdentity) ([]tenancy.TenantSummary, error) {
	f.listCalls++
	f.lastListVerified = verified
	return f.memberships, nil
}

func (f *fakeRepository) SelectedTenant(_ context.Context, verified identity.VerifiedIdentity) (tenancy.TenantContext, error) {
	f.currentCalls++
	f.lastCurrentVerified = verified
	if !f.hasSelection {
		return tenancy.TenantContext{}, tenancy.ErrNoTenantContext
	}
	return f.selection, nil
}

func (f *fakeRepository) SaveSelection(_ context.Context, verified identity.VerifiedIdentity, tenantID string) (tenancy.TenantContext, error) {
	f.saveCalls++
	f.lastSaveVerified = verified
	f.lastSavedTenantID = tenantID
	if f.saveErr != nil {
		return tenancy.TenantContext{}, f.saveErr
	}
	f.selection = tenancy.TenantContext{TenantID: tenantID, SelectedAt: time.Now().UTC()}
	f.hasSelection = true
	return f.selection, nil
}

// selectTestIdentity is the verified identity the service tests deliver.
var selectTestIdentity = identity.VerifiedIdentity{
	Issuer:  "https://issuer.tenancy.test",
	Subject: "subject-tenancy-1",
}

const selectTestTenant = "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88a"

// assertZeroPortCalls fails when the fake recorded any port call.
func assertZeroPortCalls(t *testing.T, fake *fakeRepository) {
	t.Helper()
	if fake.listCalls != 0 || fake.currentCalls != 0 || fake.saveCalls != 0 {
		t.Fatalf("port calls = list:%d current:%d save:%d, want all zero",
			fake.listCalls, fake.currentCalls, fake.saveCalls)
	}
}

// TestServiceRejectsMissingUserIdentity proves the front-door validation:
// every method rejects an identity that was never verified end to end
// (empty issuer or subject) with ErrUserRequired, and Select additionally
// rejects an empty tenant with ErrTenantRequired — all before a single
// port call (design ruling 4).
func TestServiceRejectsMissingUserIdentity(t *testing.T) {
	cases := []struct {
		name     string
		identity identity.VerifiedIdentity
	}{
		{"issuer and subject empty", identity.VerifiedIdentity{}},
		{"issuer only", identity.VerifiedIdentity{Subject: "subject-tenancy-1"}},
		{"subject only", identity.VerifiedIdentity{Issuer: "https://issuer.tenancy.test"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRepository{}
			service := tenancy.NewService(fake)

			if _, err := service.List(context.Background(), tc.identity); !errors.Is(err, tenancy.ErrUserRequired) {
				t.Fatalf("List error = %v, want ErrUserRequired", err)
			}
			if _, err := service.Current(context.Background(), tc.identity); !errors.Is(err, tenancy.ErrUserRequired) {
				t.Fatalf("Current error = %v, want ErrUserRequired", err)
			}
			if _, err := service.Select(context.Background(), tc.identity, selectTestTenant); !errors.Is(err, tenancy.ErrUserRequired) {
				t.Fatalf("Select error = %v, want ErrUserRequired", err)
			}
			assertZeroPortCalls(t, fake)
		})
	}
}

// TestServiceSelectRejectsMissingTenant proves the write-path validation:
// a verified identity with an empty tenant is rejected with
// ErrTenantRequired before any port call.
func TestServiceSelectRejectsMissingTenant(t *testing.T) {
	fake := &fakeRepository{}
	service := tenancy.NewService(fake)

	if _, err := service.Select(context.Background(), selectTestIdentity, ""); !errors.Is(err, tenancy.ErrTenantRequired) {
		t.Fatalf("Select error = %v, want ErrTenantRequired", err)
	}
	assertZeroPortCalls(t, fake)
}

// TestServiceListReturnsPortTenants proves the read path: List returns
// exactly the port's active-membership tenants for the verified identity.
func TestServiceListReturnsPortTenants(t *testing.T) {
	fake := &fakeRepository{
		memberships: []tenancy.TenantSummary{
			{TenantID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88a", Kind: "personal", Role: "owner"},
			{TenantID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88b", Kind: "business", Role: "member"},
		},
	}
	service := tenancy.NewService(fake)

	tenants, err := service.List(context.Background(), selectTestIdentity)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tenants) != 2 {
		t.Fatalf("List returned %d tenants, want 2", len(tenants))
	}
	if tenants[0].TenantID != fake.memberships[0].TenantID || tenants[1].TenantID != fake.memberships[1].TenantID {
		t.Fatalf("List tenants = %+v, want the port order %+v", tenants, fake.memberships)
	}
	if fake.listCalls != 1 || fake.lastListVerified != selectTestIdentity {
		t.Fatalf("port saw (%d calls, %+v), want (1, %+v)", fake.listCalls, fake.lastListVerified, selectTestIdentity)
	}
}

// TestServiceCurrentReturnsPortSelection proves the read path: Current
// returns the port's server-validated selection.
func TestServiceCurrentReturnsPortSelection(t *testing.T) {
	selectedAt := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	fake := &fakeRepository{
		hasSelection: true,
		selection:    tenancy.TenantContext{TenantID: selectTestTenant, SelectedAt: selectedAt},
	}
	service := tenancy.NewService(fake)

	current, err := service.Current(context.Background(), selectTestIdentity)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.TenantID != selectTestTenant || !current.SelectedAt.Equal(selectedAt) {
		t.Fatalf("Current = %+v, want {%s %s}", current, selectTestTenant, selectedAt)
	}
	if fake.currentCalls != 1 {
		t.Fatalf("port current calls = %d, want 1", fake.currentCalls)
	}
}

// TestServiceCurrentAbsentSelection classifies the read-absent case: with
// no persisted selection the port's ErrNoTenantContext flows out
// unchanged.
func TestServiceCurrentAbsentSelection(t *testing.T) {
	service := tenancy.NewService(&fakeRepository{})

	if _, err := service.Current(context.Background(), selectTestIdentity); !errors.Is(err, tenancy.ErrNoTenantContext) {
		t.Fatalf("Current error = %v, want ErrNoTenantContext", err)
	}
}

// TestServiceSelectPersistsSelection proves the write path: Select lands
// exactly one business effect on the port — the selection for the
// verified identity.
func TestServiceSelectPersistsSelection(t *testing.T) {
	fake := &fakeRepository{}
	service := tenancy.NewService(fake)

	selected, err := service.Select(context.Background(), selectTestIdentity, selectTestTenant)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if selected.TenantID != selectTestTenant {
		t.Fatalf("Select returned tenant %q, want %q", selected.TenantID, selectTestTenant)
	}
	if selected.SelectedAt.IsZero() {
		t.Fatal("Select returned a zero selected_at")
	}
	if fake.saveCalls != 1 {
		t.Fatalf("port save calls = %d, want 1", fake.saveCalls)
	}
	if fake.lastSaveVerified != selectTestIdentity || fake.lastSavedTenantID != selectTestTenant {
		t.Fatalf("port saw (%+v, %q), want (%+v, %q)", fake.lastSaveVerified, fake.lastSavedTenantID, selectTestIdentity, selectTestTenant)
	}
	if !fake.hasSelection || fake.selection.TenantID != selectTestTenant {
		t.Fatalf("port selection = %+v (present=%t), want the selected tenant", fake.selection, fake.hasSelection)
	}
}

// TestServiceSelectReplayIdempotent proves replay semantics: delivering
// the same selection again succeeds and leaves exactly one selection —
// the persistence-level effect the upsert guarantees.
func TestServiceSelectReplayIdempotent(t *testing.T) {
	fake := &fakeRepository{}
	service := tenancy.NewService(fake)

	if _, err := service.Select(context.Background(), selectTestIdentity, selectTestTenant); err != nil {
		t.Fatalf("first Select: %v", err)
	}
	replayed, err := service.Select(context.Background(), selectTestIdentity, selectTestTenant)
	if err != nil {
		t.Fatalf("replayed Select: %v", err)
	}
	if replayed.TenantID != selectTestTenant {
		t.Fatalf("replayed Select returned tenant %q, want %q", replayed.TenantID, selectTestTenant)
	}
	if fake.saveCalls != 2 {
		t.Fatalf("port save calls = %d, want 2 (each delivery is a real write)", fake.saveCalls)
	}
	if !fake.hasSelection || fake.selection.TenantID != selectTestTenant {
		t.Fatalf("port selection = %+v (present=%t), want exactly one selection for the tenant", fake.selection, fake.hasSelection)
	}
}

// TestServiceSelectPropagatesNotAnActiveMember classifies the write
// rejection: the port's ErrNotAnActiveMember (active-membership
// validation failed before any write) flows out unchanged.
func TestServiceSelectPropagatesNotAnActiveMember(t *testing.T) {
	fake := &fakeRepository{saveErr: tenancy.ErrNotAnActiveMember}
	service := tenancy.NewService(fake)

	if _, err := service.Select(context.Background(), selectTestIdentity, selectTestTenant); !errors.Is(err, tenancy.ErrNotAnActiveMember) {
		t.Fatalf("Select error = %v, want ErrNotAnActiveMember", err)
	}
	if fake.hasSelection {
		t.Fatal("port recorded a selection despite the rejected write")
	}
}
