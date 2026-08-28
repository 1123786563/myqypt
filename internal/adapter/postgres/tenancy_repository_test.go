package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/1123786563/myqypt/internal/adapter/postgres"
	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
)

// tenancyTestIdentity is one provisioned identity of a repository test:
// BindOrLoad on it creates the user with its personal tenant bundle, the
// fixture every tenancy operation below starts from.
type tenancyTestIdentity struct {
	verified identity.VerifiedIdentity
	userID   string
	tenantID string
}

// tenancyFixture drives one repository test: one migrated, truncated
// database; two provisioned users (each with its personal tenant); and
// direct-SQL fixture writes for the membership states no Stage-1 API can
// produce (design ruling 8).
type tenancyFixture struct {
	db         *sql.DB
	identities *postgres.IdentityRepository
	userA      tenancyTestIdentity
	userB      tenancyTestIdentity
}

// newTenancyFixture provisions two users against one fresh database and
// returns the fixture plus the repository under test. openIdentityTestDB
// already registers the pool/database cleanup.
func newTenancyFixture(t *testing.T) (*tenancyFixture, *postgres.TenancyRepository) {
	t.Helper()

	pool, db := openIdentityTestDB(t)

	ctx := context.Background()
	fixture := &tenancyFixture{
		db:         db,
		identities: postgres.NewIdentityRepository(pool),
	}
	fixture.userA = fixture.provisionUser(t, ctx, "https://issuer-tenancy-a.test", "subject-tenancy-a")
	fixture.userB = fixture.provisionUser(t, ctx, "https://issuer-tenancy-b.test", "subject-tenancy-b")
	return fixture, postgres.NewTenancyRepository(pool)
}

// provisionUser binds a fresh identity and looks up its personal tenant
// id, the T02 bundle the first bind provisions.
func (f *tenancyFixture) provisionUser(t *testing.T, ctx context.Context, provider, subject string) tenancyTestIdentity {
	t.Helper()

	verified := identity.VerifiedIdentity{Issuer: provider, Subject: subject}
	user, created, err := f.identities.BindOrLoad(ctx, provider, subject)
	if err != nil {
		t.Fatalf("BindOrLoad (%s, %s): %v", provider, subject, err)
	}
	if !created || user.ID == "" {
		t.Fatalf("BindOrLoad (%s, %s) created=%t id-empty=%t, want a fresh create", provider, subject, created, user.ID == "")
	}

	var tenantID string
	if err := f.db.QueryRowContext(ctx, `
		SELECT id::text
		FROM tenants
		WHERE owner_user_id = $1::uuid AND kind = 'personal'`,
		user.ID).Scan(&tenantID); err != nil {
		t.Fatalf("lookup personal tenant for (%s, %s): %v", provider, subject, err)
	}
	return tenancyTestIdentity{verified: verified, userID: user.ID, tenantID: tenantID}
}

// insertActiveMembership injects the fixture membership no Stage-1 API
// can produce: user holding role in tenant with status active.
func (f *tenancyFixture) insertActiveMembership(t *testing.T, ctx context.Context, tenantID, userID, role string) {
	t.Helper()
	if _, err := f.db.ExecContext(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3)`,
		tenantID, userID, role); err != nil {
		t.Fatalf("insert fixture membership: %v", err)
	}
}

// revokeMembership flips the fixture membership to revoked.
func (f *tenancyFixture) revokeMembership(t *testing.T, ctx context.Context, tenantID, userID string) {
	t.Helper()
	result, err := f.db.ExecContext(ctx, `
		UPDATE memberships SET status = 'revoked'
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		tenantID, userID)
	if err != nil {
		t.Fatalf("revoke fixture membership: %v", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("revoke fixture membership rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("revoke fixture membership affected %d rows, want 1", rows)
	}
}

// countSelectionRows returns the tenant_context_selections row count for
// one user (empty userID counts every row).
func (f *tenancyFixture) countSelectionRows(t *testing.T, ctx context.Context, userID string) int {
	t.Helper()
	query := `SELECT count(*) FROM tenant_context_selections`
	var args []any
	if userID != "" {
		query += ` WHERE platform_user_id = $1::uuid`
		args = append(args, userID)
	}
	var rows int
	if err := f.db.QueryRowContext(ctx, query, args...).Scan(&rows); err != nil {
		t.Fatalf("count tenant context selections: %v", err)
	}
	return rows
}

// selectionRow reads the persisted selection of one user straight from
// the table (the raw truth the re-validated read path is compared
// against).
func (f *tenancyFixture) selectionRow(t *testing.T, ctx context.Context, userID string) (string, bool) {
	t.Helper()
	var tenantID string
	err := f.db.QueryRowContext(ctx,
		`SELECT tenant_id::text FROM tenant_context_selections WHERE platform_user_id = $1::uuid`,
		userID).Scan(&tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read tenant context selection row: %v", err)
	}
	return tenantID, true
}

// TestListMembershipTenantsReturnsOnlyActiveMemberships proves the active
// filter: the list shows every active membership — the provisioned owner
// membership and an injected member membership — and the injected
// membership disappears the moment it is revoked. A never-bound identity
// holds no membership at all.
func TestListMembershipTenantsReturnsOnlyActiveMemberships(t *testing.T) {
	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	fixture.insertActiveMembership(t, ctx, fixture.userB.tenantID, fixture.userA.userID, "member")

	listed, err := repo.ListMembershipTenants(ctx, fixture.userA.verified)
	if err != nil {
		t.Fatalf("ListMembershipTenants: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("active memberships = %d, want 2 (owner + injected member)", len(listed))
	}
	byTenant := map[string]tenancy.TenantSummary{}
	for _, summary := range listed {
		byTenant[summary.TenantID] = summary
	}
	own, ok := byTenant[fixture.userA.tenantID]
	if !ok || own.Kind != "personal" || own.Role != "owner" {
		t.Fatalf("own tenant summary = %+v, want personal/owner", own)
	}
	injected, ok := byTenant[fixture.userB.tenantID]
	if !ok || injected.Kind != "personal" || injected.Role != "member" {
		t.Fatalf("injected tenant summary = %+v, want personal/member", injected)
	}

	fixture.revokeMembership(t, ctx, fixture.userB.tenantID, fixture.userA.userID)

	afterRevocation, err := repo.ListMembershipTenants(ctx, fixture.userA.verified)
	if err != nil {
		t.Fatalf("ListMembershipTenants after revocation: %v", err)
	}
	if len(afterRevocation) != 1 {
		t.Fatalf("active memberships after revocation = %d, want 1", len(afterRevocation))
	}
	if afterRevocation[0].TenantID != fixture.userA.tenantID {
		t.Fatalf("surviving membership tenant = %q, want the own tenant", afterRevocation[0].TenantID)
	}

	unbound, err := repo.ListMembershipTenants(ctx, identity.VerifiedIdentity{
		Issuer:  "https://issuer-tenancy-a.test",
		Subject: "subject-never-bound",
	})
	if err != nil {
		t.Fatalf("ListMembershipTenants for a never-bound identity: %v", err)
	}
	if len(unbound) != 0 {
		t.Fatalf("never-bound identity memberships = %d, want 0", len(unbound))
	}
}

// TestSelectedTenantFailsAfterMembershipRevocation proves the
// re-validation on read: a persisted selection keeps reading back while
// the membership stays active, reads as ErrNoTenantContext once the
// membership is revoked, and reads back again when the membership is
// re-activated — the selection row itself is never deleted.
func TestSelectedTenantFailsAfterMembershipRevocation(t *testing.T) {
	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	if _, err := repo.SelectedTenant(ctx, fixture.userA.verified); !errors.Is(err, tenancy.ErrNoTenantContext) {
		t.Fatalf("SelectedTenant without a selection = %v, want ErrNoTenantContext", err)
	}

	saved, err := repo.SaveSelection(ctx, fixture.userA.verified, fixture.userA.tenantID)
	if err != nil {
		t.Fatalf("SaveSelection: %v", err)
	}
	current, err := repo.SelectedTenant(ctx, fixture.userA.verified)
	if err != nil {
		t.Fatalf("SelectedTenant after save: %v", err)
	}
	if current.TenantID != fixture.userA.tenantID || current.SelectedAt.IsZero() || current.SelectedAt.Before(saved.SelectedAt) {
		t.Fatalf("SelectedTenant = %+v, want the saved selection", current)
	}

	fixture.revokeMembership(t, ctx, fixture.userA.tenantID, fixture.userA.userID)
	if _, err := repo.SelectedTenant(ctx, fixture.userA.verified); !errors.Is(err, tenancy.ErrNoTenantContext) {
		t.Fatalf("SelectedTenant after revocation = %v, want ErrNoTenantContext", err)
	}
	if _, exists := fixture.selectionRow(t, ctx, fixture.userA.userID); !exists {
		t.Fatal("revocation must not delete the persisted selection row")
	}

	if _, err := fixture.db.ExecContext(ctx, `
		UPDATE memberships SET status = 'active'
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		fixture.userA.tenantID, fixture.userA.userID); err != nil {
		t.Fatalf("re-activate membership: %v", err)
	}
	restored, err := repo.SelectedTenant(ctx, fixture.userA.verified)
	if err != nil {
		t.Fatalf("SelectedTenant after re-activation: %v", err)
	}
	if restored.TenantID != fixture.userA.tenantID {
		t.Fatalf("restored SelectedTenant tenant = %q, want %q", restored.TenantID, fixture.userA.tenantID)
	}
}

// TestSaveSelectionRejectsNonMemberWithZeroWrites proves the write
// rejection: selecting a tenant without an active membership — whether
// another user's tenant, a nonexistent tenant, or as a never-bound
// identity — fails with ErrNotAnActiveMember and leaves the selection
// table untouched.
func TestSaveSelectionRejectsNonMemberWithZeroWrites(t *testing.T) {
	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	cases := []struct {
		name     string
		verified identity.VerifiedIdentity
		tenantID string
	}{
		{"another user's tenant", fixture.userA.verified, fixture.userB.tenantID},
		{"nonexistent tenant", fixture.userA.verified, "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e8ff"},
		{"never-bound identity", identity.VerifiedIdentity{Issuer: "https://issuer-tenancy-a.test", Subject: "subject-never-bound"}, fixture.userA.tenantID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := fixture.countSelectionRows(t, ctx, "")
			if _, err := repo.SaveSelection(ctx, tc.verified, tc.tenantID); !errors.Is(err, tenancy.ErrNotAnActiveMember) {
				t.Fatalf("SaveSelection error = %v, want ErrNotAnActiveMember", err)
			}
			if after := fixture.countSelectionRows(t, ctx, ""); after != before {
				t.Fatalf("selection rows %d -> %d after the rejected write, want unchanged", before, after)
			}
		})
	}
}

// TestSaveSelectionSwitchIsLastWriteWins proves the switch: with an
// active membership in a second tenant, selecting it overwrites the
// previous selection (one row per user) and the re-validated read
// reflects the new tenant.
func TestSaveSelectionSwitchIsLastWriteWins(t *testing.T) {
	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	fixture.insertActiveMembership(t, ctx, fixture.userB.tenantID, fixture.userA.userID, "member")

	first, err := repo.SaveSelection(ctx, fixture.userA.verified, fixture.userA.tenantID)
	if err != nil {
		t.Fatalf("SaveSelection own tenant: %v", err)
	}
	second, err := repo.SaveSelection(ctx, fixture.userA.verified, fixture.userB.tenantID)
	if err != nil {
		t.Fatalf("SaveSelection switched tenant: %v", err)
	}
	if second.TenantID != fixture.userB.tenantID {
		t.Fatalf("switched selection tenant = %q, want %q", second.TenantID, fixture.userB.tenantID)
	}
	if second.SelectedAt.Before(first.SelectedAt) {
		t.Fatalf("switched selected_at %v precedes the first %v", second.SelectedAt, first.SelectedAt)
	}
	if rows := fixture.countSelectionRows(t, ctx, fixture.userA.userID); rows != 1 {
		t.Fatalf("selection rows for the user = %d, want 1 after the switch", rows)
	}
	current, err := repo.SelectedTenant(ctx, fixture.userA.verified)
	if err != nil {
		t.Fatalf("SelectedTenant after switch: %v", err)
	}
	if current.TenantID != fixture.userB.tenantID {
		t.Fatalf("current tenant after switch = %q, want %q", current.TenantID, fixture.userB.tenantID)
	}
	if row, exists := fixture.selectionRow(t, ctx, fixture.userA.userID); !exists || row != fixture.userB.tenantID {
		t.Fatalf("persisted row = (%q, %t), want the switched tenant", row, exists)
	}
}

// TestConcurrentSaveSelectionKeepsConsistentFinalState proves the
// advisory-lock serialization: 16 goroutines race selections of the same
// user between two tenants; every delivery succeeds, exactly one row
// survives, and the re-validated read agrees with the persisted row.
func TestConcurrentSaveSelectionKeepsConsistentFinalState(t *testing.T) {
	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	fixture.insertActiveMembership(t, ctx, fixture.userB.tenantID, fixture.userA.userID, "member")
	tenants := []string{fixture.userA.tenantID, fixture.userB.tenantID}

	const goroutines = 16
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = repo.SaveSelection(ctx, fixture.userA.verified, tenants[i%len(tenants)])
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d SaveSelection: %v", i, err)
		}
	}
	if rows := fixture.countSelectionRows(t, ctx, fixture.userA.userID); rows != 1 {
		t.Fatalf("selection rows after the race = %d, want 1", rows)
	}
	rowTenant, _ := fixture.selectionRow(t, ctx, fixture.userA.userID)
	current, err := repo.SelectedTenant(ctx, fixture.userA.verified)
	if err != nil {
		t.Fatalf("SelectedTenant after the race: %v", err)
	}
	if current.TenantID != rowTenant {
		t.Fatalf("current tenant %q disagrees with the persisted row %q", current.TenantID, rowTenant)
	}
	if current.TenantID != tenants[0] && current.TenantID != tenants[1] {
		t.Fatalf("final selection %q is neither raced tenant", current.TenantID)
	}
}

// insertInvitedMembership injects the fixture membership no Stage-1 API
// can produce on this path: a user holding role in tenant with the
// pending status invited (T05 vocabulary; T06 must treat it as not yet
// an active membership).
func (f *tenancyFixture) insertInvitedMembership(t *testing.T, ctx context.Context, tenantID, userID, role string) {
	t.Helper()
	if _, err := f.db.ExecContext(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role, status)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3, 'invited')`,
		tenantID, userID, role); err != nil {
		t.Fatalf("insert fixture invited membership: %v", err)
	}
}

// TestActiveMembershipRoleResolvesEachRole proves the T06 role
// resolution: for every role of the four-role memberships CHECK the
// single SELECT answers exactly that role — the provisioned owner
// membership plus one injected active membership each for admin,
// billing_member, and member, spread over distinct (tenant, user)
// pairs.
func TestActiveMembershipRoleResolvesEachRole(t *testing.T) {
	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	userC := fixture.provisionUser(t, ctx, "https://issuer-tenancy-c.test", "subject-tenancy-c")
	fixture.insertActiveMembership(t, ctx, fixture.userA.tenantID, fixture.userB.userID, "admin")
	fixture.insertActiveMembership(t, ctx, fixture.userB.tenantID, fixture.userA.userID, "billing_member")
	fixture.insertActiveMembership(t, ctx, fixture.userA.tenantID, userC.userID, "member")

	cases := []struct {
		name     string
		verified identity.VerifiedIdentity
		tenantID string
		want     string
	}{
		{"owner", fixture.userA.verified, fixture.userA.tenantID, "owner"},
		{"admin", fixture.userB.verified, fixture.userA.tenantID, "admin"},
		{"billing_member", fixture.userA.verified, fixture.userB.tenantID, "billing_member"},
		{"member", userC.verified, fixture.userA.tenantID, "member"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role, err := repo.ActiveMembershipRole(ctx, tc.verified, tc.tenantID)
			if err != nil {
				t.Fatalf("ActiveMembershipRole: %v", err)
			}
			if role != tc.want {
				t.Fatalf("ActiveMembershipRole = %q, want %q", role, tc.want)
			}
		})
	}
}

// TestActiveMembershipRoleClassifiesRejections proves the two rejection
// classes of the T06 role resolution and their no-oracle collapse: a
// bound principal with no active membership in the tenant — merely
// invited, revoked, a never-member stranger, or an unknown tenant — all
// answer the identical ErrNotAnActiveMember sentinel, while a
// never-bound identity answers ErrUserNotBound.
func TestActiveMembershipRoleClassifiesRejections(t *testing.T) {
	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	// userA holds only an invited (pending) row in userB's tenant; userB
	// holds an active member row in userA's tenant; the provisioned userC
	// holds no row in either of them.
	fixture.insertInvitedMembership(t, ctx, fixture.userB.tenantID, fixture.userA.userID, "admin")
	fixture.insertActiveMembership(t, ctx, fixture.userA.tenantID, fixture.userB.userID, "member")
	userC := fixture.provisionUser(t, ctx, "https://issuer-tenancy-c.test", "subject-tenancy-c")

	cases := []struct {
		name     string
		verified identity.VerifiedIdentity
		tenantID string
		wantErr  error
	}{
		{"invited not accepted", fixture.userA.verified, fixture.userB.tenantID, tenancy.ErrNotAnActiveMember},
		{"never a member", userC.verified, fixture.userA.tenantID, tenancy.ErrNotAnActiveMember},
		{"unknown tenant", fixture.userA.verified, "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e8ff", tenancy.ErrNotAnActiveMember},
		{"never-bound identity", identity.VerifiedIdentity{Issuer: "https://issuer-tenancy-a.test", Subject: "subject-never-bound"}, fixture.userA.tenantID, tenancy.ErrUserNotBound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role, err := repo.ActiveMembershipRole(ctx, tc.verified, tc.tenantID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ActiveMembershipRole error = %v, want %v", err, tc.wantErr)
			}
			if role != "" {
				t.Fatalf("ActiveMembershipRole role = %q, want empty on rejection", role)
			}
		})
	}

	// Revocation collapses onto the same sentinel: userB's active member
	// row in userA's tenant flips to revoked and the resolution answers
	// ErrNotAnActiveMember.
	fixture.revokeMembership(t, ctx, fixture.userA.tenantID, fixture.userB.userID)
	if _, err := repo.ActiveMembershipRole(ctx, fixture.userB.verified, fixture.userA.tenantID); !errors.Is(err, tenancy.ErrNotAnActiveMember) {
		t.Fatalf("ActiveMembershipRole after revocation = %v, want ErrNotAnActiveMember", err)
	}
}

// TestSaveSelectionFailureLeavesZeroResidue proves the transaction
// atomicity with a fault injected mid-transaction: a BEFORE INSERT
// trigger on tenant_context_selections (the last statement of the save
// path) raises, so SaveSelection must fail while the active-membership
// validation, the advisory lock, and the identity resolution all ran —
// and leave zero selection rows behind. Dropping the trigger afterwards
// must leave the identity free to select successfully.
func TestSaveSelectionFailureLeavesZeroResidue(t *testing.T) {
	fixture, repo := newTenancyFixture(t)
	ctx := context.Background()

	mustExec(t, ctx, fixture.db, `
		CREATE FUNCTION t03_raise_on_selection_insert() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'injected selection failure';
		END;
		$$ LANGUAGE plpgsql`)
	mustExec(t, ctx, fixture.db, `
		CREATE TRIGGER t03_fail_selection_insert
		BEFORE INSERT ON tenant_context_selections
		FOR EACH ROW EXECUTE FUNCTION t03_raise_on_selection_insert()`)
	t.Cleanup(func() {
		mustExec(t, ctx, fixture.db, `DROP TRIGGER IF EXISTS t03_fail_selection_insert ON tenant_context_selections`)
		mustExec(t, ctx, fixture.db, `DROP FUNCTION IF EXISTS t03_raise_on_selection_insert()`)
	})

	before := fixture.countSelectionRows(t, ctx, "")

	if _, err := repo.SaveSelection(ctx, fixture.userA.verified, fixture.userA.tenantID); err == nil {
		t.Fatal("SaveSelection succeeded despite the injected selection failure, want an error")
	}
	if after := fixture.countSelectionRows(t, ctx, ""); after != before {
		t.Fatalf("selection rows %d -> %d after the failed save, want unchanged", before, after)
	}

	mustExec(t, ctx, fixture.db, `DROP TRIGGER t03_fail_selection_insert ON tenant_context_selections`)
	mustExec(t, ctx, fixture.db, `DROP FUNCTION t03_raise_on_selection_insert()`)

	selected, err := repo.SaveSelection(ctx, fixture.userA.verified, fixture.userA.tenantID)
	if err != nil {
		t.Fatalf("SaveSelection after removing the injected failure: %v", err)
	}
	if selected.TenantID != fixture.userA.tenantID {
		t.Fatalf("SaveSelection after restore tenant = %q, want the own tenant", selected.TenantID)
	}
	if rows := fixture.countSelectionRows(t, ctx, fixture.userA.userID); rows != 1 {
		t.Fatalf("selection rows after restore = %d, want 1", rows)
	}
}
