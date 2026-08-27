package postgres_test

import (
	"context"
	"database/sql"
	"maps"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/1123786563/myqypt/db/migrations"
	"github.com/1123786563/myqypt/internal/adapter/postgres"
	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openIdentityTestDB connects to TEST_DATABASE_URL, applies every up
// migration, truncates the six business tables so every run starts from
// the same zero baseline, and returns a pgx pool for the repository under
// test plus a database/sql handle for row assertions. The skip guard
// mirrors migrate_test.go: without TEST_DATABASE_URL these stay
// integration tests.
func openIdentityTestDB(t *testing.T) (*pgxpool.Pool, *sql.DB) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres integration test")
	}

	ctx := context.Background()

	// The "pgx" database/sql driver is registered by the package under
	// test through its side-effect stdlib import (see migrate_test.go).
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := postgres.Migrate(ctx, db, migrations.FS); err != nil {
		db.Close()
		t.Fatalf("migrate up: %v", err)
	}

	// Reset the business tables to a clean state: on a persistent
	// database, identity rows survive TestMigrationRoundTrip's down-one
	// (it only rolls back 000004), so repeated runs — any -count, or a
	// second bare run — would otherwise collide with stale rows. All
	// six tables are listed because TRUNCATE requires every table
	// connected by foreign keys to be truncated together; uuid primary
	// keys carry no sequences, so no RESTART IDENTITY is needed. The
	// sixth table (tenant_context_selections) joined the list with T03
	// for the same repeat-safety: a previous round's selection rows
	// must not leak into the next round's baseline.
	if _, err := db.ExecContext(ctx,
		`TRUNCATE TABLE tenant_context_selections, memberships, billing_customers, tenants, identity_bindings, platform_users`,
	); err != nil {
		db.Close()
		t.Fatalf("truncate business tables: %v", err)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		db.Close()
		t.Fatalf("open pool: %v", err)
	}

	t.Cleanup(pool.Close)
	t.Cleanup(func() { _ = db.Close() })

	return pool, db
}

func TestBindIdempotentForRepeatedIssuerSubject(t *testing.T) {
	pool, db := openIdentityTestDB(t)
	repo := postgres.NewIdentityRepository(pool)
	ctx := context.Background()

	const provider = "https://issuer-idempotent.test"
	const subject = "subject-idempotent-1"

	first, created, err := repo.BindOrLoad(ctx, provider, subject)
	if err != nil {
		t.Fatalf("first BindOrLoad: %v", err)
	}
	if !created {
		t.Fatal("first BindOrLoad created = false, want true on the insert path")
	}
	if first.ID == "" {
		t.Fatal("first BindOrLoad returned an empty user id")
	}
	second, createdAgain, err := repo.BindOrLoad(ctx, provider, subject)
	if err != nil {
		t.Fatalf("second BindOrLoad: %v", err)
	}
	if createdAgain {
		t.Fatal("second BindOrLoad created = true, want false on the load path")
	}

	if first.ID != second.ID {
		t.Fatalf("repeated BindOrLoad user ids differ: %q vs %q", first.ID, second.ID)
	}
	assertBindingRows(t, ctx, db, provider, subject, 1)
	assertUserRows(t, ctx, db, provider, subject, 1)
}

func TestConcurrentBindReturnsSameUser(t *testing.T) {
	pool, db := openIdentityTestDB(t)
	repo := postgres.NewIdentityRepository(pool)
	ctx := context.Background()

	const provider = "https://issuer-concurrent.test"
	const subject = "subject-concurrent-1"

	const goroutines = 16
	users := make([]identity.User, goroutines)
	createdFlags := make([]bool, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			users[i], createdFlags[i], errs[i] = repo.BindOrLoad(ctx, provider, subject)
		}()
	}
	wg.Wait()

	createdCount := 0
	for i := range users {
		if errs[i] != nil {
			t.Fatalf("goroutine %d BindOrLoad: %v", i, errs[i])
		}
		if users[i].ID == "" {
			t.Fatalf("goroutine %d returned an empty user id", i)
		}
		if users[i].ID != users[0].ID {
			t.Fatalf("goroutine %d user id = %q, want %q", i, users[i].ID, users[0].ID)
		}
		if createdFlags[i] {
			createdCount++
		}
	}
	// The advisory xact lock serializes the deliveries: exactly one
	// transaction inserts (created=true) and every other one loads the
	// row it committed (created=false).
	if createdCount != 1 {
		t.Fatalf("created flags = %d true among %d deliveries, want exactly 1", createdCount, goroutines)
	}

	assertBindingRows(t, ctx, db, provider, subject, 1)
	assertUserRows(t, ctx, db, provider, subject, 1)
}

func TestSameSubjectDifferentIssuersAreDistinctUsers(t *testing.T) {
	pool, db := openIdentityTestDB(t)
	repo := postgres.NewIdentityRepository(pool)
	ctx := context.Background()

	const subject = "subject-shared-1"
	const providerA = "https://issuer-distinct-a.test"
	const providerB = "https://issuer-distinct-b.test"

	userA, createdA, err := repo.BindOrLoad(ctx, providerA, subject)
	if err != nil {
		t.Fatalf("BindOrLoad issuer A: %v", err)
	}
	if !createdA {
		t.Fatal("BindOrLoad issuer A created = false, want true on the insert path")
	}
	userB, createdB, err := repo.BindOrLoad(ctx, providerB, subject)
	if err != nil {
		t.Fatalf("BindOrLoad issuer B: %v", err)
	}
	if !createdB {
		t.Fatal("BindOrLoad issuer B created = false, want true on the insert path")
	}

	if userA.ID == userB.ID {
		t.Fatalf("different issuers mapped to the same user %q", userA.ID)
	}

	assertBindingRows(t, ctx, db, providerA, subject, 1)
	assertUserRows(t, ctx, db, providerA, subject, 1)
	assertBindingRows(t, ctx, db, providerB, subject, 1)
	assertUserRows(t, ctx, db, providerB, subject, 1)
}

// TestFirstBindProvisionsPersonalTenantBundle proves the T02 invariant on
// the create path: the first delivery of a new identity creates the user,
// its binding, and the complete personal-tenant bundle — one personal
// tenant, its 1:1 billing customer, and the active owner membership — in
// the same transaction.
func TestFirstBindProvisionsPersonalTenantBundle(t *testing.T) {
	pool, db := openIdentityTestDB(t)
	repo := postgres.NewIdentityRepository(pool)
	ctx := context.Background()

	const provider = "https://issuer-provision.test"
	const subject = "subject-provision-1"

	user, created, err := repo.BindOrLoad(ctx, provider, subject)
	if err != nil {
		t.Fatalf("first BindOrLoad: %v", err)
	}
	if !created {
		t.Fatal("first BindOrLoad created = false, want true on the insert path")
	}
	if user.ID == "" {
		t.Fatal("first BindOrLoad returned an empty user id")
	}

	assertBindingRows(t, ctx, db, provider, subject, 1)
	assertUserRows(t, ctx, db, provider, subject, 1)
	assertPersonalTenantBundle(t, ctx, db, provider, subject, 1)
}

// TestReplayedBindKeepsSingleTenantBundle proves the idempotency path:
// replaying the same identity takes the load path and never provisions a
// second bundle.
func TestReplayedBindKeepsSingleTenantBundle(t *testing.T) {
	pool, db := openIdentityTestDB(t)
	repo := postgres.NewIdentityRepository(pool)
	ctx := context.Background()

	const provider = "https://issuer-replay-bundle.test"
	const subject = "subject-replay-bundle-1"

	first, created, err := repo.BindOrLoad(ctx, provider, subject)
	if err != nil {
		t.Fatalf("first BindOrLoad: %v", err)
	}
	if !created {
		t.Fatal("first BindOrLoad created = false, want true on the insert path")
	}

	second, createdAgain, err := repo.BindOrLoad(ctx, provider, subject)
	if err != nil {
		t.Fatalf("replayed BindOrLoad: %v", err)
	}
	if createdAgain {
		t.Fatal("replayed BindOrLoad created = true, want false on the load path")
	}
	if first.ID != second.ID {
		t.Fatalf("replayed BindOrLoad user ids differ: %q vs %q", first.ID, second.ID)
	}

	assertPersonalTenantBundle(t, ctx, db, provider, subject, 1)
}

// TestSeparateIdentitiesGetSeparatePersonalTenants proves cross-identity
// isolation: every newly created user provisions its own tenant bundle,
// and no bundle row is ever shared between two identities.
func TestSeparateIdentitiesGetSeparatePersonalTenants(t *testing.T) {
	pool, db := openIdentityTestDB(t)
	repo := postgres.NewIdentityRepository(pool)
	ctx := context.Background()

	const provider = "https://issuer-isolate-bundle.test"
	const subjectA = "subject-isolate-bundle-a"
	const subjectB = "subject-isolate-bundle-b"

	userA, _, err := repo.BindOrLoad(ctx, provider, subjectA)
	if err != nil {
		t.Fatalf("BindOrLoad subject A: %v", err)
	}
	userB, _, err := repo.BindOrLoad(ctx, provider, subjectB)
	if err != nil {
		t.Fatalf("BindOrLoad subject B: %v", err)
	}
	if userA.ID == userB.ID {
		t.Fatalf("distinct subjects mapped to the same user %q", userA.ID)
	}

	assertPersonalTenantBundle(t, ctx, db, provider, subjectA, 1)
	assertPersonalTenantBundle(t, ctx, db, provider, subjectB, 1)
}

// TestConcurrentBindProvisionsExactlyOneTenantBundle proves the
// concurrency invariant: the advisory xact lock serializes concurrent
// first deliveries of one identity, so exactly one transaction provisions
// the bundle and the rest load it.
func TestConcurrentBindProvisionsExactlyOneTenantBundle(t *testing.T) {
	pool, db := openIdentityTestDB(t)
	repo := postgres.NewIdentityRepository(pool)
	ctx := context.Background()

	const provider = "https://issuer-concurrent-bundle.test"
	const subject = "subject-concurrent-bundle-1"

	const goroutines = 16
	users := make([]identity.User, goroutines)
	createdFlags := make([]bool, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			users[i], createdFlags[i], errs[i] = repo.BindOrLoad(ctx, provider, subject)
		}()
	}
	wg.Wait()

	createdCount := 0
	for i := range users {
		if errs[i] != nil {
			t.Fatalf("goroutine %d BindOrLoad: %v", i, errs[i])
		}
		if users[i].ID == "" {
			t.Fatalf("goroutine %d returned an empty user id", i)
		}
		if users[i].ID != users[0].ID {
			t.Fatalf("goroutine %d user id = %q, want %q", i, users[i].ID, users[0].ID)
		}
		if createdFlags[i] {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created flags = %d true among %d deliveries, want exactly 1", createdCount, goroutines)
	}

	assertPersonalTenantBundle(t, ctx, db, provider, subject, 1)
}

// TestBindFailureDuringProvisioningRollsBackAllRows proves the atomicity
// invariant with a fault injected mid-provisioning: a BEFORE INSERT
// trigger on memberships (the last provisioning statement of the create
// path) raises, so BindOrLoad must fail and leave zero rows for the
// identity across all five tables — the user and binding roll back with
// the tenant bundle. Dropping the trigger afterwards must leave the
// identity free to provision successfully.
func TestBindFailureDuringProvisioningRollsBackAllRows(t *testing.T) {
	pool, db := openIdentityTestDB(t)
	repo := postgres.NewIdentityRepository(pool)
	ctx := context.Background()

	const provider = "https://issuer-atomic-bundle.test"
	const subject = "subject-atomic-bundle-1"

	mustExec(t, ctx, db, `
		CREATE FUNCTION t02_raise_on_membership_insert() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'injected provisioning failure';
		END;
		$$ LANGUAGE plpgsql`)
	mustExec(t, ctx, db, `
		CREATE TRIGGER t02_fail_membership_insert
		BEFORE INSERT ON memberships
		FOR EACH ROW EXECUTE FUNCTION t02_raise_on_membership_insert()`)
	t.Cleanup(func() {
		mustExec(t, ctx, db, `DROP TRIGGER IF EXISTS t02_fail_membership_insert ON memberships`)
		mustExec(t, ctx, db, `DROP FUNCTION IF EXISTS t02_raise_on_membership_insert()`)
	})

	before := countAllBusinessRows(t, ctx, db)

	_, _, err := repo.BindOrLoad(ctx, provider, subject)
	if err == nil {
		t.Fatal("BindOrLoad succeeded despite the injected membership failure, want an error")
	}

	assertBindingRows(t, ctx, db, provider, subject, 0)
	assertUserRows(t, ctx, db, provider, subject, 0)
	assertPersonalTenantBundle(t, ctx, db, provider, subject, 0)
	if after := countAllBusinessRows(t, ctx, db); !maps.Equal(after, before) {
		t.Fatalf("business row totals changed after the failed bind: before=%v after=%v", before, after)
	}

	mustExec(t, ctx, db, `DROP TRIGGER t02_fail_membership_insert ON memberships`)
	mustExec(t, ctx, db, `DROP FUNCTION t02_raise_on_membership_insert()`)

	user, created, err := repo.BindOrLoad(ctx, provider, subject)
	if err != nil {
		t.Fatalf("BindOrLoad after removing the injected failure: %v", err)
	}
	if !created || user.ID == "" {
		t.Fatalf("BindOrLoad after restore created=%t user id empty=%t, want a fresh create", created, user.ID == "")
	}
	assertPersonalTenantBundle(t, ctx, db, provider, subject, 1)
}

func TestIdentitySchemaHasNoCascadeOrMutableKeyColumns(t *testing.T) {
	_, db := openIdentityTestDB(t)
	ctx := context.Background()

	type tableColumn struct {
		table    string
		name     string
		dataType string
		nullable string
	}

	rows, err := db.QueryContext(ctx, `
		SELECT table_name, column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name IN ('identity_bindings', 'platform_users')
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		t.Fatalf("query information_schema.columns: %v", err)
	}
	defer rows.Close()

	var got []tableColumn
	for rows.Next() {
		var c tableColumn
		if err := rows.Scan(&c.table, &c.name, &c.dataType, &c.nullable); err != nil {
			t.Fatalf("scan column row: %v", err)
		}
		got = append(got, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate column rows: %v", err)
	}

	// Exactly the ADR 0024 shape: no email/phone/username/organization_id
	// column may ever appear in either table.
	want := []tableColumn{
		{table: "identity_bindings", name: "identity_provider", dataType: "text", nullable: "NO"},
		{table: "identity_bindings", name: "subject", dataType: "text", nullable: "NO"},
		{table: "identity_bindings", name: "platform_user_id", dataType: "uuid", nullable: "NO"},
		{table: "identity_bindings", name: "created_at", dataType: "timestamp with time zone", nullable: "NO"},
		{table: "platform_users", name: "id", dataType: "uuid", nullable: "NO"},
		{table: "platform_users", name: "created_at", dataType: "timestamp with time zone", nullable: "NO"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("identity tables columns = %+v, want %+v", got, want)
	}

	fkRows, err := db.QueryContext(ctx, `
		SELECT rc.delete_rule, rc.update_rule
		FROM information_schema.referential_constraints rc
		JOIN information_schema.table_constraints tc
		  ON tc.constraint_name = rc.constraint_name
		 AND tc.constraint_schema = rc.constraint_schema
		WHERE tc.table_schema = current_schema()
		  AND tc.table_name = 'identity_bindings'
		  AND tc.constraint_type = 'FOREIGN KEY'`)
	if err != nil {
		t.Fatalf("query referential_constraints: %v", err)
	}
	defer fkRows.Close()

	type fkRule struct {
		deleteRule string
		updateRule string
	}
	var fks []fkRule
	for fkRows.Next() {
		var r fkRule
		if err := fkRows.Scan(&r.deleteRule, &r.updateRule); err != nil {
			t.Fatalf("scan foreign key row: %v", err)
		}
		fks = append(fks, r)
	}
	if err := fkRows.Err(); err != nil {
		t.Fatalf("iterate foreign key rows: %v", err)
	}

	// Exactly one foreign key, and it must carry no ON DELETE/UPDATE
	// action: a Keycloak-side delete can never cascade into platform data.
	if len(fks) != 1 {
		t.Fatalf("identity_bindings foreign keys = %+v, want exactly one", fks)
	}
	if fks[0].deleteRule != "NO ACTION" || fks[0].updateRule != "NO ACTION" {
		t.Fatalf("identity_bindings foreign key rules = %+v, want NO ACTION/NO ACTION", fks[0])
	}
}

// assertBindingRows asserts the exact number of identity_bindings rows for
// one (provider, subject) pair.
func assertBindingRows(t *testing.T, ctx context.Context, db *sql.DB, provider, subject string, want int) {
	t.Helper()

	var got int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM identity_bindings
		WHERE identity_provider = $1 AND subject = $2`,
		provider, subject).Scan(&got); err != nil {
		t.Fatalf("count identity_bindings: %v", err)
	}
	if got != want {
		t.Fatalf("identity_bindings rows for (%s, %s) = %d, want %d", provider, subject, got, want)
	}
}

// assertUserRows asserts the exact number of platform_users rows bound to
// one (provider, subject) pair.
func assertUserRows(t *testing.T, ctx context.Context, db *sql.DB, provider, subject string, want int) {
	t.Helper()

	var got int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM platform_users u
		WHERE EXISTS (
			SELECT 1
			FROM identity_bindings b
			WHERE b.platform_user_id = u.id
			  AND b.identity_provider = $1
			  AND b.subject = $2
		)`,
		provider, subject).Scan(&got); err != nil {
		t.Fatalf("count platform_users: %v", err)
	}
	if got != want {
		t.Fatalf("platform_users rows for (%s, %s) = %d, want %d", provider, subject, got, want)
	}
}

// tenantBundleRow is one observed provisioning row for the users bound to
// an identity: a tenant with its (left-joined) billing customer and
// memberships, so a single query exposes both the row counts and the
// exact shape of the T02 invariants.
type tenantBundleRow struct {
	kind             string
	hasBilling       bool
	billingPaired    bool
	membershipRole   sql.NullString
	membershipStatus sql.NullString
	membershipIsUser bool
}

// loadTenantBundleRows returns one row per (tenant, billing customer,
// membership) combination owned by the users bound to the identity.
func loadTenantBundleRows(t *testing.T, ctx context.Context, db *sql.DB, provider, subject string) []tenantBundleRow {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
		SELECT
			t.kind,
			(bc.tenant_id IS NOT NULL) AS has_billing,
			(bc.tenant_id = t.id) AS billing_paired,
			m.role AS membership_role,
			m.status AS membership_status,
			(m.user_id = t.owner_user_id) AS membership_is_owner
		FROM tenants t
		LEFT JOIN billing_customers bc ON bc.tenant_id = t.id
		LEFT JOIN memberships m ON m.tenant_id = t.id
		WHERE t.owner_user_id IN (
			SELECT platform_user_id
			FROM identity_bindings
			WHERE identity_provider = $1 AND subject = $2
		)`,
		provider, subject)
	if err != nil {
		t.Fatalf("query tenant bundle rows: %v", err)
	}
	defer rows.Close()

	var got []tenantBundleRow
	for rows.Next() {
		var r tenantBundleRow
		if err := rows.Scan(&r.kind, &r.hasBilling, &r.billingPaired, &r.membershipRole, &r.membershipStatus, &r.membershipIsUser); err != nil {
			t.Fatalf("scan tenant bundle row: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tenant bundle rows: %v", err)
	}
	return got
}

// assertPersonalTenantBundle asserts the complete T02 provisioning shape
// for one identity: exactly wantBundles tenants, each a personal tenant
// carrying its 1:1 billing customer and its active owner membership held
// by the bound user itself.
func assertPersonalTenantBundle(t *testing.T, ctx context.Context, db *sql.DB, provider, subject string, wantBundles int) {
	t.Helper()

	rows := loadTenantBundleRows(t, ctx, db, provider, subject)
	if len(rows) != wantBundles {
		t.Fatalf("tenant bundle rows for (%s, %s) = %d, want %d", provider, subject, len(rows), wantBundles)
	}
	for i, r := range rows {
		if r.kind != "personal" {
			t.Fatalf("bundle %d tenant kind = %q, want personal", i, r.kind)
		}
		if !r.hasBilling || !r.billingPaired {
			t.Fatalf("bundle %d billing customer is not paired 1:1 with its tenant (present=%t paired=%t)", i, r.hasBilling, r.billingPaired)
		}
		if !r.membershipRole.Valid || r.membershipRole.String != "owner" {
			t.Fatalf("bundle %d membership role = %v, want owner", i, r.membershipRole)
		}
		if !r.membershipStatus.Valid || r.membershipStatus.String != "active" {
			t.Fatalf("bundle %d membership status = %v, want active", i, r.membershipStatus)
		}
		if !r.membershipIsUser {
			t.Fatalf("bundle %d membership does not belong to the tenant's owning user", i)
		}
	}
}

// mustExec runs one statement, failing the test on error.
func mustExec(t *testing.T, ctx context.Context, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// countAllBusinessRows returns the total row count of the five business
// tables, used to prove a failed bind changed nothing anywhere.
func countAllBusinessRows(t *testing.T, ctx context.Context, db *sql.DB) map[string]int {
	t.Helper()

	tables := []string{"platform_users", "identity_bindings", "tenants", "billing_customers", "memberships"}
	totals := make(map[string]int, len(tables))
	for _, table := range tables {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s rows: %v", table, err)
		}
		totals[table] = count
	}
	return totals
}
