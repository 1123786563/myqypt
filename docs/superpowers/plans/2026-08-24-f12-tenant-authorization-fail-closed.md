# F12 Tenant 授权与 Fail-Closed 保护 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 Portal 与 Public API 共用的 Tenant 授权 Module，并证明 membership/OpenFGA/依赖异常时均拒绝访问。

**Architecture:** `tenantauth.Service` 先读 PostgreSQL active Membership，再调用 OpenFGA，最后返回仅在当前调用有效的 `TenantScope`。Transport 负责身份认证与 Tenant 输入，Repository 只能由 `TenantScope` 构造。

**Tech Stack:** Go 1.26.7, PostgreSQL, OpenFGA, oapi-codegen

**Spec:** [Issue #112](https://github.com/1123786563/myqypt/issues/112), ADR-0009, ADR-0028, ADR-0035

## Global Constraints

- 顺序固定：authenticated principal → selected/requested Tenant → active Membership → OpenFGA allowed → scope。
- denied、unknown、timeout、stale projection、数据库错误一律不进入业务 handler。
- 删除客户端传入的 `X-User-ID`, `X-Tenant-ID`, `X-Platform-*` 后才能进入内部链路。

---

## File Structure

- Create `internal/application/tenantauth/{ports,service}.go` and exhaustive tests.
- Create `internal/transport/http/tenant_authorization.go` and tests.
- Create `internal/adapter/openfga/checker.go` and contract tests.
- Add one protected `/portal-api/tenant/status` operation and generated bindings.

### Task 1: Implement one unbypassable authorization service

**Interfaces:**

```go
type Permission string
type MembershipReader interface { Active(context.Context, UserID, TenantID) (Membership, error) }
type RelationshipChecker interface { Check(context.Context, Principal, TenantID, Permission) (Decision, error) }
type TenantScope struct { TenantID TenantID; Principal Principal; Permission Permission }
func (s Service) Authorize(context.Context, Principal, TenantID, Permission) (TenantScope, error)
```

- [ ] Write a decision-table test for unauthenticated, inactive/missing membership, FGA deny/unknown/error/timeout, stale model ID, allowed, and repository error; assert checker is not called when membership fails.
- [ ] Run `go test ./internal/application/tenantauth -count=1`; confirm red.
- [ ] Implement typed denial reasons mapped later to 401/403/503; never return a partial `TenantScope`.
- [ ] Implement OpenFGA adapter with configured store/model IDs and deadline; require an exact boolean decision.
- [ ] Run unit/race/adapter contract tests; commit `feat(authz): add fail-closed tenant authorization`.

### Task 2: Prove Transport cannot bypass the service

- [ ] Write HTTP tests for stripped spoofed headers, missing session Tenant, denied/unknown/timeout status mapping, trace ID and handler-not-called assertions.
- [ ] Add protected status operation to OpenAPI and handler that requires `TenantScope` from typed request context.
- [ ] Install authorization only on protected route groups; add a source test failing if a protected generated operation is registered outside the group.
- [ ] Run `go generate ./...`, `go test ./... -race -count=1`, and F10/F11 Playwright protected-route flows.
- [ ] Commit: `git commit -m "feat(api): enforce tenant authorization boundary"`.

## Self-Review Record

- Spec coverage: membership source, OpenFGA evaluator, exact ordering, spoof stripping, fail-closed matrix and protected proof route are covered.
- Placeholder scan: decisions, status mappings, headers, ports and commands are explicit.
- Type consistency: only successful service output creates `TenantScope`; Transport cannot construct it directly.
