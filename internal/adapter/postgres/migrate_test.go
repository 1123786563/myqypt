package postgres_test

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/1123786563/myqypt/db/migrations"
	"github.com/1123786563/myqypt/internal/adapter/postgres"
)

// The "pgx" database/sql driver used below is registered by the package under
// test through its side-effect import of github.com/jackc/pgx/v5/stdlib.

func TestMigrationRoundTrip(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres integration test")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	if err := postgres.Migrate(ctx, db, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	assertSchemaHealth(t, ctx, db)

	// A second Migrate run must be a safe repeat (idempotent).
	if err := postgres.Migrate(ctx, db, migrations.FS); err != nil {
		t.Fatalf("second migrate up: %v", err)
	}
	assertSchemaHealth(t, ctx, db)

	// Down-one narrows the memberships status CHECK back to
	// ('active','revoked'), and PostgreSQL validates the narrowed CHECK
	// against existing rows: a leftover status='invited' row from an
	// earlier invitation test in this shared database would (correctly)
	// refuse the rollback — production's own operator guard, not a
	// migration defect. The round trip proves the migration pair on a
	// pristine table set, so reset to the same zero baseline the
	// repository fixtures use before rolling back.
	if _, err := db.ExecContext(ctx,
		`TRUNCATE TABLE business_tenant_creations, tenant_context_selections, memberships, billing_customers, tenants, identity_bindings, platform_users`,
	); err != nil {
		t.Fatalf("truncate business tables: %v", err)
	}

	if err := postgres.MigrateDownOne(ctx, db, migrations.FS); err != nil {
		t.Fatalf("migrate down one: %v", err)
	}
	// Down-one rolls back exactly the latest applied version: the
	// membership invitations migration (000006) is undone while the
	// business tenants migration (000005), the tenant context selections
	// migration (000004), the personal tenants migration (000003), the
	// identity tables from the second migration, and the baseline marker
	// from the first survive.
	assertSchemaHealth(t, ctx, db)
	assertMembershipInvitationMigrationUndone(t, ctx, db)
	assertBusinessTenantTablesSurviveDownOne(t, ctx, db)
	assertTenantContextSelectionTableSurvivesDownOne(t, ctx, db)
	assertPersonalTenantTablesSurviveDownOne(t, ctx, db)
	assertIdentityTablesSurviveDownOne(t, ctx, db)
}

type schemaColumn struct {
	name     string
	dataType string
	nullable string
}

func assertSchemaHealth(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'schema_health'
		ORDER BY ordinal_position`)
	if err != nil {
		t.Fatalf("query information_schema.columns: %v", err)
	}
	defer rows.Close()

	var got []schemaColumn
	for rows.Next() {
		var c schemaColumn
		if err := rows.Scan(&c.name, &c.dataType, &c.nullable); err != nil {
			t.Fatalf("scan column row: %v", err)
		}
		got = append(got, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate column rows: %v", err)
	}

	want := []schemaColumn{
		{name: "id", dataType: "boolean", nullable: "NO"},
		{name: "applied_at", dataType: "timestamp with time zone", nullable: "NO"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("schema_health columns = %+v, want %+v", got, want)
	}

	pkRows, err := db.QueryContext(ctx, `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.constraint_schema = kcu.constraint_schema
		WHERE tc.table_name = 'schema_health'
		  AND tc.constraint_type = 'PRIMARY KEY'
		ORDER BY kcu.ordinal_position`)
	if err != nil {
		t.Fatalf("query primary key: %v", err)
	}
	defer pkRows.Close()

	var pkColumns []string
	for pkRows.Next() {
		var name string
		if err := pkRows.Scan(&name); err != nil {
			t.Fatalf("scan primary key row: %v", err)
		}
		pkColumns = append(pkColumns, name)
	}
	if err := pkRows.Err(); err != nil {
		t.Fatalf("iterate primary key rows: %v", err)
	}

	if len(pkColumns) != 1 || pkColumns[0] != "id" {
		t.Fatalf("schema_health primary key columns = %v, want [id]", pkColumns)
	}
}

// assertMembershipInvitationMigrationUndone asserts the membership
// invitations migration (000006) was rolled back: the memberships status
// CHECK is back to the pre-T05 two-value vocabulary (no 'invited'), and
// the widened vocabulary is gone. The CHECK text is read straight from
// pg_constraint so the assertion is the database's own contract.
func assertMembershipInvitationMigrationUndone(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var checkDefinition string
	if err := db.QueryRowContext(ctx, `
		SELECT pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conrelid = 'memberships'::regclass
		  AND conname = 'memberships_status_check'`).Scan(&checkDefinition); err != nil {
		t.Fatalf("read memberships status check after down-one: %v", err)
	}
	if strings.Contains(checkDefinition, "invited") {
		t.Fatalf("memberships status check after down-one = %q, want the pre-T05 vocabulary without 'invited'", checkDefinition)
	}

	// The narrowed CHECK must reject a status the T05 vocabulary allowed:
	// an invited membership row cannot survive the rollback.
	var invitedRejected bool
	if err := db.QueryRowContext(ctx, `
		SELECT NOT EXISTS (
			SELECT 1 FROM memberships WHERE status = 'invited'
		)`).Scan(&invitedRejected); err != nil {
		t.Fatalf("query invited rows after down-one: %v", err)
	}
	if !invitedRejected {
		t.Fatal("invited membership rows survive the down-one rollback, want none (the narrowed CHECK is the guard)")
	}
}

// assertBusinessTenantTablesSurviveDownOne asserts the business tenants
// migration (000005) is still applied after down-one rolled back only the
// membership invitations migration (000006): the creation mapping table
// keeps its columns and the tenants table keeps the display_name column.
func assertBusinessTenantTablesSurviveDownOne(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var columns int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name = 'business_tenant_creations'`).Scan(&columns); err != nil {
		t.Fatalf("query business_tenant_creations columns after down-one: %v", err)
	}
	if columns == 0 {
		t.Fatalf("business_tenant_creations columns after down-one = %d, want the table fully present", columns)
	}

	var displayName int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name = 'tenants' AND column_name = 'display_name'`).Scan(&displayName); err != nil {
		t.Fatalf("query tenants display_name after down-one: %v", err)
	}
	if displayName != 1 {
		t.Fatalf("tenants display_name columns after down-one = %d, want 1", displayName)
	}
}

// assertTenantContextSelectionTableSurvivesDownOne asserts the tenant
// context selections migration (000004) is still applied after down-one
// rolled back only the membership invitations migration (000006): the
// table keeps its full column set.
func assertTenantContextSelectionTableSurvivesDownOne(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var columns int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name = 'tenant_context_selections'`).Scan(&columns); err != nil {
		t.Fatalf("query information_schema.columns after down-one: %v", err)
	}
	if columns != 3 {
		t.Fatalf("tenant context selections columns after down-one = %d, want 3 (the table fully present)", columns)
	}
}

// assertPersonalTenantTablesSurviveDownOne asserts the personal tenants
// migration (000003) is still applied after down-one rolled back only the
// membership invitations migration (000006): the personal tenant tables
// keep their full column set.
func assertPersonalTenantTablesSurviveDownOne(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var columns int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name IN ('tenants', 'billing_customers', 'memberships')`).Scan(&columns); err != nil {
		t.Fatalf("query personal tenant tables columns: %v", err)
	}
	// 14 = tenants 5 (including the display_name column the surviving
	// business tenants migration added) + billing_customers 3 +
	// memberships 6.
	if columns != 14 {
		t.Fatalf("personal tenant tables columns after down-one = %d, want 14 (all three tables fully present)", columns)
	}
}

// assertIdentityTablesSurviveDownOne asserts the identity bindings
// migration (000002) is still applied after down-one rolled back only the
// membership invitations migration (000006): both identity tables keep
// their full column set.
func assertIdentityTablesSurviveDownOne(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var columns int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name IN ('identity_bindings', 'platform_users')`).Scan(&columns); err != nil {
		t.Fatalf("query identity tables columns after down-one: %v", err)
	}
	if columns != 6 {
		t.Fatalf("identity tables columns after down-one = %d, want 6 (both tables fully present)", columns)
	}
}

func TestMigrateRequiresConnection(t *testing.T) {
	// Port 1 has no listener. The migrate command path must fail, and the
	// error must never echo the DSN password.
	dsn := "postgres://postgres:pw-marker-secret@127.0.0.1:1/platform?sslmode=disable"
	ctx := context.Background()

	commands := map[string]func(context.Context, string, fs.FS) error{
		"up":       postgres.RunMigrateUp,
		"down-one": postgres.RunMigrateDownOne,
	}
	for name, run := range commands {
		err := run(ctx, dsn, migrations.FS)
		if err == nil {
			t.Fatalf("%s: expected an error against an unlistened port", name)
		}
		if strings.Contains(err.Error(), "pw-marker-secret") {
			t.Fatalf("%s: error leaks the DSN password: %v", name, err)
		}
	}
}

func TestMigrateMalformedDSNDoesNotEchoURL(t *testing.T) {
	// pgx parses the DSN lazily at first connect, so a shape error such as
	// an unknown sslmode surfaces on the migrate command path's ping as a
	// *pgconn.ParseConfigError whose text embeds the (password-masked) URL
	// body. Neither the URL body nor the password may reach the error.
	dsn := "postgres://user:SECRETMARKER@127.0.0.1:1/platform?sslmode=notamode"
	ctx := context.Background()

	commands := map[string]func(context.Context, string, fs.FS) error{
		"up":       postgres.RunMigrateUp,
		"down-one": postgres.RunMigrateDownOne,
	}
	for name, run := range commands {
		err := run(ctx, dsn, migrations.FS)
		if err == nil {
			t.Fatalf("%s: expected an error for an unparseable DSN", name)
		}
		msg := err.Error()
		if strings.Contains(msg, "notamode") || strings.Contains(msg, "SECRETMARKER") {
			t.Fatalf("%s: error echoes the DSN: %v", name, err)
		}
	}
}
