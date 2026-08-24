package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1123786563/myqypt/internal/platform"
)

const (
	defaultAddress    = ":8080"
	readTimeout       = 5 * time.Second
	readHeaderTimeout = 2 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 30 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	logger := log.New(os.Stdout, "platform-api ", log.LstdFlags|log.LUTC)
	server := &http.Server{
		Addr:              listenAddress(),
		Handler:           platform.New(platform.Dependencies{}),
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
