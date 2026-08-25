package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/1123786563/myqypt/db/migrations"
	"github.com/1123786563/myqypt/internal/adapter/postgres"
	"github.com/1123786563/myqypt/internal/application/readiness"
	"github.com/1123786563/myqypt/internal/platform/cli"
	"github.com/1123786563/myqypt/internal/platform/runtime"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

const defaultAddress = ":8080"
const listenAddressFileEnv = "PLATFORM_API_ADDR_FILE"

// readinessCheckTimeout bounds each /readyz dependency check.
const readinessCheckTimeout = 5 * time.Second

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	command := cli.NewRoot(version, serve, migrateFuncs())
	command.SetArgs(os.Args[1:])
	if err := command.ExecuteContext(ctx); err != nil {
		log.Printf("platform-api: %v", err)
		os.Exit(1)
	}
}

func migrateFuncs() cli.MigrateFuncs {
	return cli.MigrateFuncs{
		Up: func(ctx context.Context) error {
			return postgres.RunMigrateUp(ctx, os.Getenv("DATABASE_URL"), migrations.FS)
		},
		DownOne: func(ctx context.Context) error {
			return postgres.RunMigrateDownOne(ctx, os.Getenv("DATABASE_URL"), migrations.FS)
		},
	}
}

func listenAddress() string {
	if value := os.Getenv("PLATFORM_API_ADDR"); value != "" {
		return value
	}

	return defaultAddress
}

func serve(ctx context.Context) error {
	// Wiring readiness before the listener keeps an unparseable DATABASE_URL
	// a startup failure with no listener to clean up, while an unreachable
	// or missing database never blocks startup.
	readinessService, closePool, err := newReadinessService(ctx)
	if err != nil {
		return err
	}
	defer closePool()

	listener, err := net.Listen("tcp", listenAddress())
	if err != nil {
		return err
	}
	if err := reportListenAddress(listener.Addr().String()); err != nil {
		_ = listener.Close()
		return err
	}
	return runtime.Serve(ctx, listener, httptransport.NewRouter(httptransport.Dependencies{
		Version:   version,
		Readiness: readinessService,
	}), runtime.DefaultConfig())
}

// newReadinessService wires the /readyz dependency checks. Without
// DATABASE_URL the process still serves, reporting a fail-closed database
// check; with one, the pool is opened lazily (no connection attempt here,
// and never a migration — serve does not migrate). Only a DATABASE_URL pgx
// cannot even parse aborts startup. The returned closer releases the pool
// and is a no-op when none was opened.
func newReadinessService(ctx context.Context) (*readiness.Service, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return &readiness.Service{
			Checks: map[string]readiness.Checker{
				"database": postgres.UnconfiguredChecker{},
			},
			Timeout: readinessCheckTimeout,
		}, func() {}, nil
	}

	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("serve: %w", err)
	}
	return &readiness.Service{
		Checks: map[string]readiness.Checker{
			"database": postgres.NewHealthChecker(pool),
		},
		Timeout: readinessCheckTimeout,
	}, pool.Close, nil
}

func reportListenAddress(address string) error {
	path := os.Getenv(listenAddressFileEnv)
	if path == "" {
		return nil
	}

	tempFile := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tempFile, []byte(address+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tempFile, path); err != nil {
		_ = os.Remove(tempFile)
		return err
	}
	return nil
}
