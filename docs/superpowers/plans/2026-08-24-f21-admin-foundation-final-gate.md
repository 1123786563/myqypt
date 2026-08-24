# F21 管理平台基础最终验收门禁 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 F01–F20 的契约、生成、测试、安全、发布和来源证明组合成可重复的最终验收，未通过任何强制项时不得宣称基础完成。

**Architecture:** 一个只编排、不隐藏子结果的 verification runner 分层执行静态、单元、集成、浏览器、网关和发布演练；输出机器可读报告与人类摘要。真实外部 Keycloak/CDN 若未配置，报告为未执行而非通过。

**Tech Stack:** Make, Go test, pnpm/Vitest/Playwright, PostgreSQL/OpenFGA/Keycloak fixtures, Higress conformance

**Spec:** [Issue #121](https://github.com/1123786563/myqypt/issues/121), extraction design §10

## Global Constraints

- 不把 build、health、自测或 focused test 称为完整验收。
- 报告逐层区分 pass/fail/skipped；强制本地 fixture 不允许 skipped。
- 报告、日志、截图和 traces 不含 Token、Cookie、DSN、OIDC code 或用户数据。

---

## File Structure

- Create `scripts/verify-admin-foundation.sh` and `scripts/render-verification-summary.mjs`.
- Create `tests/acceptance/admin_foundation_test.go` and Playwright project.
- Modify `Makefile` with `verify-admin-foundation`; create `docs/verification/admin-foundation.md`.
- Create `artifacts/.gitkeep`; runtime report remains ignored.

```json
{
  "commit": "40-character-git-sha",
  "started_at": "2026-08-24T00:00:00Z",
  "duration_ms": 0,
  "checks": [{"id":"go-test-race","command":"go test ./... -race -count=1","status":"pass","duration_ms":0,"evidence":[]}]
}
```

### Task 1: Make every prerequisite independently visible

```bash
run_check go-format "test -z \"$(gofmt -l .)\""
run_check go-test-race "go test ./... -race -count=1"
run_check web-typecheck "pnpm --dir web typecheck"
run_check web-build "pnpm --dir web build"
```

- [ ] Define report schema `{commit,started_at,duration_ms,checks:[{id,command,status,duration_ms,evidence}]}` and fixed IDs for provenance, forbidden dependencies, Go format/vet/generation/test/race, migration roundtrip, frontend lint/format-check/generation/typecheck/unit/browser/build, no-JS prerender, browser session/Tenant, authz attack matrix, SDK pack, Higress routing and release rollback.
- [ ] Write runner self-tests with fake commands proving failure propagation, skipped classification, secret redaction and non-zero exit when any mandatory check fails.
- [ ] Implement the runner with per-check timeouts and preserved exit codes; do not use a single opaque chained command.
- [ ] Run runner self-tests and commit `build: add transparent foundation verification runner`.

### Task 2: Execute the acceptance matrix

- [ ] Start isolated PostgreSQL, Keycloak, OpenFGA, fake build dispatcher/object store/CDN and Higress fixtures with random local ports; record exact image/config digests.
- [ ] Run Go format/vet, migration Up/Down/Up, API race tests, generated stale scans, forbidden-source scans, frontend lint/format check/typecheck/unit/browser/build and SDK pack smoke as separate report checks.
- [ ] Run Playwright: public pages without JS; OIDC session; zero/one/multiple Tenant; switch/revoke; Catalog table; 401/403/404/500/503; mobile/desktop keyboard flows.
- [ ] Run attack matrix: spoofed identity headers, cross-Tenant user/client, revoked binding, FGA deny/unknown/outage, DB outage and malformed cursor.
- [ ] Run publish simulation: event → one build → immutable upload → atomic switch → injected failure keeps old → rollback restores old; assert elapsed time under five minutes in fixture.
- [ ] Generate report and scan it with secret patterns; keep only sanitized summary as issue evidence.

### Task 3: Close the extraction gate honestly

- [ ] Run `make verify-admin-foundation` from a clean checkout and attach commit SHA plus each check status to Issue #121.
- [ ] Verify `git diff --exit-code` after all generators and `git status --short` contains only intentionally ignored runtime evidence.
- [ ] Review F01–F20 acceptance criteria and dependency edges one by one; leave F21 open if any required evidence is absent.
- [ ] When all mandatory checks pass, commit the durable runner/docs: `git commit -m "test: certify admin foundation extraction"`.

## Self-Review Record

- Spec coverage: provenance, contract, security, multitenancy, static/public UI, session, third-party API, build/publish/rollback and gateway are all represented.
- Placeholder scan: report schema, check IDs, fixtures, attack journeys and completion rule are exact.
- Type consistency: final gate only composes earlier public commands/contracts and introduces no runtime domain model.
