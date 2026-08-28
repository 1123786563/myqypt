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
	createCalls  int

	lastListVerified    identity.VerifiedIdentity
	lastCurrentVerified identity.VerifiedIdentity
	lastSaveVerified    identity.VerifiedIdentity
	lastSavedTenantID   string
	lastCreateVerified  identity.VerifiedIdentity
	lastCreateName      string
	lastCreateKey       string

	// createTenant/createCreated are the canned creation result the fake
	// serves; createErr (when non-nil) classifies the port rejection
	// (e.g. ErrUserNotBound).
	createTenant  tenancy.BusinessTenant
	createCreated bool
	createErr     error

	// inviteInvitation/inviteCreated are the canned invitation result the
	// fake serves; inviteErr (when non-nil) classifies the port rejection
	// (ErrUserNotBound, ErrInviterNotAuthorized, ErrInvitationNotFound).
	inviteInvitation tenancy.MembershipInvitation
	inviteCreated    bool
	inviteErr        error
	inviteCalls      int

	lastInviteVerified identity.VerifiedIdentity
	lastInviteTenant   string
	lastInviteSubject  string
	lastInviteRole     string
	lastInviteKey      string

	// acceptMembership/acceptErr are the canned accept result and its
	// classified port rejection.
	acceptMembership tenancy.ActivatedMembership
	acceptErr        error
	acceptCalls      int

	lastAcceptVerified identity.VerifiedIdentity
	lastAcceptTenant   string

	// roleResult/roleErr are the canned active-membership role the
	// capabilities path serves and its classified port rejection
	// (ErrNotAnActiveMember, ErrUserNotBound).
	roleResult string
	roleErr    error
	roleCalls  int

	lastRoleVerified identity.VerifiedIdentity
	lastRoleTenant   string
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

// CreateBusinessTenant records the creation delivery and serves the canned
// (tenant, created) pair, keeping the service contract testable without a
// database (T04 design ruling 6).
func (f *fakeRepository) CreateBusinessTenant(_ context.Context, verified identity.VerifiedIdentity, displayName, idempotencyKey string) (tenancy.BusinessTenant, bool, error) {
	f.createCalls++
	f.lastCreateVerified = verified
	f.lastCreateName = displayName
	f.lastCreateKey = idempotencyKey
	if f.createErr != nil {
		return tenancy.BusinessTenant{}, false, f.createErr
	}
	return f.createTenant, f.createCreated, nil
}

// InviteMember records the invitation delivery and serves the canned
// (invitation, created) pair, keeping the T05 service contract testable
// without a database.
func (f *fakeRepository) InviteMember(_ context.Context, verified identity.VerifiedIdentity, tenantID, inviteeSubject, role string) (tenancy.MembershipInvitation, bool, error) {
	f.inviteCalls++
	f.lastInviteVerified = verified
	f.lastInviteTenant = tenantID
	f.lastInviteSubject = inviteeSubject
	f.lastInviteRole = role
	if f.inviteErr != nil {
		return tenancy.MembershipInvitation{}, false, f.inviteErr
	}
	return f.inviteInvitation, f.inviteCreated, nil
}

// AcceptInvitation records the acceptance delivery and serves the canned
// activated membership.
func (f *fakeRepository) AcceptInvitation(_ context.Context, verified identity.VerifiedIdentity, tenantID string) (tenancy.ActivatedMembership, error) {
	f.acceptCalls++
	f.lastAcceptVerified = verified
	f.lastAcceptTenant = tenantID
	if f.acceptErr != nil {
		return tenancy.ActivatedMembership{}, f.acceptErr
	}
	return f.acceptMembership, nil
}

// ActiveMembershipRole records the role-resolution delivery and serves the
// canned role (or its classified port rejection), keeping the T06
// capabilities contract testable without a database.
func (f *fakeRepository) ActiveMembershipRole(_ context.Context, verified identity.VerifiedIdentity, tenantID string) (string, error) {
	f.roleCalls++
	f.lastRoleVerified = verified
	f.lastRoleTenant = tenantID
	if f.roleErr != nil {
		return "", f.roleErr
	}
	return f.roleResult, nil
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
	if fake.listCalls != 0 || fake.currentCalls != 0 || fake.saveCalls != 0 || fake.createCalls != 0 ||
		fake.inviteCalls != 0 || fake.acceptCalls != 0 || fake.roleCalls != 0 {
		t.Fatalf("port calls = list:%d current:%d save:%d create:%d invite:%d accept:%d role:%d, want all zero",
			fake.listCalls, fake.currentCalls, fake.saveCalls, fake.createCalls, fake.inviteCalls, fake.acceptCalls, fake.roleCalls)
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

// createTestTenant is the canned business tenant the fake serves; its
// fixed timestamp pins the pass-through assertions below.
var createTestTenant = tenancy.BusinessTenant{
	TenantID:    "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88c",
	DisplayName: "Corner Cafe",
	CreatedAt:   time.Date(2026, 8, 27, 4, 5, 6, 0, time.UTC),
}

// TestServiceCreateBusinessTenantRejectsBeforeWrite proves the T04 front
// door (design ruling 2): an unverified identity is rejected with
// ErrUserRequired, a display name that is empty or only whitespace with
// ErrDisplayNameRequired, and a missing idempotency key with
// ErrIdempotencyKeyRequired — all before a single port call.
func TestServiceCreateBusinessTenantRejectsBeforeWrite(t *testing.T) {
	cases := []struct {
		name           string
		identity       identity.VerifiedIdentity
		displayName    string
		idempotencyKey string
		wantErr        error
	}{
		{"unverified identity", identity.VerifiedIdentity{}, "Corner Cafe", "key-1", tenancy.ErrUserRequired},
		{"empty display name", selectTestIdentity, "", "key-1", tenancy.ErrDisplayNameRequired},
		{"whitespace display name", selectTestIdentity, " \t ", "key-1", tenancy.ErrDisplayNameRequired},
		{"missing idempotency key", selectTestIdentity, "Corner Cafe", "", tenancy.ErrIdempotencyKeyRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRepository{}
			service := tenancy.NewService(fake)

			_, _, err := service.CreateBusinessTenant(context.Background(), tc.identity, tc.displayName, tc.idempotencyKey)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CreateBusinessTenant error = %v, want %v", err, tc.wantErr)
			}
			assertZeroPortCalls(t, fake)
		})
	}
}

// TestServiceCreateBusinessTenantPassesThrough proves the success path:
// the service delivers exactly one port call with the verified identity,
// the display name, and the idempotency key, and returns the port's
// (tenant, created=true) pair unchanged.
func TestServiceCreateBusinessTenantPassesThrough(t *testing.T) {
	fake := &fakeRepository{createTenant: createTestTenant, createCreated: true}
	service := tenancy.NewService(fake)

	tenant, created, err := service.CreateBusinessTenant(context.Background(), selectTestIdentity, "Corner Cafe", "key-1")
	if err != nil {
		t.Fatalf("CreateBusinessTenant: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true on the insert path")
	}
	if tenant != createTestTenant {
		t.Fatalf("CreateBusinessTenant = %+v, want the port tenant %+v", tenant, createTestTenant)
	}
	if fake.createCalls != 1 {
		t.Fatalf("port create calls = %d, want 1", fake.createCalls)
	}
	if fake.lastCreateVerified != selectTestIdentity || fake.lastCreateName != "Corner Cafe" || fake.lastCreateKey != "key-1" {
		t.Fatalf("port saw (%+v, %q, %q), want (%+v, %q, %q)",
			fake.lastCreateVerified, fake.lastCreateName, fake.lastCreateKey, selectTestIdentity, "Corner Cafe", "key-1")
	}
}

// TestServiceCreateBusinessTenantReplayPassesThrough proves the replay
// path: the port's (tenant, created=false) pair — the persistence-level
// idempotency convergence — flows out unchanged.
func TestServiceCreateBusinessTenantReplayPassesThrough(t *testing.T) {
	fake := &fakeRepository{createTenant: createTestTenant, createCreated: false}
	service := tenancy.NewService(fake)

	tenant, created, err := service.CreateBusinessTenant(context.Background(), selectTestIdentity, "Corner Cafe", "key-1")
	if err != nil {
		t.Fatalf("CreateBusinessTenant: %v", err)
	}
	if created {
		t.Fatal("created = true, want false on the replay path")
	}
	if tenant != createTestTenant {
		t.Fatalf("CreateBusinessTenant = %+v, want the port tenant %+v", tenant, createTestTenant)
	}
	if fake.createCalls != 1 {
		t.Fatalf("port create calls = %d, want 1 (each delivery is a real port call)", fake.createCalls)
	}
}

// TestServiceCreateBusinessTenantPropagatesUserNotBound classifies the
// port rejection: the repository's ErrUserNotBound (the verified identity
// was never bound, so no platform user can become the owner) flows out
// unchanged.
func TestServiceCreateBusinessTenantPropagatesUserNotBound(t *testing.T) {
	fake := &fakeRepository{createErr: tenancy.ErrUserNotBound}
	service := tenancy.NewService(fake)

	_, _, err := service.CreateBusinessTenant(context.Background(), selectTestIdentity, "Corner Cafe", "key-1")
	if !errors.Is(err, tenancy.ErrUserNotBound) {
		t.Fatalf("CreateBusinessTenant error = %v, want ErrUserNotBound", err)
	}
	if fake.createCalls != 1 {
		t.Fatalf("port create calls = %d, want 1 (classification happens in the port)", fake.createCalls)
	}
}

// inviteTestTenant is the tenant the invitation tests deliver; a fixed
// canonical uuid keeps the pass-through assertions concrete.
const inviteTestTenant = "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88d"

// inviteTestInvitation is the canned invitation the fake serves; its
// fixed timestamp pins the pass-through assertions below.
var inviteTestInvitation = tenancy.MembershipInvitation{
	TenantID:  inviteTestTenant,
	Role:      "member",
	Status:    "invited",
	InvitedAt: time.Date(2026, 8, 28, 7, 8, 9, 0, time.UTC),
}

// TestServiceInviteMemberRejectsBeforeWrite proves the T05 front door
// (design ruling 3–5): an unverified identity is rejected with
// ErrUserRequired, a missing tenant with ErrTenantRequired, a missing
// invitee subject with ErrInviteeSubjectRequired, a non-invitable role
// (owner or an unknown string) with ErrRoleNotSupported, and a missing
// idempotency key with ErrIdempotencyKeyRequired — all before a single
// port call.
func TestServiceInviteMemberRejectsBeforeWrite(t *testing.T) {
	cases := []struct {
		name           string
		identity       identity.VerifiedIdentity
		tenantID       string
		inviteeSubject string
		role           string
		idempotencyKey string
		wantErr        error
	}{
		{"unverified identity", identity.VerifiedIdentity{}, inviteTestTenant, "subject-invitee-1", "member", "key-1", tenancy.ErrUserRequired},
		{"missing tenant", selectTestIdentity, "", "subject-invitee-1", "member", "key-1", tenancy.ErrTenantRequired},
		{"missing invitee subject", selectTestIdentity, inviteTestTenant, "", "member", "key-1", tenancy.ErrInviteeSubjectRequired},
		{"owner role not invitable", selectTestIdentity, inviteTestTenant, "subject-invitee-1", "owner", "key-1", tenancy.ErrRoleNotSupported},
		{"unknown role", selectTestIdentity, inviteTestTenant, "subject-invitee-1", "queen", "key-1", tenancy.ErrRoleNotSupported},
		{"missing idempotency key", selectTestIdentity, inviteTestTenant, "subject-invitee-1", "member", "", tenancy.ErrIdempotencyKeyRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRepository{}
			service := tenancy.NewService(fake)

			_, _, err := service.InviteMember(context.Background(), tc.identity, tc.tenantID, tc.inviteeSubject, tc.role, tc.idempotencyKey)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("InviteMember error = %v, want %v", err, tc.wantErr)
			}
			assertZeroPortCalls(t, fake)
		})
	}
}

// TestServiceInviteMemberPassesThrough proves the success path: the
// service delivers exactly one port call with the verified inviter, the
// tenant, the invitee subject, and the role — and no idempotency key,
// because replay convergence rides the natural key (design ruling 2) —
// returning the port's (invitation, created=true) pair unchanged.
func TestServiceInviteMemberPassesThrough(t *testing.T) {
	fake := &fakeRepository{inviteInvitation: inviteTestInvitation, inviteCreated: true}
	service := tenancy.NewService(fake)

	invitation, created, err := service.InviteMember(context.Background(), selectTestIdentity, inviteTestTenant, "subject-invitee-1", "member", "key-invite-1")
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true on the insert path")
	}
	if invitation != inviteTestInvitation {
		t.Fatalf("InviteMember = %+v, want the port invitation %+v", invitation, inviteTestInvitation)
	}
	if fake.inviteCalls != 1 {
		t.Fatalf("port invite calls = %d, want 1", fake.inviteCalls)
	}
	if fake.lastInviteVerified != selectTestIdentity ||
		fake.lastInviteTenant != inviteTestTenant ||
		fake.lastInviteSubject != "subject-invitee-1" ||
		fake.lastInviteRole != "member" ||
		fake.lastInviteKey != "" {
		t.Fatalf("port saw (%+v, %q, %q, %q, key=%q), want (%+v, %q, %q, %q, no key)",
			fake.lastInviteVerified, fake.lastInviteTenant, fake.lastInviteSubject, fake.lastInviteRole, fake.lastInviteKey,
			selectTestIdentity, inviteTestTenant, "subject-invitee-1", "member")
	}
}

// TestServiceInviteMemberReplayPassesThrough proves the replay path: the
// port's (invitation, created=false) pair — the natural-key convergence —
// flows out unchanged.
func TestServiceInviteMemberReplayPassesThrough(t *testing.T) {
	fake := &fakeRepository{inviteInvitation: inviteTestInvitation, inviteCreated: false}
	service := tenancy.NewService(fake)

	invitation, created, err := service.InviteMember(context.Background(), selectTestIdentity, inviteTestTenant, "subject-invitee-1", "member", "key-invite-1")
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	if created {
		t.Fatal("created = true, want false on the replay path")
	}
	if invitation != inviteTestInvitation {
		t.Fatalf("InviteMember = %+v, want the port invitation %+v", invitation, inviteTestInvitation)
	}
}

// TestServiceInviteMemberPropagatesClassifiedRejections classifies the
// port rejections: ErrUserNotBound (inviter or invitee never bound),
// ErrInviterNotAuthorized (no active owner/admin membership), and
// ErrInvitationNotFound (the invitee's membership row already exists in a
// non-invited state) all flow out unchanged.
func TestServiceInviteMemberPropagatesClassifiedRejections(t *testing.T) {
	for _, err := range []error{tenancy.ErrUserNotBound, tenancy.ErrInviterNotAuthorized, tenancy.ErrInvitationNotFound} {
		fake := &fakeRepository{inviteErr: err}
		service := tenancy.NewService(fake)

		if _, _, got := service.InviteMember(context.Background(), selectTestIdentity, inviteTestTenant, "subject-invitee-1", "member", "key-invite-1"); !errors.Is(got, err) {
			t.Fatalf("InviteMember error = %v, want %v", got, err)
		}
		if fake.inviteCalls != 1 {
			t.Fatalf("port invite calls = %d, want 1 (classification happens in the port)", fake.inviteCalls)
		}
	}
}

// acceptTestMembership is the canned activated membership the fake
// serves for the accept path.
var acceptTestMembership = tenancy.ActivatedMembership{
	TenantID: inviteTestTenant,
	Role:     "member",
	Status:   "active",
}

// TestServiceAcceptInvitationRejectsBeforeWrite proves the accept front
// door (design ruling 6): an unverified identity is rejected with
// ErrUserRequired and a missing tenant with ErrTenantRequired — both
// before a single port call.
func TestServiceAcceptInvitationRejectsBeforeWrite(t *testing.T) {
	cases := []struct {
		name     string
		identity identity.VerifiedIdentity
		tenantID string
		wantErr  error
	}{
		{"unverified identity", identity.VerifiedIdentity{}, inviteTestTenant, tenancy.ErrUserRequired},
		{"missing tenant", selectTestIdentity, "", tenancy.ErrTenantRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRepository{}
			service := tenancy.NewService(fake)

			if _, err := service.AcceptInvitation(context.Background(), tc.identity, tc.tenantID); !errors.Is(err, tc.wantErr) {
				t.Fatalf("AcceptInvitation error = %v, want %v", err, tc.wantErr)
			}
			assertZeroPortCalls(t, fake)
		})
	}
}

// TestServiceAcceptInvitationPassesThrough proves the accept success
// path: exactly one port call with the verified invitee and the tenant,
// returning the port's activated membership unchanged.
func TestServiceAcceptInvitationPassesThrough(t *testing.T) {
	fake := &fakeRepository{acceptMembership: acceptTestMembership}
	service := tenancy.NewService(fake)

	activated, err := service.AcceptInvitation(context.Background(), selectTestIdentity, inviteTestTenant)
	if err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if activated != acceptTestMembership {
		t.Fatalf("AcceptInvitation = %+v, want the port membership %+v", activated, acceptTestMembership)
	}
	if fake.acceptCalls != 1 {
		t.Fatalf("port accept calls = %d, want 1", fake.acceptCalls)
	}
	if fake.lastAcceptVerified != selectTestIdentity || fake.lastAcceptTenant != inviteTestTenant {
		t.Fatalf("port saw (%+v, %q), want (%+v, %q)",
			fake.lastAcceptVerified, fake.lastAcceptTenant, selectTestIdentity, inviteTestTenant)
	}
}

// TestServiceAcceptInvitationPropagatesClassifiedRejections classifies
// the port rejections: ErrUserNotBound and ErrInvitationNotFound flow
// out unchanged.
func TestServiceAcceptInvitationPropagatesClassifiedRejections(t *testing.T) {
	for _, err := range []error{tenancy.ErrUserNotBound, tenancy.ErrInvitationNotFound} {
		fake := &fakeRepository{acceptErr: err}
		service := tenancy.NewService(fake)

		if _, got := service.AcceptInvitation(context.Background(), selectTestIdentity, inviteTestTenant); !errors.Is(got, err) {
			t.Fatalf("AcceptInvitation error = %v, want %v", got, err)
		}
		if fake.acceptCalls != 1 {
			t.Fatalf("port accept calls = %d, want 1 (classification happens in the port)", fake.acceptCalls)
		}
	}
}
