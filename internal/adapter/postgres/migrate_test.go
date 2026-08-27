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

	if err := postgres.MigrateDownOne(ctx, db, migrations.FS); err != nil {
		t.Fatalf("migrate down one: %v", err)
	}
	// Down-one rolls back exactly the latest applied version: the
	// personal tenants migration (000003) is undone while the identity
	// tables from the second migration and the baseline marker from the
	// first survive.
	assertSchemaHealth(t, ctx, db)
	assertPersonalTenantTablesGone(t, ctx, db)
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

// assertPersonalTenantTablesGone asserts the personal tenants migration
// (000003) was rolled back: none of tenants, billing_customers, or
// memberships has a column left.
func assertPersonalTenantTablesGone(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var columns int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name IN ('tenants', 'billing_customers', 'memberships')`).Scan(&columns); err != nil {
		t.Fatalf("query information_schema.columns after down-one: %v", err)
	}
	if columns != 0 {
		t.Fatalf("personal tenant tables columns after down-one = %d, want 0", columns)
	}
}

// assertIdentityTablesSurviveDownOne asserts the identity bindings
// migration (000002) is still applied after down-one rolled back only the
// personal tenants migration: both identity tables keep their full
// column set.
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
