package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/1123786563/myqypt/internal/adapter/postgres"
	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
)

// invitationFixture drives one membership-invitation repository test:
// the two-user tenancy fixture plus a third user C and one business
// tenant owned by user A (provisioned through the T04 port, the
// canonical creation path).
type invitationFixture struct {
	*tenancyFixture
	repo     *postgres.TenancyRepository
	userC    tenancyTestIdentity
	tenantID string
}

// invitationTestIssuer is the single issuer the invitation world shares —
// the Stage-1 reality: one identity provider mints every subject, and the
// adapter addresses an invitee by (inviter-issuer, subject) (design
// ruling 3). The tenancy fixture's per-user issuers suit the T03/T04
// caller-resolution tests, so the invitation fixture reprovisions its own
// cast under this one issuer.
const invitationTestIssuer = "https://issuer-invitation.test"

// newInvitationFixture provisions the invitation world: users A, B, C
// (each with its personal tenant bundle, all bound under the one shared
// issuer) and a business tenant owned by A with the fixed display name.
func newInvitationFixture(t *testing.T) *invitationFixture {
	t.Helper()

	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	fixture.userA = fixture.provisionUser(t, ctx, invitationTestIssuer, "subject-invite-a")
	fixture.userB = fixture.provisionUser(t, ctx, invitationTestIssuer, "subject-invite-b")
	userC := fixture.provisionUser(t, ctx, invitationTestIssuer, "subject-invite-c")
	tenant, created, err := repo.CreateBusinessTenant(ctx, fixture.userA.verified, "Invitation Team", "key-invitation-fixture")
	if err != nil {
		t.Fatalf("CreateBusinessTenant fixture: %v", err)
	}
	if !created {
		t.Fatal("CreateBusinessTenant fixture created = false, want true on a fresh database")
	}
	return &invitationFixture{tenancyFixture: fixture, repo: repo, userC: userC, tenantID: tenant.TenantID}
}

// countTenantMemberships returns the membership rows of one tenant,
// optionally restricted to one user and one status (empty strings skip
// the filter).
func (f *invitationFixture) countTenantMemberships(t *testing.T, ctx context.Context, tenantID, userID, status string) int {
	t.Helper()

	query := `SELECT count(*) FROM memberships WHERE tenant_id = $1::uuid`
	args := []any{tenantID}
	if userID != "" {
		args = append(args, userID)
		query += fmt.Sprintf(" AND user_id = $%d::uuid", len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	return f.countRows(t, ctx, query, args...)
}

// membershipRow reads the (status, role, created_at) of one tenant
// membership straight from the table.
func (f *invitationFixture) membershipRow(t *testing.T, ctx context.Context, tenantID, userID string) (string, string) {
	t.Helper()

	var status, role string
	if err := f.db.QueryRowContext(ctx, `
		SELECT status, role FROM memberships
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		tenantID, userID).Scan(&status, &role); err != nil {
		t.Fatalf("read membership row: %v", err)
	}
	return status, role
}

// TestInviteMemberCreatesInvitedRow proves the core invitation state
// (design ruling 1): the owner's invitation inserts exactly one
// membership row with status='invited' and the granted role — and the
// invited membership stays invisible and unusable at the T03 seams: the
// active-only list does not include the tenant, and selecting it fails
// with ErrNotAnActiveMember.
func TestInviteMemberCreatesInvitedRow(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	invitation, created, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member")
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	if !created {
		t.Fatal("created = false on the insert path, want true")
	}
	if invitation.TenantID != fixture.tenantID || invitation.Role != "member" || invitation.Status != "invited" || invitation.InvitedAt.IsZero() {
		t.Fatalf("returned invitation = %+v, want the invited membership facts", invitation)
	}

	status, role := fixture.membershipRow(t, ctx, fixture.tenantID, fixture.userB.userID)
	if status != "invited" || role != "member" {
		t.Fatalf("membership row = (%s, %s), want (invited, member)", status, role)
	}
	if rows := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, ""); rows != 1 {
		t.Fatalf("membership rows for the invitee = %d, want exactly 1", rows)
	}

	tenants, err := fixture.repo.ListMembershipTenants(ctx, fixture.userB.verified)
	if err != nil {
		t.Fatalf("ListMembershipTenants for the invitee: %v", err)
	}
	for _, summary := range tenants {
		if summary.TenantID == fixture.tenantID {
			t.Fatal("invited membership is visible in the active-only tenant list, want invisible")
		}
	}
	if _, err := fixture.repo.SaveSelection(ctx, fixture.userB.verified, fixture.tenantID); !errors.Is(err, tenancy.ErrNotAnActiveMember) {
		t.Fatalf("SaveSelection for the invited member = %v, want ErrNotAnActiveMember", err)
	}
}

// TestInviteMemberReplayConverges proves the natural-key convergence
// (design ruling 2): redelivering the same (tenant, invitee) — same or
// different idempotency key at the transport — returns the identical
// invitation facts with created=false and inserts nothing.
func TestInviteMemberReplayConverges(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	first, created, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "billing_member")
	if err != nil {
		t.Fatalf("first InviteMember: %v", err)
	}
	if !created {
		t.Fatal("first delivery created = false, want true")
	}

	replayed, replayCreated, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "billing_member")
	if err != nil {
		t.Fatalf("replayed InviteMember: %v", err)
	}
	if replayCreated {
		t.Fatal("replay created = true, want false")
	}
	if replayed != first {
		t.Fatalf("replay invitation = %+v, want the identical first invitation %+v", replayed, first)
	}
	if rows := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, ""); rows != 1 {
		t.Fatalf("membership rows after the replay = %d, want 1", rows)
	}
}

// TestInviteMemberExistingMembershipIsNotFound proves the no-oracle
// convergence limit: an existing membership in any non-invited state —
// active (e.g. provisioned by other means) or revoked — classifies as
// ErrInvitationNotFound, indistinguishable from no row.
func TestInviteMemberExistingMembershipIsNotFound(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	fixture.insertActiveMembership(t, ctx, fixture.tenantID, fixture.userB.userID, "member")
	before := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, "")
	if _, _, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member"); !errors.Is(err, tenancy.ErrInvitationNotFound) {
		t.Fatalf("InviteMember over an active membership = %v, want ErrInvitationNotFound", err)
	}
	if after := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, ""); after != before {
		t.Fatalf("membership rows %d -> %d after the rejected invitation, want unchanged", before, after)
	}

	fixture.revokeMembership(t, ctx, fixture.tenantID, fixture.userB.userID)
	if _, _, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member"); !errors.Is(err, tenancy.ErrInvitationNotFound) {
		t.Fatalf("InviteMember over a revoked membership = %v, want ErrInvitationNotFound", err)
	}
}

// TestInviteMemberRequiresOwnerOrAdmin proves the authorization ruling
// (design ruling 5): a member, a non-member, and a never-bound inviter
// are all rejected — the first two with ErrInviterNotAuthorized, the
// last with ErrUserNotBound — and every rejection leaves zero rows
// behind. The owner and a promoted admin are the positive controls.
func TestInviteMemberRequiresOwnerOrAdmin(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()
	userD := fixture.provisionUser(t, ctx, invitationTestIssuer, "subject-invite-d")
	userE := fixture.provisionUser(t, ctx, invitationTestIssuer, "subject-invite-e")

	// The owner may invite (positive control).
	if _, created, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member"); err != nil || !created {
		t.Fatalf("owner invitation = (%t, %v), want (true, nil)", created, err)
	}
	// A promoted admin may invite: C accepts an admin invitation, then
	// invites D.
	if _, created, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userC.verified.Subject, "admin"); err != nil || !created {
		t.Fatalf("admin-role invitation = (%t, %v), want (true, nil)", created, err)
	}
	if _, err := fixture.repo.AcceptInvitation(ctx, fixture.userC.verified, fixture.tenantID); err != nil {
		t.Fatalf("AcceptInvitation for the admin fixture: %v", err)
	}
	if _, created, err := fixture.repo.InviteMember(ctx, fixture.userC.verified, fixture.tenantID, userD.verified.Subject, "member"); err != nil || !created {
		t.Fatalf("admin invitation = (%t, %v), want (true, nil)", created, err)
	}

	cases := []struct {
		name     string
		inviter  identity.VerifiedIdentity
		invitee  tenancyTestIdentity
		tenantID string
		wantErr  error
	}{
		// B accepted a member invitation above: an active member cannot invite.
		{"member-role inviter", fixture.userB.verified, userD, fixture.tenantID, tenancy.ErrInviterNotAuthorized},
		// E holds no membership in the tenant at all.
		{"non-member inviter", userE.verified, fixture.userB, fixture.tenantID, tenancy.ErrInviterNotAuthorized},
		{"nonexistent tenant", fixture.userA.verified, fixture.userB, "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e8fe", tenancy.ErrInviterNotAuthorized},
		{"never-bound inviter", identity.VerifiedIdentity{Issuer: fixture.userA.verified.Issuer, Subject: "subject-never-bound"}, fixture.userB, fixture.tenantID, tenancy.ErrUserNotBound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := fixture.countTenantMemberships(t, ctx, tc.tenantID, tc.invitee.userID, "")
			if _, _, err := fixture.repo.InviteMember(ctx, tc.inviter, tc.tenantID, tc.invitee.verified.Subject, "member"); !errors.Is(err, tc.wantErr) {
				t.Fatalf("InviteMember error = %v, want %v", err, tc.wantErr)
			}
			if after := fixture.countTenantMemberships(t, ctx, tc.tenantID, tc.invitee.userID, ""); after != before {
				t.Fatalf("membership rows %d -> %d after the rejected invitation, want unchanged", before, after)
			}
		})
	}
}

// TestInviteMemberNeverBoundInviteeIsNotFound proves the invitee
// addressing ruling (design ruling 3): a subject that was never bound
// resolves to no platform user, so the invitation is rejected with
// ErrUserNotBound (404, no existence oracle) and zero rows.
func TestInviteMemberNeverBoundInviteeIsNotFound(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	before := fixture.countTenantMemberships(t, ctx, fixture.tenantID, "", "")
	if _, _, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, "subject-never-bound", "member"); !errors.Is(err, tenancy.ErrUserNotBound) {
		t.Fatalf("InviteMember for a never-bound subject = %v, want ErrUserNotBound", err)
	}
	if after := fixture.countTenantMemberships(t, ctx, fixture.tenantID, "", ""); after != before {
		t.Fatalf("tenant membership rows %d -> %d after the rejected invitation, want unchanged", before, after)
	}
}

// TestAcceptInvitationActivatesMembership proves the acceptance
// transition (design ruling 6): the invitee's acceptance flips the
// single row invited -> active with the granted role intact, and the
// membership becomes immediately usable at the T03 seams — the
// active-only list includes the tenant and the selection succeeds.
func TestAcceptInvitationActivatesMembership(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	if _, _, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member"); err != nil {
		t.Fatalf("InviteMember: %v", err)
	}

	activated, err := fixture.repo.AcceptInvitation(ctx, fixture.userB.verified, fixture.tenantID)
	if err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if activated.TenantID != fixture.tenantID || activated.Role != "member" || activated.Status != "active" {
		t.Fatalf("activated membership = %+v, want the active member facts", activated)
	}

	status, role := fixture.membershipRow(t, ctx, fixture.tenantID, fixture.userB.userID)
	if status != "active" || role != "member" {
		t.Fatalf("membership row after acceptance = (%s, %s), want (active, member)", status, role)
	}
	if rows := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, ""); rows != 1 {
		t.Fatalf("membership rows after acceptance = %d, want exactly 1 (a transition, not a second row)", rows)
	}

	tenants, err := fixture.repo.ListMembershipTenants(ctx, fixture.userB.verified)
	if err != nil {
		t.Fatalf("ListMembershipTenants after acceptance: %v", err)
	}
	found := false
	for _, summary := range tenants {
		if summary.TenantID == fixture.tenantID {
			found = true
			if summary.Role != "member" {
				t.Fatalf("accepted membership role = %q, want member", summary.Role)
			}
		}
	}
	if !found {
		t.Fatal("accepted membership is missing from the active-only tenant list")
	}
	selected, err := fixture.repo.SaveSelection(ctx, fixture.userB.verified, fixture.tenantID)
	if err != nil || selected.TenantID != fixture.tenantID {
		t.Fatalf("SaveSelection after acceptance = (%+v, %v), want the accepted tenant", selected, err)
	}
}

// TestAcceptInvitationReplayConvergesWithoutSecondTransition proves the
// replay convergence (design ruling 6): accepting again returns the same
// activation facts with no error, and the row keeps its original
// created_at — the no-second-transition invariant the row exposes.
func TestAcceptInvitationReplayConvergesWithoutSecondTransition(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	if _, _, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member"); err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	first, err := fixture.repo.AcceptInvitation(ctx, fixture.userB.verified, fixture.tenantID)
	if err != nil {
		t.Fatalf("first AcceptInvitation: %v", err)
	}

	var createdAt string
	if err := fixture.db.QueryRowContext(ctx, `
		SELECT created_at::text FROM memberships
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		fixture.tenantID, fixture.userB.userID).Scan(&createdAt); err != nil {
		t.Fatalf("read membership created_at: %v", err)
	}

	replayed, err := fixture.repo.AcceptInvitation(ctx, fixture.userB.verified, fixture.tenantID)
	if err != nil {
		t.Fatalf("replayed AcceptInvitation: %v", err)
	}
	if replayed != first {
		t.Fatalf("replayed acceptance = %+v, want the identical first activation %+v", replayed, first)
	}

	var createdAtAfter string
	if err := fixture.db.QueryRowContext(ctx, `
		SELECT created_at::text FROM memberships
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		fixture.tenantID, fixture.userB.userID).Scan(&createdAtAfter); err != nil {
		t.Fatalf("read membership created_at after replay: %v", err)
	}
	if createdAtAfter != createdAt {
		t.Fatalf("membership created_at changed across the replay: %q -> %q", createdAt, createdAtAfter)
	}
	if rows := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, ""); rows != 1 {
		t.Fatalf("membership rows after the replay = %d, want 1", rows)
	}
}

// TestAcceptInvitationDeniedPathsAreNotFound proves the acceptance
// denials (design ruling 6): a user with no row, a never-bound invitee,
// and a revoked membership are indistinguishable ErrInvitationNotFound
// rejections with zero writes.
func TestAcceptInvitationDeniedPathsAreNotFound(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	// No invitation row for user C in the tenant.
	if _, err := fixture.repo.AcceptInvitation(ctx, fixture.userC.verified, fixture.tenantID); !errors.Is(err, tenancy.ErrInvitationNotFound) {
		t.Fatalf("AcceptInvitation without an invitation = %v, want ErrInvitationNotFound", err)
	}

	// A never-bound identity has no user row to match.
	neverBound := identity.VerifiedIdentity{Issuer: fixture.userA.verified.Issuer, Subject: "subject-never-bound"}
	if _, err := fixture.repo.AcceptInvitation(ctx, neverBound, fixture.tenantID); !errors.Is(err, tenancy.ErrUserNotBound) {
		t.Fatalf("AcceptInvitation for a never-bound identity = %v, want ErrUserNotBound", err)
	}

	// A revoked invitation row is not acceptable.
	if _, _, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member"); err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	fixture.revokeMembership(t, ctx, fixture.tenantID, fixture.userB.userID)
	if _, err := fixture.repo.AcceptInvitation(ctx, fixture.userB.verified, fixture.tenantID); !errors.Is(err, tenancy.ErrInvitationNotFound) {
		t.Fatalf("AcceptInvitation over a revoked invitation = %v, want ErrInvitationNotFound", err)
	}

	if rows := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userC.userID, ""); rows != 0 {
		t.Fatalf("user C membership rows after the denials = %d, want 0", rows)
	}
	if status, _ := fixture.membershipRow(t, ctx, fixture.tenantID, fixture.userB.userID); status != "revoked" {
		t.Fatalf("revoked invitation row status = %q, want revoked (the denials wrote nothing)", status)
	}
}

// TestConcurrentInviteSamePairSingleRow proves the advisory-lock
// serialization (design ruling 2): 16 goroutines race the same
// (tenant, invitee); every delivery succeeds, exactly one inserts
// (created=true), and the final state holds exactly one membership row
// for the invitee.
func TestConcurrentInviteSamePairSingleRow(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	const goroutines = 16
	type outcome struct {
		invitation tenancy.MembershipInvitation
		created    bool
		err        error
	}
	outcomes := make([]outcome, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes[i].invitation, outcomes[i].created, outcomes[i].err = fixture.repo.InviteMember(
				ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member")
		}()
	}
	wg.Wait()

	creators := 0
	for i, oc := range outcomes {
		if oc.err != nil {
			t.Fatalf("goroutine %d InviteMember: %v", i, oc.err)
		}
		if oc.invitation != outcomes[0].invitation {
			t.Fatalf("goroutine %d invitation = %+v, want the converged invitation %+v", i, oc.invitation, outcomes[0].invitation)
		}
		if oc.created {
			creators++
		}
	}
	if creators != 1 {
		t.Fatalf("creating deliveries = %d, want exactly 1", creators)
	}
	if rows := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, ""); rows != 1 {
		t.Fatalf("membership rows after the race = %d, want 1", rows)
	}
}

// TestConcurrentAcceptConvergesOnSingleActivation proves the
// acceptance serialization: 16 goroutines race the acceptance of one
// invitation; every delivery succeeds with the same activation facts
// and the row is active exactly once.
func TestConcurrentAcceptConvergesOnSingleActivation(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	if _, _, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member"); err != nil {
		t.Fatalf("InviteMember: %v", err)
	}

	const goroutines = 16
	outcomes := make([]tenancy.ActivatedMembership, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes[i], errs[i] = fixture.repo.AcceptInvitation(ctx, fixture.userB.verified, fixture.tenantID)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d AcceptInvitation: %v", i, err)
		}
		if outcomes[i] != outcomes[0] {
			t.Fatalf("goroutine %d activation = %+v, want the converged activation %+v", i, outcomes[i], outcomes[0])
		}
	}
	status, _ := fixture.membershipRow(t, ctx, fixture.tenantID, fixture.userB.userID)
	if status != "active" {
		t.Fatalf("membership status after the race = %q, want active", status)
	}
	if rows := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, ""); rows != 1 {
		t.Fatalf("membership rows after the race = %d, want 1", rows)
	}
}

// TestInviteMemberFailureLeavesZeroResidue proves the transaction
// atomicity with a fault injected mid-transaction: a BEFORE INSERT
// trigger on memberships (the last statement of the invitation path)
// raises, so InviteMember must fail after the inviter validation, the
// invitee resolution, and the advisory lock all ran — and leave zero
// invitation rows behind. Dropping the trigger afterwards must leave
// the inviter free to invite successfully.
func TestInviteMemberFailureLeavesZeroResidue(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	mustExec(t, ctx, fixture.db, `
		CREATE FUNCTION t05_raise_on_membership_insert() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'injected invitation failure';
		END;
		$$ LANGUAGE plpgsql`)
	mustExec(t, ctx, fixture.db, `
		CREATE TRIGGER t05_fail_membership_insert
		BEFORE INSERT ON memberships
		FOR EACH ROW EXECUTE FUNCTION t05_raise_on_membership_insert()`)
	t.Cleanup(func() {
		mustExec(t, ctx, fixture.db, `DROP TRIGGER IF EXISTS t05_fail_membership_insert ON memberships`)
		mustExec(t, ctx, fixture.db, `DROP FUNCTION IF EXISTS t05_raise_on_membership_insert()`)
	})

	before := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, "")

	if _, _, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member"); err == nil {
		t.Fatal("InviteMember succeeded despite the injected invitation failure, want an error")
	}
	if after := fixture.countTenantMemberships(t, ctx, fixture.tenantID, fixture.userB.userID, ""); after != before {
		t.Fatalf("membership rows %d -> %d after the failed invitation, want unchanged", before, after)
	}

	mustExec(t, ctx, fixture.db, `DROP TRIGGER t05_fail_membership_insert ON memberships`)
	mustExec(t, ctx, fixture.db, `DROP FUNCTION t05_raise_on_membership_insert()`)

	invitation, created, err := fixture.repo.InviteMember(ctx, fixture.userA.verified, fixture.tenantID, fixture.userB.verified.Subject, "member")
	if err != nil {
		t.Fatalf("InviteMember after removing the injected failure: %v", err)
	}
	if !created || invitation.Status != "invited" {
		t.Fatalf("retry after restore = (%+v, %t), want a fresh invited row", invitation, created)
	}
}

// TestMembershipsStatusCheckContract proves the migration's database
// backstop with direct SQL: the extended vocabulary admits 'invited'
// while every pre-existing constraint keeps its shape — the role CHECK
// still rejects unknown roles and the partial one-active-owner index
// still rejects a second active owner.
func TestMembershipsStatusCheckContract(t *testing.T) {
	fixture := newInvitationFixture(t)
	ctx := context.Background()

	// The widened status vocabulary admits invited (the T05 subject).
	if _, err := fixture.db.ExecContext(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role, status)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'member', 'invited')`,
		fixture.tenantID, fixture.userC.userID); err != nil {
		t.Fatalf("invited membership rejected by the status CHECK: %v, want allowed", err)
	}

	// The role CHECK still rejects unknown roles.
	if _, err := fixture.db.ExecContext(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'queen')`,
		fixture.tenantID, fixture.userC.userID); err == nil {
		t.Fatal("unknown role accepted by the role CHECK, want rejected")
	}

	// The partial one-active-owner index still rejects a second active
	// owner (A already owns the tenant).
	if _, err := fixture.db.ExecContext(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'owner')`,
		fixture.tenantID, fixture.userB.userID); err == nil {
		t.Fatal("second active owner accepted by the partial unique index, want rejected")
	}
}
