package postgres_test

import (
	"context"
	"database/sql"
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
// migration, and returns a pgx pool for the repository under test plus a
// database/sql handle for row assertions. The skip guard mirrors
// migrate_test.go: without TEST_DATABASE_URL these stay integration tests.
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
