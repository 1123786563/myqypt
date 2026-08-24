# F09 生成式 TypeScript Client 与状态页 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从同一 OpenAPI 文件生成 TypeScript 类型，封装最薄的 fetch client，并在 Portal 状态页调用 Go status endpoint。

**Architecture:** `openapi-typescript` 只生成 types；`openapi-fetch` 负责类型安全请求。Query hooks 位于 feature 层，组件不拼 URL 或手写响应模型。

**Tech Stack:** openapi-typescript 7.13.0, openapi-fetch 0.17.0, TanStack Query 5.99.x, Vitest 4.1.4

**Spec:** [Issue #109](https://github.com/1123786563/myqypt/issues/109), extraction design §§6.2,7.3

## Global Constraints

- `api/openapi/platform.yaml` 是唯一类型源；生成文件禁止手改。
- base URL 默认同源；浏览器请求不附带 bearer token。
- 非 2xx 转为 F08 `Problem`，并保留服务端 trace ID。

---

## File Structure

- Create `web/src/api/schema.d.ts`, `client.ts`, `client.test.ts`, and `generate.ts`.
- Create `web/src/features/system/use-system-status.ts` and tests.
- Create `web/src/routes/app.status.tsx` and browser test.
- Modify frontend scripts and root Make generation check.

```ts
import createClient from 'openapi-fetch'
import type { paths } from './schema'

export function createPlatformClient(baseUrl = '', fetchImpl = fetch) {
  return createClient<paths>({ baseUrl, fetch: fetchImpl, credentials: 'include' })
}
```

### Task 1: Generate a stale-checked client boundary

**Interfaces:** `createPlatformClient(options?: { baseUrl?:string; fetch?:typeof fetch })` and generated `paths`.

- [ ] Add scripts `api:generate` and `api:check`; generator reads `../api/openapi/platform.yaml` and writes a deterministic `schema.d.ts`.
- [ ] Write a client test with injected fetch asserting `GET /api/v1/system/status`, `credentials:"include"`, no Authorization header, typed success, and Problem normalization.
- [ ] Run `pnpm --dir web test --run client`; confirm red.
- [ ] Generate types, implement the openapi-fetch wrapper, and make `api:check` regenerate to a temporary file then byte-compare.
- [ ] Run `pnpm --dir web api:check && pnpm --dir web typecheck && pnpm --dir web test --run client`.
- [ ] Commit: `git commit -m "feat(web): generate platform api client"`.

### Task 2: Deliver the status route

```ts
export const systemStatusQuery = (client: ReturnType<typeof createPlatformClient>) => ({
  queryKey: ['system', 'status'] as const,
  queryFn: async () => {
    const { data, error } = await client.GET('/api/v1/system/status')
    if (error) throw normalizeProblem(error)
    return data
  },
})
```

- [ ] Write a Query hook test for success, 503 Problem and retry policy (no retry for 4xx; at most two for network/5xx).
- [ ] Write a browser test asserting version/status display, loading skeleton and F08 error UI.
- [ ] Run the focused tests and confirm red.
- [ ] Implement `useSystemStatus` with query key `['system','status']` and route `/app/status`; add the authorized navigation item locally.
- [ ] Run `pnpm --dir web test --run system-status && pnpm --dir web build`.
- [ ] Commit: `git commit -m "feat(web): add platform status page"`.

## Self-Review Record

- Spec coverage: single contract source, generated types, thin client, cookies, retry policy, stale check and visible integration are covered.
- Placeholder scan: packages, symbols, endpoint, query key and commands are exact.
- Type consistency: F02 `SystemStatus` and `Problem` flow through generated `paths`; no duplicate interface is introduced.
