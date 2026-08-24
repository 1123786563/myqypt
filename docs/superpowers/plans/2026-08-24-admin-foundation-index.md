# Admin Foundation F01–F21 Implementation Plan Index

> **For agentic workers:** Each linked plan requires `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans`. Execute only an issue whose native GitHub `blocked_by` list is empty and whose prerequisite acceptance evidence is present.

**Goal:** 将 `shadcn-admin + go-admin` 白名单抽取拆成 21 个可独立审阅、可在单个 agent 上下文中完成的实施单元。

**Spec:** [`2026-08-24-shadcn-admin-go-admin-extraction-design.md`](../specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md)

## Execution Map

| ID | Issue | Plan | Direct prerequisites |
| --- | --- | --- | --- |
| F01 | [#100](https://github.com/1123786563/myqypt/issues/100) | [Go process/livez](2026-08-24-f01-go-process-livez.md) | — |
| F02 | [#102](https://github.com/1123786563/myqypt/issues/102) | [OpenAPI strict transport](2026-08-24-f02-openapi-strict-transport.md) | F01 |
| F03 | [#103](https://github.com/1123786563/myqypt/issues/103) | [PostgreSQL migrations/readiness](2026-08-24-f03-postgres-migrations-readiness.md) | F01 |
| F04 | [#104](https://github.com/1123786563/myqypt/issues/104) | [HTTP security/observability](2026-08-24-f04-http-security-observability.md) | F02 |
| F05 | [#105](https://github.com/1123786563/myqypt/issues/105) | [Evidence/dependency gate](2026-08-24-f05-evidence-dependency-gate.md) | F02, F03, F04 |
| F06 | [#106](https://github.com/1123786563/myqypt/issues/106) | [React static landing](2026-08-24-f06-react-static-landing.md) | — |
| F07 | [#107](https://github.com/1123786563/myqypt/issues/107) | [AppShell](2026-08-24-f07-app-shell.md) | F06 |
| F08 | [#108](https://github.com/1123786563/myqypt/issues/108) | [Route error UX](2026-08-24-f08-route-error-ux.md) | F07 |
| F09 | [#109](https://github.com/1123786563/myqypt/issues/109) | [Generated TS client/status](2026-08-24-f09-generated-ts-client-status.md) | F02, F06, F08 |
| F10 | [#110](https://github.com/1123786563/myqypt/issues/110) | [Keycloak browser session](2026-08-24-f10-keycloak-browser-session.md) | #101, F04, F07, F08 |
| F11 | [#111](https://github.com/1123786563/myqypt/issues/111) | [Tenant selector](2026-08-24-f11-tenant-selector.md) | #4 (T03), F10 |
| F12 | [#112](https://github.com/1123786563/myqypt/issues/112) | [Tenant authorization](2026-08-24-f12-tenant-authorization-fail-closed.md) | #9, #10, #11, F11 |
| F13 | [#113](https://github.com/1123786563/myqypt/issues/113) | [Catalog console table](2026-08-24-f13-catalog-console-table.md) | #14 (T13), F12 |
| F14 | [#114](https://github.com/1123786563/myqypt/issues/114) | [Published snapshot API](2026-08-24-f14-published-catalog-snapshot-api.md) | #14, #16, F02, F03 |
| F15 | [#115](https://github.com/1123786563/myqypt/issues/115) | [Public product/pricing prerender](2026-08-24-f15-public-product-pricing-prerender.md) | F06, F14 |
| F16 | [#116](https://github.com/1123786563/myqypt/issues/116) | [Catalog build trigger](2026-08-24-f16-catalog-event-build-trigger.md) | F15 |
| F17 | [#117](https://github.com/1123786563/myqypt/issues/117) | [CDN atomic switch/rollback](2026-08-24-f17-cdn-atomic-switch-rollback.md) | F16 |
| F18 | [#118](https://github.com/1123786563/myqypt/issues/118) | [Client Application binding](2026-08-24-f18-client-application-identity-binding.md) | F03, F04 |
| F19 | [#119](https://github.com/1123786563/myqypt/issues/119) | [Third-party API/SDK](2026-08-24-f19-third-party-catalog-api-sdk.md) | F12, F14, F18 |
| F20 | [#120](https://github.com/1123786563/myqypt/issues/120) | [Higress routing/header cleaning](2026-08-24-f20-higress-routing-header-cleaning.md) | F09, F10, F12, F17, F19 |
| F21 | [#121](https://github.com/1123786563/myqypt/issues/121) | [Final acceptance gate](2026-08-24-f21-admin-foundation-final-gate.md) | F05, F13, F15, F20 |

## Planning Decision

- F01–F21 每份计划为 2–3 个可审阅 Task；执行时仍按每个复选项中的测试优先顺序逐项完成，不把多个 Task 合并成一次大改。
- 当前没有继续创建子 issue：进一步拆分会把同一个垂直切片的测试、接口与提交证据分离；若执行中发现某一计划无法在单个上下文完成，再从该 issue 创建具备独立验收标准的子 issue。
- #101 是既有 Identity Binding issue，不属于 F 编号；F10 明确消费其 `VerifiedIdentity -> User` 边界。

## Cross-Plan Invariants

- OpenAPI 3.1 是 Go strict server 与 TypeScript client 的唯一 HTTP 契约源。
- Browser 使用 Go BFF HttpOnly Session；第三方 Client 使用独立 Client Application principal。
- Tenant 访问只能由 PostgreSQL active fact 与 OpenFGA allow 共同产生 `TenantScope`。
- React 只生成静态资源；公开页 prerender，`/app` SPA fallback，CDN 原子发布。
- 所有 generation、policy、security、cross-tenant、publish/rollback 证据最终由 F21 分层报告，skipped 不等于 passed。
