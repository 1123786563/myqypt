# Issue #6 [T05][P3] Membership 邀请与激活 — Implementation Plan

- Issue: https://github.com/1123786563/myqypt/issues/6
- Scaffold plan (in-tree, semantic source): `docs/superpowers/plans/2026-08-24-t05-membership-invitation.md`（Issue 正文内嵌计划与本文件 1:1，已核实）
- Base: `main@1f45b23`（Issue #5 / T04 合并点）
- Branch: `codex/issue-6-t05-membership-invitation`
- Worktree: `/Users/wuyongjun/trea/myqypt-worktrees/issue-6-t05-membership-invitation`

## Goal

Owner 或 Admin 邀请 User，接受后 Membership 才进入 active。（AC1）

Delivery shape: one tracer-bullet vertical slice through the highest available seam — the public OpenAPI contract served by the compose stack (platform-api + postgres + casdoor), same black-box lighthouse position as T01–T04. A membership that has been invited but not accepted must NOT be usable: T03 list/select only see `status='active'` memberships, so the invited-not-accepted state is observable as continued denial at the tenant-context seam.

## Scope

- Migration `000006_membership_invitations.sql`: extend `memberships.status` CHECK to `('invited','active','revoked')`; keep every existing constraint byte-identical (UNIQUE(tenant_id,user_id), partial one-active-owner index, FKs). Down reverses the CHECK.
- Application layer `internal/application/tenancy`: membership-invitation vocabulary, classified errors, and two new Repository port methods (`InviteMember`, `AcceptInvitation`) following the T04 `CreateBusinessTenant` precedent (actor = verified identity; tenant-scoped; transactional).
- Adapter `internal/adapter/postgres/tenancy_repository.go`: implement both methods; advisory-lock serialization where the invariant needs it; parameterized SQL only; no error text leaking internals.
- Transport: extend `api/openapi/platform.yaml` with the two invitation endpoints, regenerate `internal/transport/http/api` (`make generate-check` must stay green), extend `TenancyHandler` + wiring in `main.go` (nil-deps fail-closed 503 precedent), Problem-shaped errors.
- Acceptance: `tests/acceptance/scenarios/t05-membership-invitation.yaml` + `t05_membership_invitation_driver.go` + `t05_membership_invitation_test.go` (journey gated by `T05_ACCEPTANCE_STACK=1`, own Casdoor org/application/client id `t05-acceptance`, audience override on stack startup — T04 precedent).

## Non-goals

- No notification/email delivery of invitations (out of scope; discovery of pending invitations by the invitee is not required by the Issue).
- No listing endpoint for invitations, no revocation workflow (T07 audit / later tickets), no OpenFGA projection (T09).
- No changes to personal-tenant flows, identity binding, or F01–F05/F07 deliverables.
- No `Organization` concept, no cross-tenant sharing (Global Constraints).
- No web/ frontend work (U-series covers portal UI).

## Design rulings

1. **The membership row IS the invitation record.** `status='invited'` is the pending invitation; acceptance is the single-row transition `invited → active`. No separate invitations table: `UNIQUE(tenant_id,user_id)` already gives replay convergence on the natural key, and T03's active-only JOIN means an invited-not-accepted membership is invisible/unusable at the tenant-context seam by construction.
2. **Idempotency semantics = (tenant, invitee) convergence.** The `Idempotency-Key` header is mandatory (missing → 400, before any write), but replay convergence rides the natural key: first invite of (tenant, invitee) → 201 `status=invited`; any repeat delivery (same or different key, still pending) → 200 replay with the same body. This replaces the scaffold plan's generic actor-key replay map (which existed because T04 tenants have no natural key) — recorded deviation, same observable guarantee (no second business effect).
3. **Invitee addressing = verified-identity subject.** The inviter names the invitee by the invitee's external subject (Casdoor username/sub) in the request body; the adapter resolves it via `platform_users` by (issuer, subject). A never-bound subject → `ErrUserNotBound` → 404 (no existence oracle, T04 precedent).
4. **Role rules.** Invitable roles: `admin`, `billing_member`, `member` (CONTEXT.md Platform roles). `owner` is NOT invitable (partial unique index `memberships_one_active_owner_per_tenant`; ownership changes are not this ticket) → 400-classified `role_not_supported`. Unknown role string → 400.
5. **Authorization to invite.** The authenticated actor must hold an `active` membership with role `owner` or `admin` in the tenant (CONTEXT.md: Admin manages Tenant membership). Any other actor (member, billing_member, non-member, revoked) → 404 no-oracle (T03/T04 denial precedent), zero writes.
6. **Acceptance is invitee-only and tenant-scoped.** `POST /api/v1/tenants/{tenantId}/membership-invitations/acceptance` authenticated as the invitee: the adapter matches the verified identity's platform user against the pending `invited` row of that tenant. Not invited / already active / someone else's row → 404 no-oracle (indistinguishable), zero writes. Success → 200 `{status:"active", role, tenant_id}` (single transition, idempotent replays converge to the same 200 without a second transition — test via row `updated`/count invariants).
7. **Contract endpoints.**
   - `POST /api/v1/tenants/{tenantId}/membership-invitations` — Bearer auth; header `Idempotency-Key` (required, `^[A-Za-z0-9-_.]{1,64}$` — same family as F04's request-id ruling); body `{invitee_subject: string(1..320), role: enum}`; responses 201 created / 200 replay / 400 problem / 401 / 404 / 503.
   - `POST /api/v1/tenants/{tenantId}/membership-invitations/acceptance` — Bearer auth (invitee); empty body; 200 / 401 / 404 / 503.
   - Both always registered; nil/unwired dependencies fail closed 503 first (existing precedent).
8. **Evidence minimization.** Assertion details carry facts only (status codes, row counts, booleans); no tokens, subjects, or credentials in YAML/JSON/report — the #100 harness double-scrubbing stays in force.
9. **Test seam layering** (matches AC2/AC3): focused unit tests at the application seam (fake Repository, classified errors, zero-side-effect guards); transport tests (status mapping, fail-closed, Problem shape, replay body identity); DB-backed integration via `TEST_DATABASE_URL` temp PostgreSQL (advisory lock, convergence, denial row-deltas); the compose-stack journey for end-to-end acceptance (AC1 observable at the tenant-context seam: B denied before acceptance, usable after).
10. **Journey denial paths** (AC2): missing Idempotency-Key → 400; unsupported role `owner` → 400; missing/tampered credentials → 401; non-authorized inviter (member-role B inviting C) → 404 + zero invitation rows; never-bound invitee subject → 404 + zero rows; C accepting an invitation that does not exist → 404 + zero writes; invited-but-not-accepted B selecting tenant → 404 (the core AC1 negative observation).

## Task breakdown

- **Task 0 (controller)**: this plan, committed as `docs(plan): add issue 6 t05 implementation plan`.
- **Task 1 (implementer, one vertical slice, one commit `feat(identity): deliver T05 membership invitation`)**: migration + application layer (RED first: application contract tests) + adapter + OpenAPI/regen + transport + focused/transport/DB tests + journey scenario/driver/test. RED evidence under `artifacts/evidence/task1/` (gitignored). Follow the plan's Step 1→9 order adapted to repo reality.
- **Task 1 reviews**: independent spec-compliance review + independent code-quality review (fresh subagents); Critical/Important → fix round ≤5 + scoped re-review.
- **Final full-branch review** (strongest available model): acceptance matrix re-run, AC1–AC4 mapping, cross-task seams, residual-risk list.

## Acceptance matrix (gates; final review re-runs each)

Environment: `GOTOOLCHAIN=local` with `/opt/homebrew/bin/go` 1.26.3 (verified working this session; plain `go` without GOTOOLCHAIN=local fails toolchain download — do not use), `GOPROXY=https://goproxy.cn,direct GOSUMDB=off`, `env -u` forbidden (use `unset`), Go test packages serial where `TestPlatformAPIProcess` is in scope (`-p 1`).

1. `go test ./internal/application/tenancy -count=1` — focused new + existing tests PASS.
2. `go test ./internal/adapter/postgres -count=1` against temp PostgreSQL (TEST_DATABASE_URL) — new DB-backed invitation tests PASS (migrate up, journey of rows, down/up idempotence where applicable).
3. `go test ./internal/transport/http/... -count=1` — handler/status-mapping/replay/fail-closed/Problem tests PASS.
4. `go test ./... -count=1 -p 1` (no DB env) — whole repo green, acceptance journeys skip with exact commands.
5. `go test ./... -race -count=1` key affected packages (`./internal/application/tenancy ./internal/adapter/postgres ./internal/transport/http/...`) with temp PG where needed.
6. `go vet ./...`, `gofmt -l .` empty, `go build ./...`, `go mod tidy -diff` empty.
7. `make generate-check` — regen contract drift-free.
8. `make policy-check` (or `node scripts/check-frontend-policy.mjs` at root) — PASS.
9. Acceptance journey (stack): `cd deploy/compose && PLATFORM_IDENTITY_OIDC_AUDIENCE=t05-acceptance PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose up -d --wait` then `T05_ACCEPTANCE_STACK=1 go test ./tests/acceptance -run TestT05MembershipInvitation -count=1 -v` — PASS, evidence JSON under `artifacts/evidence/t05/`; `docker compose down -v` zero-residue check. Fresh stack per run (stale-state precheck fails closed).
10. `bash scripts/verify-foundation.sh` phases covering UNIT/CONTRACT/INTEGRATION (+POLICY/META as repo defines) — PASS at branch tip.
11. Commit hygiene: exactly 2 commits on branch (plan + slice), tree clean, `git diff --check` clean.
12. Secret scan: grep of new diff for token/Bearer/password patterns — hits only in negative-assertion test side.
13. F01–F04 + T01–T04 zero regression: covered by gates 4/10 (no changes to their files beyond documented mechanical seams — contract regen touches `internal/transport/http/api`).

## Global constraints

- Stage-1 scale envelope, Tenant as hard boundary, billing 1:1, Product Domain Objects Product-owned, no secrets/customer content in telemetry/fixtures/evidence (issue Global Constraints verbatim).
- Blocker #5 complete (CLOSED 2026-08-27T05:00:09Z, merged `1f45b23`) — verified before implementation.
- ADR 0013 (global user vs tenant lifecycle; membership active/revoked vocabulary extended by this ticket with `invited` — the extension is the ticket's own subject, recorded here as a deliberate vocabulary addition), ADR 0022 (OpenFGA later; no authorization model changes here).
- Implementer must not: push/merge/close/comment on GitHub, derive subagents, touch `.superpowers/` (main-repo controller ledger), `web/`, or unrelated plan files.
