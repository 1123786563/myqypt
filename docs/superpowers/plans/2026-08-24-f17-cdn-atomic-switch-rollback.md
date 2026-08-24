# F17 CDN 原子发布与回滚 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将验证通过的静态构建原子切换为 CDN 当前版本，并支持快速回滚且永不暴露半上传版本。

**Architecture:** Publisher 先上传到 immutable `releases/<version>/`，验证 manifest/校验和后用带前置版本的 compare-and-swap 更新 release pointer。HTML 短缓存，哈希资产 immutable；回滚只是指针 CAS。

**Tech Stack:** Go 1.26.7, ObjectStore port, CDN invalidation adapter, SHA-256

**Spec:** [Issue #117](https://github.com/1123786563/myqypt/issues/117), extraction design §§5.3,6.1

## Global Constraints

- 版本不可覆盖；manifest 列出每个 path、size、SHA-256、content-type。
- 指针切换前必须校验所有文件和入口；失败继续服务旧版本。
- 回滚保留审计：actor/reason/from/to/time，不记录凭证。

---

## File Structure

- Create `internal/application/staticpublish/{ports,publisher}.go` and tests.
- Create `internal/adapter/objectstore/{filesystem,s3compatible}.go` contract tests.
- Create `internal/platform/cli/static_release.go` for publish/status/rollback.
- Create `deploy/cdn/cache-policy.yaml` and release manifest schema.

```go
type ManifestEntry struct { Path, SHA256, ContentType string; Size int64 }
type ReleaseManifest struct { Version string; Entries []ManifestEntry }
type ReleasePointer interface {
    Current(context.Context) (string, error)
    CompareAndSwap(context.Context, string, string) error
}
```

### Task 1: Upload and verify an immutable candidate

**Interfaces:** `ObjectStore.PutIfAbsent/Get/Head/List`, `ReleasePointer.CompareAndSwap(expected,next)`, `CDN.Invalidate(paths)`.

```go
type ObjectStore interface {
    PutIfAbsent(context.Context, string, io.Reader, ObjectMetadata) error
    Get(context.Context, string) (io.ReadCloser, ObjectMetadata, error)
    Head(context.Context, string) (ObjectMetadata, error)
    List(context.Context, string) ([]ObjectMetadata, error)
}
```

- [ ] Write contract tests for filesystem and memory store: no overwrite, checksum mismatch, missing file, wrong content type and interrupted upload.
- [ ] Write publisher tests proving pointer is untouched until every manifest entry and `index.html`, `/products/index.html`, `/pricing/index.html`, `/app/index.html` validate.
- [ ] Implement bounded parallel upload, checksum verification and candidate state; refuse path traversal/symlink/absolute manifest paths.
- [ ] Run unit/race/adapter tests; commit `feat(delivery): upload immutable static releases`.

### Task 2: Switch and roll back atomically

- [ ] Write tests for successful CAS, concurrent publisher conflict, CDN invalidation failure, rollback to known good version and rejection of unknown/unverified version.
- [ ] Implement CLI `static-release publish --dir --version`, `status`, `rollback --to --reason`; require explicit current pointer for CAS.
- [ ] Set asset cache `public,max-age=31536000,immutable`; HTML/pointer `no-cache` or short max-age; invalidate HTML paths after pointer commit.
- [ ] Simulate failure at every upload/switch step and assert clients resolve wholly old or wholly new manifest, never mixed.
- [ ] Commit: `git commit -m "feat(delivery): atomically switch static releases"`.

## Self-Review Record

- Spec coverage: immutable upload, manifest, CAS, cache policy, failure safety, audit and rollback are covered.
- Placeholder scan: keys, entrypoints, CLI arguments, cache values and failure cases are concrete.
- Type consistency: one `version` identifies F14 snapshot, F16 build and F17 release.
