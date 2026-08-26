package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/1123786563/myqypt/internal/platform/runtime"
)

const (
	// processStartupTimeout is a readiness deadline, not a performance
	// assertion: under the full-parallel package load of `go test ./...`
	// (with TEST_DATABASE_URL set, so both process tests spawn real
	// binaries) the address-file wait can transiently exceed a tight
	// budget, so it stays generous while still bounding failure
	// detection.
	processStartupTimeout = 30 * time.Second
	processRequestTimeout = 250 * time.Millisecond
)

func TestPlatformAPIProcess(t *testing.T) {
	binary := buildPlatformAPI(t)

	first := startPlatformAPI(t, binary, "127.0.0.1:0", true)
	address := waitForReportedAddress(t, first)
	waitForLivez(t, first, address)
	assertVersionCommand(t, binary, address)

	second := startPlatformAPI(t, binary, address, false)
	err := waitForProcess(second, processStartupTimeout)
	if err == nil {
		t.Fatal("second process started on an occupied address")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("second process exit error = %T %[1]v, want *exec.ExitError", err)
	}

	assertStopsAfterSignal(t, first, syscall.SIGTERM)

	sigintProcess := startPlatformAPI(t, binary, "127.0.0.1:0", true)
	sigintAddress := waitForReportedAddress(t, sigintProcess)
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

type outputBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (buffer *outputBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.Write(data)
}

func (buffer *outputBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.String()
}

type platformProcess struct {
	command       *exec.Cmd
	output        *outputBuffer
	done          chan error
	addressFile   string
	reportAddress bool
}

func startPlatformAPI(t *testing.T, binary, address string, reportAddress bool) *platformProcess {
	t.Helper()
	command := exec.Command(binary, "serve")
	command.Env = append(command.Environ(), "PLATFORM_API_ADDR="+address)
	addressFile := filepath.Join(t.TempDir(), "platform-api-address")
	if reportAddress {
		command.Env = append(command.Env, "PLATFORM_API_ADDR_FILE="+addressFile)
	}
	output := &outputBuffer{}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		t.Fatalf("start platform-api: %v", err)
	}
	process := &platformProcess{
		command:       command,
		output:        output,
		done:          make(chan error, 1),
		addressFile:   addressFile,
		reportAddress: reportAddress,
	}
	go func() {
		process.done <- command.Wait()
	}()
	t.Cleanup(func() {
		if command.ProcessState == nil || !command.ProcessState.Exited() {
			_ = command.Process.Kill()
			if err := waitForProcess(process, processStartupTimeout); err != nil {
				t.Logf("cleanup wait for platform-api: %v\n%s", err, output.String())
			}
		}
	})
	return process
}

func waitForReportedAddress(t *testing.T, process *platformProcess) string {
	t.Helper()
	if !process.reportAddress {
		t.Fatal("process was not configured to report its address")
	}

	deadline := time.Now().Add(processStartupTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-process.done:
			t.Fatalf("platform-api exited before reporting its address: %v\n%s", err, process.output.String())
		default:
		}

		data, err := os.ReadFile(process.addressFile)
		if err == nil {
			address := strings.TrimSpace(string(data))
			if address != "" {
				return address
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read reported address: %v", err)
		}

		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("platform-api did not report its address within %s\n%s", processStartupTimeout, process.output.String())
	return ""
}

func assertVersionCommand(t *testing.T, binary, address string) {
	t.Helper()

	addressFile := filepath.Join(t.TempDir(), "platform-api-version-address")
	command := exec.Command(binary, "version")
	command.Env = append(
		command.Environ(),
		"PLATFORM_API_ADDR="+address,
		"PLATFORM_API_ADDR_FILE="+addressFile,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run platform-api version: %v\n%s", err, output)
	}
	if got := string(output); got != "process-test-version\n" {
		t.Fatalf("version output=%q want=%q", got, "process-test-version\n")
	}
	if _, err := os.Stat(addressFile); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			t.Fatalf("platform-api version unexpectedly reported a listen address")
		}
		t.Fatalf("stat version address file: %v", err)
	}
}

func waitForLivez(t *testing.T, process *platformProcess, address string) {
	t.Helper()
	deadline := time.Now().Add(processStartupTimeout)
	client := &http.Client{Timeout: processRequestTimeout}
	for time.Now().Before(deadline) {
		select {
		case err := <-process.done:
			t.Fatalf("platform-api exited before /livez became available: %v\n%s", err, process.output.String())
		default:
		}
		response, err := client.Get("http://" + address + "/livez")
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil && response.StatusCode == http.StatusOK && string(body) == "{\"status\":\"alive\"}" {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("/livez did not become available within %s\n%s", processStartupTimeout, process.output.String())
}

func assertStopsAfterSignal(t *testing.T, process *platformProcess, signal syscall.Signal) {
	t.Helper()
	shutdownStarted := time.Now()
	if err := process.command.Process.Signal(signal); err != nil {
		t.Fatalf("send %s: %v", signal, err)
	}
	if err := waitForProcess(process, runtime.DefaultConfig().ShutdownTimeout); err != nil {
		t.Fatalf("process did not exit cleanly after %s: %v", signal, err)
	}
	if elapsed := time.Since(shutdownStarted); elapsed > runtime.DefaultConfig().ShutdownTimeout {
		t.Fatalf("%s shutdown took %s, want at most %s", signal, elapsed, runtime.DefaultConfig().ShutdownTimeout)
	}
}

func waitForProcess(process *platformProcess, timeout time.Duration) error {
	select {
	case err := <-process.done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("process did not exit within %s", timeout)
	}
}
