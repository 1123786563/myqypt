# Issue #104 [F04][Foundation] HTTP 安全与可观测中间件 实施计划

- **Issue:** https://github.com/1123786563/myqypt/issues/104 （[F04][Foundation] HTTP 安全与可观测中间件，OPEN，无评论）
- **系列计划（事实源，不修改）：** `docs/superpowers/plans/2026-08-24-f04-http-security-observability.md`（与 issue 正文 Implementation plan 段一致，已核对）
- **前置依赖：** F02 = #102（CLOSED，已合并 main 并复验）→ 依赖满足。F03 = #103（CLOSED，main `f1f53aa` 已含）为既有事实。
- **分支/Worktree:** `codex/issue-104-f04-http-security-observability`，基于 `main@f1f53aa`，worktree `/Users/wuyongjun/trea/myqypt-worktrees/issue-104-f04-http-security-observability`
- **执行协议:** subagent-driven-development —— 每任务独立 implementer subagent（禁止其再派生 subagent）→ 规格符合性审查 → 代码质量审查 → Critical/Important 修复（≤5 轮 + scoped re-review）→ 全部任务后最强可用模型全分支审查。台账：主仓库 `.superpowers/sdd/issue-104-f04-http-security-observability/progress.md`。

## 范围（In Scope）

1. **中间件包** `internal/transport/http/middleware/`（新建包，系列计划 File Structure 指定）：
   - `requestid.go`：自 `internal/transport/http/requestid.go` 迁入（F02 交付物），并增加**入站 X-Request-ID 格式校验**（F02 台账转发需求）：TrimSpace 后匹配 `^[A-Za-z0-9-_]{1,64}$` 才复用，否则重新生成；出站响应头与 gin context 键行为不变。
   - `security.go`：`Security(SecurityConfig) gin.HandlerFunc` —— 固定安全响应头（HSTS、CSP、nosniff、Referrer-Policy，系列计划测试清单点名的四个）+ 显式 CORS origin allowlist（精确匹配成员资格；preflight 短路 204；凭证模式禁止 `*`）。
   - `accesslog.go`：`AccessLog(*slog.Logger) gin.HandlerFunc` —— 记录 method、path、status、duration_ms、request_id，span 有效时附 trace_id；不记录 Cookie/Authorization/OIDC code/内部身份 Header/请求或响应正文。
   - `recovery.go`：`Recovery(ProblemWriter) gin.HandlerFunc` —— 恢复 panic，经注入的 ProblemWriter 返回稳定 500 `internal_error` Problem；响应不泄露堆栈、panic 值或内部错误；不记录 panic 值（内容最小化，裁定 7）。
2. **路由装配**（`internal/transport/http/router.go` 修改）：中间件按固定顺序安装 request ID → security headers/CORS → tracing（Task 2 引入 otelgin）→ access log → recovery → routes；`Dependencies` 增加可选 `Routes func(*gin.Engine)`（测试 seam，系列计划示例测试要求）、`Logger *slog.Logger`（nil → 不装 access log）、`TracerProvider trace.TracerProvider`（nil → 显式 noop provider，禁用 otel 全局）。problem.go 改用 `middleware.RequestIDFromContext`。
3. **可观测性包** `internal/platform/observability/observability.go`（新建）：`New(ctx, Config) (Resources, error)`；`Resources{Logger *slog.Logger, TracerProvider *sdktrace.TracerProvider, MeterProvider *sdkmetric.MeterProvider, Shutdown func(ctx) error}`；Shutdown 幂等（sync.Once，聚合两个 provider 的 Shutdown 错误）。Resource 属性 `service.name`、`service.version`、`deployment.environment`；OTLP endpoint 仅经 Config 注入，未配置时返回 no-op（无 exporter 的 SDK provider）；Logger 为 JSON slog handler（stdout，测试可注入 writer）。
4. **组合根注入**（`cmd/platform-api/main.go` 修改）：serve 路径经 `observability.New` 构造资源；Config 来自环境变量（`OTEL_EXPORTER_OTLP_ENDPOINT` 空=no-op、`OTEL_SERVICE_NAME` 默认 `platform-api`、`PLATFORM_DEPLOYMENT_ENVIRONMENT` 默认 `development`）；Logger/TracerProvider 注入 `httptransport.Dependencies`；HTTP 优雅关闭完成后调用 `Resources.Shutdown`。构造失败 → 启动失败。
5. **OTel 依赖**（Task 2）：`go.opentelemetry.io/otel`（+`sdk`、`exporters/otlp/otlptrace/otlptracegrpc`、`exporters/otlp/otlpmetric/otlpmetricgrpc`）与 `github.com/open-telemetry/opentelemetry-go-contrib/instrumentation/github.com/gin-gonic/gin/otelgin`，按系列计划 Tech Stack 版本（otel 1.45.0 / otelgin 0.70.0；若代理不可解析则取最近可解析版本并记偏差）。
6. **测试**：见任务拆分与验收矩阵；验收证据入 `docs/evidence/`（README L35 约定）。

## 非目标（Out of Scope）

- 不修改 OpenAPI 契约（`api/openapi/platform.yaml`）与生成代码（`internal/transport/http/api/`）：中间件对契约路径透明。
- 不修改 `web/`、`deploy/compose/`（现有 Casdoor 开发栈）、`deploy/compose.yaml`（F03 冒烟栈）。
- 不修改系列计划文件 `2026-08-24-f04-*.md` 与其他 `2026-08-24-*.md`（sync 管辖）。
- 不做审计流（ADR-0038 的独立 immutable Audit 流属后续 T 系列）、panic 值日志、限流、认证/授权、Higress 边缘策略（F20）、request-ID 之外的 header 校验。
- 不引入全局 logger/tracer runtime（`slog.SetDefault`、otel global `otel.SetTracerProvider` 均禁止——AC5）。
- 不 push、不 merge、不关闭/评论 issue（外部副作用留给用户）。

## 设计裁定（对系列计划的必要澄清与偏差记录）

1. **入站 Request-ID 校验采用 `^[A-Za-z0-9-_]{1,64}$`（F02 台账转发需求），而非系列计划文字 "UUID validation"**：现有生成 ID 为 16 位 hex（非 UUID），严格 UUID-only 会拒掉平台自身生成的合法 ID；转发需求的正则同时兼容系列计划示例测试的 UUIDv7 入站值。裁定以转发需求为准，偏差记录于此。
2. **requestid.go 迁入 `middleware` 包**（系列计划 File Structure 明列 `middleware/requestid.go`；F02 落在父包属当时实现偏差）：依赖方向保持单向 `httptransport → middleware`；`problem.go` 改用 `middleware.RequestIDFromContext`；`RequestID`/`HeaderRequestID` 导出面随之迁移，调用方仅 router.go/problem.go（已核实无其他引用）。
3. **`ProblemWriter` 以最小契约定义在 middleware 包**：`type ProblemWriter func(c *gin.Context, status int, code string)`；router.go 注入适配闭包（调用 `WriteProblem(c, newProblem(status, code))`）。middleware 不 import httptransport（避免环）。
4. **tracing 槽位**：Task 1 不引 OTel 依赖，顺序为 request ID → security → access log → recovery；Task 2 在 security 与 access log 之间插入 otelgin，终态满足固定顺序 "request ID → security headers/CORS → tracing → access log → recovery → routes"。otelgin 显式 `WithTracerProvider`（nil → `trace/noop` 实例），禁止 otel 全局。
5. **`Dependencies.Logger` nil → 不装 access log 中间件**（显式注入哲学；既有测试不产生日志噪声）。`Dependencies.TracerProvider` nil → noop provider 显式传入。
6. **CORS 细节**：`SecurityConfig{AllowedOrigins []string, AllowCredentials bool}`；origin 精确字节匹配（TrimSpace 后）；允许的 preflight（OPTIONS 且带 `Access-Control-Request-Method`）短路 204，回 `Access-Control-Allow-Origin: <origin>`、`Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS`、`Access-Control-Allow-Headers: Content-Type, X-Request-ID`、`Vary: Origin`（凭证模式另加 `Access-Control-Allow-Credentials: true`）；允许的简单跨域请求回 ACAO+Vary（+credentials）；非允许 origin 不回任何 ACAO 头。任何情况不在凭证模式下回 `*`；`AllowedOrigins` 含 `*` 且 `AllowCredentials` 为真的组合由构造期/首请求期拒绝（实现取其一并记录）。
7. **Recovery 不记录 panic 值**：panic 值可能含 secret（AC4 最小化）；访问日志记录 status=500 + request_id 提供可观测信号。审计级 panic 记录留给审计流 T 系列。
8. **MeterProvider 仅装配与 Shutdown**：系列计划 Resources 契约要求其存在；本 Issue 不引入 HTTP 指标插桩（otelgin 只产 span）。记录为有意为之。
9. **`Routes func(*gin.Engine)` 测试 seam**：非 nil 时在中间件装配后调用，供黑盒测试注册 panic 路由；nil → 现行为不变（F01/F02/F03 既有测试零改动通过）。
10. **环境变量名**：OTLP endpoint 读标准 `OTEL_EXPORTER_OTLP_ENDPOINT`（仅组合根读取，中间件/包内不读环境）；service/deployment 属性读 `OTEL_SERVICE_NAME` / `PLATFORM_DEPLOYMENT_ENVIRONMENT`，均有默认值。

## 环境事实（所有 subagent 必须遵守）

- Go 工具链：`PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local`（go1.26.7，darwin/arm64；`/opt/homebrew/bin/go` 已知损坏勿用）。
- 模块下载：`GOPROXY=https://goproxy.cn,direct GOSUMDB=off`（默认 GOPROXY=off 会失败）。
- **禁用 `env -u VAR cmd` 形式**（本机 `~/.local/bin/env` 损坏会静默 no-op 假绿）；需要时用 `unset VAR` + 裸命令。
- 本机已占用端口：3000、3030、6379、9000-9001、9100-9101、15432、50051（WeKnora 栈，勿触）；F04 可用：**55441**（可选临时 PG 回归库）、**18081**（真实进程黑盒）。
- 工作目录：worktree `/Users/wuyongjun/trea/myqypt-worktrees/issue-104-f04-http-security-observability`；gh 命令（如需只读）带 `--repo 1123786563/myqypt`。
- 禁止：push/merge/关闭/评论 GitHub、派生 subagent、改动计划外文件（尤其 `.superpowers/`、`web/`、`api/openapi/`、`internal/transport/http/api/`、`deploy/`、系列计划文件）。

## 任务拆分

### Task 0（controller）：本计划

- 提交本文件：`docs(plan): add issue 104 f04 implementation plan`。

### Task 1：HTTP 中间件契约（impl-1，全新 subagent）

TDD 顺序（先红后绿）：

1. **RED**：`internal/transport/http/middleware/` 包不存在 → 新建黑盒测试（经 `NewRouter`，含 `Dependencies.Routes` seam）覆盖：
   - 生成 request ID（无入站头，响应头非空、`trace_id` 关联）；合法入站 ID（含 UUIDv7 与 16-hex 两种形态）原样保留；**非法入站 ID（超长/非法字符/空格）被替换为新生成 ID**；
   - 安全头四项（HSTS/CSP/nosniff/Referrer-Policy）在 200/404/405/500 响应上存在；
   - CORS：允许 origin 的 preflight 204 + ACAO 回显 + Vary；拒绝 origin 无 ACAO；凭证模式无 `*`；
   - panic 恢复：500 + `application/problem+json` + code `internal_error` + trace_id 等于该请求 request ID + body 不含 panic 值（用含 marker 字符串的 panic 验证）；
   - 现有 `problem_test.go` 等既有断言保持通过（WriteProblem 行为不变）。
   运行 `go test ./internal/transport/http/... -run 'RequestID|Security|CORS|Recovery' -count=1` 确认红。
2. **GREEN**：实现 middleware 包（requestid 迁移+校验、security、accesslog、recovery）+ router.go 固定顺序装配 + Dependencies 演进 + problem.go 改引 middleware。删除旧 `internal/transport/http/requestid.go`（内容已迁移）。
3. **门禁**：聚焦测试 → `go test ./internal/transport/http/... -count=1` → `go test ./internal/transport/http/... -race -count=1` → `go vet ./...` → `gofmt -l .` 空 → `go build ./...` → 全量无 DB `go test ./... -count=1`（2 个 PG 集成用例显式 skip）→ `TestPlatformAPIProcess` 串行单跑。
4. **提交**：`git commit -m "feat(api): harden http middleware"`（单提交）。

### Task 2：显式可观测依赖与组合根注入（impl-2，全新 subagent）

TDD 顺序（先红后绿）：

1. **RED**：`internal/platform/observability` 包不存在 → 新建测试：
   - 内存 slog handler 断言访问日志字段齐全（route/method/status/duration_ms/request_id/trace_id），且 Authorization/Cookie/正文值零出现（用 marker 值请求验证）；
   - OTel span recorder（`sdk/trace/tracetest`）断言 span 属性含 method/route/status 与 request ID 关联，Authorization/Cookie 值不出现；
   - `observability.New` 无 endpoint → no-op（无导出、Shutdown 幂等两次调用）；有 endpoint → 构造 OTLP exporter（不要求真实 collector 连接成功，构造与 Shutdown 路径可测）；
   - Resource 属性 service.name/version/deployment.environment 正确。
   运行 `go test ./internal/platform/observability ./internal/transport/http/... -run Observability -count=1` 确认红。
2. **GREEN**：实现 observability 包；router.go 在 security 与 access log 之间插入 otelgin（显式 TracerProvider）；accesslog 附 trace_id；main.go 组合根注入 + 优雅关闭后 Shutdown。
3. **门禁**：聚焦测试 → `go test ./... -count=1` → `go test ./... -race -count=1` → `go vet ./...` → `gofmt -l .` 空 → `go build ./...` → `TestPlatformAPIProcess` 串行 → `go mod tidy -diff` 空 → `grep -rn 'Authorization\|Cookie' internal/transport/http/middleware internal/platform/observability` 逐条人工复核（应仅出现在测试断言的"不得出现"侧）→ 真实进程黑盒（见矩阵第 12 条）。
4. **提交**：`git commit -m "feat(platform): add slog and otel wiring"`（单提交）。

## 测试与验收矩阵（最终全分支审查逐条重跑）

| # | 命令 | 判据 |
| --- | --- | --- |
| 1 | `gofmt -l .` | 输出为空 |
| 2 | `go vet ./...` | exit 0 |
| 3 | `go build ./...` | exit 0 |
| 4 | `go test ./internal/transport/http/... -run 'RequestID\|Security\|CORS\|Recovery' -count=1` | 全 PASS |
| 5 | `go test ./internal/platform/observability ./internal/transport/http/... -run Observability -count=1` | 全 PASS |
| 6 | `go test ./... -count=1`（无 TEST_DATABASE_URL） | 全 ok，2 个 PG 集成用例显式 skip 消息精确 |
| 7 | `go test ./... -race -count=1` | 全 ok |
| 8 | `go test ./tests/platformtest -run TestPlatformAPIProcess -count=1`（串行） | PASS |
| 9 | `TEST_DATABASE_URL=<临时库> go test ./... -count=1`（controller 提供 55441 临时库；impl/审查各自独立库） | 全 ok，集成用例实跑 |
| 10 | `go mod tidy -diff` | 输出为空 |
| 11 | `git log --oneline main..HEAD` 恰 3 提交（plan + task1 + task2）；`git diff --check` | 数量正确、无空白错误 |
| 12 | 真实进程黑盒：起 `platform-api serve`（无 DB，`PLATFORM_API_ADDR=127.0.0.1:18081`），curl 验证：(a) `/livez` 200 + X-Request-ID 回显 + 四安全头；(b) 未知路径 404 problem + trace_id=回显 ID；(c) 非法入站 X-Request-ID（200 字符）被替换；(d) OPTIONS preflight 允许 origin 204 / 拒绝 origin 无 ACAO；(e) 响应与 stdout JSON 访问日志无 Authorization/Cookie marker | 逐项通过，证据入 `docs/evidence/2026-08-25-issue-104-middleware-blackbox.md` |
| 13 | 秘密最小化扫描：`grep -rn 'Authorization\|Cookie\|token' internal/transport/http/middleware internal/platform/observability --include='*.go'` | 仅测试负断言侧出现，逐条列出复核 |

**Issue 验收标准映射**：AC1（request ID 相关性与 Problem 同 ID）→ Task 1 测试 + 矩阵 4/12(b)；AC2（panic 恢复稳定 500 无泄露）→ Task 1 panic 测试 + 矩阵 4；AC3（安全头与 CORS 自动化测试）→ Task 1 security/CORS 测试 + 矩阵 4/12(a)(d)；AC4（日志/Trace 无敏感内容）→ Task 2 marker 测试 + 矩阵 5/12(e)/13；AC5（组合根显式注入无全局）→ observability 包 + main.go 装配 + 代码审查（禁 `slog.SetDefault`/`otel.SetTracerProvider`/全局单例）+ 矩阵全量。

## 全局约束

- 中间件顺序固定：request ID → security headers/CORS → tracing → access log → recovery → routes（终态）。
- 不记录 Cookie、Authorization、OIDC code、内部身份 Header 或请求/响应正文；panic 值不落日志（裁定 7）。
- CORS origin 为显式配置列表；凭证模式下禁止 `*`。
- 依赖显式注入（组合根）；禁止全局 logger/tracer runtime。
- 每任务单提交、先红后绿、测试与命令留痕（报告记录逐项 verbatim 输出）。
- 既有 F01/F02/F03 测试与行为（/livez 字节、problem 形状、readiness 语义、迁移 CLI）零回归。
