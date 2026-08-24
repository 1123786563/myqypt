# F05 来源证据与禁止依赖门禁 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将白名单抽取来源、许可证、禁止依赖和 Stage 1 验证变成可重复执行的仓库门禁。

**Architecture:** 机器可读 provenance manifest 记录上游 commit 与本地目标；Go policy test 与前端脚本检查禁止依赖；统一 Make target 产出不含凭证的证据清单。

**Tech Stack:** Go 1.26.7, pnpm 11.1.2, Make, SPDX expressions

**Spec:** [Issue #105](https://github.com/1123786563/myqypt/issues/105), extraction design §§3,5.4,6.4,11

## Global Constraints

- 固定 `shadcn-admin@e16c87f...` 与 `go-admin@1b7dcd8...`；更新必须走独立审查。
- 复制的实质代码保留 MIT notice；证据不得包含 Token、Cookie、DSN 或用户数据。
- 门禁对 Clerk、JWT、Casbin、GORM、Swaggo、Host Tenant 和全局 runtime 失败。

---

## File Structure

- Create `docs/upstream/provenance.yaml`, `THIRD_PARTY_NOTICES.md`, `LICENSES/shadcn-admin-MIT.txt`, `LICENSES/go-admin-MIT.txt`.
- Create `internal/architecture/dependency_policy_test.go`.
- Create `scripts/check-frontend-policy.mjs`, `scripts/verify-foundation.sh`, and `Makefile` targets.

```yaml
sources:
  - name: shadcn-admin
    repository: https://github.com/satnaing/shadcn-admin
    commit: e16c87f213a5ba5e45964e9b67c792105ec74d26
    license: MIT
  - name: go-admin
    repository: https://github.com/go-admin-team/go-admin
    commit: 1b7dcd843ce38fddc8c280fe3139e02735cf7574
    license: MIT
```

### Task 1: Record reproducible extraction evidence

- [ ] Write `provenance.yaml` schema entries containing repository URL, exact commit, license, copied source paths, destination paths, and local modification summary for both upstreams.
- [ ] Add both exact upstream MIT texts and attribution links to `THIRD_PARTY_NOTICES.md`.
- [ ] Add a Go test that parses the manifest and fails on a non-40-character commit, missing license file, unlisted destination, or destination outside the repository.
- [ ] Run `go test ./internal/architecture -run Provenance -count=1`; first confirm red, then implement the manifest validator and confirm green.
- [ ] Commit: `git commit -m "docs: record upstream extraction provenance"`.

### Task 2: Make forbidden architecture mechanically fail

```go
var forbiddenImports = map[string]string{
    "gorm.io/gorm": "ARCH-GORM",
    "github.com/casbin": "ARCH-CASBIN",
    "github.com/swaggo": "ARCH-SWAGGO",
}
```

- [ ] Add failing fixtures to `testdata/dependency-policy` for `gorm.io/gorm`, `github.com/casbin`, `github.com/swaggo`, Clerk, browser token storage, and `common/global`; assert each reports file and rule ID.
- [ ] Implement the Go import scanner and frontend package/source scanner; ignore generated files only by explicit path.
- [ ] Define `make generate-check`, `make policy-check`, `make test-foundation`, and aggregate `make verify-foundation`.
- [ ] Add `scripts/review-upstream-update.sh <source> <old-commit> <new-commit>` that produces a path/dependency diff and fails until authentication, Tenant, network, storage, copied-code and security-advisory review boxes are recorded; never sync an upstream branch automatically.
- [ ] Run `make verify-foundation`; expected sequence: OpenAPI generation clean, policy clean, Go tests pass, frontend type/test/build pass once F06 exists.
- [ ] Store only command/status/duration in `artifacts/foundation-verification.json`; gitignore the artifacts directory except `.gitkeep`.
- [ ] Commit: `git commit -m "build: add foundation architecture gates"`.

## Self-Review Record

- Spec coverage: immutable provenance, licenses, explicit upstream update review, forbidden dependency checks, generated stale check and evidence minimization are covered.
- Placeholder scan: upstream hashes, rule targets, filenames and commands are fixed.
- Type consistency: policy tooling observes source only and does not become runtime code.
