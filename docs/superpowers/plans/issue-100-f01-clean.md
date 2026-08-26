# Issue #100 F01 Clean Branch Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从 `d6eec55` 创建只包含 Issue #100 F01 进程闭环的可验证分支，并排除历史 Compose、PostgreSQL、Casdoor 与证据脚手架。

**Architecture:** 保留 F01 的标准 Go 模块、Gin HTTP Transport、Cobra CLI 和 `net/http` runtime 生命周期边界。新的分支只迁移 F01 所需源码与测试，不迁移旧分支中的开发依赖栈或 evidence harness。

**Tech Stack:** Go 1.26.7, Gin 1.12.0, Cobra 1.10.2, standard `net/http`

**Spec:** [GitHub Issue #100](https://github.com/1123786563/myqypt/issues/100), `docs/superpowers/plans/2026-08-24-f01-go-process-livez.md`

## Global Constraints

- Base commit is `d6eec55`; preserve the existing branch `codex/t01-1-platform-scaffold` unchanged.
- Module path is `github.com/1123786563/myqypt`; pin toolchain `go1.26.7`.
- Gin is only an HTTP Transport detail; application and runtime interfaces expose standard Go types.
- `/livez` reports process liveness only and never checks PostgreSQL, Casdoor, or other dependencies.
- The clean branch must not contain `deploy/compose/`, `deploy/compose/postgres-init/`, `tests/platformtest/`, or historical evidence report files.
- Do not push, merge, publish, close, comment on, or otherwise modify GitHub state.

---

## Preflight scope table

| Area | Required in clean F01 branch | Explicitly excluded | Decision |
|---|---|---|---|
| Process entrypoint | `cmd/platform-api/main.go`, `main_test.go` | PostgreSQL readiness wiring | Keep only signal-aware Cobra process behavior and process acceptance tests. |
| HTTP boundary | `internal/transport/http/*` | Application/domain Gin exposure | Keep Gin only in the transport package. |
| Runtime and CLI | `internal/platform/runtime/*`, `internal/platform/cli/*` | Old `internal/platform/app.go` scaffold | Keep runtime lifecycle and command parsing only. |
| Dependencies | `go.mod`, `go.sum` | Historical evidence harness dependencies | Keep only dependencies required by F01 source. |
| Development stack | None | Compose, PostgreSQL, Casdoor, init scripts | Exclude all non-F01 service files. |

## Task 1: Port the pure F01 implementation

**Files:**
- Create: `go.mod`, `go.sum`
- Create: `cmd/platform-api/main.go`, `cmd/platform-api/main_test.go`
- Create: `internal/platform/cli/root.go`, `internal/platform/cli/root_test.go`
- Create: `internal/platform/runtime/server.go`, `internal/platform/runtime/server_test.go`
- Create: `internal/transport/http/router.go`, `internal/transport/http/router_test.go`
- Create: `.gitignore`
- Do not create: `deploy/compose/**`, `tests/platformtest/**`, `internal/platform/app.go`, `internal/platform/app_test.go`, or evidence reports.

**Interfaces:**
- Produces `httptransport.NewRouter() http.Handler`.
- Produces `runtime.Config`, `runtime.DefaultConfig() runtime.Config`, and `runtime.Serve(context.Context, net.Listener, http.Handler, runtime.Config) error`.
- Produces `cli.NewRoot(version string, serve func(context.Context) error) *cobra.Command`.

- [ ] **Step 1: Port only the F01 source and tests from the preserved branch**

Use the already-reviewed F01 implementation at commit `a2efb20` as the source of the files listed above. Copy file contents into this branch while preserving the reviewed behavior: exact `/livez` JSON response, explicit read-header/read/write/idle/shutdown timeouts, version-only command behavior, signal-aware serving, occupied-port failure, and single-owner process waiting.

The clean branch must satisfy this source inventory:

```text
cmd/platform-api/main.go
cmd/platform-api/main_test.go
internal/platform/cli/root.go
internal/platform/cli/root_test.go
internal/platform/runtime/server.go
internal/platform/runtime/server_test.go
internal/transport/http/router.go
internal/transport/http/router_test.go
go.mod
go.sum
```

The following paths must remain absent:

```text
deploy/compose/
tests/platformtest/
internal/platform/app.go
internal/platform/app_test.go
```

- [ ] **Step 2: Run focused F01 tests under the pinned toolchain**

```bash
PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/transport/http ./internal/platform/runtime ./internal/platform/cli ./cmd/platform-api -count=1
```

Expected: all four F01 packages pass, including `TestPlatformAPIProcess`.

- [ ] **Step 3: Verify the clean source boundary**

```bash
test ! -e deploy/compose
test ! -e tests/platformtest
test ! -e internal/platform/app.go
test ! -e internal/platform/app_test.go
! rg -n 'postgres|casdoor|readyz|Readiness|evidence' cmd internal
```

Expected: every `test` succeeds and the boundary scan returns no matches.

- [ ] **Step 4: Run formatting, static checks, race tests, build, and version smoke**

```bash
PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local gofmt -w cmd internal
PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local go vet ./...
PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local go test ./... -count=1
PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local go test -race ./... -count=1
PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local go build -o /tmp/myqypt-platform-api-f01-clean ./cmd/platform-api
/tmp/myqypt-platform-api-f01-clean version
git diff --check
```

Expected: all commands exit 0 and `version` prints `dev` followed by a newline.

- [ ] **Step 5: Commit the clean F01 branch**

```bash
git add .gitignore go.mod go.sum cmd/platform-api internal/platform/cli internal/platform/runtime internal/transport/http docs/superpowers/plans/issue-100-f01-clean.md
git commit -m "feat(platform): isolate clean F01 process branch"
```

## Acceptance

- `git diff --name-only d6eec55..HEAD` contains only the F01 inventory plus this plan.
- No Compose, PostgreSQL, Casdoor, evidence harness, or application readiness files exist.
- F01 unit, process, race, vet, format, build, and version checks pass under Go 1.26.7.
- A task review and a final whole-branch review report no Critical/Important findings.
