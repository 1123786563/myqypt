# F03 PostgreSQL、迁移与就绪检查 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付显式版本化 SQL migration、pgx 连接池以及只在依赖可用时返回成功的 `/readyz`。

**Architecture:** `postgres` Adapter 隐藏 pgx/goose；Application `readiness.Service` 只依赖 `Checker` port。迁移通过独立 Cobra 命令运行，服务启动不执行 AutoMigrate。

**Tech Stack:** Go 1.26.7, pgx 5.10.0, goose 3.27.3, PostgreSQL 18

**Spec:** [Issue #103](https://github.com/1123786563/myqypt/issues/103), ADR-0003, extraction design §6

## Global Constraints

- PostgreSQL 是业务事实源；migration 只追加且可回滚。
- `/livez` 不访问数据库；`/readyz` 对超时、连接失败、未完成迁移均 fail closed。
- 禁止 GORM、AutoMigrate、全局连接和运行期按 Tenant 切库。

---

## File Structure

- Create `db/migrations/000001_platform_baseline.sql` with `goose Up/Down` sections.
- Create `internal/adapter/postgres/pool.go`, `migrate.go`, and focused tests.
- Create `internal/application/readiness/service.go` and `service_test.go`.
- Create `internal/transport/http/readiness.go` and `readiness_test.go`.
- Modify `internal/platform/cli/root.go`; create `deploy/compose.yaml`.

### Task 1: Add explicit migration and pool boundaries

**Interfaces:** `postgres.Open(context.Context, string) (*pgxpool.Pool, error)` and `postgres.Migrate(context.Context, *sql.DB, fs.FS) error`.

- [ ] Write `migrate_test.go` using a test PostgreSQL URL from `TEST_DATABASE_URL`; skip with an explicit message only when the variable is absent. Assert Up creates `schema_health(id boolean primary key, applied_at timestamptz not null)` and Down removes it.
- [ ] Run `TEST_DATABASE_URL=postgres://... go test ./internal/adapter/postgres -run TestMigrationRoundTrip -count=1`; confirm red because the adapter is absent.
- [ ] Add the migration, use `pgxpool.ParseConfig`, set connection lifetime/idle/health periods, ping with a five-second context, and run goose with the embedded `db/migrations` FS.
- [ ] Add `migrate up` and `migrate down-one` Cobra subcommands; require `DATABASE_URL` and return errors to the process entrypoint.
- [ ] Run `go test ./internal/adapter/postgres ./internal/platform/cli -count=1` and `go vet ./internal/adapter/postgres/...`.
- [ ] Commit: `git commit -m "feat(platform): add postgres migrations"`.

### Task 2: Separate readiness from liveness

**Interfaces:**

```go
type Checker interface { Check(context.Context) error }
type Service struct { Checks map[string]Checker; Timeout time.Duration }
type Result struct { Ready bool; Checks map[string]string }
func (s Service) Check(context.Context) Result
```

- [ ] Write table-driven tests proving all healthy => 200, one failed/timeout => 503, and `/livez` stays 200 under the same failed checker.
- [ ] Run `go test ./internal/application/readiness ./internal/transport/http -run 'Ready|Live' -count=1`; confirm red.
- [ ] Implement deterministic check ordering, per-request timeout, `database` checker backed by `pool.Ping`, and `GET /readyz` JSON containing only check names/states.
- [ ] Wire `readiness.Service` through `httptransport.Dependencies`; do not expose DSNs or raw errors.
- [ ] Run `go test ./... -count=1` and a Compose smoke: migrate, start API, then assert `/livez` 200 and `/readyz` 200.
- [ ] Commit: `git commit -m "feat(platform): add dependency readiness"`.

## Self-Review Record

- Spec coverage: append-only migrations, pool, CLI migration, readiness/liveness separation, timeout and fail-closed behavior are covered.
- Placeholder scan: schema, symbols, commands, status codes, and environment variable are concrete.
- Type consistency: `Checker`, `Service.Check`, `postgres.Open`, and `postgres.Migrate` do not leak Transport types.
