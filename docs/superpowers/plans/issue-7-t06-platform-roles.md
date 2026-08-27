# Issue #7 [T06][P4] Platform Role 权限 — Implementation Plan

- Issue: https://github.com/1123786563/myqypt/issues/7
- Scaffold plan (in-tree, semantic source): `docs/superpowers/plans/2026-08-24-t06-platform-roles.md`（Issue 正文内嵌计划与本文件 1:1 源）
- Base: `main@a02c4d6`（Issue #6 / T05 合并点）
- Branch: `codex/issue-7-t06-platform-roles`
- Worktree: `/Users/wuyongjun/trea/myqypt-worktrees/issue-7-t06-platform-roles`

## Goal

Owner、Admin、Billing Member、Member 获得各自可见且不可越权的操作。（AC1）

Delivery shape: one tracer-bullet vertical slice through the highest available seam — the public OpenAPI contract served by the compose stack (platform-api + postgres + casdoor), same black-box lighthouse position as T01–T05. Each of the four Platform Roles gets a visible, distinct capability set (its own operations) served by a new contract endpoint, and the one existing role-gated mutation (T05 invite) is pinned to the same matrix vocabulary so no role can act beyond its set (不可越权).

## Scope

- Application layer `internal/application/tenancy`: Platform Role capability matrix derived verbatim from CONTEXT.md role definitions (single authoritative source), new `RoleCapabilities` vocabulary, new service method `Capabilities(ctx, verified, tenantID)` and new Repository port method `ActiveMembershipRole(ctx, verified, tenantID)` following the T03/T05 precedents (actor = verified identity; validations classified before any port call).
- Adapter `internal/adapter/postgres/tenancy_repository.go`: implement `ActiveMembershipRole` — one parameterized SELECT resolving the actor's platform user and their `status='active'` membership role in the tenant; no active membership → `ErrNotAnActiveMember` (no existence oracle); unbound identity → `ErrUserNotBound`.
- Transport: extend `api/openapi/platform.yaml` with `GET /api/v1/tenants/{tenantId}/capabilities` (operationId `listTenantCapabilities`), regenerate `internal/transport/http/api` (`make generate-check` must stay green), extend `TenancyHandler` + wiring in `main.go` (nil-deps fail-closed 503 precedent; Bearer-auth via `authenticateTenantUser`; Problem-shaped errors; deterministic sorted capability list).
- Consistency pin (不可越权): focused unit tests prove the capability matrix and the T05 invite authorization stay in lockstep — `membership.manage` is held by exactly `owner` and `admin`, the same pair the adapter's transactional SQL guard enforces (`role IN ('owner','admin') AND status='active'`). No second runtime gate is added (TOCTOU precedent, T05 ruling 5): enforcement stays transaction-internal; T06 adds the classified vocabulary, the visible endpoint, and the matrix↔guard consistency contract.
- Acceptance: `tests/acceptance/scenarios/t06-platform-roles.yaml` + `t06_platform_roles_driver.go` + `t06_platform_roles_test.go` (journey gated by `T06_ACCEPTANCE_STACK=1`, own Casdoor org/application/client id `t06-acceptance`, audience override on stack startup — T04/T05 precedent).
- No migration: `memberships.role` with the four-role CHECK exists since `000003_personal_tenants.sql`; the capability matrix is static application vocabulary over it.

## Non-goals

- No OpenFGA projection/evaluation (T08–T10); PostgreSQL remains the business source of truth (ADR 0022).
- No membership role mutation, revocation workflow, or audit stream (T07).
- No commerce/payment/subscription/usage endpoints — later tickets consume the matrix; T06 only declares the role capability vocabulary and serves it.
- No changes to T01–T05 flows or F-series deliverables (regression gates cover them).
- No `Organization` concept, no cross-tenant sharing, no Product-internal roles (Global Constraints).
- No web/ frontend work (U-series covers portal UI).

## Design rulings

1. **Capability vocabulary = CONTEXT.md verbatim semantics.** Each capability name traces to a glossary sentence: `ownership.manage` + `billing.manage` (Owner: "ownership, deletion, billing authority"; story 11 "sole authority over ownership transfer, deletion, billing and complete access policy"), `membership.manage` + `configuration.manage` + `product_access.manage` + `purchases.manage` (Admin: "manages Tenant membership, Product purchases, configuration, and Product Access"), `payments.manage` + `subscriptions.read` + `usage.read` + `bills.read` (Billing Member: "manages payments and can inspect subscriptions, usage, and bills"), `product.use` (Member: "can use Products"). Owner holds the superset (sole accountable role: all ten capabilities); Admin, Billing Member, and Member each hold their glossary set. Four pairwise-distinct visible sets.
2. **可见: capabilities endpoint.** `GET /api/v1/tenants/{tenantId}/capabilities` — Bearer auth; 200 `{tenant_id, role, capabilities:[...sorted]}`; the role is the actor's active-membership role in that tenant; the list is sorted for replay-deterministic bodies. 404 no-oracle for every non-active-membership principal (never-member, revoked, invited-not-accepted, stranger, unknown tenant — indistinguishable); 401 missing/tampered; 503 unwired dependency first (fail-closed precedent).
3. **不可越权: matrix↔guard lockstep, not a second gate.** The invite path's authorization stays the T05 transactional SQL check (TOCTOU-safe). T06 adds the matrix as the classified vocabulary and unit-level consistency contracts: `membership.manage` holders == exactly {owner, admin} == the SQL guard's role set. Deviation from the scaffold's `PlatformRolesPort.Apply` shape is deliberate and recorded: repo reality routes authorization through the Repository port inside one transaction (T05 precedent), and a service-external pre-check would widen the race window without adding authority.
4. **Baseline membership operations stay out of the matrix.** Listing/selecting a tenant context and reading capabilities follow from holding an active membership (T03 semantics), not from a Platform Role; the matrix carries only role-derived authority so later OpenFGA projection (T08) maps 1:1.
5. **Unknown role from persistence is a classified contract breach.** `memberships.role` CHECK already constrains the four roles; a role string outside the matrix answers `ErrRoleNotSupported` (reused vocabulary) — defensive, testable, never a 500.
6. **Determinism.** Capability lists are served sorted; identical requests produce byte-identical bodies (replay assertion in the journey).
7. **Evidence minimization.** Journey assertions carry statuses, exact capability lists, and row counts only; no tokens, subjects, or credentials in YAML/JSON/report — the #100 harness double-scrubbing stays in force.
8. **Test seam layering** (AC2/AC3): focused unit tests at the application seam (matrix derivation pinned per role, closed vocabulary, sorted output, validation-before-port, zero-port-call guards); transport tests (status mapping 200/401/404/503, Problem shape, fail-closed, body determinism); DB-backed integration via `TEST_DATABASE_URL` temp PostgreSQL (role resolution per role/status: active→role, invited/revoked/non-member→`ErrNotAnActiveMember`, unbound→`ErrUserNotBound`, no oracle); the compose-stack journey for end-to-end acceptance (AC1 observable: four distinct visible sets + non-escalation denials).
9. **Journey denial paths** (AC2): member-role D invites → 404 + zero invitation rows; billing_member C invites → 404 + zero rows; invited-not-accepted E reads capabilities → 404; never-member F reads capabilities → 404; unknown tenant id → 404; missing Authorization → 401; tampered signature → 401. Admin B invites E → 201 proves the membership.manage path end to end.
10. **Zero schema change.** First slice without a migration; `memberships.role` CHECK (000003) is the persisted closed vocabulary the matrix must match (consistency test pins the four-role set against the adapter too).

## Task breakdown

- **Task 0 (controller)**: this plan, committed as `docs(plan): add issue 7 t06 implementation plan`.
- **Task 1 (implementer, one vertical slice, one commit `feat(identity): deliver T06 platform-roles`)**: application layer (RED first: focused contract tests incl. matrix pins) + adapter + OpenAPI/regen + transport + focused/transport/DB tests + journey scenario/driver/test. RED evidence under `artifacts/evidence/task1/` (gitignored). Follow the scaffold plan's Step 1→9 order adapted to repo reality.
- **Task 1 reviews**: independent spec-compliance review + independent code-quality review (fresh subagents); Critical/Important → fix round ≤5 + scoped re-review.
- **Final full-branch review** (strongest available model): acceptance matrix re-run, AC1–AC4 mapping, cross-task seams, residual-risk list.

## Acceptance matrix (gates; final review re-runs each)

Environment: `GOTOOLCHAIN=local` with `/opt/homebrew/bin/go` (plain `go` without GOTOOLCHAIN=local fails toolchain download — do not use), `GOPROXY=https://goproxy.cn,direct GOSUMDB=off`, `env -u` forbidden (use `unset`), Go test packages serial where `TestPlatformAPIProcess` is in scope (`-p 1`). WeKnora ports 3000/3030/6379/9000-9001/9100-9101/15432/50051 must not be touched; temp PostgreSQL on 55xxx with full teardown; main workspace `web/node_modules` must stay in place (FRONTEND phase prerequisite).

1. `go test ./internal/application/tenancy -count=1` — focused new + existing tests PASS.
2. `go test ./internal/adapter/postgres -count=1` against temp PostgreSQL (TEST_DATABASE_URL) — new DB-backed role-resolution tests PASS.
3. `go test ./internal/transport/http/... -count=1` — handler/status-mapping/fail-closed/Problem tests PASS.
4. `go test ./... -count=1 -p 1` (no DB env) — whole repo green, acceptance journeys skip with exact commands.
5. `go test ./... -race -count=1` key affected packages (`./internal/application/tenancy ./internal/adapter/postgres ./internal/transport/http/...`) with temp PG where needed.
6. `go vet ./...`, `gofmt -l .` empty, `go build ./...`, `go mod tidy -diff` empty.
7. `make generate-check` — regen contract drift-free.
8. `make policy-check` — PASS.
9. Acceptance journey (stack): `cd deploy/compose && PLATFORM_IDENTITY_OIDC_AUDIENCE=t06-acceptance PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose up -d --wait` then `T06_ACCEPTANCE_STACK=1 go test ./tests/acceptance -run TestT06PlatformRoles -count=1 -v` — PASS, evidence JSON under `artifacts/evidence/t06/`; `docker compose down -v` zero-residue check. Fresh stack per run (stale-state precheck fails closed).
10. `bash scripts/verify-foundation.sh` phases covering UNIT/CONTRACT/INTEGRATION (+POLICY/META as repo defines) — PASS at branch tip.
11. Commit hygiene: exactly 2 commits on branch (plan + slice), tree clean, `git diff --check` clean.
12. Secret scan: grep of new diff for token/Bearer/password patterns — hits only in negative-assertion test side.
13. T01–T05 + F-series zero regression: covered by gates 4/10 (no changes to their files beyond documented mechanical seams — contract regen touches `internal/transport/http/api`).

## Global constraints

- Stage-1 scale envelope, Tenant as hard boundary, billing 1:1, Product Domain Objects Product-owned, no secrets/customer content in telemetry/fixtures/evidence (issue Global Constraints verbatim).
- Blocker #6 complete (CLOSED completed 2026-08-27T11:48:47Z, merged `a02c4d6`) — verified before implementation.
- ADR 0013 (global user vs tenant lifecycle), ADR 0022 (OpenFGA is a later Authorization Projection; PostgreSQL stays the source of truth — no authorization-model change here), ADR 0002 (Product-owned domain objects untouched), ADR 0009 (no cross-tenant sharing).
- Platform Roles stay separate from Product-internal Roles (story 16; CONTEXT.md _Avoid_ lists).
- Implementer must not: push/merge/close/comment on GitHub, derive subagents, touch `.superpowers/` (main-repo controller ledger), `web/`, or unrelated plan files.
