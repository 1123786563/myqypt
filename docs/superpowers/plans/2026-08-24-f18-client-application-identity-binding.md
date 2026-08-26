# F18 第三方 Client Application 身份与 Tenant 绑定 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为第三方客户端/Product Adapter 建立独立于 User Session 的 Client Application 身份，并将其显式绑定到允许访问的 Tenant。

**Architecture:** Casdoor client credentials 验证产生 `ClientPrincipal`；PostgreSQL 保存 `ClientApplication` 与可撤销 `ClientTenantBinding` 事实；F12 授权服务通过统一 `Principal` 联合类型处理人和客户端。

**Tech Stack:** Go 1.26.7, Casdoor OIDC/JWT verification, PostgreSQL, OpenFGA

**Spec:** [Issue #118](https://github.com/1123786563/myqypt/issues/118), ADR-0001, ADR-0009, extraction design §§7.3,8.2

## Global Constraints

- Client Application 不是 User，不能创建浏览器 Session 或继承 Membership。
- 身份键固定为 `issuer + client_id(azp)`；验证 signature/issuer/audience/expiry/client grant。
- `X-Tenant-ID` 只表达请求目标，必须存在 active ClientTenantBinding 且 OpenFGA allow。

---

## File Structure

- Modify `CONTEXT.md` with Client Application/Binding definitions; add ADR for identity and revocation boundary before code.
- Add migration `000005_client_applications.sql`.
- Create `internal/domain/clientapplication`, Application service/ports and tests.
- Create Casdoor client principal verifier, PostgreSQL repository, OpenFGA projection adapter and HTTP auth middleware tests.

```go
type PrincipalKind string
type Principal struct { Kind PrincipalKind; Subject, Issuer string }
const ( UserPrincipal PrincipalKind = "user"; ClientPrincipal PrincipalKind = "client" )
type ClientTenantBinding struct {
    ClientApplicationID ClientApplicationID
    TenantID TenantID
    State BindingState
}
```

### Task 1: Record the durable domain decision

- [ ] Add glossary/invariants: Client Application owner, immutable issuer/client ID key, binding state active/revoked, no Membership substitution, audit requirements.
- [ ] Add ADR comparing Casdoor-only groups, local API keys and explicit PostgreSQL binding; select verified Casdoor principal + PostgreSQL fact + OpenFGA projection.
- [ ] Run documentation link/ADR index checks and ensure F12 `Principal` union is updated consistently.
- [ ] Commit: `git commit -m "docs(domain): define client application identity"`.

### Task 2: Bind and authorize machine principals

**Interfaces:** `Principal{Kind:user|client,Subject,Issuer}`, `ClientBindingReader.Active(ctx, ClientApplicationID,TenantID)`, `ClientAuthenticator.Authenticate(ctx,bearer) (ClientPrincipal,error)`.

```go
type ClientAuthenticator interface {
    Authenticate(context.Context, string) (Principal, error)
}
type ClientBindingReader interface {
    Active(context.Context, ClientApplicationID, TenantID) (ClientTenantBinding, error)
}
```

- [ ] Write domain/service tests for register, duplicate issuer/client, bind, revoke, cross-Tenant request, inactive client, rotated Casdoor secret, wrong audience/issuer/azp and OpenFGA unknown.
- [ ] Add tables with UUID IDs, unique `(issuer,external_client_id)`, binding unique `(client_application_id,tenant_id)`, state/timestamps and append-only audit rows.
- [ ] Implement token verification with cached JWKS respecting key rotation; never log token/claims wholesale.
- [ ] Extend F12 membership stage: user uses active Membership; client uses active ClientTenantBinding; both still require OpenFGA.
- [ ] Run unit/integration/race tests plus spoofed-header HTTP matrix.
- [ ] Commit: `git commit -m "feat(auth): bind client applications to tenants"`.

## Self-Review Record

- Spec coverage: domain decision, separate machine identity, Casdoor verification, binding fact, revocation, OpenFGA and audit are covered.
- Placeholder scan: identity key, schema uniqueness, claim checks and authorization branch are concrete.
- Type consistency: `Principal` is a tagged union; `ClientPrincipal` can never satisfy User Membership ports.
