package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/1123786563/myqypt/db/migrations"
	"github.com/1123786563/myqypt/internal/adapter/postgres"
)

// TestHealthCheckerTracksMigrationState walks the readiness transition a
// real deployment goes through: un-migrated (failed) -> migrated (ok) ->
// rolled back (failed).
func TestHealthCheckerTracksMigrationState(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres integration test")
	}

	ctx := context.Background()

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open database handle: %v", err)
	}
	defer db.Close()

	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()

	checker := postgres.NewHealthChecker(pool)

	// Start from the un-migrated floor regardless of any prior test state.
	// Down-one rolls back a single version, so repeat until goose has
	// nothing left; the terminating error is irrelevant here, the
	// subsequent Check outcome is what matters.
	migrateDownToFloor(ctx, db)

	if err := checker.Check(ctx); err == nil {
		t.Fatal("check before migrate = nil, want failure on the un-migrated schema")
	}

	if err := postgres.Migrate(ctx, db, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if err := checker.Check(ctx); err != nil {
		t.Fatalf("check after migrate = %v, want nil", err)
	}

	// Roll back down to the floor: readiness must fail again once the
	// marker table is gone.
	migrateDownToFloor(ctx, db)
	if err := checker.Check(ctx); err == nil {
		t.Fatal("check after migrate-down = nil, want failure once the marker table is gone")
	}
}

// migrateDownToFloor rolls back one migration version at a time until
// goose has nothing left to roll back. The terminating error is expected
// and swallowed; the resulting schema floor is what callers assert on.
func migrateDownToFloor(ctx context.Context, db *sql.DB) {
	for {
		if err := postgres.MigrateDownOne(ctx, db, migrations.FS); err != nil {
			return
		}
	}
}

func TestUnconfiguredCheckerAlwaysFails(t *testing.T) {
	if err := (postgres.UnconfiguredChecker{}).Check(context.Background()); err == nil {
		t.Fatal("check = nil, want the fixed fail-closed failure")
	}
}
