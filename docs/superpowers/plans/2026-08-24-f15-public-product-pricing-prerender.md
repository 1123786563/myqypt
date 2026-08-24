# F15 产品与价格公开页预渲染 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 F14 Snapshot 在构建期生成产品目录、产品详情和价格页，使其无 JavaScript 也可读且 SEO 完整。

**Architecture:** 构建脚本先下载并校验一个 immutable Snapshot，再向 React Router 提供固定 prerender route 列表。路由组件只读取构建期数据文件，不在浏览器重新获取 Catalog。

**Tech Stack:** React Router 8.3.0, React 19, Vite 8, TypeScript, Playwright

**Spec:** [Issue #115](https://github.com/1123786563/myqypt/issues/115), extraction design §§5.3,8.3

## Global Constraints

- 生成 `/products`, `/products/:slug`, `/pricing`; slug 唯一且由发布事实提供。
- 构建固定 Snapshot version；构建期间版本变化必须重启而非混用。
- HTML 包含 canonical、title、description、Open Graph、JSON-LD 和可见价格/币种。

---

## File Structure

- Create `web/scripts/fetch-catalog-snapshot.ts`, `catalog-routes.ts`, `verify-public-pages.mjs`.
- Create `web/src/features/public-catalog/{model,repository}.ts` and tests.
- Create public product index/detail/pricing route modules and component tests.
- Modify `react-router.config.ts` to use the generated route list.

```ts
export type BuildCatalog = {
  version: string
  generated_at: string
  products: readonly PublishedProduct[]
}

export const publicRoutes = (catalog: BuildCatalog) => [
  '/products', '/pricing', ...catalog.products.map((p) => `/products/${p.slug}`),
].sort()
```

### Task 1: Freeze one validated build input

- [ ] Write tests rejecting missing version, duplicate slug, private/unknown fields, unsupported currency, invalid effective time and hash mismatch.
- [ ] Run `pnpm --dir web test --run public-catalog`; confirm red.
- [ ] Implement fetch with builder credential supplied only to build process, ETag/version verification and atomic write to `.cache/catalog/<version>.json`; do not emit credential to Vite env/client bundle.
- [ ] Generate sorted routes `['/products','/pricing',...detailRoutes]`; fail when a published product has no slug.
- [ ] Run focused tests and scan output with `rg -n 'CATALOG_BUILDER|Authorization' web/build web/.cache` expecting no credential value.
- [ ] Commit: `git commit -m "feat(web): add catalog build input"`.

### Task 2: Prerender and validate public pages

```tsx
export function CatalogVersion({ catalog }: { catalog: BuildCatalog }) {
  return <p>Catalog {catalog.version.slice(0, 12)} · {new Date(catalog.generated_at).toISOString()}</p>
}
```

- [ ] Write component/browser tests for directory cards, detail metadata, offer/pricing display, visible Snapshot version/effective time, unavailable offer omission, canonical links and JSON-LD schema.
- [ ] Implement routes from the local snapshot repository; share pure presentation components, not admin table components.
- [ ] Build and run `verify-public-pages.mjs` over every expected HTML file; assert meaningful body with JavaScript disabled and no draft sentinel.
- [ ] Run Playwright desktop/mobile accessibility smoke and link crawl across all generated routes.
- [ ] Commit: `git commit -m "feat(web): prerender product and pricing pages"`.

## Self-Review Record

- Spec coverage: frozen snapshot, dynamic route enumeration, SEO, no-JS content, pricing and secret exclusion are covered.
- Placeholder scan: route set, validations, cache path, metadata and commands are concrete.
- Type consistency: build model validates F14 read DTO once; routes consume the validated model.
