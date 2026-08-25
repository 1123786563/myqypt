package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/1123786563/myqypt/internal/platform/cli"
	"github.com/1123786563/myqypt/internal/platform/runtime"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

const defaultAddress = ":8080"

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
	return runtime.Serve(ctx, listener, httptransport.NewRouter(), runtime.DefaultConfig())
}
