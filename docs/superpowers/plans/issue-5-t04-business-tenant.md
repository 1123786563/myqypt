# Issue #5 [T04][P2] 创建 Business Tenant — Implementation Plan

- Issue: https://github.com/1123786563/myqypt/issues/5 （OPEN，0 评论；label `ready-for-agent`；唯一前置 #3 T02 已 CLOSED 2026-08-27T01:12:04Z，gh 亲验；本机 main=origin/main=aa747cc 亲验）
- Branch: `codex/issue-5-t04-business-tenant`（base = main@aa747cc）
- Worktree: `/Users/wuyongjun/trea/myqypt-worktrees/issue-5-t04-business-tenant`
- 源计划：Issue 正文与 `docs/superpowers/plans/2026-08-24-t04-business-tenant.md` 逐字一致（通用模板，落地按下述裁定修订）
- Spec 引用：`CONTEXT.md`（Business Tenant / Owner / Membership / Billing Customer 词条）、ADR-0004（Billing Customer 与 Tenant 严格 1:1）、ADR-0013（全局用户与 Tenant 生命周期分离）、ADR-0024（稳定身份键）

## Goal

已验证身份的 User 显式创建一个 Business Tenant 并成为其唯一 Owner：创建是原子的（tenant + 1:1 billing customer + 唯一 active owner membership 同事务），带 idempotency key 的重放收敛到同一 Tenant（首创建 201 / 重放 200），验证/拒绝全部发生在任何写入之前，T03 的 list/select 端点立即可见新 Tenant。

## 设计裁定（controller，2026-08-27）

1. **接缝裁定：公开 Portal API 进 OpenAPI 契约。** `api/openapi/platform.yaml` 新增一个操作：
   - `POST /api/v1/tenants`（operationId `createTenant`，security bearerAuth）——为认证用户创建 Business Tenant；
   - 请求头参数 `Idempotency-Key`（`in: header`，`required: true`，`minLength: 1`，`maxLength: 128`）——重试键是传输/重试关切，不进资源体（Stripe 式先例；资源体保持纯域字段）；
   - 请求体 `CreateTenantRequest{display_name: string, required, minLength 1, maxLength 100, additionalProperties false}`；
   - 201 响应 `CreatedTenant{tenant_id(uuid), kind: const "business", display_name, created_at(date-time), role: const "owner"}`——回显创建事实；重放响应 200 复用同 schema `CreatedTenant`（T01 identity callback 的 created=true→201 / 重放→200 先例，逐字沿用）；
   - 缺 `Idempotency-Key` 头 / 畸形体 → 既有 OpenAPI validator 400 `invalid_request`（免费提供）。
   经 `go generate ./...` 重生成 `internal/transport/http/api/server.gen.go`（generate-check 门禁覆盖）。`GET /api/v1/tenants` 的 `TenantSummary` **不改动**（display_name 的列表暴露归 F11/U 系列，非 T04 范围）。
2. **领域裁定：沿用 `internal/application/tenancy` 包（T03 计划裁定 4 预告）。** 新类型与端口方法：
   - `BusinessTenant{TenantID, DisplayName string; CreatedAt time.Time}`；
   - Repository 端口新增 `CreateBusinessTenant(ctx, verified identity.VerifiedIdentity, displayName, idempotencyKey string) (BusinessTenant, bool, error)`（bool=created，true 恰在本调用插入时）；
   - Service 新增 `CreateBusinessTenant(ctx, verified, displayName, idempotencyKey)`：写前分类拒绝 `ErrUserRequired`（verified 空）/ `ErrDisplayNameRequired`（trim 后空）/ `ErrIdempotencyKeyRequired`（空）——三者均零端口调用；通过后透传仓储；
   - 仓储分类错误 `ErrUserNotBound`（verified 从未绑定 → 无 platform user，不能成为 Owner）。
3. **持久化裁定：迁移 `000005_business_tenants.sql`。**
   - `ALTER TABLE tenants ADD COLUMN display_name text` + `CHECK (kind = 'personal' OR display_name IS NOT NULL)`（business 必有显示名，DB 级兜底；personal 保持 NULL）；
   - 新表 `business_tenant_creations(actor_user_id uuid NOT NULL REFERENCES platform_users(id), idempotency_key text NOT NULL, tenant_id uuid NOT NULL REFERENCES tenants(id), created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (actor_user_id, idempotency_key))`——重放映射的持久层；
   - Up/Down 对称（Down: DROP TABLE + DROP COLUMN）；`migrate_test.go` down-one 地板连锁更新（000005 撤销、000004 tenant_context_selections 存活断言新增）。
4. **仓储实现裁定（`internal/adapter/postgres/business_tenant_repository.go`，同包新文件）：** 单事务：`pg_advisory_xact_lock(hashtextextended(user_id || ':' || idempotency_key, 0))` 串行化同键并发（BindOrLoad 先例）→ `boundUserID` 解析绑定（无绑定 → `ErrUserNotBound`）→ 查 `business_tenant_creations(actor, key)`：命中 → 返回既有 (tenant, created=false)；未命中 → INSERT tenants(kind='business', display_name) + INSERT billing_customers（ADR-0004 1:1）+ INSERT memberships(role='owner', status 默认 active)（唯一 Owner 由 000003 的 `memberships_one_active_owner_per_tenant` 部分唯一索引兜底）+ INSERT business_tenant_creations → commit → (tenant, created=true)。任一步失败整体回滚零残留。**不限制每 User 的 business tenant 数**（000003 刻意只约束 personal；一个 User 可拥有多个业务实体，小企业多主体正当；防误建靠 idempotency key）。
5. **transport 裁定：`TenancyHandler` 新增 `CreateTenant` 严格方法**，复用 `authenticateTenantUser`（401/503 语义逐字沿 T03）。状态映射：`ErrDisplayNameRequired`/`ErrIdempotencyKeyRequired`/`ErrUserRequired` → 400 `invalid_request`；`ErrUserNotBound` → 404 `not_found`（沿 T03「不建存在性 oracle」裁定：未绑定与不存在不可区分）；created=true → 201、false → 200；validator 已挡缺头/畸形体，handler 内对 `request.Params.IdempotencyKey == nil` 仍显式 400（防御纵深）。`Dependencies` 不变（Tenancy 装配已含 Verifier+Repository）；main.go 零改动。
6. **测试裁定（RED→GREEN 两段留痕沿 T02/T03）：**
   - **聚焦无 DB（先红）**：tenancy Service 合约测试（in-memory fake）——三种写前拒绝零端口调用、成功路径透传 (tenant, created)、重放 created=false 透传、`ErrUserNotBound` 透传分类；
   - **聚焦带 DB（后红）**：postgres 仓储测试（临时 PG）——bundle 原子性（tenant kind=business + display_name 落库 + billing_customers 恰 1 行 + memberships 恰 1 行 owner/active + creations 恰 1 行）、同键重放同 tenant created=false、异键新 tenant、并发 16 goroutine 同键终态恰 1 tenant（advisory lock）、未绑定 ErrUserNotBound、personal 空名 vs business 空名（CHECK：personal 允许 NULL、business 拒绝空串——经服务层前已挡，DB CHECK 为兜底，测试直接 SQL 断言约束存在）、事务中途失败零残留（连一个会失败的注入点）；
   - **transport 测试**：httptest fake deps——201 形状（字段逐字 + created_at RFC3339）、200 重放同 tenant_id、400（空 display_name 经 fake repo 语义 / nil body / nil Params）、401（缺/篡改 Bearer）、404（ErrUserNotBound）、503（nil deps / ErrProviderUnavailable）；
   - **contract 回归**：#102 contract_test 既有断言面不受影响（路径定向，不枚举全路径——已核）；新端点响应形状受 embedded spec 校验（严格契约分歧检测免费覆盖）。
7. **验收旅程裁定：seam `lighthouse-business-tenant`。** 新文件 `tests/acceptance/scenarios/t04-business-tenant.yaml` + `t04_business_tenant_driver.go` + `t04_business_tenant_test.go`（`T04_ACCEPTANCE_STACK=1` 门控，skip 消息携带起栈命令——含 `PLATFORM_IDENTITY_OIDC_AUDIENCE=t04-acceptance` 覆盖——与 down -v 重置命令，沿 T03 先例）。旅程（复用 journey helpers：provisionCasdoor/ensureNamedUser/mintTokenFor/postCallback/openPlatformDB/countPlatformUsers）：
   - stale-state 预检（platform_users=0）；
   - 用户 A callback 首绑（201，获 personal tenant bundle）；
   - `POST /api/v1/tenants`（display_name="t04-accept-team"，Idempotency-Key: t04-key-a）→ **201**，响应 kind=business/role=owner/tenant_id 回显/display_name 回显/created_at RFC3339；
   - DB 断言：该 tenant 行 kind=business 且 display_name 匹配；billing_customers 恰 1 行；memberships 恰 1 行且 role=owner status=active（唯一 Owner）；business_tenant_creations 恰 1 行；
   - **同键重放** → **200** 同 tenant_id；DB tenants 总数不变（A 名下恰 2：1 personal + 1 business）；
   - **异键第二创建** → 201 新 tenant（多业务主体设计裁定 4 的实证）；
   - `GET /api/v1/tenants`（A）→ 恰 3（1 personal + 2 business，全 owner）；
   - **T03 集成**：`PUT /api/v1/tenant-context` 选第一个 business tenant → 200（创建即可用）；
   - **拒绝路径**：缺 Idempotency-Key 头 → 400；空 display_name → 400；无 Authorization → 401；篡改签名 → 401；
   - **未绑定身份**：用户 B mint token 但**不** callback → POST → 404（无存在性 oracle）；
   - 断言集 ~15 条（driver t04AssertionNames 与 YAML 逐名同序机械对照）。
8. **回归义务**：T01、T02、T03 旅程独立栈周期重跑（各自 audience）；`migrate_test.go` down-one 连锁更新；`identity_repository_test.go` TRUNCATE 清单 6→7 表（+business_tenant_creations，沿 repeat-safe 先例）；T03 既有测试语义零触碰。
9. **禁止事项（全局）**：不 push/merge/关闭 Issue/评论 GitHub；implementer 内不派生 subagent；不改 web/、deploy/（compose 零改动，audience 经 env 覆盖）；不动 T01-T03 既有测试断言语义；秘密/凭证不入日志与证据。

## Scope（范围）

- `api/openapi/platform.yaml`：+1 操作（POST /api/v1/tenants）+2 schema（CreateTenantRequest/CreatedTenant）+header 参数
- `internal/transport/http/api/server.gen.go`：go generate 重生成（豁免文件，policy-check 不扫）
- `internal/application/tenancy/tenancy.go`：+BusinessTenant 类型、+CreateBusinessTenant 端口方法、+错误
- `internal/application/tenancy/service.go`：+Service.CreateBusinessTenant（写前拒绝）
- `internal/application/tenancy/service_test.go`：+合约测试（无 DB）
- `internal/adapter/postgres/business_tenant_repository.go` + `business_tenant_repository_test.go`（新文件，同包）
- `internal/transport/http/tenancy.go`：+CreateTenant handler；`tenancy_test.go`：+httptest 用例
- `db/migrations/000005_business_tenants.sql`（embed 自动）+ `internal/adapter/postgres/migrate_test.go`（down-one 地板）
- `internal/adapter/postgres/identity_repository_test.go`：TRUNCATE 清单 6→7 表
- `tests/acceptance/`：+scenarios/t04-business-tenant.yaml、+t04_business_tenant_driver.go、+t04_business_tenant_test.go

## Non-goals（非目标）

- 不做成员邀请/激活（T05/#6）、Platform Role 管理（T06/#7）、Membership 审计（T07/#8）
- 不做 display_name 修改/删除 API（Tenant 资料管理后续域）
- 不改 `GET /api/v1/tenants` 响应形状（display_name 列表暴露归 F11/U 系列）
- 不做 OpenFGA 投影（T08+）、不签发 Platform Context（T22）
- 不做 Owner 转移/Tenant 删除/User Deactivation（ADR-0013 生命周期后续）
- 不做前端（F10/F11/U 系列）、不改 web/
- 不限制每 User business tenant 数量（设计裁定 4）

## Task 拆分（SDD）

- **Task 0（controller，已完成）**：本计划提交 `docs(plan): add issue 5 t04 business tenant implementation plan`。
- **Task 1（impl-1，全新 subagent）**：单一垂直切片（迁移→应用包→适配器→transport→契约重生成→旅程），两段 RED 留痕，门禁 1-14，恰一实现提交 `feat(tenancy): deliver T04 business-tenant`。
- **双审**：规格符合性（新 subagent）+ 代码质量（新 subagent）并行；Critical/Important → 修复轮 ≤5 + scoped re-review。
- **终审**：最强可用模型全分支审查（矩阵逐条亲跑）。

## 测试与验收命令（门禁）

环境：`PATH=/Users/wuyongjun/.local/go1.26.7/bin:$PATH GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct GOSUMDB=off`；禁 `env -u`（用 `unset`）；全量测试 `-p 1`；TestPlatformAPIProcess 单独串行；临时 PG 端口用前核实空闲（impl=55475 起，逐次 +1）。

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
12. T04 旅程（栈周期）：`T04_ACCEPTANCE_STACK=1 go test ./tests/acceptance -run TestT04BusinessTenant -count=1`（起栈命令含 audience 覆盖；重跑前 down -v）
13. T01+T02+T03 回归（独立栈周期，各自 audience）
14. `make verify-foundation`（GENERATE POLICY UNIT CONTRACT INTEGRATION FRONTEND META 七相位）

## Global Constraints（沿 Issue 全局约束 + 仓库规约）

- Stage 1 规模：单中国大陆 Region、100 付费 Tenant、1,000 MAU、100 并发 AI 请求、50 控制面 RPS。
- Tenant 是安全/数据/计费硬边界；契约不引入 `Organization`；禁止 Product Domain Object 跨 Tenant 共享。
- Billing Customer 与 Tenant 严格 1:1（ADR-0004；创建 business tenant 必须同事务建 billing customer）；`actor_user_id` 永不替代 `tenant_id` 成为计费边界。
- Product Domain Object 与 Product 内部 Role 归 Product 所有；Platform 经 Adapter 契约集成。
- 秘密、原始 prompt、文档体、原始支付载荷、敏感个人信息不入日志/trace/指标/Audit/Usage/fixture/证据（旅程 details 只携带状态码/行数/匹配布尔）。
- Docker Compose 仅限开发/CI/集成/≤10 受控 beta Tenant。
- 聚焦单测/健康端点/静态审计/smoke 不替代具名验收接缝（旅程 12 为 T04 具名接缝）。
- 前置 #3（T02）已 CLOSED，依赖满足。

## Self-Review Record

- Spec 覆盖：AC1（创建并唯一 Owner）→ 裁定 1/3/4/7（原子 bundle + owner 唯一索引 + DB 断言）；AC2（正常+拒绝/重试/幂等/失败路径最高接缝）→ 裁定 6/7（400/401/404/重放 200/并发/零残留）；AC3（聚焦 vs 集成证据区分）→ 门禁 1-3 vs 12-14 + 证据 JSON；AC4（领域词汇/ADR 边界/证据脱敏）→ 裁定 2/3 词汇对齐 CONTEXT.md 与 ADR-0004/0013、全程脱敏。
- 占位扫描：无 deferred 标记；错误处理全部分类化（裁定 2/5）。
- 一致性：类型/端口/契约/旅程断言四层命名一致（display_name/tenant_id/kind/role/created_at）。
- 尺度：单一垂直切片、一次 RED→GREEN、一个具名接缝门禁、一个可审查提交——无嵌套子 Issue 需要。
