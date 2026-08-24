package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/1123786563/myqypt/internal/platform/runtime"
)

const (
	processStartupTimeout = 5 * time.Second
	processRequestTimeout = 250 * time.Millisecond
)

func TestPlatformAPIProcess(t *testing.T) {
	binary := buildPlatformAPI(t)
	address := unusedAddress(t)

	first := startPlatformAPI(t, binary, address)
	waitForLivez(t, first, address)

	second := startPlatformAPI(t, binary, address)
	if err := waitForProcess(second, processStartupTimeout); err == nil {
		t.Fatal("second process started on an occupied address")
	}

	assertStopsAfterSignal(t, first, syscall.SIGTERM)

	sigintAddress := unusedAddress(t)
	sigintProcess := startPlatformAPI(t, binary, sigintAddress)
	waitForLivez(t, sigintProcess, sigintAddress)
	assertStopsAfterSignal(t, sigintProcess, syscall.SIGINT)
}

func buildPlatformAPI(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "platform-api")
	command := exec.Command("go", "build", "-o", binary, "-ldflags=-X main.version=process-test-version", ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build platform-api: %v\n%s", err, output)
	}
	return binary
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release address: %v", err)
	}
	return address
}

func startPlatformAPI(t *testing.T, binary, address string) *exec.Cmd {
	t.Helper()
	command := exec.Command(binary, "serve")
	command.Env = append(command.Environ(), "PLATFORM_API_ADDR="+address)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatalf("start platform-api: %v", err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil || !command.ProcessState.Exited() {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	return command
}

func waitForLivez(t *testing.T, process *exec.Cmd, address string) {
	t.Helper()
	deadline := time.Now().Add(processStartupTimeout)
	client := &http.Client{Timeout: processRequestTimeout}
	for time.Now().Before(deadline) {
		response, err := client.Get("http://" + address + "/livez")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if process.ProcessState != nil && process.ProcessState.Exited() {
			t.Fatalf("platform-api exited before /livez became available")
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("/livez did not become available within %s", processStartupTimeout)
}

func assertStopsAfterSignal(t *testing.T, process *exec.Cmd, signal syscall.Signal) {
	t.Helper()
	shutdownStarted := time.Now()
	if err := process.Process.Signal(signal); err != nil {
		t.Fatalf("send %s: %v", signal, err)
	}
	if err := waitForProcess(process, runtime.DefaultConfig().ShutdownTimeout); err != nil {
		t.Fatalf("process did not exit cleanly after %s: %v", signal, err)
	}
	if elapsed := time.Since(shutdownStarted); elapsed > runtime.DefaultConfig().ShutdownTimeout {
		t.Fatalf("%s shutdown took %s, want at most %s", signal, elapsed, runtime.DefaultConfig().ShutdownTimeout)
	}
}

func waitForProcess(command *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("process did not exit within %s", timeout)
	}
}
