package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/1123786563/myqypt/internal/adapter/postgres"
	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
)

// newBusinessTenantFixture provisions one bound user (with its personal
// tenant bundle) against one fresh database and returns the fixture plus
// the repository under test. The T03 tenancy fixture provisions two
// users; the business tenant journey only ever needs one owner at a time.
func newBusinessTenantFixture(t *testing.T) (*tenancyFixture, *postgres.TenancyRepository) {
	t.Helper()

	pool, db := openIdentityTestDB(t)

	ctx := context.Background()
	fixture := &tenancyFixture{
		db:         db,
		identities: postgres.NewIdentityRepository(pool),
	}
	fixture.userA = fixture.provisionUser(t, ctx, "https://issuer-business-tenant.test", "subject-business-tenant")
	return fixture, postgres.NewTenancyRepository(pool)
}

// businessTenantRow is the raw tenants row of one created business
// tenant: kind, display name, and the owner the schema recorded.
type businessTenantRow struct {
	kind        string
	displayName *string
	ownerUserID string
}

// loadBusinessTenant reads one tenants row straight from the table.
func (f *tenancyFixture) loadBusinessTenant(t *testing.T, ctx context.Context, tenantID string) businessTenantRow {
	t.Helper()

	var row businessTenantRow
	if err := f.db.QueryRowContext(ctx,
		`SELECT kind, display_name, owner_user_id::text FROM tenants WHERE id = $1::uuid`,
		tenantID).Scan(&row.kind, &row.displayName, &row.ownerUserID); err != nil {
		t.Fatalf("read business tenant row: %v", err)
	}
	return row
}

// countRows counts rows of one table restricted to one tenant id column.
func (f *tenancyFixture) countRows(t *testing.T, ctx context.Context, query string, args ...any) int {
	t.Helper()

	var rows int
	if err := f.db.QueryRowContext(ctx, query, args...).Scan(&rows); err != nil {
		t.Fatalf("count rows (%s): %v", query, err)
	}
	return rows
}

// countUserTenants returns the tenants owned by one user, optionally
// filtered by kind (empty kind counts every tenant).
func (f *tenancyFixture) countUserTenants(t *testing.T, ctx context.Context, userID, kind string) int {
	t.Helper()

	query := `SELECT count(*) FROM tenants WHERE owner_user_id = $1::uuid`
	args := []any{userID}
	if kind != "" {
		query += ` AND kind = $2`
		args = append(args, kind)
	}
	return f.countRows(t, ctx, query, args...)
}

// countCreations returns the business_tenant_creations row count for one
// actor (empty userID counts every row).
func (f *tenancyFixture) countCreations(t *testing.T, ctx context.Context, userID string) int {
	t.Helper()

	query := `SELECT count(*) FROM business_tenant_creations`
	var args []any
	if userID != "" {
		query += ` WHERE actor_user_id = $1::uuid`
		args = append(args, userID)
	}
	return f.countRows(t, ctx, query, args...)
}

// TestCreateBusinessTenantProvisionsAtomicBundle proves the atomic
// creation bundle (design ruling 4): one call inserts the business tenant
// with its display name, exactly one 1:1 billing customer, exactly one
// active owner membership, and exactly one creation mapping row.
func TestCreateBusinessTenantProvisionsAtomicBundle(t *testing.T) {
	fixture, repo := newBusinessTenantFixture(t)
	ctx := context.Background()

	created, isFirst, err := repo.CreateBusinessTenant(ctx, fixture.userA.verified, "Corner Cafe", "key-atomic")
	if err != nil {
		t.Fatalf("CreateBusinessTenant: %v", err)
	}
	if !isFirst {
		t.Fatal("created flag = false on the insert path, want true")
	}
	if created.TenantID == "" || created.DisplayName != "Corner Cafe" || created.CreatedAt.IsZero() {
		t.Fatalf("returned tenant = %+v, want a populated business tenant", created)
	}

	row := fixture.loadBusinessTenant(t, ctx, created.TenantID)
	if row.kind != "business" {
		t.Fatalf("tenants.kind = %q, want business", row.kind)
	}
	if row.displayName == nil || *row.displayName != "Corner Cafe" {
		t.Fatalf("tenants.display_name = %v, want Corner Cafe", row.displayName)
	}
	if row.ownerUserID != fixture.userA.userID {
		t.Fatalf("tenants.owner_user_id = %q, want the creator", row.ownerUserID)
	}

	if billing := fixture.countRows(t, ctx,
		`SELECT count(*) FROM billing_customers WHERE tenant_id = $1::uuid`, created.TenantID); billing != 1 {
		t.Fatalf("billing_customers rows = %d, want exactly 1 (ADR 0004 1:1)", billing)
	}
	if memberships := fixture.countRows(t, ctx,
		`SELECT count(*) FROM memberships WHERE tenant_id = $1::uuid AND role = 'owner' AND status = 'active'`,
		created.TenantID); memberships != 1 {
		t.Fatalf("active owner memberships = %d, want exactly 1", memberships)
	}
	if creations := fixture.countCreations(t, ctx, fixture.userA.userID); creations != 1 {
		t.Fatalf("business_tenant_creations rows = %d, want exactly 1", creations)
	}
	if total := fixture.countUserTenants(t, ctx, fixture.userA.userID, ""); total != 2 {
		t.Fatalf("user tenants = %d, want 2 (1 personal + 1 business)", total)
	}
}

// TestCreateBusinessTenantReplaySameKeyConverges proves the idempotency
// convergence: redelivering the same (user, key) returns the very same
// tenant with created=false and inserts nothing.
func TestCreateBusinessTenantReplaySameKeyConverges(t *testing.T) {
	fixture, repo := newBusinessTenantFixture(t)
	ctx := context.Background()

	first, isFirst, err := repo.CreateBusinessTenant(ctx, fixture.userA.verified, "Corner Cafe", "key-replay")
	if err != nil {
		t.Fatalf("first CreateBusinessTenant: %v", err)
	}
	if !isFirst {
		t.Fatal("first delivery created = false, want true")
	}

	replayed, isReplayCreate, err := repo.CreateBusinessTenant(ctx, fixture.userA.verified, "Corner Cafe", "key-replay")
	if err != nil {
		t.Fatalf("replayed CreateBusinessTenant: %v", err)
	}
	if isReplayCreate {
		t.Fatal("replay created = true, want false")
	}
	if replayed.TenantID != first.TenantID {
		t.Fatalf("replay tenant = %q, want the first tenant %q", replayed.TenantID, first.TenantID)
	}
	if replayed.DisplayName != first.DisplayName {
		t.Fatalf("replay display name = %q, want %q", replayed.DisplayName, first.DisplayName)
	}
	if business := fixture.countUserTenants(t, ctx, fixture.userA.userID, "business"); business != 1 {
		t.Fatalf("business tenants after the replay = %d, want 1", business)
	}
	if creations := fixture.countCreations(t, ctx, fixture.userA.userID); creations != 1 {
		t.Fatalf("business_tenant_creations rows after the replay = %d, want 1", creations)
	}
}

// TestCreateBusinessTenantDifferentKeyNewTenant proves the multi-entity
// design (design ruling 4): a different idempotency key provisions a
// second, distinct business tenant for the same user.
func TestCreateBusinessTenantDifferentKeyNewTenant(t *testing.T) {
	fixture, repo := newBusinessTenantFixture(t)
	ctx := context.Background()

	first, firstCreated, err := repo.CreateBusinessTenant(ctx, fixture.userA.verified, "Corner Cafe", "key-one")
	if err != nil {
		t.Fatalf("first CreateBusinessTenant: %v", err)
	}
	second, secondCreated, err := repo.CreateBusinessTenant(ctx, fixture.userA.verified, "Second Shop", "key-two")
	if err != nil {
		t.Fatalf("second CreateBusinessTenant: %v", err)
	}
	if !firstCreated || !secondCreated {
		t.Fatalf("created flags = (%t, %t), want both true", firstCreated, secondCreated)
	}
	if first.TenantID == second.TenantID {
		t.Fatal("different keys converged onto the same tenant, want distinct tenants")
	}
	if business := fixture.countUserTenants(t, ctx, fixture.userA.userID, "business"); business != 2 {
		t.Fatalf("business tenants = %d, want 2", business)
	}
	if creations := fixture.countCreations(t, ctx, fixture.userA.userID); creations != 2 {
		t.Fatalf("business_tenant_creations rows = %d, want 2", creations)
	}
	if row := fixture.loadBusinessTenant(t, ctx, second.TenantID); row.displayName == nil || *row.displayName != "Second Shop" {
		t.Fatalf("second tenant display_name = %v, want Second Shop", row.displayName)
	}
}

// TestConcurrentCreateBusinessTenantSameKeySingleTenant proves the
// advisory-lock serialization (design ruling 4): 16 goroutines race the
// same (user, key); every delivery succeeds, exactly one inserts
// (created=true), and the final state holds exactly one business tenant
// and one creation mapping.
func TestConcurrentCreateBusinessTenantSameKeySingleTenant(t *testing.T) {
	fixture, repo := newBusinessTenantFixture(t)
	ctx := context.Background()

	const goroutines = 16
	type outcome struct {
		tenant  tenancy.BusinessTenant
		created bool
		err     error
	}
	outcomes := make([]outcome, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes[i].tenant, outcomes[i].created, outcomes[i].err = repo.CreateBusinessTenant(
				ctx, fixture.userA.verified, "Corner Cafe", "key-race")
		}()
	}
	wg.Wait()

	creators := 0
	for i, oc := range outcomes {
		if oc.err != nil {
			t.Fatalf("goroutine %d CreateBusinessTenant: %v", i, oc.err)
		}
		if oc.tenant.TenantID != outcomes[0].tenant.TenantID {
			t.Fatalf("goroutine %d tenant = %q, want the converged tenant %q", i, oc.tenant.TenantID, outcomes[0].tenant.TenantID)
		}
		if oc.created {
			creators++
		}
	}
	if creators != 1 {
		t.Fatalf("creating deliveries = %d, want exactly 1", creators)
	}
	if business := fixture.countUserTenants(t, ctx, fixture.userA.userID, "business"); business != 1 {
		t.Fatalf("business tenants after the race = %d, want 1", business)
	}
	if creations := fixture.countCreations(t, ctx, fixture.userA.userID); creations != 1 {
		t.Fatalf("business_tenant_creations rows after the race = %d, want 1", creations)
	}
}

// TestCreateBusinessTenantUnboundIdentityRejected proves the owner
// precondition: a verified identity that was never bound holds no
// platform user and cannot own anything — rejected with ErrUserNotBound
// and zero rows written.
func TestCreateBusinessTenantUnboundIdentityRejected(t *testing.T) {
	fixture, repo := newBusinessTenantFixture(t)
	ctx := context.Background()

	tenantsBefore := fixture.countUserTenants(t, ctx, fixture.userA.userID, "")
	creationsBefore := fixture.countCreations(t, ctx, "")

	unbound := identity.VerifiedIdentity{
		Issuer:  "https://issuer-business-tenant.test",
		Subject: "subject-never-bound",
	}
	if _, _, err := repo.CreateBusinessTenant(ctx, unbound, "Corner Cafe", "key-unbound"); !errors.Is(err, tenancy.ErrUserNotBound) {
		t.Fatalf("CreateBusinessTenant error = %v, want ErrUserNotBound", err)
	}
	if after := fixture.countUserTenants(t, ctx, fixture.userA.userID, ""); after != tenantsBefore {
		t.Fatalf("tenants %d -> %d after the rejected creation, want unchanged", tenantsBefore, after)
	}
	if after := fixture.countCreations(t, ctx, ""); after != creationsBefore {
		t.Fatalf("creations %d -> %d after the rejected creation, want unchanged", creationsBefore, after)
	}
}

// TestTenantsDisplayNameCheckContract proves the database-level backstop
// (design ruling 3) with direct SQL: a business tenant row without a
// display name violates the CHECK and is rejected, while a personal
// tenant keeps display_name NULL. The service layer already rejects
// empty names before any write; this constraint is the belt-and-braces
// floor for any writer that bypasses the port.
func TestTenantsDisplayNameCheckContract(t *testing.T) {
	fixture, _ := newBusinessTenantFixture(t)
	ctx := context.Background()

	if _, err := fixture.db.ExecContext(ctx, `
		INSERT INTO tenants (id, owner_user_id, kind, display_name)
		VALUES (gen_random_uuid(), $1::uuid, 'business', NULL)`,
		fixture.userA.userID); err == nil {
		t.Fatal("business tenant with NULL display_name was accepted, want the CHECK to reject it")
	}

	// A fresh platform user with no personal tenant yet: only such a user
	// can legally receive the personal-NULL insert (migration 000003
	// caps one personal tenant per owner).
	var freshUserID string
	if err := fixture.db.QueryRowContext(ctx,
		`INSERT INTO platform_users (id) VALUES (gen_random_uuid()) RETURNING id::text`,
	).Scan(&freshUserID); err != nil {
		t.Fatalf("insert fixture platform user: %v", err)
	}
	if _, err := fixture.db.ExecContext(ctx, `
		INSERT INTO tenants (id, owner_user_id, kind)
		VALUES (gen_random_uuid(), $1::uuid, 'personal')`,
		freshUserID); err != nil {
		t.Fatalf("personal tenant with NULL display_name rejected: %v, want allowed", err)
	}
}

// TestCreateBusinessTenantFailureLeavesZeroResidue proves the transaction
// atomicity with a fault injected mid-transaction: a BEFORE INSERT
// trigger on business_tenant_creations (the last statement of the
// creation path) raises, so CreateBusinessTenant must fail after the
// tenant, billing customer, and membership inserts already ran — and the
// rollback must leave none of them behind. Dropping the trigger
// afterwards must leave the user free to create successfully.
func TestCreateBusinessTenantFailureLeavesZeroResidue(t *testing.T) {
	fixture, repo := newBusinessTenantFixture(t)
	ctx := context.Background()

	mustExec(t, ctx, fixture.db, `
		CREATE FUNCTION t04_raise_on_creation_insert() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'injected creation failure';
		END;
		$$ LANGUAGE plpgsql`)
	mustExec(t, ctx, fixture.db, `
		CREATE TRIGGER t04_fail_creation_insert
		BEFORE INSERT ON business_tenant_creations
		FOR EACH ROW EXECUTE FUNCTION t04_raise_on_creation_insert()`)
	t.Cleanup(func() {
		mustExec(t, ctx, fixture.db, `DROP TRIGGER IF EXISTS t04_fail_creation_insert ON business_tenant_creations`)
		mustExec(t, ctx, fixture.db, `DROP FUNCTION IF EXISTS t04_raise_on_creation_insert()`)
	})

	tenantsBefore := fixture.countUserTenants(t, ctx, fixture.userA.userID, "")
	billingBefore := fixture.countRows(t, ctx, `SELECT count(*) FROM billing_customers`)
	membershipsBefore := fixture.countRows(t, ctx, `SELECT count(*) FROM memberships`)

	if _, _, err := repo.CreateBusinessTenant(ctx, fixture.userA.verified, "Corner Cafe", "key-fault"); err == nil {
		t.Fatal("CreateBusinessTenant succeeded despite the injected creation failure, want an error")
	}
	if after := fixture.countUserTenants(t, ctx, fixture.userA.userID, ""); after != tenantsBefore {
		t.Fatalf("tenants %d -> %d after the failed creation, want unchanged", tenantsBefore, after)
	}
	if after := fixture.countRows(t, ctx, `SELECT count(*) FROM billing_customers`); after != billingBefore {
		t.Fatalf("billing_customers %d -> %d after the failed creation, want unchanged", billingBefore, after)
	}
	if after := fixture.countRows(t, ctx, `SELECT count(*) FROM memberships`); after != membershipsBefore {
		t.Fatalf("memberships %d -> %d after the failed creation, want unchanged", membershipsBefore, after)
	}
	if creations := fixture.countCreations(t, ctx, ""); creations != 0 {
		t.Fatalf("business_tenant_creations rows after the failed creation = %d, want 0", creations)
	}

	mustExec(t, ctx, fixture.db, `DROP TRIGGER t04_fail_creation_insert ON business_tenant_creations`)
	mustExec(t, ctx, fixture.db, `DROP FUNCTION t04_raise_on_creation_insert()`)

	created, isFirst, err := repo.CreateBusinessTenant(ctx, fixture.userA.verified, "Corner Cafe", "key-fault")
	if err != nil {
		t.Fatalf("CreateBusinessTenant after removing the injected failure: %v", err)
	}
	if !isFirst || created.TenantID == "" {
		t.Fatalf("retry after restore = (%+v, %t), want a fresh create", created, isFirst)
	}
	if creations := fixture.countCreations(t, ctx, fixture.userA.userID); creations != 1 {
		t.Fatalf("business_tenant_creations rows after restore = %d, want 1", creations)
	}
}
