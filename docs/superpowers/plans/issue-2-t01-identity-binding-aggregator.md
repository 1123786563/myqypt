# Issue #2 [T01][P0] User 注册与 Identity Binding — Aggregator 实施计划

- Issue: https://github.com/1123786563/myqypt/issues/2 （OPEN，0 评论，label ready-for-agent）
- 源计划（Issue 内嵌，仓库亦有 `docs/superpowers/plans/2026-08-24-t01-identity-binding.md`）：本文是对它的可执行化裁定版；与仓库现实冲突处以本文件为准。
- 子 Issue：#100（CLOSED，已合并 main）、#101（CLOSED，合并于 20a64e2）。
- 分支：`codex/issue-2-t01-identity-binding`（base `main@1c43f8d`）。
- Worktree：`/Users/wuyongjun/trea/myqypt-worktrees/issue-2-t01-identity-binding`。
- 执行协议：subagent-driven-development —— 每个实施任务由全新 implementer subagent 完成（禁止其再派生 subagent），每任务经独立规格符合性审查 + 独立代码质量审查，Critical/Important 发现进入最多 5 轮修复与 scoped re-review，全部任务后由最强可用模型做全分支审查。外部副作用（push/merge/关闭 Issue/评论 GitHub）一律保留给用户明确批准。

## 目标（Goal）

组合 #100 的最小 Platform/测试 harness 与 #101 的 Casdoor Identity Binding，在真实 Docker Compose 栈（platform-api + postgres + casdoor:3.159.0）上跑通一条黑盒验收旅程，证明**一次真实 Casdoor 注册只创建一个稳定 Platform User 绑定**：

1. 正常路径：经 Casdoor 签发的真实 RS256 OIDC token 调 `POST /internal/v1/identity/callback` → 201 + `{"user_id":"<uuid>"}`；
2. 幂等路径：同一 token 重放 → 200 + 完全相同的 `user_id`，绑定行数仍为 1；
3. 拒绝路径：未通过验证的凭据（无/篡改/伪签 token）→ 401，不产生绑定；
4. 证据：`platformtest` 报告 JSON（含稳定测试标识与依赖版本，绝无 token/凭据/个人档案值）。

## 裁定（源计划 vs 仓库现实，实施以本节为准）

1. **Scenario schema 适配**：#100 交付的 `platformtest.Scenario` 契约是 `{id, seam, timeout, inputs, assertions, metadata}` 且 `KnownFields(true)` 严格解码；源计划字面 YAML（`request/expect/replay/denial` 顶层键）会被拒为 `decode_error`。裁定：语义 1:1 保留，改用真实 schema 表达 —— journey 参数进 `inputs`（如 `issuer`、`callback_path`、`replay_deliveries`），验收断言进 `assertions[].name/want`（如 `first_bind_status=201`、`replay_status=200`、`replay_same_user`、`binding_count=1`、`denial_status=401`、`denial_binding_count=0`）。JSON Schema（`tests/platformtest/schema/scenario.schema.json`）允许自由形态 object，无需改动 #100 交付物。
2. **驱动注册**：seam `lighthouse-black-box` 的 driver 由 `tests/acceptance` 包 `init()` 经 `platformtest.Register` 注册；driver 承担：栈健康预检、Casdoor 供给（用户注册+token 获取）、旅程执行、DB 断言、断言结果回填。
3. **真实主体**：真实 Casdoor token 的 `sub` 是 Casdoor 用户标识（形如 `org/name`），不是源计划字面的 `subject-t01`。测试标识稳定性体现在 Casdoor 用户名/组织等供给参数（固定测试标识，如 `t01-accept`），`sub` 的实际值由驱动从旅程中读取后用于 DB 断言，不硬编码。
4. **focused 测试集**：源计划写 `go test ./internal/identity/...`，该路径不存在。裁定 focused = `./internal/application/identity/... ./internal/adapter/oidc/... ./internal/transport/http/...`（无 DB），加 `./internal/adapter/postgres/...`（需 `TEST_DATABASE_URL` 临时库）。
5. **Compose 装配**：`deploy/compose/compose.yaml` 的 platform-api 服务当前未设 identity/DATABASE_URL 环境 → callback 未注册。裁定：补装配（这是组合布线，不是第二套 identity 实现）——`DATABASE_URL`（由既有 `PLATFORM_POSTGRES_*` 必填变量拼出）、`PLATFORM_IDENTITY_OIDC_ISSUER`（默认 `http://casdoor:8000`，即 compose 内 `origin`）、`PLATFORM_IDENTITY_OIDC_AUDIENCE`（默认值由 Task 1 spike 定，见裁定 6）、迁移（command 链 `go run ./cmd/platform-api migrate && go run ./cmd/platform-api serve`，serve 不迁移是 #101 既有裁定）、容器内 `GOPROXY=https://goproxy.cn,direct GOSUMDB=off`（golang:1.26.7 容器默认 proxy.golang.org 在本网络不可达）+ `/go/pkg/mod` 具名卷（模块缓存跨 run 复用）。
6. **Casdoor 供给路径（强制 spike 先行）**：Casdoor 3.159.0 的管理 API 鉴权方式、`add-application` 是否接受调用方指定 `clientId/clientSecret`、token 端点支持的 grant、discovery/JWKS 形状、token 的 `aud`（= Application clientId）与 `sub` 格式，均必须先对**活的** compose 容器用 curl 实验确认并记录，再写驱动代码。回退阶梯（按序尝试、记录选中的那级）：
   a. 管理员凭据（镜像内置，预期 `built-in/admin`）经 `/api/login`（JSON）或 OAuth client_credentials 获取管理会话/令牌；
   b. 经管理 API 创建专用 Application（固定 clientId 如 `t01-acceptance`，若服务端坚持自生成 clientId，则读回实际值并由 compose 环境变量注入 platform-api —— 此时集成命令需在 `docker compose up` 前导出 `PLATFORM_IDENTITY_OIDC_AUDIENCE`）；
   c. token 获取优先尝试 `grant_type=password`，不可用则用 `/api/login` 取 code 换 token（client_secret_post 或 Basic）；
   d. 若 JWKS 端点非标准形状导致 #101 verifier 无法消费，**停下报告**（这属于 #101 交付物缺陷，超出 #2 aggregator 范围，升级 controller 裁定）。
7. **拒绝路径语义**：源计划 `remove_verified_claims: true` 在真实系统 = 验证必然失败的凭据。裁定至少覆盖：缺失 Authorization、`Bearer` + 篡改签名 token（对合法 token 签名段做确定位翻转）→ 均 401；不产生绑定行。
8. **RED 先行**：先写 scenario + 测试（driver 未实现/栈未起）跑 `go test ./tests/acceptance -run TestT01IdentityBinding -count=1` 确认红；实现 driver 并起栈后转绿。`tests/acceptance` 目录当前不存在，属新包。
9. **证据卫生**：#100 的 Report 已内建脱敏（非空 summary/details 一律替换为固定脱敏文案）。驱动额外义务：断言 details 只写状态码/行数等事实，绝不写 token、密码、用户档案；Casdoor 管理凭据只经环境变量/进程内存，不落任何被提交的文件；evidence 落 `artifacts/evidence/t01-identity-binding/`（gitignored）。
10. **栈生命周期**：集成命令保持源计划形态 `docker compose -f deploy/compose/compose.yaml up -d --wait && go test ./tests/acceptance -run TestT01IdentityBinding -count=1`（`--wait` 依赖 healthcheck；platform-api 无 healthcheck 时裁定补一个 `/livez` CMD healthcheck）。driver 自身做可达性预检：栈不可达 → `t.Fatalf` 明确指因（不静默 skip；无 docker 时允许 `testing.Short()` 或显式 env gate 跳过并打印精确原因——选定后者：`T01_ACCEPTANCE_STACK=1` 之外的默认行为是 skip 并给出起栈命令，保证无栈环境 `go test ./...` 仍绿；集成运行时设置该变量）。

## 范围（Scope）

- 新增 `tests/acceptance/`（driver、Casdoor 供给 helper、`TestT01IdentityBinding`）。
- 新增 `tests/acceptance/scenarios/t01-identity-binding.yaml`（真实 schema）。
- 修改 `deploy/compose/compose.yaml`（仅 platform-api 服务装配 + 必要 healthcheck；不动 casdoor/postgres 服务定义既有语义）。
- 上述文件的测试与证据。

## 非目标（Non-goals）

- 不改 #101 交付物（`internal/application/identity`、`internal/adapter/oidc`、`internal/adapter/postgres`、`internal/transport/http/identity.go`、迁移 000002）；发现其缺陷时停下上报，不顺手修。
- 不改 `tests/platformtest/`（#100 交付物）。
- 不动 `web/`、`api/openapi/`、`deploy/compose.yaml`（F03 栈）、系列计划文件、`db/migrations/`。
- 不引入新 Go 依赖（驱动用标准库 + 既有 pgx；如确需 casdoor SDK 一律拒绝）。
- 不做 push/merge/关闭/评论 GitHub。
- 不实现 Tenant/Membership（T02+ 的范围）。

## 任务拆分

- **Task 0（controller，已完成）**：本计划落盘提交；SDD ledger 建立；worktree 就绪。
- **Task 1（单个 implementer subagent）**：完整验收切片，内部步骤有序：
  1. Spike：起 postgres+casdoor 两服务（platform-api 可暂缓），curl 实验回答裁定 6 的全部问题，结论写入 task 报告（含每条实验的请求/响应摘要，凭据脱敏）；
  2. RED：scenario YAML + 空实现 driver 骨架 + 测试，确认红；
  3. compose 装配（裁定 5）+ driver 全量实现（供给→token→201→重放 200 同 user→拒绝 401→DB 断言 binding_count）；
  4. GREEN：起全栈跑集成命令转绿；focused 集分开跑；
  5. 单提交（建议 `test(acceptance): prove T01 identity binding journey against real casdoor`，spike 结论进 commit body 或 task 报告）。
- **Final review（最强可用模型，全新独立上下文）**：全分支 diff 审查 + 全量门禁复跑。

## 测试与验收命令（全部通过才算本地完成）

按序（环境变量见下节）：

1. `gofmt -l .` 输出为空；
2. `go vet ./...`；
3. `go build ./...`；
4. `go mod tidy -diff` 无差异；
5. `make generate-check`；
6. `make policy-check`；
7. focused（无 DB）：`go test ./internal/application/identity/... ./internal/adapter/oidc/... ./internal/transport/http/... -count=1`；
8. focused（带 DB，临时 PG 容器）：`go test ./internal/adapter/postgres/... -count=1`（`TEST_DATABASE_URL` 指向临时库）；
9. 全量无 DB（串行，规避 R7）：`go test ./... -count=1 -p 1`（无栈时 acceptance 包默认 skip，须打印精确起栈命令）；
10. `TestPlatformAPIProcess` 串行单跑 PASS（R7 前科，禁并行负载下跑）；
11. 集成：`PLATFORM_POSTGRES_DB=... PLATFORM_POSTGRES_USER=... PLATFORM_POSTGRES_PASSWORD=... CASDOOR_POSTGRES_DB=... CASDOOR_POSTGRES_USER=... CASDOOR_POSTGRES_PASSWORD=... docker compose -f deploy/compose/compose.yaml up -d --wait && T01_ACCEPTANCE_STACK=1 go test ./tests/acceptance -run TestT01IdentityBinding -count=1 -v`；证据 JSON 落 `artifacts/evidence/t01-identity-binding/`；
12. `make verify-foundation`（七相位；FRONTEND 相位前确保 `web/node_modules` 已装；临时 PG 按脚本自身约定提供）。

## 全局约束

- 禁止 push/merge/close/评论 GitHub；禁止派生 subagent；禁止改动计划外文件（尤其 `.superpowers/`、`web/`、`api/openapi/`、`tests/platformtest/`、#101 全部交付物、`deploy/compose.yaml`）。
- 证据与提交绝不含 token、密码、个人档案值；稳定测试标识（如 `t01-accept`）+ 依赖版本可以含。
- 领域词汇沿 CONTEXT.md/ADR-0024：Casdoor 拥有登录身份与稳定 subject；Platform 拥有 User 与 Identity Binding；identity key 恒为 `identity_provider + subject`。
- 失败必须先诊断修复或如实上报（含精确命令/错误/已完成验证），不得伪报通过；环境阻塞必须给出准确命令与错误。
- Docker/网络事实：本机 Docker 29.4.0；`golang:1.26.7`、`casdoor:3.159.0`、`postgres:17` 镜像本地均无，需拉取；若 Docker Hub 不可达，可用镜像源（如 `docker.m.daocloud.io`）拉取后打 tag，spike 记录。
- 测试后清理：compose 栈 `docker compose down -v`，确认容器/卷/网络零残留；临时 PG 容器同理。

## 环境事实（供所有 subagent，承接既有台账并更新）

- **Go 工具链（新位置，旧 /tmp 工具链已被清理）**：`/Users/wuyongjun/.local/go1.26.7/bin/go`（1.26.7，亲证可构建本仓库）；使用时 `PATH=/Users/wuyongjun/.local/go1.26.7/bin:$PATH GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct GOSUMDB=off`。勿用 `/opt/homebrew/bin/go`（brew 1.26.3，会被 go.mod 的 toolchain 行诱导去下载且默认 GOPROXY 关闭，前科“假绿/失败”）。
- **禁用 `env -u VAR cmd`**（`~/.local/bin/env` 损坏会静默 no-op）；用 `unset VAR` + 裸命令。
- 端口：8080/8000/5432 空闲（T01 栈用）；3000/3030/6379/9000-9001/9100-9101/15432/50051 为 WeKnora 占用勿触；临时 PG 建议用 55xxx 段并先核空闲。
- R7 前科：`TestPlatformAPIProcess` 启动预算 5s 对负载敏感 —— 全量测试一律 `-p 1` 串行；该测试单独串行跑。
- pnpm 11.7.0 / node v26.7.0；worktree 的 `web/node_modules` 需自行 `pnpm --dir web install`（gitignored）。
