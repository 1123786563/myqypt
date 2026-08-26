# Issue #105 F05 Task 2 架构门禁黑盒证据 — 2026-08-26

- **Worktree:** `/Users/wuyongjun/trea/myqypt-worktrees/issue-105-f05-evidence-dependency-gate`（branch `codex/issue-105-f05-evidence-dependency-gate`，基线 `1c80d48`）
- **工具链:** go1.26.7（`GOTOOLCHAIN=local`）、node v26.7.0、pnpm 11.7.0、GNU Make 3.81、Docker 29.x（postgres:18 本地镜像）
- **临时 PG:** 容器 `f05-impl2-pg-55446`（`postgres:18`，`127.0.0.1:55446`，`POSTGRES_PASSWORD=postgres`），DSN `postgres://postgres:postgres@127.0.0.1:55446/postgres?sslmode=disable`，仅经 `TEST_DATABASE_URL` 传入
- **证据文件落盘:** `artifacts/foundation-verification.json`（被 `.gitignore` 覆盖，不入库；schema 遵设计裁定 9：零 env 值、零 DSN、零 Token，命令为静态目标名）

## ⑦ `make test-foundation`（三相位分别报告，临时 PG 55446）

```
$ TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55446/postgres?sslmode=disable' make test-foundation
PHASE UNIT         PASS   10133 ms  go test ./... -count=1 -skip ^TestContract
PHASE CONTRACT     PASS    1058 ms  go test ./internal/transport/http -run ^TestContract -count=1
PHASE INTEGRATION  PASS     998 ms  go test ./internal/adapter/postgres -count=1
evidence JSON written: artifacts/foundation-verification.json
verify-foundation: PASS
```

反假绿补充（裁定 10）：INTEGRATION 相位同一临时库上 `-v` 留痕，迁移/健康测试真实执行、零 SKIP：

```
$ TEST_DATABASE_URL='...' go test ./internal/adapter/postgres -count=1 -v
=== RUN   TestHealthCheckerTracksMigrationState
--- PASS: TestHealthCheckerTracksMigrationState (0.08s)
=== RUN   TestUnconfiguredCheckerAlwaysFails
--- PASS: TestUnconfiguredCheckerAlwaysFails (0.00s)
=== RUN   TestMigrationRoundTrip
--- PASS: TestMigrationRoundTrip (0.03s)
=== RUN   TestMigrateRequiresConnection
--- PASS: TestMigrateRequiresConnection (0.00s)
=== RUN   TestMigrateMalformedDSNDoesNotEchoURL
--- PASS: TestMigrateMalformedDSNDoesNotEchoURL (0.00s)
PASS
ok  	github.com/1123786563/myqypt/internal/adapter/postgres	0.739s
```

## ⑧ 无 `TEST_DATABASE_URL` → INTEGRATION 相位 FAIL（真空验收被拒）且 JSON 落盘

```
$ unset TEST_DATABASE_URL; make verify-foundation
PHASE GENERATE     PASS     408 ms  make generate-check
PHASE POLICY       PASS     836 ms  make policy-check
PHASE UNIT         PASS    8131 ms  go test ./... -count=1 -skip ^TestContract
PHASE CONTRACT     PASS    1075 ms  go test ./internal/transport/http -run ^TestContract -count=1
INTEGRATION: TEST_DATABASE_URL is not set; refusing vacuum acceptance — phase FAILS (no silent skip)
PHASE INTEGRATION  FAIL       6 ms  go test ./internal/adapter/postgres -count=1
PHASE FRONTEND     PASS    9530 ms  pnpm --dir web run typecheck && pnpm --dir web run test && pnpm --dir web run build && pnpm --dir web run verify:static
PHASE META         PASS     210 ms  meta:revision-and-tool-versions
evidence JSON written: artifacts/foundation-verification.json
verify-foundation: FAILED
```

判定：✅ 总体非零退出；INTEGRATION 相位因 DSN 缺失直接 FAIL（无 `t.Skip` 漂绿）；证据 JSON 仍落盘并记录该 FAIL（摘录）：

```
    {"name": "INTEGRATION", "command": "go test ./internal/adapter/postgres -count=1", "status": "FAIL", "duration_ms": 6},
```

## ⑨ 带 `TEST_DATABASE_URL` → `make verify-foundation` 全相位 PASS

```
$ TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55446/postgres?sslmode=disable' make verify-foundation
PHASE GENERATE     PASS     434 ms  make generate-check
PHASE POLICY       PASS    1969 ms  make policy-check
PHASE UNIT         PASS    8035 ms  go test ./... -count=1 -skip ^TestContract
PHASE CONTRACT     PASS     947 ms  go test ./internal/transport/http -run ^TestContract -count=1
PHASE INTEGRATION  PASS    1294 ms  go test ./internal/adapter/postgres -count=1
PHASE FRONTEND     PASS    2981 ms  pnpm --dir web run typecheck && pnpm --dir web run test && pnpm --dir web run build && pnpm --dir web run verify:static
PHASE META         PASS     207 ms  meta:revision-and-tool-versions
evidence JSON written: artifacts/foundation-verification.json
verify-foundation: PASS
```

### ⑨ 执行异常（诚实披露）：既有测试 `TestPlatformAPIProcess` 负载时序 flaky

`go test ./...` 全套并行下，F04 既有测试 `cmd/platform-api` 的 `TestPlatformAPIProcess`（5s 进程启动预算：`processStartupTimeout = 5 * time.Second`，25ms 轮询地址文件）在本机间歇性失败：

```
--- FAIL: TestPlatformAPIProcess (5.77s)
    main_test.go:30: platform-api did not report its address within 5s
    main_test.go:115: cleanup wait for platform-api: signal: killed
FAIL
FAIL	github.com/1123786563/myqypt/cmd/platform-api	6.487s
```

- 通过的完整套件运行：门禁②（无 DSN）、③（-race 无 DSN）、⑦ UNIT（带 DSN）、⑧ UNIT（无 DSN）、隔离直跑 1（带 DSN）、⑨ 第 4 次尝试（带 DSN，上表）。失败运行：⑨ 首跑、⑨ 重跑、⑨ 三跑、隔离直跑 2 —— 症状全部相同。
- 聚焦运行（门禁⑫）稳定 PASS（1.76s，余量充足），证明测试本身与本任务改动无关（`cmd/` 零触碰；`internal/architecture` 不被任何运行时包 import）。
- 处置：按原命令重试至全相位 PASS（第 4 次尝试，上表即为留痕），未改动任何被禁文件。该 flaky 属 F04 遗留时序敏感问题，建议后续 issue 放宽预算或降低并行度（超出本任务范围）。

## ⑬ 禁止依赖篡改黑盒：/tmp scratch 克隆注入 gorm import → exit 1 + ARCH-GORM + 文件:行号

scratch 克隆（`git clone --shared --no-checkout` 本 worktree，`git checkout 1c80d48`）后复制本任务 policy 工具（`internal/architecture/dependency_policy.go`、`dependency_policy_test.go`、`testdata/dependency-policy/`、`Makefile`、`scripts/check-frontend-policy.mjs`），注入：

```go
// /tmp/f05-tamper/internal/application/readiness/gorm_tamper.go
package readiness

// Blackbox tamper: injected forbidden import.
import _ "gorm.io/gorm"
```

运行 `make policy-check`：

```
$ cd /tmp/f05-tamper && make policy-check
--- FAIL: TestDependencyPolicyRepoScan (0.01s)
    dependency_policy_test.go:214: real repository must scan clean, got 1 violation(s):
        ARCH-GORM: internal/application/readiness/gorm_tamper.go:4: import "gorm.io/gorm" matches forbidden prefix "gorm.io/gorm" (rule ARCH-GORM)
FAIL
FAIL	github.com/1123786563/myqypt/internal/architecture	0.484s
FAIL
make: *** [policy-check] Error 2
```

判定：✅ 唯一失败测试即仓库扫描；输出含规则 ID `ARCH-GORM`、文件 `internal/application/readiness/gorm_tamper.go`、行号 `:4`。黑盒毕即删（`/tmp/f05-tamper` 已移除）。

## 临时容器拆除记录（用毕即拆，零残留）

```
$ docker rm -f f05-impl2-pg-55446
f05-impl2-pg-55446
$ docker ps -a --filter name=f05-impl2 --format '{{.Names}} {{.Status}}' | wc -l
       0
$ docker volume ls --filter name=f05-impl2 --format '{{.Name}}' | wc -l
       0
$ lsof -i :55446
(无输出, exit 1 — 端口空闲)
```

判定：✅ 容器 0 残留、卷 0 残留、端口 55446 释放。
