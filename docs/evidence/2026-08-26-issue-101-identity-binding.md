# Issue #101 Identity Binding 黑盒验收证据 — 2026-08-26

- **Worktree:** `/Users/wuyongjun/trea/myqypt-worktrees/issue-101-t012-keycloak-identity-binding`（branch `codex/issue-101-t012-keycloak-identity-binding`）
- **源修订（Task 4 门禁执行时）:** `ee5b629002e1b2854b664a4bf977b022daee42f9`（`git rev-parse HEAD`，Task 4 提交前的 4 提交态）
- **日期偏差说明:** 计划（In Scope 第 9 条）暂写 `2026-08-27-issue-101-identity-binding.md`；按计划自己的「日期以实际执行日为准」条款，本证据以实际执行日 **2026-08-26** 命名（与计划 Task 0 提交日、Task 1–3 执行日一致）。
- **被测对象:** 真实 `platform-api serve` 进程（黑盒，`go build` 产物），`POST /internal/v1/identity/callback`；进程内 httptest OIDC IdP（discovery + JWKS，RS256 手工装配 JWT）；真实 postgres:18。
- **测试容器:** `t012-impl4-pg`（`postgres:18`，镜像 `sha256:1ffbf339f5b8e78c394cfaad3711ef6dbc4e14546bf70428e0bb30cba66e8e4d`，端口 55460）；每次运行在 TEST_DATABASE_URL 服务器上 `CREATE DATABASE identity_blackbox_<pid>` → goose up → 矩阵 → `DROP DATABASE ... WITH (FORCE)`。
- **500/恢复行设计（controller 裁定 (a)）:** 进程级 TCP 代理封闭故障 —— 被测进程的 `DATABASE_URL` 指向测试内的 `net.Listen` 代理，代理把字节拼接转发到真实 postgres；`breakDatabase()` 杀掉已建连接并让新拨号立即关闭（确定性连接拒绝），`restoreDatabase()` 重开转发路径。同一进程、同一 token、同一数据库完成 201 → 500 → 200 恢复闭环，全程自包含、不依赖 docker 编排。

## 工具版本

```
$ go version
go version go1.26.7 darwin/arm64
$ node --version
v26.7.0
$ pnpm --version
11.7.0
postgres image: postgres:18 (sha256:1ffbf339f5b8e78c394cfaad3711ef6dbc4e14546bf70428e0bb30cba66e8e4d)
```

## 依赖声明（AC3）

零新增依赖：`go mod tidy -diff` 输出为空（下方矩阵第 7 行）；`git status --short` 不含 `go.mod`/`go.sum`（下方第 8 行）。JWT 装配与 RS256 验签交互全部位于测试内标准库（`crypto/rsa`、`crypto/sha256`、`encoding/base64`、`encoding/json`）。

## 防伪影：`-list` 非零匹配留痕（裁定 10）

```
$ go test ./cmd/platform-api -list 'TestPlatformAPIIdentity.*' -count=1
TestPlatformAPIIdentityProcess
ok  	github.com/1123786563/myqypt/cmd/platform-api	0.917s

$ go test ./cmd/platform-api -list 'TestPlatformAPIProcess' -count=1
TestPlatformAPIProcess
ok  	github.com/1123786563/myqypt/cmd/platform-api	1.083s
```

两个正则各命中 1 个顶层测试（非空），随后才执行对应的 `-run` 命令。

## Skip 守卫（无 DB 语义，裁定 8）

```
$ unset TEST_DATABASE_URL
$ go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1 -v
=== RUN   TestPlatformAPIIdentityProcess
    identity_process_test.go:60: TEST_DATABASE_URL not set; skipping postgres integration test
--- SKIP: TestPlatformAPIIdentityProcess (0.00s)
PASS
ok  	github.com/1123786563/myqypt/cmd/platform-api	2.535s
```

## 黑盒矩阵结果（AC1/AC2，DSN = 测试容器 55460）

命令：`TEST_DATABASE_URL='postgres://…@localhost:55460/platform?sslmode=disable' go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1 -v` → `--- PASS: TestPlatformAPIIdentityProcess (14.59s)`，`ok github.com/1123786563/myqypt/cmd/platform-api 15.634s`，exit 0。全部 16 条 PASS 行：

```
--- PASS: TestPlatformAPIIdentityProcess (14.59s)
--- PASS: TestPlatformAPIIdentityProcess/ac1_first_bind_201_then_idempotent_rebind_200_same_user (2.10s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes (0.15s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/no_authorization_header (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/non_bearer_scheme (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/token_signed_with_wrong_rsa_key (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/alg_none_token (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/wrong_issuer_claim (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/wrong_audience_claim (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/expired_token (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/not_yet_valid_nbf (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/401_unauthorized_causes/tampered_signature (0.00s)
--- PASS: TestPlatformAPIIdentityProcess/503_dependency_unavailable_when_idp_unreachable (0.12s)
--- PASS: TestPlatformAPIIdentityProcess/503_dependency_unavailable_when_database_unconfigured (0.29s)
--- PASS: TestPlatformAPIIdentityProcess/500_during_database_outage_then_recovery_to_same_user (0.28s)
--- PASS: TestPlatformAPIIdentityProcess/404_when_identity_env_unconfigured (0.07s)
```

矩阵语义（与计划 Task 4 逐条对应）：

- **AC1**：合法 RS256 token（httptest IdP 签发）→ 201，body 恰 `{"user_id":"<uuid>"}`（正则 `^\{"user_id":"[0-9a-f-]{36}"\}$` 锁定）；同一 token 重投 → 200 且 body 与首次逐字节相同；DB 断言该 (issuer, subject) 恰 1 `platform_users` 行 + 1 `identity_bindings` 行且绑定 id 与响应一致。
- **401 全因**（9 子测试）：无 Authorization 头；`Basic xyz` 非 bearer；异键签名；alg=none；错 iss；错 aud；过期 exp；未来 nbf；篡改签名 → 全部 401 `unauthorized` problem。
- **503（IdP 不可达）**：第二进程 issuer 指向已释放端口（连接拒绝）；bearer 为格式合法的 RS256 JWT（格式非法会在任何网络 IO 前 401）→ 503 `dependency_unavailable`。
- **503（DATABASE_URL 未配置）**：identity 已配置但无 DATABASE_URL 的进程 → 路由注册但仓储端口未装配 → 503 fail-closed（裁定 6）。
- **500 + 恢复**：TCP 代理故障窗口内 → 500 `internal_error`；恢复后同一 token → 200 且 user_id 与故障前相同；DB 断言仍恰 1+1 行（无重复业务效果）。
- **404 对照**：未配置 identity env 的第三进程 → POST callback → 404 `not_found`。
- **证据卫生**：矩阵全部响应体对 token 串、subject 值、issuer URL、audience 值做 contains 扫描，零命中；成功体仅含 `user_id`。
- **表总量断言**：矩阵结束 `platform_users` 与 `identity_bindings` 各恰 2 行（AC1 与 500/恢复两个身份），一切被拒投递零落库。

## 防伪造作 round-trip（无 RED 协议的补红证）

临时把 AC1 子测试期望 201 改为 200（仅此一处），运行观察到真实失败，复原后复绿：

```
[broken] go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1
--- FAIL: TestPlatformAPIIdentityProcess (13.67s)
    --- FAIL: TestPlatformAPIIdentityProcess/ac1_first_bind_201_then_idempotent_rebind_200_same_user (2.88s)
        identity_process_test.go:89: first callback status = 201, want 200; body: {"user_id":"3064d0c8-4d7f-4554-862d-ed67fdea1d78"}
FAIL
FAIL	github.com/1123786563/myqypt/cmd/platform-api	15.574s
exit=1

[restored] go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1 -v
--- PASS: TestPlatformAPIIdentityProcess (3.33s)
PASS
ok  	github.com/1123786563/myqypt/cmd/platform-api	4.049s
exit=0
```

## 验收命令矩阵（Task 4 门禁逐条 verbatim）

| # | 命令 | 结果 |
|---|------|------|
| 1 | `bash -n scripts/verify-foundation.sh` | exit 0（`bash-n OK`） |
| 2 | `unset TEST_DATABASE_URL` + `go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1 -v` | exit 0，`--- SKIP` 带精确消息（上方 Skip 守卫节） |
| 3 | `TEST_DATABASE_URL=<55460 容器 DSN> go test ./cmd/platform-api -run '^TestPlatformAPIIdentity' -count=1 -v` | exit 0，16 条 PASS（上方黑盒矩阵节） |
| 4 | `unset TEST_DATABASE_URL` + `go test ./cmd/platform-api -run '^TestPlatformAPIProcess$' -count=1 -v` | exit 0：`--- PASS: TestPlatformAPIProcess (9.33s)` / `ok … 13.169s`（无需 DB） |
| 5 | `bash scripts/verify-foundation.sh --phases INTEGRATION`（unset TEST_DATABASE_URL） | exit 1，`PHASE INTEGRATION FAIL … 112 ms` + JSON 落盘（下方真空拒绝节）——语义保持 |
| 6 | `TEST_DATABASE_URL=<55460 容器 DSN> make verify-foundation` | exit 0，七相位全 PASS（下方） |
| 7 | `unset TEST_DATABASE_URL` + `bash scripts/verify-foundation.sh --phases GENERATE,POLICY,UNIT,CONTRACT,FRONTEND,META` | exit 0，六相位全 PASS（下方） |
| 8 | `unset TEST_DATABASE_URL` + `go test ./... -count=1 -p 1` | exit 0，12 包全 ok（下方） |
| 9 | `go vet ./...` | exit 0 |
| 10 | `gofmt -l .` | 空 |
| 11 | `go build ./...` | exit 0 |
| 12 | `go mod tidy -diff` | 空（零新增依赖） |

### 七相位全 PASS（#6）

```
$ TEST_DATABASE_URL='postgres://…@localhost:55460/platform?sslmode=disable' make verify-foundation
PHASE GENERATE     PASS     327 ms  make generate-check
PHASE POLICY       PASS     643 ms  make policy-check
PHASE UNIT         PASS   14442 ms  go test ./... -count=1 -skip ^TestContract
PHASE CONTRACT     PASS    2205 ms  go test ./internal/transport/http -run ^TestContract -count=1
PHASE INTEGRATION  PASS    8434 ms  go test ./internal/adapter/postgres -count=1 && go test ./cmd/platform-api -run ^TestPlatformAPIIdentity -count=1
PHASE FRONTEND     PASS    5503 ms  pnpm --dir web run typecheck && pnpm --dir web run test && pnpm --dir web run build && pnpm --dir web run verify:static
PHASE META         PASS     306 ms  meta:revision-and-tool-versions
verify-foundation: PASS
exit=0
```

### 无 DB 六相位（#7）

```
$ unset TEST_DATABASE_URL
$ bash scripts/verify-foundation.sh --phases GENERATE,POLICY,UNIT,CONTRACT,FRONTEND,META
PHASE GENERATE     PASS    1346 ms  make generate-check
PHASE POLICY       PASS    1856 ms  make policy-check
PHASE UNIT         PASS   13571 ms  go test ./... -count=1 -skip ^TestContract
PHASE CONTRACT     PASS    2421 ms  go test ./internal/transport/http -run ^TestContract -count=1
PHASE FRONTEND     PASS    4693 ms  pnpm --dir web run typecheck && pnpm --dir web run test && pnpm --dir web run build && pnpm --dir web run verify:static
PHASE META         PASS     328 ms  meta:revision-and-tool-versions
verify-foundation: PASS
exit=0
```

诚实披露：本六相位组合的首次执行中 UNIT 相位出现过一次瞬态 FAIL（该次输出仅保留相位行，失败包详情未留存）；随后同一命令完整重跑 PASS，且裸命令 `go test ./... -count=1 -skip '^TestContract'` 连续 3 次全绿、七相位全量运行中 UNIT 亦 PASS。未能复现，无已知的非确定性来源；以重跑的完整 PASS 记录为准。

### 真空拒绝（#5，INTEGRATION 单相位，unset TEST_DATABASE_URL）

```
$ unset TEST_DATABASE_URL
$ bash scripts/verify-foundation.sh --phases INTEGRATION --json /tmp/t012-impl4-vacuum.json
INTEGRATION: TEST_DATABASE_URL is not set; refusing vacuum acceptance — phase FAILS (no silent skip)
PHASE INTEGRATION  FAIL     112 ms  go test ./internal/adapter/postgres -count=1 && go test ./cmd/platform-api -run ^TestPlatformAPIIdentity -count=1
evidence JSON written: /tmp/t012-impl4-vacuum.json
verify-foundation: FAILED
exit=1
```

JSON 落盘（节选，相位状态 FAIL）：

```json
{
  "schema_version": 1,
  "generated_at": "2026-08-26T07:45:16Z",
  "revision": "ee5b629",
  "tools": {"go": "go1.26.7", "node": "v26.7.0", "pnpm": "11.7.0", "make": "GNU Make 3.81"},
  "phases": [
    {"name": "INTEGRATION", "command": "go test ./internal/adapter/postgres -count=1 && go test ./cmd/platform-api -run ^TestPlatformAPIIdentity -count=1", "status": "FAIL", "duration_ms": 112}
  ]
}
```

### 无 DB 全量 `-p 1`（#8）

```
$ unset TEST_DATABASE_URL
$ go test ./... -count=1 -p 1
ok  	github.com/1123786563/myqypt/cmd/platform-api	2.668s
ok  	github.com/1123786563/myqypt/internal/adapter/oidc	2.192s
ok  	github.com/1123786563/myqypt/internal/adapter/postgres	0.525s
ok  	github.com/1123786563/myqypt/internal/application/identity	0.449s
ok  	github.com/1123786563/myqypt/internal/application/readiness	0.473s
ok  	github.com/1123786563/myqypt/internal/architecture	0.485s
ok  	github.com/1123786563/myqypt/internal/platform/cli	0.450s
ok  	github.com/1123786563/myqypt/internal/platform/observability	2.669s
ok  	github.com/1123786563/myqypt/internal/platform/runtime	0.552s
ok  	github.com/1123786563/myqypt/internal/transport/http	0.560s
ok  	github.com/1123786563/myqypt/internal/transport/http/middleware	0.616s
ok  	github.com/1123786563/myqypt/tests/platformtest	0.475s
exit=0
```

（DB 依赖用例按 skip 守卫显式跳过；`db/migrations` 无测试文件。）

## 零 Secret / 零客户内容声明（AC3）

- 本文档不含任何 token 串、签名材料、claim 值或 DSN 密码：矩阵记录中以占位标记（`<uuid>`、`<55460 容器 DSN>`、`postgres://…`）代替；上方 round-trip 节中的 `user_id` 为本测试自建临时库中测试自生成的一次性 UUID，非任何真实用户数据，且随临时库 DROP 已不存在。
- 被测进程的响应体断言：成功体恰 `{"user_id": ...}`，矩阵全响应体对 token/subject/issuer/audience 零包含（证据卫生子测试）。
- accesslog 中间件不记录 header（既有事实，Issue #104 证据 (e) 已验证），token 不入日志。

## 容器/数据库拆除记录

- 每次矩阵运行的 `identity_blackbox_<pid>` 临时库由测试 cleanup `DROP DATABASE ... WITH (FORCE)`；全部门禁结束后复核 `psql -l` 中 `identity_blackbox` 计数 = 0（服务器仅余 platform/postgres/template0/template1）。
- 测试容器：`docker rm -f -v t012-impl4-pg` → 输出 `t012-impl4-pg`；随后 `docker ps -a` 中 `t012` 前缀容器计数 = 0；端口 55460 无监听（lsof 空）。零残留。
