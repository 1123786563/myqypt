package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1123786563/myqypt/internal/platform"
)

const (
	defaultAddress      = ":8080"
	defaultPostgresPort = "5432"
	readTimeout         = 5 * time.Second
	readHeaderTimeout   = 2 * time.Second
	writeTimeout        = 10 * time.Second
	idleTimeout         = 30 * time.Second
	shutdownTimeout     = 10 * time.Second
)

func main() {
	logger := log.New(os.Stdout, "platform-api ", log.LstdFlags|log.LUTC)
	server := &http.Server{
		Addr:              listenAddress(),
		Handler:           appHandler(),
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		logger.Printf("shutdown signal received")
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("graceful shutdown failed: %v", err)
			os.Exit(1)
		}
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("server exited: %v", err)
			os.Exit(1)
		}
	}
}

func listenAddress() string {
	if value := os.Getenv("PLATFORM_API_ADDR"); value != "" {
		return value
	}

	return defaultAddress
}

func appHandler() http.Handler {
	return platform.New(platform.Dependencies{
		ReadinessDependencies: []platform.ReadinessDependency{
			postgresReadinessDependency{
				host:     os.Getenv("PLATFORM_POSTGRES_HOST"),
				port:     postgresPortFromEnv(),
				database: os.Getenv("PLATFORM_POSTGRES_DB"),
				user:     os.Getenv("PLATFORM_POSTGRES_USER"),
				password: os.Getenv("PLATFORM_POSTGRES_PASSWORD"),
			},
		},
	})
}

func postgresAddressFromEnv() string {
	host := os.Getenv("PLATFORM_POSTGRES_HOST")
	if host == "" {
		return ""
	}

	return net.JoinHostPort(host, postgresPortFromEnv())
}

func postgresPortFromEnv() string {
	if value := os.Getenv("PLATFORM_POSTGRES_PORT"); value != "" {
		return value
	}

	return defaultPostgresPort
}

type postgresReadinessDependency struct {
	host     string
	port     string
	database string
	user     string
	password string
}

func (d postgresReadinessDependency) Name() string {
	return "postgres"
}

func (d postgresReadinessDependency) CheckReadiness(ctx context.Context) error {
	switch {
	case d.host == "":
		return errors.New("host is required")
	case d.port == "":
		return errors.New("port is required")
	case d.database == "":
		return errors.New("database is required")
	case d.user == "":
		return errors.New("user is required")
	case d.password == "":
		return errors.New("password is required")
	}

	conn, err := new(net.Dialer).DialContext(ctx, "tcp", net.JoinHostPort(d.host, d.port))
	if err != nil {
		return err
	}

	return conn.Close()
}
