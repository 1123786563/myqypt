# F19 第三方 Catalog API 与 SDK Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 发布正式版本化的第三方 Catalog API 和可复现 TypeScript SDK，供 Product Adapter 使用并遵守 F18/F12 授权边界。

**Architecture:** `/api/v1` strict handlers 调用与 Portal 相同的 Application Modules；OpenAPI 描述 bearer client credentials、Tenant header、分页与 Problem。SDK 由契约生成并加入兼容性/发布门禁。

**Tech Stack:** OpenAPI 3.1, oapi-codegen 2.8.0, openapi-typescript 7.13.0, TypeScript, Go

**Spec:** [Issue #119](https://github.com/1123786563/myqypt/issues/119), extraction design §§7.3,8.2

## Global Constraints

- endpoint 位于 `/api/v1/catalog/products` 和 `/api/v1/catalog/products/{product_id}`；不复用 `/portal-api` 响应聚合。
- `Authorization: Bearer` 认证 Client Application，`X-Tenant-ID` 必填但不可信。
- 分页有稳定 cursor/limit，limit 默认 50 最大 100；错误全部为 Problem Details。

---

## File Structure

- Extend `api/openapi/platform.yaml` with public operations/security/schemas/examples.
- Create strict handlers and contract/security tests.
- Create `sdk/typescript/{package.json,src/index.ts,tests}` and generated types.
- Create API compatibility and SDK pack smoke scripts.

```ts
export type MyQYPTClientOptions = {
  baseUrl: string
  tenantId: string
  getAccessToken(signal?: AbortSignal): Promise<string>
  fetch?: typeof globalThis.fetch
}

export function createMyQYPTClient(options: MyQYPTClientOptions): MyQYPTClient
```

### Task 1: Publish tenant-authorized operations

```yaml
securitySchemes:
  clientBearer:
    type: http
    scheme: bearer
    bearerFormat: JWT
```

- [ ] Write OpenAPI contract tests asserting operation IDs, client bearer security, required UUID Tenant header, cursor/limit bounds, 200/400/401/403/404/503 Problems and no internal fields.
- [ ] Write HTTP matrix for valid binding, missing/wrong Tenant, revoked binding, FGA deny/unknown, cross-Tenant product ID and pagination cursor tampering.
- [ ] Implement opaque signed cursor carrying tenant/query/order position; validate before repository use and apply F12 `TenantScope` to every query.
- [ ] Generate Go/TS and run stale/compatibility checks against the previous committed OpenAPI baseline.
- [ ] Commit: `git commit -m "feat(api): publish third-party catalog endpoints"`.

### Task 2: Package a minimal generated SDK

**Interfaces:** `createMyQYPTClient({baseUrl,getAccessToken,tenantId,fetch?})`, `catalog.list({cursor,limit})`, `catalog.get(productId)`.

- [ ] Write SDK tests for auth/tenant headers, token refresh callback per request, typed success, Problem error, AbortSignal and no token persistence.
- [ ] Implement generated types plus a handwritten thin transport facade; package only dist/types/license/readme.
- [ ] Run `pnpm --dir sdk/typescript test`, typecheck, build, `pnpm pack`, install tarball into a temporary sample and compile one list/get example.
- [ ] Add SemVer/OpenAPI breaking-change gate and changelog requirement for public contract changes.
- [ ] Commit: `git commit -m "feat(sdk): add typescript catalog client"`.

## Self-Review Record

- Spec coverage: versioned public API, client auth, tenant binding, pagination, SDK generation, package smoke and compatibility are covered.
- Placeholder scan: paths, headers, limits, SDK methods and checks are exact.
- Type consistency: SDK types originate only from the OpenAPI schemas used by Go strict handlers.
