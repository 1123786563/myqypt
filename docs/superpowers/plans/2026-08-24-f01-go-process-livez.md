# F01 Go 进程骨架与存活检查 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付可启动、可探活、可报告版本并能在终止信号后优雅退出的最小 Go Platform API 进程。

**Architecture:** Cobra 只负责命令解析，`runtime.Server` 负责 `http.Server` 生命周期，Gin 只存在于 HTTP Transport。`/livez` 只证明进程事件循环存活，不检查数据库或外部依赖。

**Tech Stack:** Go 1.26.7, Gin 1.12.0, Cobra 1.10.2, standard `net/http`

**Spec:** [GitHub Issue #100](https://github.com/1123786563/myqypt/issues/100), `docs/superpowers/specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md`, `CONTEXT.md`

## Global Constraints

- Module path is `github.com/1123786563/myqypt`; pin toolchain `go1.26.7`.
- Gin is an HTTP Transport detail; Application and Domain interfaces expose only standard Go types.
- Configure read-header, read, write, idle, and shutdown timeouts explicitly.
- `/livez` never reports dependency readiness and never counts as acceptance evidence.
- Do not copy go-admin globals, JWT, Casbin, GORM, Swaggo, default credentials, or demo domains.

---

## File Structure

- Create `go.mod` for the pinned Go module and direct dependencies.
- Create `cmd/platform-api/main.go` as the process entrypoint only.
- Create `internal/platform/cli/root.go` for Cobra command construction.
- Create `internal/platform/cli/root_test.go` for version and serve behavior.
- Create `internal/platform/runtime/server.go` for `http.Server` lifecycle.
- Create `internal/platform/runtime/server_test.go` for shutdown behavior.
- Create `internal/transport/http/router.go` for the minimal Gin router.
- Create `internal/transport/http/router_test.go` for `/livez`.

### Task 1: Expose process liveness

**Files:**
- Create: `go.mod`
- Create: `internal/transport/http/router.go`
- Test: `internal/transport/http/router_test.go`

**Interfaces:**
- Produces: `httptransport.NewRouter() http.Handler` and `GET /livez -> 200 {"status":"alive"}`.

- [ ] **Step 1: Write the failing liveness test**

```go
package httptransport_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

func TestLivezReportsOnlyProcessLiveness(t *testing.T) {
    request := httptest.NewRequest(http.MethodGet, "/livez", nil)
    response := httptest.NewRecorder()
    httptransport.NewRouter().ServeHTTP(response, request)
    if response.Code != http.StatusOK { t.Fatalf("status=%d", response.Code) }
    if got := response.Body.String(); got != "{\"status\":\"alive\"}" { t.Fatalf("body=%q", got) }
}
```

- [ ] **Step 2: Run the test and confirm red**

Run: `go test ./internal/transport/http -run TestLivezReportsOnlyProcessLiveness -count=1`

Expected: FAIL because the module and `NewRouter` do not exist.

- [ ] **Step 3: Add the module and minimal router**

```go
package httptransport

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

func NewRouter() http.Handler {
    gin.SetMode(gin.ReleaseMode)
    router := gin.New()
    router.GET("/livez", func(c *gin.Context) {
        c.Header("Content-Type", "application/json")
        c.String(http.StatusOK, `{"status":"alive"}`)
    })
    return router
}
```

Create `go.mod` with `module github.com/1123786563/myqypt`, `go 1.26.0`, `toolchain go1.26.7`, Gin `v1.12.0`, and Cobra `v1.10.2`.

- [ ] **Step 4: Run the focused test**

Run: `go test ./internal/transport/http -count=1`

Expected: PASS without Docker or network dependencies.

- [ ] **Step 5: Commit the liveness slice**

```bash
git add go.mod go.sum internal/transport/http
git commit -m "feat(platform): add gin liveness transport"
```

### Task 2: Add commands and graceful lifecycle

**Files:**
- Create: `internal/platform/runtime/server.go`
- Create: `internal/platform/runtime/server_test.go`
- Create: `internal/platform/cli/root.go`
- Create: `internal/platform/cli/root_test.go`
- Create: `cmd/platform-api/main.go`

**Interfaces:**
- Consumes: `httptransport.NewRouter() http.Handler`.
- Produces: `runtime.Config`, `runtime.Serve(context.Context, net.Listener, http.Handler, Config) error`, and `cli.NewRoot(version string, serve func(context.Context) error) *cobra.Command`.

- [ ] **Step 1: Write lifecycle and command tests**

```go
func TestServeStopsAfterContextCancellation(t *testing.T) {
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil { t.Fatal(err) }
    ctx, cancel := context.WithCancel(context.Background())
    done := make(chan error, 1)
    go func() { done <- runtime.Serve(ctx, listener, httptransport.NewRouter(), runtime.DefaultConfig()) }()
    cancel()
    if err := <-done; err != nil { t.Fatal(err) }
}

func TestVersionDoesNotRunServer(t *testing.T) {
    calls := 0
    command := cli.NewRoot("be4cc10", func(context.Context) error { calls++; return nil })
    command.SetArgs([]string{"version"})
    if err := command.Execute(); err != nil { t.Fatal(err) }
    if calls != 0 { t.Fatalf("serve calls=%d", calls) }
}
```

- [ ] **Step 2: Run the tests and confirm red**

Run: `go test ./internal/platform/runtime ./internal/platform/cli -count=1`

Expected: FAIL because `Serve`, `DefaultConfig`, and `NewRoot` do not exist.

- [ ] **Step 3: Implement the lifecycle boundary**

```go
type Config struct {
    ReadHeaderTimeout time.Duration
    ReadTimeout time.Duration
    WriteTimeout time.Duration
    IdleTimeout time.Duration
    ShutdownTimeout time.Duration
}

func DefaultConfig() Config {
    return Config{5*time.Second, 15*time.Second, 30*time.Second, 60*time.Second, 10*time.Second}
}

func Serve(ctx context.Context, listener net.Listener, handler http.Handler, cfg Config) error {
    server := &http.Server{Handler: handler, ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout}
    result := make(chan error, 1)
    go func() { result <- server.Serve(listener) }()
    select {
    case err := <-result:
        if errors.Is(err, http.ErrServerClosed) { return nil }
        return err
    case <-ctx.Done():
        shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
        defer cancel()
        if err := server.Shutdown(shutdownCtx); err != nil { return err }
        err := <-result
        if errors.Is(err, http.ErrServerClosed) { return nil }
        return err
    }
}
```

`cli.NewRoot` creates only `serve` and `version`. `cmd/platform-api/main.go` builds the listener, uses a signal-aware command context, and exits non-zero on startup or shutdown failure.

- [ ] **Step 4: Run process tests and command smoke test**

Run: `go test ./internal/platform/runtime ./internal/platform/cli ./internal/transport/http -count=1`

Run: `go build -o /tmp/myqypt-platform-api ./cmd/platform-api && /tmp/myqypt-platform-api version`

Expected: tests PASS and the command prints exactly the injected version followed by a newline.

- [ ] **Step 5: Commit the process**

```bash
git add cmd/platform-api internal/platform
git commit -m "feat(platform): add command and graceful lifecycle"
```

## Self-Review Record

- Spec coverage: liveness, version, explicit timeouts, signal shutdown, Gin isolation, and no readiness substitution are covered.
- Placeholder scan: code-producing steps name exact files, symbols, commands, and outcomes.
- Type consistency: `NewRouter`, `DefaultConfig`, `Serve`, and `NewRoot` signatures are stable.
