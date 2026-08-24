# F06 React 静态工程与首页 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 React/TypeScript 静态工程，预渲染 `/`，产出可由 CDN 独立发布且不需要 Node 服务端的文件。

**Architecture:** React Router Framework Mode 设置 `ssr:false`；公开路由构建期 prerender，`/app/*` 作为 SPA。基础样式来自 Tailwind 4 与干净 shadcn 组件，不复制 Clerk 或演示领域。

**Tech Stack:** React 19.2.5, TypeScript 6.0.3, Vite 8.0.8, React Router 8.3.0, Tailwind CSS 4.2.2, pnpm 11.1.2

**Spec:** [Issue #106](https://github.com/1123786563/myqypt/issues/106), extraction design §5

## Global Constraints

- 浏览器构建物只有 HTML/CSS/JS/字体/图片；生产不运行 Node SSR。
- `/` 必须生成真实 `index.html`，包含 title、description、canonical 和可见正文。
- 禁止 Clerk、本地 Token、上游品牌、假登录和演示业务数据。

---

## File Structure

- Create `web/package.json`, `pnpm-lock.yaml`, `pnpm-workspace.yaml`, `web/react-router.config.ts`, `web/vite.config.ts`, `web/tsconfig.json`.
- Create `web/src/root.tsx`, `web/src/routes.ts`, `web/src/routes/home.tsx`, `web/src/styles/app.css`.
- Create `web/tests/home.test.tsx`, `web/scripts/assert-static-build.mjs`.

```ts
import type { Config } from '@react-router/dev/config'

export default {
  appDirectory: 'src',
  ssr: false,
  prerender: ['/'],
} satisfies Config
```

### Task 1: Bootstrap the static-only build

```tsx
it('renders the public entry without authentication state', () => {
  render(<Home />)
  expect(screen.getByRole('heading', { level: 1 })).toBeVisible()
  expect(screen.getByRole('link', { name: /产品/i })).toHaveAttribute('href', '/products')
  expect(screen.getByRole('link', { name: /价格/i })).toHaveAttribute('href', '/pricing')
})
```

- [ ] Add package scripts `dev`, `typecheck`, `test`, `build`, `verify:static`; pin every dependency and `packageManager: pnpm@11.1.2`.
- [ ] Write `home.test.tsx` asserting heading, product/pricing links, and accessible login link; run `pnpm --dir web test --run` and confirm red.
- [ ] Configure `react-router.config.ts` as `satisfies Config` with `appDirectory:"src"`, `ssr:false` and `prerender:["/"]`; define the home route and Tailwind theme variables.
- [ ] Implement the test with Testing Library; use semantic header/main/footer and no copied upstream content.
- [ ] Run `pnpm --dir web typecheck && pnpm --dir web test --run`.
- [ ] Commit: `git commit -m "feat(web): add static react foundation"`.

### Task 2: Prove CDN-ready output

- [ ] Write `assert-static-build.mjs` to fail unless `web/build/client/index.html` exists, has canonical/title/description/heading, references hashed assets, and contains no `/_react-router/`, Clerk, localhost, or server runtime import.
- [ ] Run `pnpm --dir web build && pnpm --dir web verify:static`; confirm the checker fails before required metadata is present, then add metadata in `root.tsx`.
- [ ] Serve `web/build/client` with a static file server and run Playwright smoke for `/` with JavaScript enabled and disabled; both must show the primary heading.
- [ ] Add build output to `.gitignore`; keep only source and lockfile.
- [ ] Commit: `git commit -m "test(web): verify static landing output"`.

## Self-Review Record

- Spec coverage: React 19, TypeScript, Tailwind 4, Framework Mode, prerender, SEO and static-only serving are covered.
- Placeholder scan: dependencies, paths, scripts, assertions and forbidden strings are exact.
- Type consistency: public route metadata and component types stay inside `web`.
