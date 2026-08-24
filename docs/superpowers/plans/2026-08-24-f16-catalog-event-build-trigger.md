# F16 Catalog 发布事件触发前端构建 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Catalog 发布提交后可靠、幂等地触发一次前端构建，并可观测从事件到候选静态版本的分钟级延迟。

**Architecture:** Catalog transaction 写 Outbox；worker claim 事件后调用 `BuildDispatcher` port。幂等键是 snapshot version，重复事件复用同一 build；失败按有界策略重试并保留上一生产版本。

**Tech Stack:** Go 1.26.7, PostgreSQL outbox, Cobra worker, CI build API adapter

**Spec:** [Issue #116](https://github.com/1123786563/myqypt/issues/116), extraction design §§5.3,8.3

## Global Constraints

- 事件与 Catalog publish 在同一数据库事务；禁止 publish 成功但事件丢失。
- 同一 snapshot version 最多一个 active/succeeded build；重试不生成新版本语义。
- 目标 SLO：事件创建到候选版本完成不超过 5 分钟；超限可查询/告警。

---

## File Structure

- Add migration `000004_catalog_build_outbox.sql`.
- Create `internal/application/catalogbuild/{ports,coordinator}.go` and tests.
- Create PostgreSQL outbox adapter and integration tests.
- Create `internal/platform/worker/catalog_build.go`; add `platform-worker catalog-build` command.

```go
type BuildRequest struct { SnapshotVersion, IdempotencyKey string }
type BuildRef struct { ID string }
type BuildDispatcher interface {
    Dispatch(context.Context, BuildRequest) (BuildRef, error)
    FindByIdempotencyKey(context.Context, string) (BuildRef, error)
}
```

### Task 1: Commit publish and event atomically

**Interfaces:** `PublishTx.PublishAndEnqueue(ctx, command) (SnapshotVersion,error)` and outbox row keyed by `(event_type, snapshot_version)`.

```sql
CREATE UNIQUE INDEX catalog_build_once
ON catalog_build_outbox(event_type, snapshot_version);
```

- [ ] Write PostgreSQL tests forcing failure before/after the outbox insert; assert both Catalog state and event commit or neither commits.
- [ ] Add migration columns `id`, `snapshot_version`, `state`, `attempts`, `available_at`, `claimed_at`, `last_error_code`, timestamps and unique version constraint.
- [ ] Implement transaction/repository using `FOR UPDATE SKIP LOCKED` claim and lease recovery.
- [ ] Run integration/race tests; commit `feat(catalog): enqueue static build transactionally`.

### Task 2: Dispatch one observable build

**Interfaces:** `BuildDispatcher.Dispatch(ctx, BuildRequest{SnapshotVersion,IdempotencyKey}) (BuildRef,error)` and coordinator `RunOnce(context.Context) (Outcome,error)`.

- [ ] Write tests for first dispatch, duplicate delivery, two workers, transient retry with capped exponential schedule, terminal invalid snapshot, crash after remote acceptance and 5-minute breach.
- [ ] Implement coordinator with remote lookup by idempotency key before redispatch; store only error code and remote build ref.
- [ ] Add worker command, slog/OTel metrics `catalog_build_latency`, `catalog_build_failures`, `catalog_build_backlog`.
- [ ] Run worker integration test with fake dispatcher and assert exactly one accepted request.
- [ ] Commit: `git commit -m "feat(worker): dispatch catalog static builds"`.

## Self-Review Record

- Spec coverage: transactional outbox, concurrency, idempotency, retry, crash recovery, SLO and evidence are covered.
- Placeholder scan: key, columns, states, timings, port and commands are fixed.
- Type consistency: F14 snapshot version is the build identity used through F15–F17.
