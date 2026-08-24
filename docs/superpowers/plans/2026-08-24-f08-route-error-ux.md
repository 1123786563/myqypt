# F08 标准错误体验 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Portal 对 401/403/404/500/503 和 RFC Problem Details 呈现一致、可恢复且不泄露内部信息的体验。

**Architecture:** API client 将响应归一化为 `Problem`; `RouteErrorBoundary` 只按状态/稳定 code 选择页面。trace ID 可复制，原始堆栈与服务错误不渲染。

**Tech Stack:** React 19.2.5, React Router 8.3.0, Vitest Browser 4.1.4

**Spec:** [Issue #108](https://github.com/1123786563/myqypt/issues/108), extraction design §5.2

## Global Constraints

- 401 只提供重新登录；403 不暗示不存在的权限；503 提供安全重试。
- UI 只显示 `title`, `detail` 的允许内容和 `trace_id`，不显示 stack/raw error。
- 未识别错误归一为 500 `internal_error`。

---

## File Structure

- Create `web/src/lib/problem.ts` and tests.
- Create `web/src/components/platform/route-error-boundary.tsx`.
- Create `web/src/components/platform/status-pages.tsx` and browser tests.
- Modify `web/src/root.tsx` to export the root error boundary.

```ts
export type Problem = {
  type: string
  title: string
  status: 400 | 401 | 403 | 404 | 409 | 422 | 500 | 503
  code: string
  trace_id?: string
  detail?: string
}

export const statusAction = { 401: 'login', 403: 'back', 404: 'home', 500: 'reload', 503: 'retry' } as const
```

### Task 1: Normalize errors at one boundary

**Interfaces:** `Problem`, `isProblem(value): value is Problem`, `normalizeProblem(error): Problem`.

- [ ] Write table tests for valid Problem, malformed JSON, network failure, Response 401/403/404/503 and thrown Error; assert stable status/code and optional trace ID.
- [ ] Run `pnpm --dir web test --run problem`; confirm red.
- [ ] Implement normalization without trusting arbitrary HTML or object fields; cap displayed detail at 500 characters.
- [ ] Run the focused tests and typecheck.
- [ ] Commit: `git commit -m "feat(web): normalize problem details"`.

### Task 2: Render recoverable status pages

```tsx
it.each([[401, 'login'], [403, 'back'], [404, 'home'], [500, 'reload'], [503, 'retry']] as const)(
  'maps %s to %s', (status, action) => {
    render(<RouteErrorBoundary error={{ type: 'about:blank', title: 'x', status, code: 'x' }} />)
    expect(screen.getByTestId(`action-${action}`)).toBeVisible()
  },
)
```

- [ ] Write browser tests mapping 401→login action, 403→back action, 404→home, 500→reload, 503→retry; assert trace copy and absence of injected stack text.
- [ ] Run `pnpm --dir web test --run route-error`; confirm red.
- [ ] Implement a shared accessible status layout, route-aware actions and `navigator.clipboard` fallback; preserve the failing route for retry.
- [ ] Wire root and `/app` error boundaries; test keyboard order and mobile layout.
- [ ] Run `pnpm --dir web test --run && pnpm --dir web typecheck`.
- [ ] Commit: `git commit -m "feat(web): add standard route error UX"`.

## Self-Review Record

- Spec coverage: all required statuses, Problem mapping, trace display, retry and leakage prevention are covered.
- Placeholder scan: mappings, actions, cap and commands are concrete.
- Type consistency: `Problem` matches F02 fields and remains a transport-facing web type.
