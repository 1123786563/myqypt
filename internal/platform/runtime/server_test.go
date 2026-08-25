package runtime_test

import (
	"context"
	"net"
	"testing"

	"github.com/1123786563/myqypt/internal/platform/runtime"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

func TestServeStopsAfterContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runtime.Serve(ctx, listener, httptransport.NewRouter(httptransport.Dependencies{Version: "test"}), runtime.DefaultConfig())
	}()

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
