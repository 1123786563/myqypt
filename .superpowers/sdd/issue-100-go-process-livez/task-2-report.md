# Task 2 implementer report — commands and graceful lifecycle

## Status

`DONE_WITH_CONCERNS`

Final implementation commit:

```text
42aeb49c47adb7b3fbedea3ee9205c3e96e8fe27 feat(platform): add command and graceful lifecycle
```

The earlier local commit `41b9a59` was amended before handoff; it is superseded by the final commit above.

## Changed files

- `cmd/platform-api/main.go`
  - Replaced the old direct `http.Server`, readiness, and PostgreSQL wiring with signal-aware Cobra execution and a listener supplied to `runtime.Serve`.
  - Added link-time injectable `version` (default `dev`).
- `cmd/platform-api/main_test.go`
  - Replaced PostgreSQL readiness tests with an isolated built-binary process test.
  - Verifies startup, `/livez`, occupied-port startup failure, and bounded shutdown for both SIGTERM and SIGINT.
- `internal/platform/runtime/server.go`
  - Added `Config`, `DefaultConfig`, and `Serve` as the sole `net/http` server lifecycle boundary.
- `internal/platform/runtime/server_test.go`
  - Added cancellation-driven graceful shutdown coverage.
- `internal/platform/cli/root.go`
  - Added a Cobra root containing only `serve` and `version`.
- `internal/platform/cli/root_test.go`
  - Verifies `version` prints the injected value and never calls `serve`.
- `internal/platform/app.go`, `internal/platform/app_test.go`
  - Removed the obsolete readiness/PostgreSQL HTTP scaffold.
- `go.mod`, `go.sum`
  - Added Cobra's missing indirect module requirements and checksums via `go mod tidy`; no version was upgraded.

## TDD and command record

The repository requests `toolchain go1.26.7`, but this host only has Go 1.26.3 and automatic toolchain download was unavailable. Verification therefore used `GOTOOLCHAIN=local`; the first command below records the unmodified failure.

### RED

```text
$ go test ./internal/platform/runtime ./internal/platform/cli -count=1
exit status: 1
go: downloading go1.26.7 (darwin/arm64)
go: download go1.26.7 for darwin/arm64: toolchain not available
```

```text
$ GOTOOLCHAIN=local go test ./internal/platform/runtime ./internal/platform/cli -count=1
exit status: 1
github.com/1123786563/myqypt/internal/platform/runtime: no non-test Go files in /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold/internal/platform/runtime
github.com/1123786563/myqypt/internal/platform/cli: no non-test Go files in /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold/internal/platform/cli
FAIL github.com/1123786563/myqypt/internal/platform/runtime [build failed]
FAIL github.com/1123786563/myqypt/internal/platform/cli [build failed]
FAIL
```

### GREEN and dependency repair

```text
$ GOTOOLCHAIN=local go test ./internal/platform/runtime ./internal/platform/cli -count=1
exit status: 1
ok   github.com/1123786563/myqypt/internal/platform/runtime 1.254s
# github.com/1123786563/myqypt/internal/platform/cli
... github.com/spf13/cobra@v1.10.2 requires github.com/cpuguy83/go-md2man/v2@v2.0.6: missing go.sum entry for go.mod file
FAIL github.com/1123786563/myqypt/internal/platform/cli [setup failed]
FAIL
```

`GOTOOLCHAIN=local go mod tidy` added the missing Cobra dependency graph entries.

```text
$ GOTOOLCHAIN=local go test ./internal/platform/runtime ./internal/platform/cli -count=1
exit status: 0
ok   github.com/1123786563/myqypt/internal/platform/runtime 0.197s
ok   github.com/1123786563/myqypt/internal/platform/cli 0.283s
```

### Process-test fixture correction

The first process run found a test-fixture path error, not a production failure: package tests execute from `cmd/platform-api`, so `go build ./cmd/platform-api` addressed a non-existent nested directory. The helper was corrected to build `.`.

```text
$ GOTOOLCHAIN=local go test ./internal/platform/runtime ./internal/platform/cli ./internal/transport/http -count=1 && GOTOOLCHAIN=local go build -o /tmp/myqypt-platform-api ./cmd/platform-api && /tmp/myqypt-platform-api version && GOTOOLCHAIN=local go test ./cmd/platform-api -run 'TestPlatformAPIProcess' -count=1
exit status: 1
ok   github.com/1123786563/myqypt/internal/platform/runtime 0.290s
ok   github.com/1123786563/myqypt/internal/platform/cli 0.138s
ok   github.com/1123786563/myqypt/internal/transport/http 0.436s
dev
--- FAIL: TestPlatformAPIProcess (0.01s)
    main_test.go:23: build platform-api: exit status 1
        stat /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold/cmd/platform-api/cmd/platform-api: directory not found
FAIL
```

```text
$ GOTOOLCHAIN=local go test ./cmd/platform-api -run 'TestPlatformAPIProcess' -count=1
exit status: 0
ok   github.com/1123786563/myqypt/cmd/platform-api 0.839s
```

After extending the process test to exercise both signals:

```text
$ GOTOOLCHAIN=local go test ./cmd/platform-api -run 'TestPlatformAPIProcess' -count=1
exit status: 0
ok   github.com/1123786563/myqypt/cmd/platform-api 2.962s
```

### Final verification

```text
$ GOTOOLCHAIN=local go test ./internal/platform/runtime ./internal/platform/cli ./internal/transport/http -count=1
exit status: 0
ok   github.com/1123786563/myqypt/internal/platform/runtime 0.573s
ok   github.com/1123786563/myqypt/internal/platform/cli 1.166s
ok   github.com/1123786563/myqypt/internal/transport/http 1.750s
```

```text
$ GOTOOLCHAIN=local go build -o /tmp/myqypt-platform-api ./cmd/platform-api && /tmp/myqypt-platform-api version
exit status: 0
dev
```

```text
$ GOTOOLCHAIN=local go test ./cmd/platform-api -run 'TestPlatformAPIProcess' -count=1
exit status: 0
ok   github.com/1123786563/myqypt/cmd/platform-api 1.579s
```

```text
$ GOTOOLCHAIN=local go test ./... -count=1
exit status: 0
ok   github.com/1123786563/myqypt/cmd/platform-api 3.916s
ok   github.com/1123786563/myqypt/internal/platform/cli 1.138s
ok   github.com/1123786563/myqypt/internal/platform/runtime 2.655s
ok   github.com/1123786563/myqypt/internal/transport/http 3.196s
ok   github.com/1123786563/myqypt/tests/platformtest 1.698s
```

```text
$ GOTOOLCHAIN=local go build -o /tmp/myqypt-platform-api-injected -ldflags='-X main.version=be4cc10' ./cmd/platform-api && /tmp/myqypt-platform-api-injected version
exit status: 0
be4cc10
```

`git diff --check` completed with exit status 0 before both commits/amendment.

## Acceptance coverage

| Item | Evidence |
| --- | --- |
| Cobra only parses commands | `cli.NewRoot` contains exactly `serve` and `version`; the process startup closure is injected from `main`. |
| Runtime owns `net/http` lifecycle | `runtime.Serve` constructs and serves `http.Server`, then uses a 10-second bounded graceful shutdown. |
| Explicit timeout configuration | `DefaultConfig`: read header 5s, read 15s, write 30s, idle 60s, shutdown 10s. |
| `/livez` process liveness | The built child process must return HTTP 200 before the test proceeds; Task 1's Gin router unit test also checks the exact `{"status":"alive"}` body. |
| Occupied port fails startup | A second built `serve` process on the active address is required to exit non-zero. |
| Bounded signal termination | Independent built processes receive SIGTERM and SIGINT; each must exit cleanly within `runtime.DefaultConfig().ShutdownTimeout`. |
| Version avoids listener startup | Unit test proves the injected `serve` callback is never invoked; link-time smoke prints exactly `be4cc10`. |
| No F01 readiness/dependency behavior | Removed the old readiness/PostgreSQL scaffold; a scoped source scan found no `readyz`, `postgres`, `Readiness`, `OpenAPI`, or `evidence` references under `cmd/platform-api` or `internal/platform`. |
| Gin isolation | `main` and `runtime` import only `net/http` abstractions; Gin remains in `internal/transport/http`. |

## Self-review

- `runtime.Serve` waits for the `Serve` goroutine after a successful shutdown and normalizes only `http.ErrServerClosed` to success.
- Listener creation stays in `main`; no listener is created for `version`.
- Process tests use localhost ephemeral ports, no Docker and no external services. Each launched process has cleanup that kills and reaps it if a preceding assertion fails.
- The unrelated untracked `docs/superpowers/plans/issue-100-go-process-livez.md` file was preserved and not committed.

## Concerns

- The configured Go 1.26.7 toolchain is unavailable on this host. All successful verification used the locally installed Go 1.26.3 with `GOTOOLCHAIN=local`; plain Go commands currently fail before compiling while attempting the unavailable download.

## Fix round 1 — 2026-08-24

### Status

- Review finding addressed: a working task-local `go1.26.7` toolchain was obtained on this host and the full Task 2 command matrix was rerun under that exact binary.
- No production code changed in this round; this append updates the verification record only.

### Root cause of the earlier toolchain failure

The host Go configuration persisted `GOPROXY=off` and `GOSUMDB=off` in `GOENV`, so the Homebrew-installed `go1.26.3` binary could not auto-fetch the `toolchain go1.26.7` requirement from `go.mod`.

```text
$ cd /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold && go version
go: downloading go1.26.7 (darwin/arm64)
go: download go1.26.7 for darwin/arm64: toolchain not available
```

```text
$ go env GOENV && go env -json | rg 'GOPROXY|GOSUMDB|GOTOOLCHAIN|GOENV'
/Users/wuyongjun/Library/Application Support/go/env
        "GOENV": "/Users/wuyongjun/Library/Application Support/go/env",
        "GOPROXY": "off",
        "GOSUMDB": "off",
        "GOTOOLCHAIN": "auto",
```

For reference, the locally installed host binary remained `go1.26.3`:

```text
$ cd /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold && GOTOOLCHAIN=local go version && GOTOOLCHAIN=local go env GOVERSION GOROOT GOPATH GOPROXY GOSUMDB GOHOSTOS GOHOSTARCH
go version go1.26.3 darwin/arm64
go1.26.3
/opt/homebrew/Cellar/go/1.26.3/libexec
/Users/wuyongjun/go
off
off
darwin
arm64
```

### Task-local Go 1.26.7 acquisition

I downloaded the official darwin/arm64 archive to a dedicated temporary directory, verified its SHA-256, extracted it, and used that binary directly for the remaining checks.

```text
$ task_root=$(mktemp -d /tmp/issue100-task2-go1267-retry.XXXXXX); archive_path="$task_root/go1.26.7.darwin-arm64.tar.gz"; curl -L --fail --retry 5 --retry-all-errors --continue-at - https://dl.google.com/go/go1.26.7.darwin-arm64.tar.gz -o "$archive_path"; tar -C "$task_root" -xzf "$archive_path"; shasum -a 256 "$archive_path"; echo "$task_root"; "$task_root/go/bin/go" version
020a1e8224811be75163e920bc77e0926a1390a6aeea19bdcf23f74b9d749f6d  /tmp/issue100-task2-go1267-retry.E59JCp/go1.26.7.darwin-arm64.tar.gz
/tmp/issue100-task2-go1267-retry.E59JCp
go version go1.26.7 darwin/arm64
```

```text
$ cd /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold && /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go env GOVERSION GOROOT GOTOOLDIR GOPROXY GOSUMDB
go1.26.7
/tmp/issue100-task2-go1267-retry.E59JCp/go
/tmp/issue100-task2-go1267-retry.E59JCp/go/pkg/tool/darwin_arm64
off
off
```

### Re-run under Go 1.26.7

Focused runtime/CLI/transport tests:

```text
$ cd /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold && /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go test ./internal/platform/runtime ./internal/platform/cli ./internal/transport/http -count=1
ok   github.com/1123786563/myqypt/internal/platform/runtime 1.157s
ok   github.com/1123786563/myqypt/internal/platform/cli 1.658s
ok   github.com/1123786563/myqypt/internal/transport/http 2.214s
```

Build + version smoke test:

```text
$ cd /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold && /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go build -o /tmp/myqypt-platform-api-go1267 ./cmd/platform-api && /tmp/myqypt-platform-api-go1267 version && /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go build -o /tmp/myqypt-platform-api-go1267-injected -ldflags='-X main.version=be4cc10' ./cmd/platform-api && /tmp/myqypt-platform-api-go1267-injected version
dev
be4cc10
```

Process acceptance test:

```text
$ cd /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold && /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go test ./cmd/platform-api -run 'TestPlatformAPIProcess' -count=1
ok   github.com/1123786563/myqypt/cmd/platform-api 3.483s
```

Full package sweep:

```text
$ cd /Users/wuyongjun/trea/myqypt-worktrees/t01-1-platform-scaffold && /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go test ./... -count=1
ok   github.com/1123786563/myqypt/cmd/platform-api 3.031s
ok   github.com/1123786563/myqypt/internal/platform/cli 0.457s
ok   github.com/1123786563/myqypt/internal/platform/runtime 1.959s
ok   github.com/1123786563/myqypt/internal/transport/http 0.957s
ok   github.com/1123786563/myqypt/tests/platformtest 2.419s
```

### Residual concern

- The host-default `go` path still points at Homebrew `go1.26.3` with `GOPROXY=off` and `GOSUMDB=off`, so plain `go` commands in this worktree will continue to fail toolchain auto-resolution until the host Go environment is adjusted or the task-local `go1.26.7` binary is used explicitly.

## Final-review fix — 2026-08-25 01:08:10 CST

### Status

- Final-review findings 2 and 3 addressed with the smallest scoped changes.
- Controller ruling recorded for finding 1: the historical Compose/PostgreSQL/Keycloak/evidence scaffold is pre-existing committed context that the plan says to leave untouched. This fix intentionally preserved those files and services; destructive branch rebuilding or deletion of historical work is outside this automation.
- Deferred Minor findings were not addressed.

### Changes

- `deploy/compose/compose.yaml`
  - Changed `platform-api` image from `golang:1.26.3` to the exact pinned toolchain image `golang:1.26.7`.
  - Changed the container command from `go run ./cmd/platform-api` to `go run ./cmd/platform-api serve` so Cobra starts the serving subcommand.
  - Preserved the existing PostgreSQL, Keycloak, volume, and environment scaffold.
- `cmd/platform-api/main_test.go`
  - Introduced `platformProcess` so exactly one goroutine owns `exec.Cmd.Wait`.
  - Refactored cleanup to kill only still-running children and drain the existing waiter instead of calling `Wait` directly.
  - Made startup polling observe early child exit through the same waiter and include captured output in failures.
  - Tightened the occupied-port assertion to require a prompt non-zero process exit represented by `*exec.ExitError`; a timeout now fails explicitly.
  - Preserved startup, `/livez`, occupied-port, SIGTERM, and SIGINT coverage.

### TDD note

The stricter occupied-port expectation and single-waiter helper were written before the Compose production fix. The tightened process test passed immediately because the server already exits non-zero on bind failure; the real defect was the test harness accepting timeout as success and overlapping `Wait` ownership.

### Verification

Compose config validation with required environment variables:

```text
$ env PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=platform KEYCLOAK_POSTGRES_DB=keycloak KEYCLOAK_POSTGRES_USER=keycloak KEYCLOAK_POSTGRES_PASSWORD=keycloak KEYCLOAK_ADMIN=admin KEYCLOAK_ADMIN_PASSWORD=admin docker compose -f deploy/compose/compose.yaml config
exit status: 0
<no stdout emitted by Docker Compose v5.1.2 in this environment>
```

Source check for the Compose service fields that were fixed:

```text
$ rg -n "image: golang|command: go run" deploy/compose/compose.yaml
3:    image: golang:1.26.7
5:    command: go run ./cmd/platform-api serve
```

Focused runtime/CLI/transport tests under the exact task-local Go 1.26.7 binary:

```text
$ /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go test ./internal/platform/runtime ./internal/platform/cli ./internal/transport/http -count=1
ok  	github.com/1123786563/myqypt/internal/platform/runtime	1.736s
ok  	github.com/1123786563/myqypt/internal/platform/cli	1.215s
ok  	github.com/1123786563/myqypt/internal/transport/http	2.251s
```

Focused process acceptance under the exact task-local Go 1.26.7 binary:

```text
$ /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go test ./cmd/platform-api -run 'TestPlatformAPIProcess' -count=1
ok  	github.com/1123786563/myqypt/cmd/platform-api	3.346s
```

Full package sweep under the exact task-local Go 1.26.7 binary:

```text
$ /tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go test ./... -count=1
ok  	github.com/1123786563/myqypt/cmd/platform-api	3.675s
ok  	github.com/1123786563/myqypt/internal/platform/cli	3.081s
ok  	github.com/1123786563/myqypt/internal/platform/runtime	2.625s
ok  	github.com/1123786563/myqypt/internal/transport/http	2.099s
ok  	github.com/1123786563/myqypt/tests/platformtest	1.623s
```

### Concerns

- Docker Compose v5.1.2 returned success for `config` but emitted no rendered YAML/JSON in this environment, including with `--format json`, `--services`, and `--output`. The source-level check above records the corrected resolved fields in the Compose file, and the required validation command exited 0.
- The host-default `go` remains unsuitable for this worktree because it is still Homebrew `go1.26.3` with persisted `GOPROXY=off`/`GOSUMDB=off`; final verification therefore used `/tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go` as requested.
