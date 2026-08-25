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
	// On a fresh database down-one has nothing to roll back; that error is
	// irrelevant here, the subsequent Check outcome is what matters.
	_ = postgres.MigrateDownOne(ctx, db, migrations.FS)

	if err := checker.Check(ctx); err == nil {
		t.Fatal("check before migrate = nil, want failure on the un-migrated schema")
	}

	if err := postgres.Migrate(ctx, db, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if err := checker.Check(ctx); err != nil {
		t.Fatalf("check after migrate = %v, want nil", err)
	}

	if err := postgres.MigrateDownOne(ctx, db, migrations.FS); err != nil {
		t.Fatalf("migrate down one: %v", err)
	}
	if err := checker.Check(ctx); err == nil {
		t.Fatal("check after down-one = nil, want failure once the marker table is gone")
	}
}

func TestUnconfiguredCheckerAlwaysFails(t *testing.T) {
	if err := (postgres.UnconfiguredChecker{}).Check(context.Background()); err == nil {
		t.Fatal("check = nil, want the fixed fail-closed failure")
	}
}
