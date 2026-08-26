# Issue #103 [F03][Foundation] PostgreSQL Migration 与 Readiness 实施计划

- **Issue:** https://github.com/1123786563/myqypt/issues/103 （[F03][Foundation] PostgreSQL Migration 与 Readiness，OPEN，无评论）
- **系列计划（事实源，不修改）：** `docs/superpowers/plans/2026-08-24-f03-postgres-migrations-readiness.md`（与 issue 正文 Implementation plan 段逐字节一致，已核对）
- **前置依赖：** F01 = #100（CLOSED，交付并复验）→ 依赖满足。F02 = #102（CLOSED）为当前 main（`94ec337`）上的既有事实。
- **分支/Worktree:** `codex/issue-103-f03-postgres-migrations-readiness`，基于 `main@94ec337`，worktree `/Users/wuyongjun/trea/myqypt-worktrees/issue-103-f03-postgres-migrations-readiness`
- **执行协议:** `superpowers:subagent-driven-development` —— 每任务独立 implementer subagent（禁止其再派生 subagent）→ 规格符合性审查 → 代码质量审查 → Critical/Important 修复（≤5 轮 + scoped re-review）→ 全部任务后最强可用模型全分支审查。台账：主仓库 `.superpowers/sdd/issue-103-f03-postgres-migrations-readiness/progress.md`。

## 范围（In Scope）

1. **显式追加式 SQL migration**：`db/migrations/000001_platform_baseline.sql`（goose Up/Down 注解），Up 创建 `schema_health(id boolean PRIMARY KEY, applied_at timestamptz NOT NULL)`，Down 删除该表；`db/migrations` 以 Go embed 包暴露 FS。
2. **pgx 连接池**：`internal/adapter/postgres/pool.go` 提供 `Open(context.Context, string) (*pgxpool.Pool, error)` —— `pgxpool.ParseConfig` 校验 DSN、设置连接生命周期/空闲/健康检查周期、惰性建池（不主动 Ping）。DSN 形状错误返回**不含 URL 本体**的错误。
3. **迁移执行**：`internal/adapter/postgres/migrate.go` 提供 `Migrate(context.Context, *sql.DB, fs.FS) error`（Up，幂等）与 `MigrateDownOne(context.Context, *sql.DB, fs.FS) error`（回退一个版本）。goose 经 embedded FS 执行。
4. **CLI**：`platform-api migrate up` 与 `platform-api migrate down-one` Cobra 子命令；要求 `DATABASE_URL`，缺失时报错并返回给进程入口（exit 1）；执行路径先以 5 秒超时 Ping 验证连接（fail fast、错误不泄露 URL/密码）。
5. **就绪检查分离**：`internal/application/readiness/service.go`（`Checker` port、`Service{Checks map[string]Checker; Timeout time.Duration}`、`Result{Ready bool; Checks map[string]string}`、确定性排序、每检查独立超时）。
6. **数据库检查器**：`internal/adapter/postgres` 提供 Ping + `schema_health` 迁移标记查询的 Checker（迁移未完成即失败）；以及 `DATABASE_URL` 未配置时的 fail-closed 检查器。
7. **`/readyz` 端点**：`internal/transport/http/readiness.go`，经 `httptransport.Dependencies` 注入；全 ok → 200，任一失败/超时/未配置 → 503；响应体仅含检查名与状态（`{"checks":{"database":"ok"}}`），不含错误文本、DSN、主机名。`/livez` 行为与字节保持不变。
8. **compose 冒烟栈**：新建 `deploy/compose.yaml`（postgres:18 + platform-api migrate-then-serve，健康检查依赖），端口避开本机占用（postgres `15532`、API `18080`）。
9. **测试**：见下文矩阵；集成测试经 `TEST_DATABASE_URL` 驱动真实 PostgreSQL 18 容器。

## 非目标（Out of Scope）

- 不引入 GORM、AutoMigrate、sqlc 生成、多数据库 Driver、任何未使用的数据库依赖（验收标准明令禁止；go.mod 新增仅限 pgx v5.10.0 + goose v3.27.3 及其传递依赖）。
- 不做 Tenant 切库、连接的全局单例、运行期自动迁移（serve 启动不执行迁移）。
- 不修改 OpenAPI 契约（`api/openapi/platform.yaml`）：`/readyz` 与 `/livez` 同属运营端点，不进公共产品契约（系列计划 File Structure 未列契约改动）。
- 不修改 `deploy/compose/compose.yaml`（现有 Casdoor 开发栈）与 `docs/superpowers/plans/2026-08-24-f03-*.md`（sync 管辖的系列计划）。
- 不做 F04 范围的 HTTP 安全头/可观测中间件、request-ID 格式校验（已登记为 F04 需求）。
- 不 push、不 merge、不关闭/评论 issue（外部副作用留给用户）。

## 设计裁定（对系列计划的必要澄清与偏差记录）

1. **系列计划 Tech Stack 写 "PostgreSQL 18"，现有 `deploy/compose/compose.yaml` 用 postgres:17** —— 裁定：F03 交付物（集成测试与冒烟栈）按系列计划使用 **postgres:18**；现有 17 栈属 Casdoor 开发环境，不改。
2. **系列计划引用 "ADR-0003" 与 PostgreSQL 无关**（ADR-0003 是 Product Catalog 决策）—— 裁定：以抽取设计 §6.1/§6.3/§12.2 与 issue 正文为准，不追改系列计划文件。
3. **serve 不因数据库不可用而退出**（关键裁定）：`TestPlatformAPIProcess`（F01 既有测试）在无 `DATABASE_URL` 环境下启动 serve 并要求 `/livez` 可达——serve 必须在数据库缺失/不可用时保持存活且 `/readyz` 503（fail closed），数据库恢复后 `/readyz` 转为 200（readiness transition，无需重启进程）。因此：
   - `Open` 建池**惰性连接**（`pgxpool.NewWithConfig` 不主动连接），serve 路径永不因 Ping 失败退出；
   - 系列计划 Task 1 的 "ping with a five-second context" 落在 **migrate 命令路径**（`sql.DB.PingContext` 5 秒超时，fail fast）；
   - `DATABASE_URL` 缺失：serve 正常启动，`database` 检查器报 `unconfigured`（503）；`DATABASE_URL` 形状非法：`ParseConfig` 报错，serve 启动失败（配置错误理应 fail fast）。
4. **"migration 未完成时 /readyz 失败"的实现**：数据库检查器 = Ping + 对 `schema_health` 的 SELECT（表缺失即 42P01 → failed）。仅靠 Ping 无法区分"已迁移/未迁移"，此为实现验收标准 (2) 的必要扩展。
5. **目录名**：系列计划 `internal/adapter/postgres`（设计文档 §11 的 `internal/adapters/postgres` 为旧稿）；按系列计划执行。
6. **goose API 形态**：优先无全局状态的 provider API（若对 v3.27.3 编译通过）；若使用经典 `goose.SetBaseFS` 全局 API，adapter 测试必须串行并在用后清理全局状态。由 implementer 依实际 API 裁定并在报告记录。
7. **响应体形状**：`/readyz` 返回 `{"checks":{"<name>":"<state>"}}`（map 序列化自动按 key 排序）；状态值 `ok` / `failed`；HTTP 状态码承载 Ready 语义。`Result.Ready` 驱动状态码而不进响应体。
8. **CLI 签名演进**：`cli.NewRoot(version string, serve ServeFunc, migrate MigrateFuncs)`（`MigrateFuncs{Up, DownOne func(context.Context) error}`）；既有 `root_test.go` 语义保持（version 不触发 serve/migrate）。

## 环境事实（所有 subagent 必须遵守）

- Go 工具链：`PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local`（go1.26.7，darwin/arm64；`/opt/homebrew/bin/go` 已知损坏勿用）。
- 模块下载：`GOPROXY=https://goproxy.cn,direct GOSUMDB=off`（默认 GOPROXY=off 会失败）。pgx v5.10.0 / goose v3.27.3 经此代理已验证可解析。
- Docker 29.4.0 + Compose v5.1.2 可用；`postgres:18`、`golang:1.26.7` 镜像已拉取。本机已占用端口：3000、6379、9000-9001、9100-9101、15432、50051、3030 —— 测试/冒烟一律使用 **55432**（临时测试库）、**15532/18080**（compose 冒烟）。
- 工作目录：worktree `/Users/wuyongjun/trea/myqypt-worktrees/issue-103-f03-postgres-migrations-readiness`；所有 gh 命令（如需只读）带 `--repo 1123786563/myqypt`。
- 禁止：push/merge/关闭/评论 GitHub、派生 subagent、改动计划外文件（尤其 `.superpowers/`、`web/`、`deploy/compose/`、`api/openapi/`、系列计划文件）。

## 任务拆分

### Task 0（controller）：本计划

- 提交本文件：`docs(plan): add issue 103 f03 implementation plan`。

### Task 1：显式迁移与连接池边界（impl-1，全新 subagent）

TDD 顺序（先红后绿，逐项执行）：

1. **RED**：写 `internal/adapter/postgres/migrate_test.go`：
   - `TestMigrationRoundTrip`：从 `TEST_DATABASE_URL` 取库；变量缺失时 `t.Skip("TEST_DATABASE_URL not set; skipping postgres integration test")`。断言 `Migrate`（Up）后 `schema_health` 存在且列结构为 `id boolean PRIMARY KEY, applied_at timestamptz NOT NULL`（查 `information_schema.columns`）；重复执行 `Migrate` 再次成功（幂等/安全重复）；`MigrateDownOne` 后表被删除。
   - `TestMigrateRequiresConnection`：指向未监听端口的 DSN（如 `postgres://u:pw@127.0.0.1:1/db?sslmode=disable`）在 migrate 命令路径（Ping 5s 超时）失败，且错误信息不含 `pw`（秘密不泄露）。
   - 运行：`TEST_DATABASE_URL=postgres://postgres:pw-...@127.0.0.1:55432/platform?sslmode=disable go test ./internal/adapter/postgres -run 'TestMigration|TestMigrateRequires' -count=1` —— 因包不存在而编译失败（正确红色）。
2. **GREEN**：实现使其通过：
   - `db/migrations/000001_platform_baseline.sql`（goose Up/Down）；`db/migrations/migrations.go`（`package migrations`，`//go:embed *.sql`，导出 `FS`）。
   - `internal/adapter/postgres/pool.go`：`Open(ctx, databaseURL) (*pgxpool.Pool, error)`（ParseConfig；`MaxConnLifetime=30m`、`MaxConnIdleTime=5m`、`HealthCheckPeriod=1m`；惰性建池；错误不回显 URL）。
   - `internal/adapter/postgres/migrate.go`：`Migrate` / `MigrateDownOne`（goose + fs.FS，见设计裁定 6）。
   - `internal/platform/cli/root.go`：`migrate up` / `migrate down-one` 子命令（`DATABASE_URL` 必需，缺失返回 error 给入口）；`root_test.go` 补：无 `DATABASE_URL` 时 `migrate up` 返回错误且不触发 serve；`version` 语义不变。
   - `cmd/platform-api/main.go`：构造 migrate 函数（`sql.Open("pgx", url)` + `PingContext` 5 秒 + 调 adapter）。
   - `go.mod/go.sum`：新增 pgx v5.10.0、goose v3.27.3（经 GOPROXY）。
3. 门禁：`go test ./internal/adapter/postgres ./internal/platform/cli -count=1`（含 TEST_DATABASE_URL 与不含各跑一遍，后者验证 skip 消息）、`go vet ./internal/adapter/postgres/... ./internal/platform/cli/...`、`gofmt -l .` 为空、`go build ./...`。
4. 提交：`feat(platform): add postgres migrations`。

### Task 2：就绪检查分离与 Compose 冒烟（impl-2，全新 subagent）

TDD 顺序：

1. **RED**：写 `internal/application/readiness/service_test.go` 与 `internal/transport/http/readiness_test.go`：
   - service 表驱动：全部健康 → Ready；一个失败 → !Ready 且对应 state=failed；阻塞超过 `Timeout` 的检查器 → failed（确定性：channel 控制的慢检查器）；`/livez` 在同一失败条件下仍 200（transport 层）。
   - transport：200 时体恰为 `{"checks":{"database":"ok"}}`、Content-Type `application/json`；失败 503 体 `{"checks":{"database":"failed"}}`；`Dependencies` 零值（无 Readiness）→ 503 `{"checks":{}}`（fail closed）；响应体不含错误文本。
   - 运行：`go test ./internal/application/readiness ./internal/transport/http -run 'Ready|Live' -count=1` —— 确认红。
2. **GREEN**：
   - `internal/application/readiness/service.go`（接口与语义见范围 5；检查名排序；每检查 `context.WithTimeout`；失败与超时一律 `failed`，错误文本不进入 Result）。
   - `internal/adapter/postgres/health.go`：Ping + `schema_health` 查询的 Checker；`UnconfiguredChecker`（固定错误）。集成测试（TEST_DATABASE_URL）：迁移前 Check 失败 → 迁移后 ok → down-one 后再失败（readiness transition）。
   - `internal/transport/http/readiness.go` + `router.go` 注册 `/readyz`（与 `/livez` 同级，不进契约组）；`Dependencies.Readiness *readiness.Service`。
   - `cmd/platform-api/main.go` serve：按设计裁定 3 装配（惰性池；未配置→Unconfigured；退出时关池）。
   - `deploy/compose.yaml`：postgres:18（healthcheck、`15532:5432`）+ platform-api（golang:1.26.7、`migrate up && serve`、`DATABASE_URL`、`18080:8080`、`GOPROXY=https://goproxy.cn,direct`、`GOTOOLCHAIN=local`、模块缓存卷）。
3. 门禁（受影响面 + 全量）：
   - `go test ./internal/application/readiness ./internal/transport/http ./internal/adapter/postgres -count=1`（含/不含 TEST_DATABASE_URL）；
   - `go test ./... -count=1`（含 TEST_DATABASE_URL，全部包 ok）；
   - **Compose 冒烟**（记录进 `docs/evidence/2026-08-25-issue-103-compose-smoke.md`，README 约定）：`docker compose -f deploy/compose.yaml up -d --wait` → curl `/livez`=200、`/readyz`=200 → `docker compose stop postgres` → `/readyz`=503（过渡证据）→ `docker compose start postgres` → `/readyz`=200 → `docker compose down -v`。
4. 提交：`feat(platform): add dependency readiness`。

## 测试与验收命令（全分支最终矩阵）

在 worktree 根、Go 1.26.7 工具链环境下逐条执行并留痕（exit code + 关键输出）：

1. `gofmt -l .` → 空。
2. `go vet ./...` → 0。
3. `go build ./...` → 0。
4. `go test ./... -count=1`（无 TEST_DATABASE_URL）→ 全 ok（集成测试以显式消息 skip）。
5. `TEST_DATABASE_URL=... go test ./... -count=1`（postgres:18 容器 alive）→ 全 ok，集成用例实跑。
6. `TEST_DATABASE_URL=... go test -race ./...` → 0。
7. `go test ./cmd/platform-api -run TestPlatformAPIProcess -count=1` → PASS（serve 在无 DATABASE_URL 下存活、优雅退出）。
8. Compose 冒烟（范围见 Task 2 门禁 3；证据文件入库）。
9. 边界 grep：`grep -riE 'gorm|automigrate' --include='*.go' --include='go.mod' .` → 无匹配；`go.mod` 数据库相关直接依赖仅 pgx/goose；无 `database/sql` 之外的驱动注册（`sql.Open("pgx", …)` 的驱动来自 pgx/stdlib，非额外 Driver）。
10. 秘密不泄露：`DATABASE_URL=postgres://u:SECRET@127.0.0.1:1/db ./platform-api migrate up` 的 stderr/stdout 不含 `SECRET`；`/readyz` 失败体不含 DSN/主机/错误文本。
11. `go mod tidy -diff` → 空；`git diff --check` → 干净；提交历史恰为 Task 0–2 的 3 个提交。
12. 未受影响面回归（沿 F02 先例）：`pnpm --dir web install --frozen-lockfile && pnpm --dir web run typecheck && pnpm --dir web run lint && pnpm --dir web run format:check && pnpm --dir web run test && pnpm --dir web run build` → 全绿（F03 不触 web/，防回归）。

验收标准对照（issue 五条）：(1) Task 1 幂等迁移 + CLI；(2) Task 2 readyz 200/503 + 迁移未完成失败 + compose 冒烟过渡；(3) 裁定 3/4 + 测试 10；(4) 集成测试矩阵（首次迁移/重复/连接失败/转换）；(5) 边界 grep + go.mod 审计。

## 全局约束

- PostgreSQL 是业务事实源；migration 只追加、可回滚（Down 与 Up 成对）；不得有 N+1 版本跳跃或手工 SQL 入口绕过 goose 版本表。
- `/livez` 不访问数据库且字节不变（`{"status":"alive"}`）；`/readyz` 对超时、连接失败、迁移未完成、未配置一律 fail closed（503）。
- 数据库凭据只来自 `DATABASE_URL` 环境变量；任何错误路径、日志、测试输出不得回显 URL/密码（以 marker 秘密测试）。
- 每任务 = 全新 implementer subagent；implementer 不得派生 subagent；任务必须依次通过规格符合性审查与代码质量审查（独立 subagent 各一）；Critical/Important 发现 → 修复 + scoped re-review（≤5 轮）；全任务后最强可用模型全分支审查。
- 不修改：`api/openapi/`、`web/`、`deploy/compose/`（现有栈）、系列计划文件、`tools/`；不执行任何 GitHub 写操作与 push/merge。
- 提交信息按 Task 定义；工作树保持干净；所有命令在 worktree 内执行。
