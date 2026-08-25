package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/1123786563/myqypt/internal/platform/cli"
	"github.com/1123786563/myqypt/internal/platform/runtime"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

const defaultAddress = ":8080"
const listenAddressFileEnv = "PLATFORM_API_ADDR_FILE"

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	command := cli.NewRoot(version, serve)
	command.SetArgs(os.Args[1:])
	if err := command.ExecuteContext(ctx); err != nil {
		log.Printf("platform-api: %v", err)
		os.Exit(1)
	}
}

func listenAddress() string {
	if value := os.Getenv("PLATFORM_API_ADDR"); value != "" {
		return value
	}

	return defaultAddress
}

func serve(ctx context.Context) error {
	listener, err := net.Listen("tcp", listenAddress())
	if err != nil {
		return err
	}
	if err := reportListenAddress(listener.Addr().String()); err != nil {
		_ = listener.Close()
		return err
	}
	return runtime.Serve(ctx, listener, httptransport.NewRouter(httptransport.Dependencies{Version: version}), runtime.DefaultConfig())
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
