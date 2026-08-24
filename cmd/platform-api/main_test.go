package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestAppHandlerRequiresPostgresReadinessConfiguration(t *testing.T) {
	t.Setenv("PLATFORM_POSTGRES_HOST", "")
	t.Setenv("PLATFORM_POSTGRES_PORT", "")
	t.Setenv("PLATFORM_POSTGRES_DB", "")
	t.Setenv("PLATFORM_POSTGRES_USER", "")
	t.Setenv("PLATFORM_POSTGRES_PASSWORD", "")

	handler := appHandler()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestAppHandlerChecksPostgresReachability(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	t.Setenv("PLATFORM_POSTGRES_HOST", "127.0.0.1")
	t.Setenv("PLATFORM_POSTGRES_PORT", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	t.Setenv("PLATFORM_POSTGRES_DB", "platform")
	t.Setenv("PLATFORM_POSTGRES_USER", "platform")
	t.Setenv("PLATFORM_POSTGRES_PASSWORD", "platform-pass")

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	handler := appHandler()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	<-acceptDone

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusOK)
	}
}

func TestPostgresAddressFromEnvDefaultsPort(t *testing.T) {
	t.Setenv("PLATFORM_POSTGRES_HOST", "postgres")
	t.Setenv("PLATFORM_POSTGRES_PORT", "")

	if got := postgresAddressFromEnv(); got != "postgres:5432" {
		t.Fatalf("address=%q want postgres:5432", got)
	}
}

func TestPostgresAddressFromEnvUsesConfiguredPort(t *testing.T) {
	t.Setenv("PLATFORM_POSTGRES_HOST", "postgres")
	t.Setenv("PLATFORM_POSTGRES_PORT", "6543")

	if got := postgresAddressFromEnv(); got != "postgres:6543" {
		t.Fatalf("address=%q want postgres:6543", got)
	}
}
