# Issue #102 [F02][Foundation] OpenAPI Strict Transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec / Issue:** [Issue #102 — [F02][Foundation] OpenAPI Strict Transport](https://github.com/1123786563/myqypt/issues/102), extraction design §6.2、§7.3、§8.2、§9、§12.2（`docs/superpowers/specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md`），`CONTEXT.md`，F01 已合并实现（`main@1a66525`）。

**Goal:** 以 OpenAPI 3.1 契约为唯一 HTTP 契约源，通过 oapi-codegen 生成 Gin strict handler，交付无业务副作用的 `GET /api/v1/system/status` 与稳定 RFC Problem Details（RFC 9457 `application/problem+json`）错误响应。

**Supersedes:** `docs/superpowers/plans/2026-08-24-f02-openapi-strict-transport.md`（batch 计划假设了不存在的 `NewRouter(Dependencies)` 签名、未覆盖 Problem Details 的 NoRoute/NoMethod/strict-hook 接线、请求 ID 来源、契约偏移检测测试、工具链/代理环境事实与 web 回归；本计划为 Issue #102 的执行计划，差异均体现在 Global Constraints 与任务拆分中）。

## Issue selection rationale（2026-08-25 唤醒）

- 无未完成的 Issue 实施：三个遗留 worktree（`issue-100-f01-clean`、`t01-1-platform-scaffold`、`issue-106-f06-react-static-landing`）的分支均已并入 `main`（merge commits `0838a45`、`ae71e6a`、`1a66525`，main 与 origin/main 一致）；`.superpowers/sdd/` 下无 `progress.md` 台账，两份历史 task report 均以 final-review fix 收尾。
- Issue #100（F01）实现已由用户合并进 main，本机在 Go 1.26.7 下 `go test ./... -count=1` 全部通过（2026-08-25 证据，见 SDD ledger）；其 GitHub 状态为 OPEN 属用户保留的关闭动作，本流程不得代为关闭。
- 按编号从小到大：#72–#86（T 系列）各自 `Blocked by` 未完成的 T/F issue（如 #72 依赖 #19、#71）；#87、#90–#99 为 `ready-for-human` 档案票；#88/#89 被 T 系列与未创建的 U 系列阻塞；#100 已交付（见上）；#101（T01.2）明确 "starts only after F05 (#105) completes" 而被阻塞；#102（F02）仅依赖 #100，其实质依赖（F01 代码 + 通过的验收）已在 main 满足，描述含五条可验收标准与既定技术栈，可执行 → 选定 #102。

## Scope（范围）

- 新建 `api/openapi/platform.yaml`（OpenAPI 3.1.0，仅 `GET /api/v1/system/status` + `SystemStatus` schema）与 `api/openapi/oapi-codegen.yaml`（gin-server + strict-server + models + embedded-spec）。
- 新建 `tools/tools.go`（build tag `tools`，blank-import oapi-codegen CLI，将 `github.com/oapi-codegen/oapi-codegen/v2 v2.8.0` 钉入 go.mod）与 `internal/transport/http/api/generate.go`（`//go:generate` 指令）；生成并提交 `internal/transport/http/api/server.gen.go`。
- `internal/transport/http/status.go`：实现 `api.StrictServerInterface` 的 `StatusHandler`，返回 `{"status":"available","version":"<version>"}`。
- `internal/transport/http/problem.go`：`Problem` 结构与 `WriteProblem(*gin.Context, Problem)`，稳定 code `invalid_request`、`not_found`、`method_not_allowed`、`internal_error`。
- `internal/transport/http/requestid.go`：最小请求 ID 中间件（复用入站 `X-Request-ID`，否则生成），写入响应头与 gin context，供 `trace_id` 使用；F04 后续扩展为完整可观测中间件。
- `internal/transport/http/router.go`：`NewRouter(deps Dependencies) http.Handler`；安装 request-ID、`ginmiddleware.OapiRequestValidatorWithOptions`（校验失败映射 Problem）、`api.RegisterHandlers(router, api.NewStrictHandler(...))`（strict request/response error hook 映射 Problem）、`NoRoute`/`NoMethod` Problem 映射；保留 F01 的 `/livez` 原样。
- 调用方适配：`cmd/platform-api/main.go` 传 `Dependencies{Version: version}`；`internal/transport/http/router_test.go`、`internal/platform/runtime/server_test.go` 适配新签名。
- 测试：`status_test.go`（黑盒 status、livez 回归、gin mode 回归）、Problem Details 映射测试（真实校验拒绝路径、404、405、strict hook、trace_id 非空）、契约偏移检测测试（以 embedded spec 校验真实响应，并含故意偏移的负控制）。
- go.mod/go.sum：新增 `oapi-codegen/v2 v2.8.0`（tools）、`oapi-codegen/runtime`、`oapi-codegen/gin-middleware v1.1.0`、`getkin/kin-openapi`（校验）。

## Non-goals（非目标）

- 不实现任何带业务副作用的端点、鉴权、租户、Session、OpenFGA（F10–F12、T 系列）。
- 不做 `/readyz`、PostgreSQL、migration（F03）；不做 Request ID 之外的日志/安全头/CORS/panic recovery 完整中间件栈（F04）；不做证据门禁（F05）。
- 不生成 TypeScript 客户端（F09 消费同一契约时生成）；不做 Higress/边缘路由（F20）。
- 不把 `/livez` 纳入 OpenAPI 契约（进程存活检查不属于公开 API 面）。
- 不改动 `web/`、`deploy/`、`tests/platformtest/`、既有 F01 行为语义（`/livez` 精确 body、优雅退出、version 不监听端口）。
- 不 push、merge、发布、关闭 Issue 或修改 GitHub 状态。

## Global Constraints（全局约束）

- `api/openapi/platform.yaml` 是唯一公开 HTTP 契约源；生成物只属于 `internal/transport/http/api`，禁止被 `internal/transport/http` 之外的包 import（`cmd/` 也不得 import 生成包）；生成类型不得成为 Domain Model 或数据库 Model。
- 生成组合固定为 `gin-server + strict-server + models + embedded-spec`；生成器版本钉在 `github.com/oapi-codegen/oapi-codegen/v2 v2.8.0`（经 `tools/tools.go` blank import 纳入 go.mod，`go generate` 用 `go run` 无 `@version` 形式执行）；重复生成必须幂等（`git diff --exit-code -- internal/transport/http/api/server.gen.go`）。
- 请求校验由 `github.com/oapi-codegen/gin-middleware v1.1.0` 承担（strict 生成不做请求校验）；业务不变量校验属 Application Module（本 Issue 无业务端点）。
- 所有错误响应：`Content-Type: application/problem+json`，稳定 `code`、正确 HTTP `status`、非空 `trace_id`（来自请求 ID 中间件），不泄漏内部错误原文；gin 默认 404/405 空 body 不得再出现。
- `NewRouter` 签名变更为唯一 Transport 构造入口：`NewRouter(deps Dependencies) http.Handler`，`Dependencies{Version string}`；`/livez` 保持返回精确 `{"status":"alive"}`（Content-Type application/json）；`version` 缺省仍为 `dev` 且 `version` 命令不启动监听。
- 环境事实：仓库 `toolchain go1.26.7`；本机默认 `go` 为 1.26.3 且 `GOPROXY=off`/`GOSUMDB=off`，所有 Go 命令必须使用 `PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local`；需要拉取模块时追加 `GOPROXY=https://goproxy.cn,direct GOSUMDB=off`（已验证可下载 oapi-codegen v2.8.0 与 gin-middleware v1.1.0）。
- 只允许新增/修改：`api/openapi/**`、`tools/tools.go`、`internal/transport/http/**`、`internal/platform/runtime/server_test.go`（仅 NewRouter 调用签名适配）、`cmd/platform-api/main.go`（仅依赖注入适配）、`go.mod`、`go.sum`、本计划文件；不得改动 `web/**`、`deploy/**`、`tests/**`、根 `.gitignore`、其他计划文档。
- 测试必须断言真实行为（禁止 assert-nothing 测试）；TDD：每个任务先写失败测试并记录 RED 证据再实现（GREEN）；`go test` 输出必须干净（无 stray warning）。
- implementer 不得派生 subagent；每个任务的实现、规格审查、质量审查分别由独立的新 subagent 完成。
- 不执行 push、merge、发布、关闭 Issue 或任何 GitHub 状态修改。

## 测试与验收命令（全部在 worktree 根执行；GO=PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local）

1. 生成幂等：`go generate ./internal/transport/http/api/... && git diff --exit-code -- internal/transport/http/api/server.gen.go`
2. 聚焦测试：`go test ./internal/transport/http/... -count=1`
3. 格式：`gofmt -l .` 输出为空
4. 静态检查：`go vet ./...`
5. 构建冒烟：`go build ./... && go build -o /tmp/myqypt-platform-api-f02 ./cmd/platform-api && /tmp/myqypt-platform-api-f02 version`（输出 `dev`）
6. 全量单测：`go test ./... -count=1`
7. 竞态：`go test -race ./... -count=1`
8. 生成类型边界：`! grep -rn "transport/http/api" --include="*.go" cmd internal/platform tests`（退出码非 0 即未越界）；`git diff --check`
9. web 回归（F02 不触碰 web，仍按仓库门禁执行）：`pnpm --dir web typecheck && pnpm --dir web lint && pnpm --dir web format:check && pnpm --dir web test && pnpm --dir web build && pnpm --dir web verify:static && pnpm --dir web verify:forbidden && pnpm --dir web test:e2e`
10. 进程级回归（F01 验收不被破坏）：`go test ./cmd/platform-api -run 'TestPlatformAPIProcess' -count=1`

---

## File Structure

- `api/openapi/platform.yaml`、`api/openapi/oapi-codegen.yaml`
- `tools/tools.go`
- `internal/transport/http/api/generate.go`、`internal/transport/http/api/server.gen.go`（生成）
- `internal/transport/http/status.go`、`internal/transport/http/problem.go`、`internal/transport/http/requestid.go`、`internal/transport/http/router.go`
- `internal/transport/http/status_test.go`、`internal/transport/http/problem_test.go`、`internal/transport/http/router_test.go`（适配）、`internal/transport/http/contract_test.go`
- `cmd/platform-api/main.go`（适配）、`internal/platform/runtime/server_test.go`（适配）

### Task 1: Generate and serve the strict contract

**Interfaces:**
- `GET /api/v1/system/status` → 200 `{"status":"available","version":Version}`。
- `type Dependencies struct{ Version string }`；`NewRouter(deps Dependencies) http.Handler`。
- `StatusHandler` 实现 `api.StrictServerInterface`（`GetSystemStatus(context.Context, api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error)`）。

- [ ] **Step 1（RED）**: 在 `status_test.go` 写黑盒失败测试：`NewRouter(Dependencies{Version: "be4cc10"})` 响应 `GET /api/v1/system/status`，断言 200、`Content-Type: application/json`、body 精确包含 `"status":"available"` 与 `"version":"be4cc10"`；同时保留/适配 `/livez` 精确 body 断言与 gin mode 断言。运行 `go test ./internal/transport/http -run 'TestSystemStatus|TestLivez|TestNewRouter' -count=1` 记录 RED（缺少生成契约与 status.go，编译失败或测试失败均算 RED，需如实记录）。
- [ ] **Step 2**: 写 `api/openapi/platform.yaml`（openapi 3.1.0；`operationId: getSystemStatus`；200 → `SystemStatus`：`additionalProperties: false`、`required: [status, version]`、`status: {type: string, const: available}`、`version: {type: string, minLength: 1}`）与 `api/openapi/oapi-codegen.yaml`（`package: api`；`generate: {gin-server: true, strict-server: true, models: true, embedded-spec: true}`；`output: server.gen.go`，注意 generate 在 `internal/transport/http/api` 目录执行时路径的正确性）。
- [ ] **Step 3**: 建 `tools/tools.go`（`//go:build tools`，blank import `github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`）与 `internal/transport/http/api/generate.go`（`//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ../../../api/openapi/oapi-codegen.yaml ../../../api/openapi/platform.yaml`）。用 `GOPROXY=https://goproxy.cn,direct GOSUMDB=off` 执行 `go mod tidy` 与 `go generate ./internal/transport/http/api/...` 生成 `server.gen.go`。
- [ ] **Step 4**: 实现 `status.go`（`StatusHandler`，编译期断言 `var _ api.StrictServerInterface = (*StatusHandler)(nil)`）与 `router.go` 新签名：`gin.New()` → `api.RegisterHandlers(router, api.NewStrictHandler(&StatusHandler{Version: deps.Version}, nil))` → `/livez` 保持原样；适配三处调用方（`main.go` 传 `Dependencies{Version: version}`，两处测试传测试值）。
- [ ] **Step 5（GREEN + 幂等）**: `go test ./internal/transport/http/... -count=1` 通过；重跑 `go generate` 后 `git diff --exit-code -- internal/transport/http/api/server.gen.go`；`gofmt -l .` 为空、`go vet ./...`、`go test ./... -count=1` 通过并记录输出。
- [ ] **Step 6**: Commit `feat(api): add strict system status contract`（含 `api/openapi`、`tools/tools.go`、`internal/transport/http`、`cmd/platform-api/main.go`、`internal/platform/runtime/server_test.go`、`go.mod`、`go.sum`）。

### Task 2: Standardize Problem Details

**Interfaces:**
- `problem.go`：`type Problem struct { Type, Title string; Status int; Code, TraceID string }`（json: type/title/status/code/trace_id）与 `WriteProblem(c *gin.Context, p Problem)`；稳定 code 常量 `invalid_request`/`not_found`/`method_not_allowed`/`internal_error`。
- `requestid.go`：请求 ID 中间件（入站 `X-Request-ID` 非空则复用，否则生成如 16 hex；写入响应头与 gin context key）。
- Router 接线：request-ID → `ginmiddleware.OapiRequestValidatorWithOptions(GetSwagger(), options)`（`ErrorHandler` 写 400 `invalid_request` Problem）→ strict handlers（`StrictHTTPServerOptions.RequestErrorHandlerFn` → 400 `invalid_request`、`ResponseErrorHandlerFn` → 500 `internal_error`）→ `NoRoute` 404 `not_found`、`NoMethod` 405 `method_not_allowed`（`HandleMethodNotAllowed = true`）。

- [ ] **Step 1（RED）**: `problem_test.go` 断言（全部走 `NewRouter` 的真实中间件链）：(a) 405：对契约路径发 `POST /api/v1/system/status` → 405、`application/problem+json`、`code=method_not_allowed`、非空 `trace_id` 且等于响应头 `X-Request-ID`；(b) 404：`GET /api/v1/does-not-exist` → 404 problem（`code=not_found`）；(c) 入站 `X-Request-ID: test-trace-42` 被复用为 `trace_id`；(d) 真实 OpenAPI 校验拒绝路径：用一个含必填 query 参数的极小内联 spec（`openapi3.Loader` 加载 YAML 字符串）+ 生产同款 `OapiRequestValidatorWithOptions` 接线（提取为可测的构造函数），发缺失参数请求 → 400、`code=invalid_request`、`application/problem+json`。运行记录 RED。
- [ ] **Step 2**: 实现 `problem.go`、`requestid.go` 与 router 接线（校验器/strict hook/NoRoute/NoMethod 全部收敛到 `WriteProblem`；`Type` 用稳定 URI 前缀 `https://api.myqypt.dev/problems/<code>`；`Title` 用可展示短句；不返回内部错误原文）。
- [ ] **Step 3（GREEN）**: `go test ./internal/transport/http/... -count=1` 通过；`gofmt -l .` 空、`go vet ./...`、`go test ./... -count=1` 通过并记录输出。
- [ ] **Step 4**: Commit `feat(api): standardize problem details`。

### Task 3: Contract divergence detection and full gates

- [ ] **Step 1（RED）**: `contract_test.go`：(a) 从 `api.GetSwagger()` 解析 `/api/v1/system/status` GET 200 的 `SystemStatus` schema，对真实 handler 响应 body 做 `schema.VisitJSON` 校验（存在、类型、required、additionalProperties）；(b) 负控制：对故意偏移的 payload（删除 `version`、加未知字段、`status:"dead"`）断言校验报错——证明检测器能发现实现与契约不一致。先写测试并对"尚未接入真实响应"的形态记录 RED，再接通。
- [ ] **Step 2（GREEN）**: 全部聚焦测试通过；执行完整验收命令 1–10 并记录全部输出（含 `go test -race ./... -count=1`、web 回归与进程级回归；`git diff --check`）。
- [ ] **Step 3**: 边界扫描：`! grep -rn "transport/http/api" --include="*.go" cmd internal/platform tests` 确认生成类型只在 Transport 使用。
- [ ] **Step 4**: Commit `test(api): enforce contract divergence detection`。

## Self-Review Record

- Spec coverage: Issue #102 五条验收标准分别由 Task 1（可重复生成 + strict handler 成功响应 + 生成类型仅 Transport）、Task 2（稳定 Problem Details 含 code/status/trace ID）、Task 3（契约偏移检测 + 全门禁）覆盖。
- Placeholder scan: 版本（oapi-codegen v2.8.0、gin-middleware v1.1.0）已用 registry 验证可下载；路径、命令、断言、环境变量均为精确值。
- Type consistency: `Dependencies`/`Problem`/`StatusHandler`/生成类型命名封闭一致；`NewRouter` 是唯一构造入口，调用方适配点列举完整。
