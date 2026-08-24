# F14 已发布 Catalog 快照 API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为静态构建提供确定性、只读、仅含已发布且当前生效内容的 Catalog Snapshot API。

**Architecture:** Snapshot Application Module 从 Product Catalog 事实生成 canonical JSON；版本是 canonical bytes 的 SHA-256。Builder credential 只能读指定发布视图，不能访问草稿或 Tenant 管理 API。

**Tech Stack:** Go 1.26.7, PostgreSQL, OpenAPI 3.1, SHA-256

**Spec:** [Issue #114](https://github.com/1123786563/myqypt/issues/114), T13/T14/T16 plans, extraction design §§5.3,8.3

## Global Constraints

- 只包含 `published` 且 `effective_from <= as_of < effective_to/null` 的 Product Version、Offer 和公开元数据。
- 排序、时间格式和 JSON 字段稳定；相同事实产生相同 version/hash。
- Snapshot 不含内部成本、凭证、Tenant 私有配置、草稿或未来版本。

---

## File Structure

- Create `internal/application/catalogsnapshot/{model,service}.go` and tests.
- Create tenant-independent published-view SQL/repository with explicit public fields.
- Extend OpenAPI with `GET /api/v1/catalog/snapshots/current` and conditional ETag support.
- Create contract, determinism and leakage tests.

```go
type Snapshot struct {
    Version string `json:"version"`
    GeneratedAt time.Time `json:"generated_at"`
    Products []PublishedProduct `json:"products"`
}
type SnapshotReader interface { ReadPublished(context.Context, time.Time) (PublishedFacts, error) }
```

### Task 1: Define deterministic snapshot semantics

**Interfaces:** `SnapshotReader.ReadPublished(context.Context, time.Time) (Facts,error)` and `SnapshotService.Current(context.Context, time.Time) (Snapshot,error)` where `Snapshot{Version,GeneratedAt,Products}`.

```go
func snapshotVersion(canonicalContent []byte) string {
    sum := sha256.Sum256(canonicalContent)
    return hex.EncodeToString(sum[:])
}
```

- [ ] Write fixtures containing draft, future, expired, disabled and currently published versions across shuffled DB order.
- [ ] Assert only current published facts appear, products/offers sort by stable IDs, timestamps use UTC RFC3339, nil vs empty arrays are fixed, and repeated builds yield identical canonical bytes/version.
- [ ] Run `go test ./internal/application/catalogsnapshot -count=1`; confirm red.
- [ ] Implement explicit DTO projection and canonical encoder; compute lowercase hex SHA-256 from content excluding `generated_at`, then set ETag to the quoted version.
- [ ] Run property test over randomized input order and commit `feat(catalog): build deterministic published snapshots`.

### Task 2: Expose the builder read API

- [ ] Add OpenAPI response schemas and `If-None-Match`/304 behavior; authenticate a dedicated `catalog_builder` client principal.
- [ ] Write HTTP tests for valid builder 200/ETag, matching 304, wrong audience 401, normal user 403, and serialized absence of every private fixture sentinel.
- [ ] Implement strict handler through a narrow `SnapshotReader` dependency; add rate/timeout limits without caching authorization decisions.
- [ ] Generate Go/TS, run stale checks and full focused tests.
- [ ] Commit: `git commit -m "feat(api): expose published catalog snapshot"`.

## Self-Review Record

- Spec coverage: published/effective filtering, canonical output, hash/ETag, narrow builder identity and leakage tests are covered.
- Placeholder scan: state/time predicates, hash rules, identities, endpoint and status cases are exact.
- Type consistency: snapshot DTO is a public read model, never reused as mutable Catalog domain state.
