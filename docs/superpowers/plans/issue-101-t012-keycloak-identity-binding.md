# Issue #101 [T01.2][P0] Keycloak Verified Subject 与 Identity Binding 实施计划

- **Issue:** https://github.com/1123786563/myqypt/issues/101 （[T01.2][P0] Keycloak Verified Subject 与 Identity Binding，OPEN，0 评论）
- **内嵌历史计划（事实源，不修改）：** issue 正文 Implementation plan 段 = `docs/superpowers/plans/2026-08-24-t01-2-keycloak-verified-identity-binding.md`。正文明确该历史计划**从属于** 2026-08-24 的依赖更新注记（原合并 #100 范围已拆为 F01–F05）。本计划是其执行细则，并裁决下列已核实偏差。
- **规格锚点：** `CONTEXT.md`（Identity Binding / User 词条）、`docs/adr/0024-separate-platform-users-from-keycloak-identities.md`、ADR 0013。
- **前置依赖：** #105（F05）CLOSED 且已合并 main（`58322a7` 已含，gh 亲验）→ 依赖满足。
- **分支/Worktree:** `codex/issue-101-t012-keycloak-identity-binding`，基于 `main@58322a7`，worktree `/Users/wuyongjun/trea/myqypt-worktrees/issue-101-t012-keycloak-identity-binding`。
- **执行协议：** subagent-driven-development —— 每任务全新 implementer subagent（禁止其再派生 subagent、禁止读主仓库 `.superpowers/`）→ 独立规格符合性审查 + 独立代码质量审查 → Critical/Important 修复（≤5 轮 + scoped re-review）→ 全部任务后最强可用模型全分支审查。台账：主仓库 `.superpowers/sdd/issue-101-t012-keycloak-identity-binding/progress.md`（含会话所有权声明，#104 教训④⑦）。
- **提交纪律：** 全分支恰 5 个提交 —— 计划（Task 0）→ `feat(identity): bind verified subjects in postgres`（Task 1）→ `feat(oidc): verify keycloak bearer tokens`（Task 2）→ `feat(platform-api): wire identity callback endpoint`（Task 3）→ `test(platform-api): add black-box identity acceptance and gates`（Task 4）。

## 验收条件（来自 issue，逐条映射）

- **AC1 正常路径最高可行接缝自动化测试**：真实 platform-api 进程（黑盒）携带经本地 OIDC 测试 IdP（httptest discovery+JWKS）签发的 RS256 JWT 调用 `POST /internal/v1/identity/callback` → 201 `{"user_id": <uuid>}`；同一 token 重复投递 → 200 同一 user_id。
- **AC2 非法身份/依赖失败/重试/重复投递确定性结局**：401（无凭证/bad bearer/错 issuer/错 audience/过期/篡改签名/alg=none）；503（IdP discovery/JWKS 不可达、DATABASE_URL 未配置）；500（DB 中途故障）；DB 故障恢复后同一 token 重投 → 200 同一 user_id，无重复业务效果（恰 1 user + 1 binding 行）。
- **AC3 证据含源修订与依赖版本、零 Secret/客户内容**：`docs/evidence/2026-08-27-issue-101-identity-binding.md` 记录 revision、工具版本、零新增依赖声明；access log 与证据文档均不含 token/claims（accesslog 中间件不记录 header，既有事实）。
- **AC4 遵循 CONTEXT.md、ADR 0024、Platform 自有业务事实边界**：仅 `issuer + subject` 为身份键；schema 无 email/phone/username/organization_id/级联 FK；Keycloak 侧删除不可能级联删除 Platform 数据（结构性验证）。

## 范围（In Scope）

1. **迁移** `db/migrations/000002_identity_bindings.sql`（goose Up/Down）：`platform_users` + `identity_bindings`，主键 `(identity_provider, subject)`，`UNIQUE (platform_user_id, identity_provider)`，FK 无级联（裁定 5）。
2. **应用层** `internal/application/identity/`：`VerifiedIdentity{Issuer, Subject}`、`User{ID}`、`Service.Bind`（空 issuer/subject → `ErrUnverifiedIdentity`）、Repository 与 Verifier 两个端口、context 注入/读取助手。
3. **OIDC 适配器** `internal/adapter/oidc/`：RS256 验证器 —— discovery（`{issuer}/.well-known/openid-configuration`）→ JWKS → `crypto/rsa` PKCS#1 v1.5 + SHA-256 验签；claims 校验（iss/aud/exp/nbf/sub/nonce，裁定 3）；kid 缓存 + 未知 kid 单次重取（轮转）；类型化错误 `ErrInvalidToken`/`ErrProviderUnavailable`。**仅用 Go 标准库**。
4. **Postgres 适配器** `internal/adapter/postgres/identity_repository.go`：事务型 `BindOrLoad`（裁定 4：advisory xact lock 串行化，零孤儿 user，并发重复投递竞态安全）。
5. **传输层** `internal/transport/http/identity.go` + `problem.go` 新增问题码：bearer 中间件（仅从验证上下文取身份，绝不读 body/header 中的 issuer/subject）+ 回调处理器（首绑 201 / 幂等重绑 200，裁定 6）+ `Dependencies` 可选装配（未配置 → 路由不注册，404）。
6. **装配** `cmd/platform-api/main.go`：`PLATFORM_IDENTITY_OIDC_ISSUER` + `PLATFORM_IDENTITY_OIDC_AUDIENCE`（两者齐备才启用；复用既有 pgx pool；Verifier 惰性 discovery —— 启动不依赖 Keycloak 可达性，沿 F03 哲学）。
7. **黑盒验收** `cmd/platform-api/identity_process_test.go` `TestPlatformAPIIdentityProcess`（AC1/AC2 全矩阵，临时数据库隔离，裁定 8）。
8. **门禁扩展** `scripts/verify-foundation.sh` INTEGRATION 相位追加 `go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1`（真空拒绝语义不变，裁定 9）。
9. **证据文档** `docs/evidence/2026-08-27-issue-101-identity-binding.md`（日期以实际执行日为准）。

## 非目标（Out of Scope）

- 不部署真实 Keycloak（验证器以进程内 httptest discovery/JWKS 服务测试；真实 Keycloak 属 F10 #110 浏览器会话链路）。
- 不做浏览器流/授权码流/nonce 往返绑定/刷新令牌/会话（F10 范围，裁定 3）。
- 不改 `api/openapi/platform.yaml`（`/internal/v1/identity/callback` 是内部端点，如 /livez 位于公共契约之外）。
- 不持久化任何可变 profile claims（email/phone/username 不入库、不做键）。
- 不做 Audit 流、Membership、Product Access（后续 T 系列）。
- 不动 `web/`（零改动；FRONTEND 相位仅照常运行）。
- 不做 CI、不引入任何新 go.mod 依赖（`go mod tidy -diff` 必须保持空）。
- 不 push、不 merge、不关闭/评论 issue（外部副作用留给用户批准）。

## 设计裁定（对内嵌历史计划的必要澄清与偏差记录）

1. **内嵌计划与现状的 reconcilation**：其 Task 1 假设「verified OIDC claims from #100」已存在——实际 #100 已关闭为 F01 进程 livez，OIDC 验证层从未交付。本计划把它补齐为 Task 2（`internal/adapter/oidc`）并重排文件布局（裁定 2）。其迁移文件名 `000001_identity_bindings.sql` 不可用（已被 F03 的 `000001_platform_baseline.sql` 占据）→ 改为 `000002_identity_bindings.sql`（goose 追加式编号）。
2. **文件布局遵循仓库既有分层**（偏离内嵌计划的 `internal/identity/` 单包布局）：`application/identity` 持有域服务与端口；`adapter/oidc` 与 `adapter/postgres` 实现端口（先例：`adapter/postgres` 已 import `application/readiness`）；`transport/http` 持有 HTTP 处理器（先例：`readiness.go`）；仅 `cmd/platform-api` 读环境变量与装配。
3. **验证面**：alg 白名单恰 `RS256`（Keycloak 默认；none/HS*/RS384/RS512 一律拒）；iss 与配置精确相等；aud 为字符串时精确相等、为数组时须包含配置值；exp ≤ now 拒（零 leeway，测试自控 exp）；nbf > now 拒；sub 非空；**nonce 若出现必须为非空字符串，完整 nonce↨会话往返绑定属 F10 浏览器流**（内嵌计划 "verifies nonce" 的诚实子集，记录为移交项）。token 取自 `Authorization: Bearer`，其余来源一律忽略。
4. **BindOrLoad 并发语义**：单事务内 `pg_advisory_xact_lock(hashtextextended(identity_provider || ':' || subject, 0))` → SELECT → 不存在则 INSERT user + INSERT binding。确定性幂等、零孤儿 `platform_users` 行；并发重复投递测试（多 goroutine + `-race`）断言同一 user、恰 1+1 行。
5. **Schema 逐字采纳内嵌计划**（AC4 结构性满足）：无 email/phone/username/organization_id；FK `REFERENCES platform_users(id)` 无 ON DELETE 动作；`identity_provider` 列存**配置的 issuer URL 字符串**（经验证与 token iss 相等）。
6. **状态码映射**：首绑 201 + `{"user_id": uuid}`；幂等重绑 200 + 同构 body；`ErrInvalidToken`/无验证上下文 → 401 `unauthorized`；`ErrProviderUnavailable` 与 DB 未配置 → 503 `dependency_unavailable`；其余服务错误 → 500 `internal_error`（Problem Details，新增两个稳定问题码）。identity 未配置（issuer/audience 任一缺）→ 路由不注册 → 404 `not_found`。
7. **零新增依赖**：JWS 验签以标准库 `crypto/rsa.VerifyPKCS1v15` + `crypto/sha256` + base64url 实现；`github.com/golang-jwt`/`dgrijalva/jwt-go` 被 ARCH-UPSTREAM-JWT 门禁禁止，`make policy-check` 即为机械证据。
8. **黑盒隔离**：`TestPlatformAPIIdentityProcess` 在 TEST_DATABASE_URL 服务器上 `CREATE DATABASE identity_blackbox_<pid>`，goose up，起真实进程（env：临时库 DSN + httptest issuer/audience），全矩阵后 DROP。无 TEST_DATABASE_URL → 显式 t.Skip 带精确消息（沿既有集成测试约定；真空拒绝由 INTEGRATION 相位承担）。
9. **verify-foundation.sh INTEGRATION 扩展**：追加 `go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1`（先 `-run` 聚焦，避免整包重跑 TestPlatformAPIProcess）；TEST_DATABASE_URL 未设时该相位整体 FAIL 的语义不变。
10. **命令矩阵防伪影**（#104 R8 教训）：所有正则形态测试命令先 `go test -list <regex>` 证实非零匹配再执行，留痕于证据文档。

## 环境事实（所有 subagent 必须遵守）

- Go 工具链：`PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local`（go1.26.7；`/opt/homebrew/bin/go` 已知损坏勿用）。模块下载：`GOPROXY=https://goproxy.cn,direct GOSUMDB=off`。
- **禁用 `env -u VAR cmd` 形式**（本机 `~/.local/bin/env` 损坏会静默 no-op 假绿）；用 `unset VAR` + 裸命令。
- docker 可用，postgres:18 镜像本地已有。端口分配：impl-1 临时 PG=**55460**、Task 1 规格审=55461、质量审=55462（Task 2/3 审查复用 55462）、终审=55463（用前核实空闲，用毕拆除容器与卷）。占用勿触 3000/3030/6379/9000-9001/9100-9101/15432/50051 及既有 55432-55450。
- 本 worktree `web/node_modules` 缺失：FRONTEND 相位前 `pnpm --dir web install`（gitignored 不脏树）；pnpm 11.7.0。
- gh 已认证；一切 gh 命令带 `--repo 1123786563/myqypt`；implementer 只允许只读 gh（issue 正文）。
- 工作目录：本 worktree。禁止：push/merge/关闭/评论 GitHub、派生 subagent、读主仓库 `.superpowers/` 台账、改动计划外文件（`web/`、`api/openapi/`、系列计划文件）。主仓库 main 工作区有用户自己的未提交 AGENTS.md 编辑（不触碰）。

## 任务拆分

### Task 0（controller）：本计划

- 提交本文件：`docs(plan): add issue 101 t01.2 identity binding implementation plan`。

### Task 1：迁移 + 应用服务 + Postgres BindOrLoad（impl-1，全新 subagent）

- RED：`internal/application/identity/service_test.go`（空 issuer/subject → `ErrUnverifiedIdentity`）与 `internal/adapter/postgres/identity_repository_test.go`（幂等、并发重复投递、跨 issuer 同 subject 两用户、无级联结构性断言、round-trip 由既有 `TestMigrationRoundTrip` 自动覆盖新迁移）先行 → 编译红。
- GREEN：`000002_identity_bindings.sql`（裁定 5 schema + goose 标记 + Down）；`application/identity`（Service/端口/错误/context 助手）；`adapter/postgres` 仓储（裁定 4）。
- 门禁（verbatim 留痕）：①聚焦两包（无 DB skip 消息精确）②临时 PG=55460 上两包全绿 ③同前 `-race` ④`go vet ./...` ⑤`gofmt -l .` 空 ⑥`go build ./...` ⑦`go mod tidy -diff` 空 ⑧无 DB 全量 `go test ./... -count=1 -p 1` ⑨`git diff --stat` 无 web//api/openapi/ 改动。
- 单提交 `feat(identity): bind verified subjects in postgres`。

### Task 2：OIDC RS256 验证器（impl-2，全新 subagent）

- RED：`internal/adapter/oidc/verifier_test.go` 全矩阵先行（测试内生成 RSA 密钥对 + httptest discovery/JWKS 服务）→ 编译红。矩阵：合法 token 通过；篡改 payload/签名拒；alg=none/HS256 拒；错 iss/aud/exp/nbf 拒；sub 空拒；nonce 出现且为空拒、出现且非空过；未知 kid 触发单次 JWKS 重取（轮转后新 kid 通过）；discovery/JWKS 不可达 → `ErrProviderUnavailable`；JWKS 非 RSA/kty 或 use 不符拒。
- GREEN：`verifier.go`（裁定 3/7：惰性 discovery、kid 缓存 + 未知 kid 单次重取、RS256-only、类型化错误、5s HTTP 超时、并发安全）。
- 门禁：①聚焦包 ②无 DB 全量 ③vet/gofmt/build/tidy ④`go test ./internal/adapter/oidc -race -count=1` ⑤`git diff --stat` 约束同前。
- 单提交 `feat(oidc): verify keycloak bearer tokens`。

### Task 3：传输层 + 装配（impl-3，全新 subagent）

- RED：`internal/transport/http/identity_test.go` 先行（stub Verifier/Repository：无凭证 401；验证失败 401；首绑 201；重绑 200 同 body；`ErrProviderUnavailable` 503；服务错误 500；Dependencies 未配置 → 路由 404；body/header 伪造 issuer 不生效）→ 编译红。
- GREEN：`problem.go` 新增 `unauthorized`/`dependency_unavailable` 码与标题；`identity.go`（bearer 中间件 + 回调处理器 + 路由注册）；`Dependencies` 增可选 `Identity` 装配（nil → 不注册）；`cmd/platform-api/main.go` 环境变量读取与装配（复用 pool；DB 未配置且 identity 配置 → 注册但 503 fail-closed，裁定 6）；`router_test.go` 既有断言零回归。
- 门禁：①聚焦三包 ②无 DB 全量 ③vet/gofmt/build/tidy ④`make generate-check`（生成器零漂移）⑤`make policy-check`（ARCH-UPSTREAM-JWT 亲证零禁用 JWT 库）⑥`git diff --stat` 无 web//api/openapi 改动（server.gen.go 仅因零改而零漂移）。
- 单提交 `feat(platform-api): wire identity callback endpoint`。

### Task 4：黑盒验收 + 门禁扩展 + 证据（impl-4，全新 subagent）

- 无 RED（本任务仅新增测试/门禁接线/证据，不新增生产行为；新测试对回归的检出力由矩阵运行本身证明）。若黑盒矩阵首跑即全绿且从未见过红，须以「临时篡改一处断言观察到失败再复原」补红证（防 tautology）。
- `cmd/platform-api/identity_process_test.go`（裁定 8）：临时库 + goose up + 真实进程 + httptest IdP。矩阵：AC1 全路径（201→200 同 user_id；DB 恰 1 user + 1 binding）；401 全因；503（IdP 闭端口）；500 + 恢复后 200 同 user（停/起临时库或等效确定性手段）；未配置 identity env 的对照进程 → 404；证据卫生断言（响应体恰 `{"user_id": ...}`，无 token/claims）。
- `scripts/verify-foundation.sh` INTEGRATION 相位追加（裁定 9）；`bash -n` 语法校验；真空拒绝复验（unset TEST_DATABASE_URL → INTEGRATION FAIL + JSON 落盘）。
- 证据文档 `docs/evidence/2026-08-27-issue-101-identity-binding.md`：黑盒矩阵逐条 verbatim、验收命令矩阵（下节）逐条 verbatim、-list 非零匹配留痕（裁定 10）、容器/库拆除记录。
- 门禁：①`-list` 证非零匹配 ②黑盒聚焦（临时 PG=55460）③`TestPlatformAPIProcess` 聚焦 ④临时 PG 上 `make verify-foundation` 七相位全 PASS（FRONTEND 前装 node_modules）⑤无 DB `make verify-foundation --phases GENERATE,POLICY,UNIT,CONTRACT,FRONTEND,META`（INTEGRATION 显式 FAIL 语义另行单验）⑥无 DB 全量 `-p 1` ⑦vet/gofmt/build/tidy ⑧`git diff --stat` 终检。
- 单提交 `test(platform-api): add black-box identity acceptance and gates`。

## 测试与验收命令（全分支，终审矩阵）

| # | 命令 | 期望 |
|---|------|------|
| 1 | `gofmt -l .` | 空 |
| 2 | `go vet ./...` | exit 0 |
| 3 | `go build ./...` | exit 0 |
| 4 | `go mod tidy -diff` | 空（零新增依赖） |
| 5 | `make generate-check` | exit 0，零漂移 |
| 6 | `make policy-check` | exit 0（含 ARCH-UPSTREAM-JWT） |
| 7 | `go test ./... -count=1 -p 1`（无 DB） | 全 ok，DB 用例显式 skip |
| 8 | `TEST_DATABASE_URL=... go test ./... -count=1`（临时 PG=55463） | 全 ok |
| 9 | `TEST_DATABASE_URL=... go test ./internal/adapter/postgres ./internal/adapter/oidc -race -count=1` | 全 ok |
| 10 | `TEST_DATABASE_URL=... go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1 -v`（先 `-list` 留痕） | PASS |
| 11 | `go test ./cmd/platform-api -run '^TestPlatformAPIProcess$' -count=1 -v` | PASS |
| 12 | 临时 PG 上 `make verify-foundation` | 七相位全 PASS |
| 13 | unset TEST_DATABASE_URL 后 INTEGRATION 单相位 | FAIL（真空拒绝）+ JSON 落盘 |
| 14 | `git diff main...HEAD --stat` | 恰 5 提交；无 web/、api/openapi/ 改动 |

## 全局约束

- 仅验证过的 `issuer + subject` 标识用户；email/phone/username 不得成为跨系统键（schema 层禁止 + 门禁无相关列）。
- 重复回调幂等并返回同一 Platform User。
- Keycloak 身份删除/禁用不得级联删除 Platform 历史或产品数据（无级联 FK + 结构性测试）。
- Token、凭证、可变 claims 不进入 Audit 或测试证据（证据卫生断言 + accesslog 不记 header 既有事实）。
- 每任务全新 implementer subagent；implementer 禁止派生 subagent、禁止 GitHub 写操作、禁止读主仓库台账。
- 每任务独立规格符合性 + 代码质量双审；Critical/Important → 修复轮（≤5）+ scoped re-review。
- 全部任务后最强可用模型全分支终审（上表 14 条逐条重跑）。
- 本地完成后停止：不 push/merge/关闭/评论 GitHub——等待用户明确批准。
