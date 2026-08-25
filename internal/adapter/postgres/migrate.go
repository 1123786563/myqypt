package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the database/sql "pgx" driver
	"github.com/pressly/goose/v3"
)

// errInvalidDatabaseURL is the static, URL-free error for a database URL
// pgx cannot parse, mirroring pool.Open: pgx's own parse error embeds the
// (password-masked) URL body, which must never be echoed.
var errInvalidDatabaseURL = errors.New("postgres: invalid database URL")

// migratePingTimeout bounds the fail-fast connectivity check on the migrate
// command path.
const migratePingTimeout = 5 * time.Second

// Migrate applies every pending up migration from fsys using goose. It is
// idempotent: a database already at the latest version is left untouched.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return fmt.Errorf("postgres: build migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("postgres: migrate up: %w", err)
	}
	return nil
}

// MigrateDownOne rolls back the most recently applied migration version from
// fsys.
func MigrateDownOne(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return fmt.Errorf("postgres: build migration provider: %w", err)
	}
	if _, err := provider.Down(ctx); err != nil {
		return fmt.Errorf("postgres: migrate down one: %w", err)
	}
	return nil
}

// RunMigrateUp is the migrate-up command path shared by the process
// entrypoint and tests: it opens a database handle via the pgx database/sql
// driver, fails fast when the database is unreachable within
// migratePingTimeout, applies all up migrations from fsys, and closes the
// handle afterwards.
func RunMigrateUp(ctx context.Context, databaseURL string, fsys fs.FS) error {
	return runMigrateCommand(ctx, databaseURL, fsys, Migrate)
}

// RunMigrateDownOne is the migrate down-one command path: the same
// connect, ping, then migrate flow as RunMigrateUp, rolling back exactly one
// migration version.
func RunMigrateDownOne(ctx context.Context, databaseURL string, fsys fs.FS) error {
	return runMigrateCommand(ctx, databaseURL, fsys, MigrateDownOne)
}

func runMigrateCommand(ctx context.Context, databaseURL string, fsys fs.FS, migrate func(context.Context, *sql.DB, fs.FS) error) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errInvalidDatabaseURL
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, migratePingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		// The pgx stdlib driver parses the DSN lazily at first connect, so
		// a shape error surfaces here; its error carries the URL body.
		var parseErr *pgconn.ParseConfigError
		if errors.As(err, &parseErr) {
			return errInvalidDatabaseURL
		}
		return fmt.Errorf("postgres: ping database before migrate: %w", err)
	}

	return migrate(ctx, db, fsys)
}
