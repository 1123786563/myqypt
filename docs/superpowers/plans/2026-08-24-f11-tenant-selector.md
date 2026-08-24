# F11 租户选择器与当前上下文 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让已登录用户列出可用 Tenant、显式选择当前 Tenant，并在失效或被撤权时安全退回选择页。

**Architecture:** Portal BFF 调用既有 T03 Tenant Context Application Module；当前选择绑定到服务端 Session。React `TenantSelector` 只展示服务端返回的 active memberships。

**Tech Stack:** Go 1.26.7, PostgreSQL, React 19, TanStack Query

**Spec:** [Issue #111](https://github.com/1123786563/myqypt/issues/111), `CONTEXT.md`, T03 plan, ADR-0009

## Global Constraints

- Tenant ID 不能从 Host、任意 Header、Query 或持久化浏览器 Token 推导。
- 选择动作重新验证 Session 用户与 active Membership；不能信任列表缓存。
- Membership 撤销后下一请求 fail closed 并清除 session 当前 Tenant。

---

## File Structure

- Extend session with `SelectedTenantID *TenantID`; add migration `000003_session_tenant.sql`.
- Create `internal/application/portaltenant/service.go` and tests consuming T03 ports.
- Extend OpenAPI with `GET /portal-api/tenants` and `PUT /portal-api/session/tenant`.
- Create `web/src/features/tenant/{tenant-selector,use-tenants}.tsx` and tests; create `/app/select-tenant`.

```go
type TenantSummary struct { ID TenantID; Name string; Kind TenantKind }
type TenantContextReader interface {
    ListForUser(context.Context, UserID) ([]TenantSummary, error)
    SelectForSession(context.Context, SessionID, TenantID) (TenantContext, error)
}
```

### Task 1: Bind selection to active membership

**Interfaces:** `TenantContextReader.ListForUser(ctx, UserID) ([]TenantSummary,error)` and `SelectForSession(ctx, SessionID, TenantID) (TenantContext,error)`.

- [ ] Write tests for zero/one/multiple active memberships, suspended membership, cross-user Tenant ID, revoked membership after selection and repository outage.
- [ ] Run `go test ./internal/application/portaltenant -count=1`; confirm red.
- [ ] Implement service using T03 membership facts; sort summaries by normalized display name/id; auto-select only when exactly one active membership exists.
- [ ] Persist selected Tenant on server session and clear it when revalidation fails.
- [ ] Run focused and PostgreSQL integration tests; commit `feat(tenant): bind session tenant selection`.

### Task 2: Add selector UX

```ts
export const tenantKeys = {
  all: ['tenants'] as const,
  scoped: (tenantId: string) => ['tenant', tenantId] as const,
}
```

- [ ] Add/generate exact API schemas `TenantSummary{id,name,kind}` and `SelectTenantRequest{tenant_id}`.
- [ ] Write browser tests for zero-state, auto-select, search among multiple tenants, keyboard selection, stale 403 and revoked selection redirect.
- [ ] Implement query/mutation hooks, selector dialog and `/app/select-tenant`; invalidate all tenant-scoped query keys after a successful switch.
- [ ] Ensure AppShell receives current Tenant display only after BFF confirms it.
- [ ] Run Go contract tests, frontend tests/typecheck and Playwright switch flow.
- [ ] Commit: `git commit -m "feat(portal): add tenant selector"`.

## Self-Review Record

- Spec coverage: active membership list, explicit selection, server binding, revocation and accessible UX are covered.
- Placeholder scan: endpoints, request/response fields, sort and cache behavior are concrete.
- Type consistency: uses T03 `TenantID/TenantContext`; does not create a competing Tenant model.
