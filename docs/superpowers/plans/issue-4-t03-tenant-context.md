# Issue #4 [T03][P2] Tenant Context 选择与切换 — Implementation Plan

- Issue: https://github.com/1123786563/myqypt/issues/4 （OPEN，0 评论；label `ready-for-agent`；唯一前置 #3 T02 已 CLOSED 2026-08-27T01:12:04Z，本机 main=origin/main=a86732d 亲验）
- Branch: `codex/issue-4-t03-tenant-context`（base = main@a86732d）
- Worktree: `/Users/wuyongjun/trea/myqypt-worktrees/issue-4-t03-tenant-context`
- 源计划：Issue 正文与 `docs/superpowers/plans/2026-08-24-t03-tenant-context.md` 逐字一致（通用模板，落地按下述裁定修订）
- Spec 引用：`CONTEXT.md`（Tenant Context 词条）、ADR-0028（Platform Context 仅在可信边缘签发）、ADR-0013（全局用户与 Tenant 生命周期分离）、ADR-0009（禁止 Cross-Tenant 共享）

## Goal

User 明确选择 Tenant Context，并能在所属 Tenant 间安全切换。客户端提交的 Tenant ID 只是选择请求；服务端验证 active Membership 后持久化选择，读取时再次验证（撤销即失效）；非所属 Tenant 的选择被拒绝。

## 设计裁定（controller，2026-08-27）

1. **接缝裁定：公开 Portal API 进 OpenAPI 契约，不进 /internal/v1。** 新增三个操作到 `api/openapi/platform.yaml`，经 `go generate ./...` 重生成 `internal/transport/http/api/server.gen.go`（oapi-codegen，generate-check 门禁覆盖）：
   - `GET /api/v1/tenants`（operationId `listTenants`）——列出认证用户具有 active Membership 的 Tenant；
   - `GET /api/v1/tenant-context`（`getTenantContext`）——返回服务端验证过的当前选择；
   - `PUT /api/v1/tenant-context`（`putTenantContext`）——显式选择/切换，幂等。
   理由：F11（Tenant 选择器前端）与 F09（OpenAPI 生成客户端）以类型化契约为消费面；identity callback 的 /internal/v1 先例是机器对机器管线，不适用用户面。现在入契约避免 F 系列返工。
2. **认证裁定：Bearer 复用 `identity.Verifier` 端口，认证在 handler 方法内执行。** 生成的平铺注册（RegisterHandlersWithOptions 单组全量注册）无法携带 per-route 中间件，故三个新 handler 方法共用一个 `authenticateTenantUser(c)` 助手：解析 `Authorization: Bearer`（复用 `bearerToken`）→ Verify → verified identity。状态映射沿 identity callback 设计裁定 6：缺/畸变凭证与 `ErrInvalidToken` → 401 unauthorized；`ErrProviderUnavailable` → 503 dependency_unavailable；认证依赖未装配（nil）→ 503 fail-closed 先行。
3. **装配裁定：transport `Dependencies` 新增 `Tenancy *TenancyDependencies{Verifier identity.Verifier; Repository tenancy.Repository}`。** 契约路径恒注册（契约面完整），`Tenancy` 为 nil 时三个方法一律 503 dependency_unavailable（identity callback 的 fail-closed per-dependency 先例）。main.go 组合根：与 identity 共用同一 `PLATFORM_IDENTITY_OIDC_ISSUER`/`PLATFORM_IDENTITY_OIDC_AUDIENCE` env 对与同一 pgxpool；issuer/audience 任一缺失 → Tenancy 为 nil（与 identity 相同的装配语义）；无 DATABASE_URL → Repository 留 nil → 503。
4. **领域裁定：新应用包 `internal/application/tenancy`（领域类型+端口+Service）。** 类型：`TenantSummary{TenantID, Kind, Role}`、`TenantContext{TenantID, SelectedAt}`。端口 `Repository`：`ListMembershipTenants(ctx, userID)`（仅 active）、`SelectedTenant(ctx, userID)`（JOIN active membership，缺席或已失效 → `ErrNoTenantContext`）、`SaveSelection(ctx, userID, tenantID)`（事务内验证 active membership + upsert）。Service `Service`：`List`/`Current`/`Select`；分类错误 `ErrUserRequired`、`ErrTenantRequired`、`ErrNotAnActiveMember`（写前拒绝）、`ErrNoTenantContext`（读缺席）。T04/T05 将沿用本包承载 tenancy 领域。
5. **持久化裁定：迁移 `000004_tenant_context_selections.sql`。** 单表 `tenant_context_selections(platform_user_id uuid PRIMARY KEY REFERENCES platform_users(id) ON DELETE CASCADE, tenant_id uuid NOT NULL REFERENCES tenants(id), selected_at timestamptz NOT NULL DEFAULT now())`；Up 建表、Down 恰 DROP 本表（down-one 地板随移，`migrate_test.go` 连锁更新沿 T02 先例）。每用户至多一行（PK），切换即 upsert（last-write-wins，选择是用户显式动作）；并发切换用事务内 `pg_advisory_xact_lock(hashtextextended(user_id))` 串行化。读取路径 JOIN active membership 再验证（撤销立即失效，不删行）。FK 到 tenants 不加 cascade（Stage-1 无 Tenant 删除路径，RESTRICT 即默认）。
6. **HTTP 契约 schema 裁定：**
   - `GET /api/v1/tenants` 200：`{tenants: [{tenant_id(uuid), kind enum[personal,business], role enum[owner,admin,billing_member,member]}]}`（数组与三字段全 required，additionalProperties false）——枚举与迁移 000003 的 CHECK 约束逐字一致；
   - `GET /api/v1/tenant-context` 200：`{tenant_id(uuid), selected_at(date-time)}`；无有效选择 → 404 Problem（`not_found`）；
   - `PUT /api/v1/tenant-context` 请求 `{tenant_id(uuid) required}` → 200 `{tenant_id, selected_at}`（幂等切换；singleton 资源状态，PUT 语义 200 非 201）；非 active membership → 404 Problem `not_found`；
   - **拒绝状态裁定 404（非 403）**：选择请求引用无 active membership 的 Tenant 与引用不存在 Tenant 对客户端不可区分——不建存在性 oracle；可切换集可经 GET /api/v1/tenants 全量枚举，404 覆盖「已撤销」与「不存在」两种拒绝（F11 AC3）；
   - 畸形 uuid / 未知字段 → OpenAPI validator 400 `invalid_request`（既有中间件免费提供）；
   - 契约声明 `components/securitySchemes.bearerAuth`（http bearer）并在三操作上 `security`，仅文档化（validator 不强制 security，认证由 handler 内助手执行）。
7. **测试裁定（RED→GREEN 两段留痕沿 T02）：**
   - **聚焦无 DB（先红）**：tenancy Service 合约测试（in-memory Repository fake）——ErrUserRequired/ErrTenantRequired 前置拒绝零端口调用、Select 成功一条业务效应、重复 Select 幂等、ErrNotAnActiveMember 分类、Current 缺席 ErrNoTenantContext；
   - **聚焦带 DB（后红）**：postgres 仓储测试（临时 PG）——ListMembershipTenants 仅 active、SelectedTenant JOIN 再验证（撤销后失效）、SaveSelection 非成员拒绝且零写入、upsert 切换 last-write-wins、并发 16 goroutine 同用户切换终态一致（advisory lock）、故障注入（SaveSelection 事务中途失败零残留）；
   - **transport 测试**：三端点 httptest（fake deps）——200 形状（字段逐字）、401（缺/畸变/篡改 Bearer）、503（nil deps/ErrProviderUnavailable）、404（非成员选择/无选择读取）、PUT 幂等重放 200；
   - **contract 回归**：F02 contract_test.go 既有路径不受影响（其为路径定向断言，不枚举全路径——已核）；新增端点不进其断言面（留 F09 收口）。
8. **验收旅程裁定：seam `lighthouse-tenant-context`。** 新文件 `tests/acceptance/scenarios/t03-tenant-context.yaml` + `t03_tenant_context_driver.go` + `t03_tenant_context_test.go`（`T03_ACCEPTANCE_STACK=1` 门控，skip 消息携带起栈命令——含 `PLATFORM_IDENTITY_OIDC_AUDIENCE=t03-acceptance` 覆盖——与 down -v 重置命令，沿 T02 先例）。旅程（复用 journey helper：provisionCasdoor/mintToken/postCallback/openPlatformDB/tamperSignature）：
   - Casdoor 双用户（t03-select-a / t03-select-b）双 token；
   - 双 callback 首绑（各 201）——各自获得 personal tenant + owner membership（T02 交付物）；
   - stale-state 预检：platform_users 计数为 0（沿 T02）；
   - `GET /api/v1/tenants`（token-a）→ 200 恰 1 个 personal/owner；
   - `PUT /api/v1/tenant-context`（选 A 自有 tenant）→ 200，tenant_id 回显，selected_at RFC3339；重放同 PUT → 200 幂等，DB 恰 1 行选择；
   - **切换证明（fixture 设置裁定）**：driver 直连 DB 为 user A 注入 tenant B 的第二 membership（`INSERT INTO memberships ... ('member','active')`）——业务上多成员归 T04/T05，无 API 可用；此为 driver 侧 fixture 写入（非断言读取），沿 T02 driver 直连 DB 先例的自然延伸，计划内披露。此后 `GET /api/v1/tenants`（token-a）→ 2 个；`PUT` 切到 B → 200；`GET` 反映 B；切回 A → 200（真实两次切换）；
   - **撤销失效**：driver 直连 DB `UPDATE memberships SET status='revoked'`（A 在 B 的 membership）→ `GET /api/v1/tenant-context` → 404（服务端再验证否决已持久化选择）；`GET /api/v1/tenants` → 回到 1 个；重新选 A 自有 → 200；
   - **跨租户攻击拒绝**：token-a PUT 选 B 的 tenant（撤销后）→ 404；token-b PUT 选 A 的 tenant → 404；denial 行增量断言（tenant_context_selections 行数不变）；
   - **凭证拒绝**：无 Authorization → 401；篡改签名 token → 401（三端点各验其一）；
   - 断言集 ~16 条（见 driver t03AssertionNames，与 YAML 逐名同序机械对照）。
9. **回归义务**：T01、T02 旅程独立栈周期重跑（绿）；`migrate_test.go` down-one 地板连锁更新；identity/repository 既有测试零触碰；`openIdentityTestDB` 的 TRUNCATE 清单从五表扩为六表（+tenant_context_selections）——沿 T02 修复轮 R1 的 repeat-safe 先例：迁移 up 后残留的上一轮选择行会让 -count≥2 的第二轮撞脏态，扩清单保持同持久库重复运行幂等（断言虽不依赖该表行数绝对值，仍按先例防御）。
10. **禁止事项（全局）**：不 push/merge/关闭 Issue/评论 GitHub；不派生 subagent（implementer 内）；不改 web/、deploy/（除 compose 无需改——audience 经 env 覆盖）；不动 T01/T02 既有测试语义；秘密/凭证不入日志与证据。

## Scope（范围）

- `api/openapi/platform.yaml`：+3 操作 +securitySchemes +4 schema（TenantSummary/TenantList/TenantContextSelection/SelectedTenantContext 命名以生成代码为准）
- `internal/transport/http/api/server.gen.go`：go generate 重生成（豁免文件，policy-check 不扫）
- `internal/application/tenancy/tenancy.go` + `service.go` + `service_test.go`（新包）
- `internal/adapter/postgres/tenancy_repository.go` + `tenancy_repository_test.go`（新）
- `internal/transport/http/tenancy.go` + `tenancy_test.go`（新）
- `internal/transport/http/router.go`：Dependencies +Tenancy；StatusHandler 旁挂新 strict handler（组合结构裁定：`api.ServerInterface` 实现者改为聚合 `statusTenancyAPI{StatusHandler; TenancyHandler}` 或等价——以最小 diff 为准）
- `cmd/platform-api/main.go`：tenancyDependencies 装配（复用 identity env 对与 pool）
- `db/migrations/000004_tenant_context_selections.sql` + `db/migrations/migrations.go`（embed 自动）+ `internal/adapter/postgres/migrate_test.go`（down-one 地板）
- `internal/adapter/postgres/identity_repository_test.go`：TRUNCATE 清单 5→6 表（裁定 9）
- `tests/acceptance/`：+scenarios/t03-tenant-context.yaml、+t03_tenant_context_driver.go、+t03_tenant_context_test.go

## Non-goals（非目标）

- 不签发 Platform Context token（T22/#23：OpenFGA 验证 + 短期 audience-bound 断言）
- 不做 OpenFGA 授权投影/立即撤权（T08/T09/T10）
- 不创建 Business Tenant（T04/#5）、不做成员邀请/激活（T05/#6）、不做成员管理 API（U07）
- 不做撤销 API（无任何 membership 写 API；旅程撤销为 driver 直连 DB fixture）
- 不做前端（F10/F11）、不改 web/
- 不做多设备/多会话选择同步语义（每用户单行，last-write-wins）
- 不做 Tenant 显示名/资料（tenants 表无 name 列，T04 域）

## Task 拆分（SDD）

- **Task 0（controller）**：本计划提交 `docs(plan): add issue 4 t03 tenant-context implementation plan`。
- **Task 1（impl-1，全新 subagent）**：单一垂直切片（迁移→应用包→适配器→transport→契约重生成→旅程），两段 RED 留痕，门禁 1-14，恰一实现提交 `feat(tenancy): deliver T03 tenant-context`。
- **双审**：规格符合性（新 subagent）+ 代码质量（新 subagent）并行；Critical/Important → 修复轮 ≤5 + scoped re-review。
- **终审**：最强可用模型全分支审查（矩阵逐条亲跑）。

## 测试与验收命令（门禁）

环境：`PATH=/Users/wuyongjun/.local/go1.26.7/bin:$PATH GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct GOSUMDB=off`；禁 `env -u`（用 `unset`）；全量测试 `-p 1`；TestPlatformAPIProcess 单独串行；临时 PG 端口用前核实空闲（impl=55470 起）。

1. `go test ./internal/application/tenancy -count=1`（无 DB，含 RED 留痕）
2. `go test ./internal/adapter/postgres -count=1`（TEST_DATABASE_URL 指临时 PG，含 RED 留痕；同库 `-count=3` repeat-safe）
3. `go test ./internal/transport/http -count=1`
4. `gofmt -l .` 空
5. `go vet ./...`
6. `go build ./...`
7. `go mod tidy -diff` 空（go.mod/go.sum 零改动预期）
8. `make generate-check`（go generate + internal/transport/http/api 干净 diff）
9. `make policy-check`
10. `go test ./... -count=1 -p 1`（无 DB 全量）
11. `go test ./cmd/platform-api -run TestPlatformAPIProcess -count=1`
12. T03 旅程（栈周期）：`T03_ACCEPTANCE_STACK=1 go test ./tests/acceptance -run TestT03TenantContext -count=1`（起栈命令含 audience 覆盖；重跑前 down -v）
13. T01+T02 回归（独立栈周期，各自 audience）
14. `make verify-foundation`（GENERATE POLICY UNIT CONTRACT INTEGRATION FRONTEND META 七相位）

## Global Constraints（沿 Issue 全局约束 + 仓库规约）

- Stage 1 规模：单中国大陆 Region、100 付费 Tenant、1,000 MAU、100 并发 AI 请求、50 控制面 RPS。
- Tenant 是安全/数据/计费硬边界；契约不引入 `Organization`；禁止 Product Domain Object 跨 Tenant 共享。
- Billing Customer 与 Tenant 严格 1:1；`actor_user_id` 永不替代 `tenant_id` 成为计费边界。
- Product Domain Object 与 Product 内部 Role 归 Product 所有；Platform 经 Adapter 契约集成。
- 秘密、原始 prompt、文档体、原始支付载荷、敏感个人信息不入日志/trace/指标/Audit/Usage/fixture/证据。
- Docker Compose 仅限开发/CI/集成/≤10 受控 beta Tenant。
- 聚焦单测/健康端点/静态审计/smoke 不替代具名验收接缝（旅程 12 为 T03 具名接缝）。
- 前置 #3（T02）已 CLOSED，依赖满足。

## Self-Review Record

- Spec 覆盖：AC1（显式选择+安全切换）→ 裁定 1/4/6/8；AC2（最高接缝自动测试含拒绝路径）→ 裁定 7/8；AC3（聚焦 vs 集成证据区分、闭环前附证据）→ 门禁 1-3 vs 12-14 + 证据 JSON；AC4（领域词汇/ADR 边界/证据无敏感内容）→ 裁定 4/6 词汇对齐 CONTEXT.md、全程脱敏。
- 占位扫描：无 deferred 标记；错误处理全部分类化（裁定 4/6）。
- 一致性：类型/端口/契约/旅程断言四层命名一致（tenant_id/kind/role/selected_at）。
- 尺度：单一垂直切片、一次 RED→GREEN、一个具名接缝门禁、一个可审查提交——无嵌套子 Issue 需要。
