# T01.1 Minimal Platform Scaffold 与 Test Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 Go Platform API、PostgreSQL migration、Docker Compose 开发栈，以及 acceptance、conformance、production-gates 共用的可执行证据 harness。

**Architecture:** Start a Go modular-monolith Control Plane with focused packages and a composition root; keep PostgreSQL behind repositories and external capabilities behind Provider/Adapter ports. The shared test harness loads declarative scenarios and delegates execution to registered acceptance, conformance, or Production Gate drivers so a health check cannot masquerade as evidence.

**Tech Stack:** Go 1.26, `net/http`, PostgreSQL, Docker Compose, YAML scenario contracts

**Spec:** [GitHub Issue #100](https://github.com/1123786563/myqypt/issues/100), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`

## Global Constraints

- Module path is `github.com/1123786563/myqypt`; Go language baseline is 1.26.
- Docker Compose is development/CI/integration only and never production evidence by itself.
- Platform PostgreSQL is the business source of truth; each later external dependency sits behind a typed port.
- The harness must record revision, dependency versions, timestamps, assertions, and redacted references while excluding customer content and Secrets.

---

### Task 1: Create the executable Platform composition root

**Files:**
- Create: `go.mod`
- Create: `cmd/platform-api/main.go`
- Create: `internal/platform/app.go`
- Create: `internal/platform/app_test.go`
- Create: `deploy/compose/compose.yaml`

**Interfaces:**
- Produces: `platform.New(platform.Dependencies) http.Handler`, `/livez`, and `/readyz`; readiness checks dependency reachability but is never an acceptance result.

- [ ] **Step 1: Write the failing route contract**

```go
func TestPlatformRoutes(t *testing.T) {
    handler := platform.New(platform.Dependencies{})
    request := httptest.NewRequest(http.MethodGet, "/livez", nil)
    response := httptest.NewRecorder()
    handler.ServeHTTP(response, request)
    if response.Code != http.StatusOK {
        t.Fatalf("status=%d", response.Code)
    }
}
```

- [ ] **Step 2: Confirm the repository has no executable module**

Run: `go test ./internal/platform -run TestPlatformRoutes -count=1`

Expected: FAIL because `go.mod` and `internal/platform` do not exist.

- [ ] **Step 3: Add the module and minimal composition root**

```go
type Dependencies struct{}

func New(_ Dependencies) http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(`{"status":"alive"}`))
    })
    return mux
}
```

Create `go.mod` with `module github.com/1123786563/myqypt` and `go 1.26.0`; `main.go` must build dependencies, start the HTTP server with explicit read/header/write/idle timeouts, and shut down on `SIGINT`/`SIGTERM`.

- [ ] **Step 4: Run the focused scaffold tests**

Run: `go test ./internal/platform ./cmd/platform-api -count=1`

Expected: PASS without requiring Docker.

### Task 2: Create the shared evidence harness and development stack

**Files:**
- Create: `tests/platformtest/scenario.go`
- Create: `tests/platformtest/run.go`
- Create: `tests/platformtest/run_test.go`
- Create: `tests/platformtest/schema/scenario.schema.json`
- Modify: `deploy/compose/compose.yaml`

**Interfaces:**
- Produces: `platformtest.Register(seam string, driver Driver)`, `platformtest.Run(t *testing.T, scenarioPath string) Report`, and `Driver.Execute(context.Context, Scenario) (Report, error)`.

- [ ] **Step 1: Write a failing test for unknown seams and redaction**

```go
func TestRunRejectsUnknownSeamWithoutLeakingInput(t *testing.T) {
    path := writeScenario(t, `seam: unknown
secret: must-not-appear
`)
    report := Run(t, path)
    if report.Passed || strings.Contains(report.Summary, "must-not-appear") {
        t.Fatalf("report=%+v", report)
    }
}
```

- [ ] **Step 2: Confirm the harness test fails**

Run: `go test ./tests/platformtest -run TestRunRejectsUnknownSeamWithoutLeakingInput -count=1`

Expected: FAIL because the scenario registry and redacted report do not exist.

- [ ] **Step 3: Implement the exact driver registry**

```go
type Driver interface {
    Execute(context.Context, Scenario) (Report, error)
}

var drivers sync.Map

func Register(seam string, driver Driver) {
    if seam == "" || driver == nil {
        panic("platformtest: seam and driver are required")
    }
    if _, loaded := drivers.LoadOrStore(seam, driver); loaded {
        panic("platformtest: duplicate seam " + seam)
    }
}
```

`Run` must parse with unknown-field rejection, select the registered driver, enforce a context timeout, redact keys matching `secret|token|prompt|document|payment_payload`, and write a JSON report beneath `artifacts/evidence/<scenario-id>/`.

- [ ] **Step 4: Add PostgreSQL and Keycloak to the Compose development stack**

```yaml
services:
  postgres:
    image: postgres:17
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U platform"]
      interval: 2s
      timeout: 2s
      retries: 30
  keycloak:
    image: quay.io/keycloak/keycloak:26.3
    command: ["start-dev", "--health-enabled=true"]
    depends_on:
      postgres:
        condition: service_healthy
```

Credentials come only from uncommitted environment files; committed Compose content uses variable references and fails fast when required values are absent.

- [ ] **Step 5: Verify focused and Compose integration separately**

Run focused: `go test ./tests/platformtest ./internal/platform -count=1`

Run integration: `docker compose -f deploy/compose/compose.yaml config && docker compose -f deploy/compose/compose.yaml up -d --wait`

Expected: tests PASS, Compose configuration resolves without embedded Secrets, and both dependencies become healthy.

- [ ] **Step 6: Commit the foundation**

```bash
git add go.mod go.sum cmd/platform-api internal/platform tests/platformtest deploy/compose/compose.yaml
git commit -m "build(platform): add executable scaffold and evidence harness"
```

## Self-Review Record

- Spec coverage: executable API, PostgreSQL/Keycloak dev dependencies, and all three named evidence seams have an owned foundation.
- Placeholder scan: exact paths, interfaces, redaction keys, commands, and expected outcomes are stated.
- Type consistency: the `Driver`, `Scenario`, `Report`, `Register`, and `Run` signatures are the shared contract consumed by later plans.
- Right-sizing: setup is folded into the harness deliverable that needs it; Identity Binding remains in #101.
