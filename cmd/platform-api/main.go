package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/1123786563/myqypt/db/migrations"
	"github.com/1123786563/myqypt/internal/adapter/oidc"
	"github.com/1123786563/myqypt/internal/adapter/postgres"
	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/readiness"
	"github.com/1123786563/myqypt/internal/platform/cli"
	"github.com/1123786563/myqypt/internal/platform/observability"
	"github.com/1123786563/myqypt/internal/platform/runtime"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/1123786563/myqypt/internal/transport/http/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultAddress = ":8080"
const listenAddressFileEnv = "PLATFORM_API_ADDR_FILE"

// allowedOriginsEnv carries the CORS origin allowlist, comma-separated.
const allowedOriginsEnv = "PLATFORM_API_ALLOWED_ORIGINS"

// identityOIDCIssuerEnv and identityOIDCAudienceEnv carry the OIDC issuer
// and audience the identity callback accepts. Both must be set for the
// endpoint to be wired (design ruling 6 fail-closed assembly).
const (
	identityOIDCIssuerEnv   = "PLATFORM_IDENTITY_OIDC_ISSUER"
	identityOIDCAudienceEnv = "PLATFORM_IDENTITY_OIDC_AUDIENCE"
)

// readinessCheckTimeout bounds each /readyz dependency check.
const readinessCheckTimeout = 5 * time.Second

// observabilityShutdownTimeout bounds the telemetry flush once the HTTP
// listener has drained.
const observabilityShutdownTimeout = 5 * time.Second

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

// observabilityConfig reads the standard OTEL_EXPORTER_OTLP_ENDPOINT (empty
// means no exporter, a network-free local run) plus the service identity
// with platform defaults. Only the composition root reads these variables.
func observabilityConfig() observability.Config {
	return observability.Config{
		ServiceName:           envOrDefault("OTEL_SERVICE_NAME", "platform-api"),
		ServiceVersion:        version,
		DeploymentEnvironment: envOrDefault("PLATFORM_DEPLOYMENT_ENVIRONMENT", "development"),
		OTLPEndpoint:          os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// securityConfig reads the CORS origin allowlist from
// PLATFORM_API_ALLOWED_ORIGINS (controller ruling addition to design ruling
// 10). Blank or unset yields an empty allowlist — fail-closed: security
// headers stay on, no origin receives a CORS grant.
func securityConfig() *middleware.SecurityConfig {
	return &middleware.SecurityConfig{AllowedOrigins: parseAllowedOrigins(os.Getenv(allowedOriginsEnv))}
}

// parseAllowedOrigins splits the comma-separated origin allowlist, trimming
// surrounding whitespace and dropping empty entries.
func parseAllowedOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var origins []string
	for _, entry := range strings.Split(raw, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			origins = append(origins, entry)
		}
	}
	return origins
}

// identityDependencies wires the identity callback assembly. Only when
// both PLATFORM_IDENTITY_OIDC_ISSUER and PLATFORM_IDENTITY_OIDC_AUDIENCE
// are set is the endpoint enabled: either one absent yields a nil assembly
// and the route stays unregistered. The verifier is lazy (no discovery or
// key fetch happens here), so startup never depends on identity provider
// reachability. pool may be nil (no DATABASE_URL): the endpoint is then
// registered but its repository port stays unwired, so every callback
// fails closed with 503 dependency_unavailable (design ruling 6).
func identityDependencies(pool *pgxpool.Pool) *httptransport.IdentityDependencies {
	issuer := os.Getenv(identityOIDCIssuerEnv)
	audience := os.Getenv(identityOIDCAudienceEnv)
	if issuer == "" || audience == "" {
		return nil
	}
	var repository identity.Repository
	if pool != nil {
		repository = postgres.NewIdentityRepository(pool)
	}
	return &httptransport.IdentityDependencies{
		Verifier:   oidc.NewVerifier(issuer, audience),
		Repository: repository,
	}
}

func serve(ctx context.Context) error {
	resources, err := observability.New(ctx, observabilityConfig())
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	// Wiring readiness before the listener keeps an unparseable DATABASE_URL
	// a startup failure with no listener to clean up, while an unreachable
	// or missing database never blocks startup.
	readinessService, databasePool, closePool, err := newReadinessService(ctx)
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
	serveErr := runtime.Serve(ctx, listener, httptransport.NewRouter(httptransport.Dependencies{
		Version:        version,
		Readiness:      readinessService,
		Logger:         resources.Logger,
		TracerProvider: resources.TracerProvider,
		Security:       securityConfig(),
		Identity:       identityDependencies(databasePool),
	}), runtime.DefaultConfig())
	// The listener has drained; flush telemetry with a fresh context because
	// ctx is already cancelled by the shutdown signal at this point.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), observabilityShutdownTimeout)
	defer cancel()
	return errors.Join(serveErr, resources.Shutdown(shutdownCtx))
}

// newReadinessService wires the /readyz dependency checks. Without
// DATABASE_URL the process still serves, reporting a fail-closed database
// check; with one, the pool is opened lazily (no connection attempt here,
// and never a migration — serve does not migrate). Only a DATABASE_URL pgx
// cannot even parse aborts startup. The pool is also returned so callers
// can reuse it for further wiring (the identity repository); it is nil when
// none was opened. The returned closer releases the pool and is a no-op
// when none was opened.
func newReadinessService(ctx context.Context) (*readiness.Service, *pgxpool.Pool, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return &readiness.Service{
			Checks: map[string]readiness.Checker{
				"database": postgres.UnconfiguredChecker{},
			},
			Timeout: readinessCheckTimeout,
		}, nil, func() {}, nil
	}

	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("serve: %w", err)
	}
	return &readiness.Service{
		Checks: map[string]readiness.Checker{
			"database": postgres.NewHealthChecker(pool),
		},
		Timeout: readinessCheckTimeout,
	}, pool, pool.Close, nil
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
