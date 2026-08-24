# F13 Catalog 控制台表格 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付第一个真实 Tenant-scoped 控制台页面，以服务端分页/排序/筛选验证 AppShell、DataTable、API 与授权链路。

**Architecture:** Catalog Application query 接受显式白名单字段和 F12 `TenantScope`；sqlc 查询始终包含 tenant_id。`PlatformDataTable` 封装 URL 状态、加载、空状态、错误和分页。

**Tech Stack:** Go 1.26.7, pgx/sqlc 1.31.1, React 19, TanStack Table 8.21.3, TanStack Query

**Spec:** [Issue #113](https://github.com/1123786563/myqypt/issues/113), T13/T14 plans, extraction design §5.2

## Global Constraints

- page size 允许 20/50/100，最大 100；排序仅 `name`, `status`, `updated_at`。
- 查询必须要求 TenantScope，SQL 中显式 `tenant_id = $1`。
- 无通用 CRUD、反射过滤或客户端全量下载后分页。

---

## File Structure

- Create `internal/application/catalog/query.go` and tests.
- Create `internal/adapter/postgres/catalog.sql`, generated `catalog.sql.go`, repository and integration tests.
- Extend OpenAPI with `GET /portal-api/catalog/products`.
- Create `web/src/components/platform/platform-data-table.tsx`; create Catalog page/hooks/tests.

```go
type CatalogQuery struct { Page, PageSize int; Sort, Direction, Filter string }
type CatalogPage struct { Items []CatalogProduct; Page, PageSize int; Total int64 }
type CatalogReader interface {
    List(context.Context, tenantauth.TenantScope, CatalogQuery) (CatalogPage, error)
}
```

### Task 1: Deliver a tenant-safe paged query

**Interfaces:** `CatalogQuery{Page,PageSize,Sort,Direction,Filter}`, `CatalogPage{Items,Page,PageSize,Total}`, `CatalogReader.List(context.Context,TenantScope,CatalogQuery)`.

```sql
ALTER TABLE catalog_products ENABLE ROW LEVEL SECURITY;
ALTER TABLE catalog_products FORCE ROW LEVEL SECURITY;
CREATE POLICY catalog_products_tenant ON catalog_products
USING (tenant_id = current_setting('app.tenant_id', true)::uuid);
```

- [ ] Write validation tests for default/maximum page, allowed sort/direction, normalized filter and rejected unknown fields.
- [ ] Write PostgreSQL tests with two tenants proving no cross-tenant count or row leakage under every sort/filter combination, including a deliberately omitted application predicate that database RLS must still reject.
- [ ] Run focused tests and confirm red.
- [ ] Add RLS policy driven by transaction-local trusted Tenant ID, force RLS on Tenant-owned Catalog tables, and add sqlc query with allowlisted ORDER BY branches, stable ID tie-breaker, and separate tenant-scoped count.
- [ ] Add generated OpenAPI response and strict handler using F12 scope.
- [ ] Run generation stale checks and `go test ./internal/application/catalog ./internal/adapter/postgres/... -count=1`; commit `feat(catalog): add tenant product query`.

### Task 2: Build the reusable server-driven table

```ts
export type PlatformDataTableProps<T> = {
  columns: ColumnDef<T>[]
  rows: readonly T[]
  page: number
  pageSize: 20 | 50 | 100
  total: number
  sort: { id: 'name' | 'status' | 'updated_at'; desc: boolean }
  filter: string
  state: 'loading' | 'ready' | 'empty' | 'error'
  onQueryChange(next: TableQuery): void
}
```

- [ ] Define `PlatformDataTableProps<TData>` with columns, page state, sort state, filter text, total, loading, problem and callbacks; no API import inside the component.
- [ ] Write browser tests for URL synchronization, debounced filter, allowed sorting, next/previous, empty/loading/error, keyboard row action and selection reset after Tenant switch.
- [ ] Implement `/app/catalog` query hook and columns for name/version/status/updated time; hide actions not supplied by authorized navigation/action model.
- [ ] Run frontend tests/typecheck/build and Playwright with two tenants.
- [ ] Commit: `git commit -m "feat(portal): add catalog data table"`.

## Self-Review Record

- Spec coverage: real Catalog page, server pagination/filter/sort, tenant SQL isolation, reusable table states and URL sync are covered.
- Placeholder scan: allowed fields, sizes, types, SQL behavior and tests are concrete.
- Type consistency: query types are Application types; generated HTTP and table types adapt at boundaries.
