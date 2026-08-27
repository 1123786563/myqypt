# Issue #3 [T02][P1] 自动创建 Personal Tenant — 实施计划

- Issue: https://github.com/1123786563/myqypt/issues/3 （OPEN，0 评论，label ready-for-agent）
- 源计划（Issue 内嵌，仓库亦有 `docs/superpowers/plans/2026-08-24-t02-personal-tenant.md`）：本文是对它的可执行化裁定版；与仓库现实冲突处以本文件为准。
- 前置：#2（T01）CLOSED（合并 9960596）——依赖满足。
- 分支：`codex/issue-3-t02-personal-tenant`（base `main@9960596`）。
- Worktree：`/Users/wuyongjun/trea/myqypt-worktrees/issue-3-t02-personal-tenant`。
- 执行协议：subagent-driven-development —— 每个实施任务由全新 implementer subagent 完成（禁止其再派生 subagent），每任务经独立规格符合性审查 + 独立代码质量审查，Critical/Important 发现进入最多 5 轮修复与 scoped re-review，全部任务后由最强可用模型做全分支审查。外部副作用（push/merge/关闭 Issue/评论 GitHub）一律保留给用户明确批准。
- 技能可用性：本会话技能目录无 `superpowers:subagent-driven-development`（skill 工具报 invalid skill name，与 #101–#107/#2 全部先例一致）；按本计划所载 SDD 规程执行。

## 目标（Goal）

在 T01 交付的真实注册事件（`POST /internal/v1/identity/callback` 首次绑定创建新 Platform User）上，原子地交付 T02 不变式：**新 User 获得唯一 Personal Tenant、Billing Customer 和 Owner Membership**，并以黑盒验收旅程在真实 Compose 栈（platform-api + postgres + casdoor:3.159.0）上证明：

1. 正常路径：新身份首次回调 → 201 + `{"user_id":"<uuid>"}`（T01 契约不变），且 DB 中恰有 1 行 `tenants(kind=personal, owner=该 user)`、1 行 `billing_customers(tenant↔1:1)`、1 行 `memberships(role=owner, status=active, user, tenant)`；
2. 幂等路径：同一 token 重放 → 200 + 相同 user_id，三表行数仍各恰 1；
3. 拒绝路径：缺失/篡改凭据 → 401，全部业务表 0 行增量；
4. 原子性：provisioning 任一环节失败 → 整个事务回滚（user/binding/tenant 全部 0 行）；
5. 证据：`platformtest` 报告 JSON（稳定测试标识与依赖版本，绝无 token/凭据/个人档案值），落 `artifacts/evidence/t02-personal-tenant/`。

## 裁定（源计划 vs 仓库现实，实施以本节为准）

1. **触发裁定——provisioning 原子并入首次绑定事务，不新增命令面**：源计划的 `PersonalTenantCommand{ActorUserID, UserID, IdempotencyKey}` + 独立 `PersonalTenantService.Execute` 命令面在真实系统中没有调用方：CONTEXT.md 定义 Personal Tenant「created for and owned by an individual Billing Customer **during self-registration**」，而本平台唯一的 self-registration 事件就是 T01 的首次 identity 绑定（回调 201 创建新 User）。裁定：在 `IdentityRepository.BindOrLoad` 既有事务（advisory xact lock 之后、insert user/binding 同一事务内）追加 tenants/billing_customers/memberships 三行插入；不新增端点、不新增命令/IdempotencyKey 表面——幂等键即自然键 `(identity_provider, subject)`（绑定）与 DB 唯一约束（租户行），重放走既有 load 路径天然不重复。回调响应体保持 `{"user_id":"<uuid>"}` 逐字节不变（T01 验收钉死了该形状，#3 不得破坏 #2 的已验收契约）。
2. **源计划路径不适用**：`internal/identity/personal-tenant/` 不符合仓库分层（application/adapter/transport）；且裁定 1 已消除独立 service 的存在理由。裁定：本切片的应用层逻辑为零新增输入面（无新校验入口），域不变式由迁移 CHECK/唯一索引承载（裁定 3），focused 测试落在 adapter 集成测试（裁定 5）；模板的 `EvidenceSink` 由 #100 `platformtest` 证据管道替代（Report JSON 已内建脱敏）。`internal/application/identity.Repository` 端口签名**不变**，仅更新其文档注释以声明更宽的原子单元（create 路径同时 provision personal tenant）；不新建 `internal/application/tenancy` 包（本切片无其承载的策略；T03 Tenant Context 需要读端口时再建，属其范围）。
3. **Schema 裁定（迁移 000003，沿 #101 000002 先例：goose Up/Down 成对、无级联 FK、无可变键列）**：
   - `tenants(id uuid PK, owner_user_id uuid NOT NULL REFERENCES platform_users(id), kind text NOT NULL CHECK (kind IN ('personal','business')), created_at timestamptz NOT NULL DEFAULT now())`；部分唯一索引 `UNIQUE (owner_user_id) WHERE kind='personal'` ——「每 User 唯一 Personal Tenant」由 DB 强制。
   - `billing_customers(id uuid PK, tenant_id uuid NOT NULL UNIQUE REFERENCES tenants(id), created_at ...)` —— `tenant_id UNIQUE` + 单 FK 即双向 1:1（ADR-0004：每 Tenant 恰一 Billing Customer、每 Billing Customer 恰一 Tenant；`actor_user_id` 永不替代 `tenant_id` 作为计费边界）。
   - `memberships(id uuid PK, tenant_id uuid NOT NULL REFERENCES tenants(id), user_id uuid NOT NULL REFERENCES platform_users(id), role text NOT NULL CHECK (role IN ('owner','admin','billing_member','member')), status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')), created_at ..., UNIQUE (tenant_id, user_id))` —— 每 (tenant,user) 一行 Membership、单 Platform role（CONTEXT.md Membership 定义）；部分唯一索引 `UNIQUE (tenant_id) WHERE role='owner' AND status='active'` —— 每 Tenant 恰一在位 Owner。status 取值仅 CONTEXT.md 既有词汇 active/revoked（ADR-0013 生命周期分离的落点），不引入其他状态。
   - kind 含 'business' 是域词汇完整而非投机：CHECK 枚举与 CONTEXT.md 两类 Tenant 一一对应，T04 建业务租户时无需破坏性迁移。
4. **Scenario schema 适配（沿 T01 aggregator 裁定 1）**：#100 `platformtest.Scenario` 契约是 `{id, seam, timeout, inputs, assertions, metadata}` 且 `KnownFields(true)`；源计划字面 YAML（`scope/normal/guard/replay` 顶层键）必被拒为 `decode_error`。语义 1:1 映射：journey 参数进 `inputs`，验收断言进 `assertions[].name/want`。新 seam `lighthouse-personal-tenant`（T01 已注册 `lighthouse-black-box`，`platformtest.Register` 对重复 seam panic；两旅程各自 driver）。
5. **focused 测试集**：源计划写 `go test ./internal/identity/personal-tenant`，路径不存在。裁定 focused = `./internal/adapter/postgres/...`（需 `TEST_DATABASE_URL` 临时库；新增 provisioning 断言 + 原子性故障注入）+ 回归 `./internal/application/identity/... ./internal/adapter/oidc/... ./internal/transport/http/...`（无 DB，应零改动全绿）。
6. **原子性故障注入测试（AC2 failure path 的机械化）**：集成测试中以 `CREATE FUNCTION ... RAISE EXCEPTION` + `CREATE TRIGGER BEFORE INSERT ON memberships` 注入 provisioning 中途失败 → `BindOrLoad` 必须报错且该身份在 platform_users/identity_bindings/tenants/billing_customers/memberships 五表全部 0 行（整事务回滚）→ `DROP TRIGGER/FUNCTION` 复原。选触发器而非 DROP TABLE：无 schema 损失、注入点精确、复原干净。
7. **迁移测试连锁更新（沿 #101 Ruling #1 同型事实）**：追加 000003 后，`TestMigrationRoundTrip` 的 down-one 语义必然回滚 000003 而非 000002——`assertIdentityTablesGone` 及其注释需更新为：down-one 后 tenants/billing_customers/memberships 0 列、identity 两表存活；`TestHealthCheckerTracksMigrationState` 用 floor 循环（`migrateDownToFloor`），对新迁移天然免疫，预期零改动。
8. **拒绝路径语义（沿 T01 aggregator 裁定 7）**：源计划 `remove_required_scope_or_inject_dependency_failure` → 真实系统 = 缺失 Authorization、篡改签名 token → 均 401、五表 0 行增量（以行数 delta 断言）。
9. **RED 先行**：两段 RED 证据——(a) 仓储 RED：先写 provisioning 集成测试（表不存在 → SQL 错误红）+ 迁移，实现前跑一次留痕；(b) 旅程 RED：先落 scenario + 测试文件（driver 未注册 → unsupported_seam 红）留痕，再实现 driver。
10. **栈生命周期（沿 T01 aggregator 裁定 10 修订版）**：默认（未设 `T02_ACCEPTANCE_STACK=1`）skip 并打印精确起栈/重置命令；旅程证明首次绑定（201），须干净库——预检五表全 0 行，否则 fail-closed `driver_error` 并给出 `down -v` 重置命令（T01 R1 修复轮先例）。集成命令形态：六必填 env + `docker compose up -d --wait && T02_ACCEPTANCE_STACK=1 go test ./tests/acceptance -run TestT02PersonalTenant -count=1 -v`。
11. **T01 回归义务**：本切片改写了 T01 已验收的 create 路径——集成阶段必须在独立栈周期重跑 `TestT01IdentityBinding`（先 reset 再 T01，再 reset 再 T02，或反之；两个周期各自 `down -v`）。
12. **凭证卫生（沿 T01 aggregator 裁定 9 修订版）**：一次性验收栈 fixture 凭据（镜像公开默认管理口令、测试自造 client secret/用户密码/DB 密码）可作 driver 常量默认值随仓库提交，条件：全部可 `T02_*` 环境变量覆盖、仅分钟级临时栈内有效、绝非真实系统凭据、任何情况下不进证据/日志/断言 details。断言 details 只写状态码/行数/布尔事实。Casdoor 供给参数沿用 T01 driver 既有 helper（同包复用 `journey` 结构与方法，换 t02- 前缀标识：org `t02-accept-org`、app `t02-acceptance-app`、client `t02-acceptance`、user `t02-accept`），保证 T01/T02 旅程互不污染。

## 范围（Scope）

- 新增 `db/migrations/000003_personal_tenants.sql`（goose Up/Down）。
- 修改 `internal/adapter/postgres/identity_repository.go`（create 路径追加三表插入；load 路径不变）。
- 修改 `internal/application/identity/identity.go`（仅 Repository 端口文档注释，签名不变）。
- 修改 `internal/adapter/postgres/identity_repository_test.go`（新增 provisioning/幂等/并发/原子性故障注入测试）。
- 修改 `internal/adapter/postgres/migrate_test.go`（round-trip down-one 期望连锁更新）。
- 新增 `tests/acceptance/scenarios/t02-personal-tenant.yaml`、`tests/acceptance/t02_personal_tenant_driver.go`、`tests/acceptance/t02_personal_tenant_test.go`。
- 上述文件的测试与证据。

## 非目标（Non-goals）

- 不改回调响应体/状态码语义（T01 已验收契约）；不改 `internal/transport/http/` 与 `cmd/`（装配零变化——compose 已在 command 链跑 `migrate up`，迁移经 `//go:embed *.sql` 自动收录）。
- 不改 `tests/platformtest/`（#100 交付物）与 T01 交付文件（`t01_*` 场景/测试/driver 既有断言；T01 driver helper 的**复用**是读不是改）。
- 不实现 Business Tenant 创建、Tenant Context、成员邀请、OpenFGA 投影（T03–T09 范围）；不新建 `internal/application/tenancy` 包。
- 不动 `web/`、`api/openapi/`、`deploy/compose/compose.yaml`、系列计划文件。
- 不引入新 Go 依赖；不清理 #101 遗留 Minor/Nit（M-Q1 提交错误前缀、M-Q2 孤儿断言——DEFER 项不越权代清）。
- 不做 push/merge/关闭/评论 GitHub。

## 任务拆分

- **Task 0（controller，已完成）**：本计划落盘提交；SDD ledger 建立；worktree 就绪。
- **Task 1（单个 implementer subagent，一个垂直切片，恰一提交 `feat(tenancy): deliver T02 personal-tenant`）**，内部步骤有序：
  1. RED-仓储：新增 provisioning 集成测试（临时 PG `TEST_DATABASE_URL`，表尚缺 → 红）留痕；
  2. 迁移 000003（裁定 3）+ `migrate_test.go` 连锁更新（裁定 7）→ 仓储 RED 转绿验证迁移本身；
  3. `identity_repository.go` create 路径三表插入 + 端口文档注释（裁定 1/2）→ focused 全绿（含原子性故障注入裁定 6、并发/跨身份/幂等）；
  4. RED-旅程：scenario YAML + 测试文件（unsupported_seam 红）留痕；
  5. driver 实现（复用 T01 helper，新 seam）→ 无栈全量仍绿（skip 带命令）；
  6. 集成：起栈跑 T02 旅程转绿（证据 JSON）+ 热栈负例（driver_error fail-closed）+ `down -v` + 独立栈周期重跑 T01 回归（裁定 11）；
  7. 全门禁（下节 1–10）+ 单提交。
- **Final review（最强可用模型，全新独立上下文）**：全分支 diff 审查（裁定 1-12 逐条 / 跨提交接缝 / 质量轴）+ 验收矩阵 12 条逐条亲跑复验。

## 测试与验收命令（全部通过才算本地完成）

按序（环境变量见下节）：

1. `gofmt -l .` 输出为空；
2. `go vet ./...`；
3. `go build ./...`；
4. `go mod tidy -diff` 无差异；
5. `make generate-check`；
6. `make policy-check`；
7. focused 无 DB：`go test ./internal/application/identity/... ./internal/adapter/oidc/... ./internal/transport/http/... -count=1`（零改动回归全绿）；
8. focused 带 DB（临时 PG 容器）：`go test ./internal/adapter/postgres/... -count=1`（`TEST_DATABASE_URL`；含新增 provisioning/原子性/round-trip 连锁）+ `-race` 聚焦本包；
9. 全量无 DB（串行，规避 R7）：`go test ./... -count=1 -p 1`（无栈时 acceptance 包默认 skip，须打印精确起栈命令）；
10. `TestPlatformAPIProcess` 串行单跑 PASS（R7 前科，禁并行负载下跑）；
11. 集成（两个独立栈周期，各自 `down -v` + 容器/卷/网络零残留核验）：
    a. **T02 旅程**：六必填 env `docker compose -f deploy/compose/compose.yaml up -d --wait && T02_ACCEPTANCE_STACK=1 go test ./tests/acceptance -run TestT02PersonalTenant -count=1 -v` → 证据 JSON 落 `artifacts/evidence/t02-personal-tenant/`；随后热栈不重置复跑一次 → 必须 `driver_error` fail-closed（预检拦下，0 断言、零副作用）；
    b. **T01 回归**：reset 后 `T01_ACCEPTANCE_STACK=1 go test ./tests/acceptance -run TestT01IdentityBinding -count=1 -v` → 绿；
12. `make verify-foundation`（七相位；FRONTEND 前确保 worktree `web/node_modules` 已装；INTEGRATION 需 `TEST_DATABASE_URL`）。

## 全局约束

- 禁止 push/merge/close/评论 GitHub；禁止派生 subagent；禁止改动计划外文件（尤其 `.superpowers/`、`web/`、`api/openapi/`、`tests/platformtest/`、`tests/acceptance/` 既有 T01 文件、`internal/transport/http/`、`internal/adapter/oidc/`、`cmd/`、`deploy/`、系列计划文件）。
- 证据与提交绝不含 token、密码、个人档案值；稳定测试标识（t02-accept 等）+ 依赖版本可以含。
- 领域词汇沿 CONTEXT.md / ADR-0004 / 0013 / 0024：Tenant 是硬边界；Billing Customer↔Tenant 1:1；`actor_user_id` 不替代 `tenant_id`；Membership 单 role；owner 每 tenant 唯一。
- 失败必须先诊断修复或如实上报（含精确命令/错误/已完成验证），不得伪报通过；环境阻塞必须给出准确命令与错误。
- 测试后清理：两栈周期 `docker compose down -v`，容器/卷/网络三重零残留；临时 PG 用毕 `docker rm -f -v`。

## 环境事实（供所有 subagent）

- **Go 工具链**：`/Users/wuyongjun/.local/go1.26.7/bin/go`（1.26.7，亲证可构建本仓库）；`PATH=/Users/wuyongjun/.local/go1.26.7/bin:$PATH GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct GOSUMDB=off`。勿用 `/opt/homebrew/bin/go`（brew 1.26.3，假绿前科）。
- **禁用 `env -u VAR cmd`**（`~/.local/bin/env` 损坏会静默 no-op）；用 `unset VAR` + 裸命令。
- Docker 29.4.0；镜像已就绪：`golang:1.26.7`、`casbin/casdoor:3.159.0`、`postgres:17`（本地已拉取，无需再拉）。
- 端口：8080/8000/5432 为验收栈；临时 PG 用 55xxx 段并先核空闲（T02 集成/终审分配：impl 临时 PG=55470、规格审=55471、质量审=55472、终审=55473，用前核实）；3000/3030/6379/9000-9001/9100-9101/15432/50051 为 WeKnora 占用勿触。
- R7 前科：`TestPlatformAPIProcess` 启动预算 5s 对负载敏感——全量测试一律 `-p 1` 串行；该测试单独串行跑。
- pnpm 11.7.0 / node v26.7.0；worktree 的 `web/node_modules` 需自行 `pnpm --dir web install`（gitignored）。
- 既有 compose 六必填 env 形态见 T01 driver 常量 `stackStartupCommand`（`deploy/compose` 下 `PLATFORM_POSTGRES_*` + `CASDOOR_POSTGRES_*` 三对）。
