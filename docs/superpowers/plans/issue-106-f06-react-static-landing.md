# Issue #106 [F06][Portal] React 静态官网入口 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec / Issue:** [Issue #106 — [F06][Portal] React 静态官网入口](https://github.com/1123786563/myqypt/issues/106), extraction design §5, §12.1, §13 (`docs/superpowers/specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md`)

**Goal:** 在 `web/` 建立自包含的 React 19 + TypeScript + Vite + Tailwind CSS 4 + shadcn 约定 + React Router Framework Mode 前端工程，运行期 `ssr:false`，构建期预渲染 `/`，产出可由 CDN 独立发布、无需 JavaScript 即可读取标题/正文/canonical/metadata 的静态官网首页。

**Supersedes:** `docs/superpowers/plans/2026-08-24-f06-react-static-landing.md`（batch 计划缺少 lint/format 脚本、THIRD_PARTY_NOTICES 任务与可用版本校验；本计划为 Issue #106 的执行计划，差异均在本文件 Global Constraints 与任务拆分中体现）。

## Scope（范围）

- 新建 `web/` 单一 pnpm 工程（package.json、pnpm-lock.yaml、全部配置），不创建根级 workspace 文件。
- React Router Framework Mode：`react-router.config.ts` 设 `appDirectory:"src"`、`ssr:false`、`prerender:["/"]`。
- 首页路由 `/`：语义化 header/main/footer、平台导航（产品 `/products`、价格 `/pricing`、进入控制台 `/app`）、平台自有品牌文案（中文）。
- `web/src/styles/app.css`：Tailwind 4 + shadcn oklch CSS Variables（`:root` 与 `.dark`）、平台品牌色、暗色模式基础（无 FOUC 内联脚本 + 最小主题切换按钮）。
- shadcn 约定组件：`components/ui/button.tsx`（canonical shadcn Button）+ `lib/utils.ts` 的 `cn()`。
- 工程门禁脚本：`typecheck`、`lint`（ESLint flat config）、`format:check`/`format`（Prettier）、`test`（Vitest + Testing Library + jsdom）、`build`、`verify:static`、`test:e2e`（Playwright 冒烟，JS 开/关两种上下文）、`verify:forbidden`（禁止内容扫描）。
- 仓库根新增 `THIRD_PARTY_NOTICES.md`：记录 shadcn-admin 固定 commit、MIT 许可证、抽取范围与本地修改；记录 shadcn/ui canonical 组件来源。
- `web/.gitignore` 忽略 `node_modules/`、`build/`、Playwright 产物；仓库根 `.gitignore` 不改动。

## Non-goals（非目标）

- 不实现 `/products`、`/pricing` 真实页面与数据（F13–F15）；本计划只交付指向它们的导航链接。
- 不实现 `/app` AppShell、Sidebar、错误页（F07/F08）、任何会话/登录/认证流程（F10）；禁止演示登录、注册、OTP。
- 不做 CDN 发布、Catalog 构建触发、原子切换（F16/F17）。
- 不做 i18n、完整 AppearancePreferences 模块、TanStack Query/Table（后续 Foundation 任务）。
- 不改动任何 Go 源码、go.mod、部署配置；不新增 go-admin 相关内容（F05 负责）。
- 不复制上游品牌图片、Logo、营销文案、演示业务数据、上游完整 lockfile。

## Global Constraints（全局约束）

- 生产不运行 Node SSR：浏览器交付物只有 HTML/CSS/JS/字体/图片；`/` 必须生成真实 `web/build/client/index.html`，无 JS 时可读 title、description、canonical 与可见正文。
- 前端明确拒绝清单（spec §5.4，全部纳入 `verify:forbidden` 扫描）：`@clerk/*` 与 Clerk 路由；localStorage/sessionStorage 中的 access token（唯一允许的存储键 `ui-theme`，仅存主题偏好，扫描脚本需显式 allowlist 并注释理由）；演示登录/注册/OTP；Tasks/Chats/Users/Apps 假数据；上游品牌资产与文案；`/_react-router/` 服务器运行时引用；localhost 引用；未使用的上游依赖。
- 依赖版本精确固定（registry.npmmirror.com，2026-08-25 验证存在）：react 19.2.8、react-dom 19.2.8、react-router 8.3.0、@react-router/dev 8.3.0、@react-router/node 8.3.0、@react-router/serve 8.3.0、typescript 6.0.3（typescript-eslint 8.68.0 peer 上限 `<6.1.0`，故不用 7.x）、vite 8.2.2、tailwindcss 4.3.3、@tailwindcss/vite 4.3.3、vitest 4.1.11、jsdom 30.0.1、@testing-library/react 16.3.2、@testing-library/jest-dom 7.0.1、@testing-library/dom 10.x（按 peer 解析的精确版本）、prettier 3.9.6、eslint 10.9.1、@eslint/js 10.0.1、typescript-eslint 8.68.0、eslint-plugin-react-hooks 7.1.1、eslint-plugin-react-refresh 0.5.4、globals 17.11.0、@playwright/test 1.62.1（本机浏览器缓存已就绪）、class-variance-authority 0.7.1、clsx 2.1.1、tailwind-merge 3.6.0、@radix-ui/react-slot 1.3.3、@types/react 19.2.18、@types/react-dom 19.2.5、@types/node 精确最新。`packageManager: "pnpm@11.7.0"`（本机 pnpm 11.7.0、node v26.7.0）。lockfile 必须提交。不引入 lucide-react、axios、TanStack、Clerk 等非必需依赖。
- 只允许新增/修改：`web/**`、`THIRD_PARTY_NOTICES.md`、本计划文件；不得改动 Go 源码、go.mod/go.sum、根 `.gitignore`、其他计划文档。
- 测试必须断言真实行为（禁止 assert-nothing 测试）；测试输出必须干净（无 stray warning）。
- 环境事实：npm registry 为 registry.npmmirror.com；Playwright 浏览器缓存在默认路径；Go 1.26.7 二进制位于 `/tmp/issue100-task2-go1267-retry.E59JCp/go/bin/go`（仅回归验证用）。
- 不执行 push、merge、发布、关闭 Issue 或任何 GitHub 状态修改。

## 测试与验收命令（全部在仓库根执行）

1. `pnpm --dir web install --frozen-lockfile`（Task 1 首次安装后以 lockfile 提交为准）
2. `pnpm --dir web typecheck`
3. `pnpm --dir web lint`
4. `pnpm --dir web format:check`
5. `pnpm --dir web test`
6. `pnpm --dir web build`
7. `pnpm --dir web verify:static`
8. `pnpm --dir web test:e2e`
9. `pnpm --dir web verify:forbidden`
10. Go 回归（不受本任务影响，仍须通过）：`PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH gofmt -l .`（输出为空）、`go vet ./...`、`go build ./...`、`go test ./... -count=1`

---

## File Structure

- `web/package.json`、`web/pnpm-lock.yaml`、`web/vite.config.ts`、`web/react-router.config.ts`、`web/tsconfig.json`、`web/eslint.config.js`、`web/.prettierrc.json`、`web/.prettierignore`、`web/vitest.config.ts`、`web/playwright.config.ts`、`web/.gitignore`
- `web/src/root.tsx`、`web/src/routes.ts`、`web/src/routes/home.tsx`
- `web/src/styles/app.css`、`web/src/lib/utils.ts`、`web/src/components/ui/button.tsx`、`web/src/components/theme-toggle.tsx`
- `web/tests/setup.ts`、`web/tests/home.test.tsx`、`web/tests/smoke.home.spec.ts`
- `web/scripts/assert-static-build.mjs`、`web/scripts/serve.mjs`、`web/scripts/check-forbidden-content.mjs`
- `THIRD_PARTY_NOTICES.md`（仓库根）

### Task 1: Bootstrap the static-only React foundation

- [ ] 创建 `web/` 工程（按 Global Constraints 版本矩阵精确固定），脚本：`dev`、`build`、`typecheck`、`lint`、`format`、`format:check`、`test`（vitest run）。
- [ ] 先写 `web/tests/home.test.tsx`（TDD RED）：断言 `h1` 可见、`产品` 链接 href=`/products`、`价格` 链接 href=`/pricing`、可访问的控制台入口链接 href=`/app`；运行 `pnpm --dir web test` 确认失败并记录 RED 证据。
- [ ] 配置 `react-router.config.ts`（`satisfies Config`，`appDirectory:"src"`、`ssr:false`、`prerender:["/"]`）、`vite.config.ts`（reactRouter + tailwindcss 插件）、`routes.ts` 定义 `/`、`root.tsx` 输出 lang、title、description、canonical 等基础 metadata、ESLint flat config、Prettier、`tsconfig.json`。
- [ ] 实现 `home.tsx`（语义化 header/main/footer，中文平台文案，无上游内容）、`app.css`（Tailwind 4 + shadcn oklch 变量 + `.dark` + 无 FOUC 主题脚本）、`button.tsx`（canonical shadcn Button）、`theme-toggle.tsx`、`lib/utils.ts` 的 `cn()`。
- [ ] 运行 `pnpm --dir web typecheck && pnpm --dir web lint && pnpm --dir web format:check && pnpm --dir web test` 全部通过（GREEN），提交 lockfile。
- [ ] Commit: `feat(web): add static react foundation`

### Task 2: Prove CDN-ready static output

- [ ] 编写 `web/scripts/assert-static-build.mjs`（接受可选目标目录参数，默认 `web/build/client`）：`index.html` 存在；包含非空 `<title>`、meta description、`rel="canonical"`、可见 `h1` 文本；引用带内容哈希的 `/assets/*`；不包含 `/_react-router/`、Clerk、`localhost`、服务器运行时导入。
- [ ] RED 证据：对缺少 canonical/metadata 的临时 fixture HTML 运行 checker，确认按预期失败；再对真实构建运行并通过。
- [ ] 编写 `web/scripts/serve.mjs`（node:http 静态服务器，服务 `web/build/client`，端口 4173）与 `web/playwright.config.ts`（webServer 复用该脚本）。
- [ ] `web/tests/smoke.home.spec.ts`：JS 启用与禁用两种上下文加载 `/`，均可见主标题与正文；JS 启用时导航链接可点击。
- [ ] 新增脚本 `verify:static`、`test:e2e`；`web/.gitignore` 忽略 `node_modules/`、`build/`、`playwright-report/`、`test-results/`。
- [ ] 运行 `pnpm --dir web build && pnpm --dir web verify:static && pnpm --dir web test:e2e` 通过。
- [ ] Commit: `test(web): verify static landing output`

### Task 3: Provenance and forbidden-content gate

- [ ] 创建仓库根 `THIRD_PARTY_NOTICES.md`：shadcn-admin（repo `satnaing/shadcn-admin`，commit `e16c87f213a5ba5e45964e9b67c792105ec74d26`，MIT）——抽取范围（工程配置模式、Tailwind 4 + shadcn CSS Variables 主题方式、组件约定）、明确未复制项（Clerk 认证、演示领域、品牌资产、lockfile）、本地修改说明；shadcn/ui canonical 组件（Button，MIT，本地维护副本）来源与差异；F05 将补充 go-admin 部分（本任务不写占位 stub）。
- [ ] 编写 `web/scripts/check-forbidden-content.mjs`：扫描 `web/src`、`web/tests`、`web/scripts`、`web` 根配置文件，拒绝 `@clerk`/Clerk 路由、token 类键名（access/id/refresh token 等）进入 storage、`Bearer` 字面量、上游品牌字符串（shadcn-admin 营销文案）、演示路由（tasks/chats/users apps）、`/_react-router/`、`localhost`；`ui-theme` 为显式 allowlist 项并注释理由；退出码非 0 表示失败。
- [ ] 新增脚本 `verify:forbidden`；运行通过并记录输出。
- [ ] 运行完整验收命令 1–10 并记录全部输出（含 Go 回归，Go 二进制路径见 Global Constraints）。
- [ ] Commit: `docs(web): record third-party provenance and content gates`

## Self-Review Record

- Spec coverage: Issue #106 五条验收标准分别由 Task 1（栈/样式/门禁脚本）、Task 2（无 JS 可读 + 静态构建）、Task 3（禁止内容 + THIRD_PARTY_NOTICES）覆盖。
- Placeholder scan: 版本均为 registry 实测存在值；路径、脚本名、断言、禁止字符串精确。
- Type consistency: 元数据类型与组件类型全部封闭在 `web` 内；不触碰 Go 侧类型。
