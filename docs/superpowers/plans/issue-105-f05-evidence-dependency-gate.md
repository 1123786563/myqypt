# Issue #105 [F05][Foundation] Go 证据与依赖门禁 实施计划

- **Issue:** https://github.com/1123786563/myqypt/issues/105 （[F05][Foundation] Go 证据与依赖门禁，OPEN，0 评论）
- **系列计划（事实源，不修改）：** `docs/superpowers/plans/2026-08-24-f05-evidence-dependency-gate.md`（与 issue 正文 Implementation plan 段一致，已核对）
- **设计规格：** `docs/superpowers/specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md` §§3（上游基线）、5.4（前端拒绝清单）、6.4（后端拒绝清单）、11（目标目录）
- **前置依赖：** F02=#102、F03=#103、F04=#104 全部 CLOSED 且已合并 main（`2d6648c` 已含）→ 依赖满足（gh 亲验）。
- **分支/Worktree:** `codex/issue-105-f05-evidence-dependency-gate`，基于 `main@2d6648c`，worktree `/Users/wuyongjun/trea/myqypt-worktrees/issue-105-f05-evidence-dependency-gate`
- **执行协议:** subagent-driven-development —— 每任务全新 implementer subagent（禁止其再派生 subagent、禁止读主仓库 `.superpowers/`）→ 独立规格符合性审查 + 独立代码质量审查 → Critical/Important 修复（≤5 轮 + scoped re-review）→ 全部任务后最强可用模型全分支审查。台账：主仓库 `.superpowers/sdd/issue-105-f05-evidence-dependency-gate/progress.md`。
- **提交纪律：** 全分支恰 3 个提交 —— 计划 + `docs: record upstream extraction provenance`（Task 1）+ `build: add foundation architecture gates`（Task 2）。

## 范围（In Scope）

1. **Provenance manifest** `docs/upstream/provenance.yaml`（新建）：两上游（shadcn-admin、go-admin）机器可读记录 —— repository URL、40-hex commit、SPDX license、许可证文本文件路径、抽取模式、复制源路径、本地目标路径、本地修改摘要（schema 见裁定 1）。
2. **许可证文本** `LICENSES/shadcn-admin-MIT.txt`、`LICENSES/go-admin-MIT.txt`（新建）：与上游对应 commit 的 LICENSE 文件逐字节一致（经 gh api 只读核对；shadcn-admin 文件名 `LICENSE`，go-admin 文件名 `LICENSE.md`）。
3. **THIRD_PARTY_NOTICES.md**（已存在，F06 产物）：**外科手术式扩展** —— 新增 go-admin 段（commit/许可证/采纳范围 §3 可转移能力/拒绝范围 §6.4/零逐字复制声明），既有 shadcn-admin 段与 shadcn/ui Button 段原样保留，仅在 MIT 段补两个 LICENSES 文件指针。
4. **Provenance 校验器** `internal/architecture/`（新建包）：`provenance.go` + `provenance_test.go` —— 解析 manifest 并按裁定 2 规则集校验（RED 先行：包不存在 → 编译红）。
5. **禁止依赖门禁**（Task 2）：
   - `internal/architecture/dependency_policy.go` + `dependency_policy_test.go` + 夹具 `internal/architecture/testdata/dependency-policy/`：Go import 扫描（裁定 3 规则表）+ 生产文件内容扫描（裁定 4 规则表），生成文件仅按显式路径豁免（裁定 5）。
   - `scripts/check-frontend-policy.mjs`（仓库根 scripts/，新建）：依赖级扫描（web/package.json + web/pnpm-lock.yaml，裁定 6）+ 源级委托 F06 既有 `web/scripts/check-forbidden-content.mjs`（只读调用，不修改 web/ 任何文件）。
   - `Makefile`（新建）：`generate-check` / `policy-check` / `test-foundation`（unit、contract、integration 三相位分别执行报告，裁定 7）/ 聚合 `verify-foundation`。
   - `scripts/review-upstream-update.sh <source> <old> <new> [--record <file>]`（新建）：只读 gh api 差异摘要 + 六项评审复选框全勾前 exit 1，绝不 fetch/merge/同步上游（裁定 8）。
   - `scripts/verify-foundation.sh`（新建）：串行执行 GENERATE/POLICY/UNIT/CONTRACT/INTEGRATION/FRONTEND(+META) 相位，产出 `artifacts/foundation-verification.json`（仅 命令/状态/耗时 + revision 与工具版本，零 env 值零 secret，裁定 9）。
   - `.gitignore` 增加 `artifacts/*`（保留 `.gitkeep`）；新建 `artifacts/.gitkeep`。
6. **验收证据**：`docs/evidence/2026-08-26-issue-105-foundation-gates.md`（README L35 约定；文件名日期以实际执行日为准），逐门禁 verbatim 留痕。

## 非目标（Out of Scope）

- 不修改 `web/` 任何文件（check-frontend-policy 只读消费；`web/node_modules` 缺失时 FRONTEND 相位 FAIL 并附安装提示，不自动改环境）。
- 不修改 `api/openapi/`、`internal/transport/http/`、`internal/` 既有包、`deploy/`、`cmd/`、`db/`（既有交付物零触碰；server.gen.go 仅作为被豁免路径被扫描读取）。
- 不修改系列计划文件 `2026-08-24-*.md` 与其他既有计划/规格文件（sync 管辖）。
- 不做上游同步/回灌、不做 CI（.github/）接线、不引入 SPDX 工具链（SPDX 仅作为表达式字符串出现在 manifest 字段）。
- 门禁工具只观察源码，不成为运行时代码（系列计划 Self-Review 硬约束）：`internal/architecture` 不得被任何非测试包 import。
- 不 push、不 merge、不关闭/评论 issue（外部副作用留给用户）。

## 设计裁定（对系列计划的必要澄清与偏差记录）

1. **manifest schema 以 `extraction_mode` 显式区分两类抽取**：`verbatim`（逐字复制，destinations 必填且须在仓库内）与 `pattern-only`（模式抽取，destinations 必空但 `local_modifications` 必非空）。两上游当前均为 pattern-only、零逐字复制（规格 §3"可转移能力"均为思路/方法）；shadcn/ui Button 逐字复制件是第三上游，**不属本 manifest 两源清单**（其记录仅存于 THIRD_PARTY_NOTICES.md 既有段）。顶层 `schema_version: 1`。
2. **Provenance 校验规则集（稳定规则 ID）**：`PROV-SCHEMA-VERSION`、`PROV-SOURCE-EMPTY`、`PROV-NAME`（空/重复）、`PROV-REPOSITORY`（非 https URL）、`PROV-COMMIT`（非 40 位小写 hex）、`PROV-LICENSE`（非 SPDX MIT）、`PROV-LICENSE-FILE-MISSING`/`PROV-LICENSE-FILE-EMPTY`（文件缺失/空）、`PROV-LICENSE-TEXT`（不含 "MIT License" 与 "Copyright"）、`PROV-MODE`（非法枚举）、`PROV-DEST-FORBIDDEN-MODE`（pattern-only 却有 destinations）、`PROV-DEST-MISSING`（verbatim 的 destination 在仓库不存在）、`PROV-DEST-OUTSIDE-REPO`（绝对路径/含 `..`）、`PROV-DEST-EMPTY`（verbatim 却无 destinations）、`PROV-MODIFICATIONS-EMPTY`（摘要为空）。"仓库存在未申报衍生文件"不可机械反推，归评审责任（系列计划语义如此）。YAML 解析用既有直接依赖 `gopkg.in/yaml.v3`（go.mod L91 已在，零新增）。
3. **Go import 规则表（前缀匹配，稳定 rule ID）**：`gorm.io/gorm`→`ARCH-GORM`；`github.com/casbin`→`ARCH-CASBIN`；`github.com/swaggo`→`ARCH-SWAGGO`；`github.com/go-admin-team/go-admin`→`ARCH-GO-ADMIN`（覆盖上游 `common/global` 与 `common/apis`/`common/actions` 全部子路径）；`github.com/golang-jwt`、`github.com/dgrijalva/jwt-go`→`ARCH-UPSTREAM-JWT`。import 规则扫描**全部**非豁免 .go 文件（含 `_test.go` 与 tools.go）。
4. **内容规则表（仅扫非 `_test.go` 生产文件，逐行匹配）**：`ARCH-DEFAULT-CREDENTIALS`（子串 `admin123`、`123456`、`password123`；标识符 `DefaultAdmin`、`DefaultPassword`；不区分大小写短语 `default admin`、`default password`、`默认管理员`、`默认密码`）；`ARCH-HOST-TENANT`（标识符正则 `[Tt]enantFromHost|[Hh]ost[Tt]enant`；不区分大小写短语 `host-based tenant`）。测试字面量豁免依据：F02 交付物 `requestid_test.go` 的 `0123456789abcdef` 含 "123456" 子串 —— 内容规则只扫生产文件（系列计划 §6.4"默认管理员、默认密码和演示数据"语义）。
5. **豁免仅显式路径**：生成文件豁免清单 = 恰一个条目 `internal/transport/http/api/server.gen.go`；夹具目录豁免 = 任何含 `testdata` 路径段的目录（Go 工具链不编译 testdata，属语言语义而非本仓库随意豁免）。扫描器以参数化（root, exemptions）实现，夹具测试可用受控相对路径验证豁免逻辑本身。
6. **前端策略分层**：依赖级（`scripts/check-frontend-policy.mjs` 解析 package.json 全部依赖段 + pnpm-lock.yaml 包名，规则 `FRONTEND-DEP-CLERK`：包名匹配 `/clerk/i`）+ 源级委托（`node web/scripts/check-forbidden-content.mjs`，F06 既有 clerk/token-key/brand 等规则，FAIL 归类 `FRONTEND-SOURCE-CONTENT`）。脚本支持 `--fixture <dir>` 夹具模式供 RED 测试，不触碰真实 web/ 源。
7. **verify-foundation 相位与 test-foundation 拆分**：`test-foundation` = 三相位分别执行分别报告 —— UNIT `go test ./... -count=1 -skip '^TestContract'`、CONTRACT `go test ./internal/transport/http -run '^TestContract' -count=1`、INTEGRATION `go test ./internal/adapter/postgres -count=1`（**TEST_DATABASE_URL 未设 → INTEGRATION 相位直接 FAIL（拒绝真空验收，AC1），不由 t.Skip 静默漂绿**）。`verify-foundation` = GENERATE（make generate-check）→ POLICY（make policy-check）→ UNIT → CONTRACT → INTEGRATION → FRONTEND（web typecheck+test+build+verify:static；node_modules 缺 → FAIL 附 `pnpm --dir web install` 提示）→ META（revision/工具版本）。健康检查类测试（/livez 等）不构成任何相位的充分条件（AC1）。
8. **review-upstream-update.sh 只读 + 六框记录**：`--record <file>` 需恰含六行全勾复选框（`authentication`/`tenant`/`network`/`storage`/`copied-code`/`security-advisory`）；无文件或有未勾项 → exit 1 分类 `REVIEW-INCOMPLETE`。diff 摘要经 `gh api repos/<owner>/<repo>/compare/<old>...<new>`（只读），source 名与 commit 先对 manifest 校验（`PROV-*` 复用语义）。绝不 fetch/merge/同步上游分支。
9. **证据最小化（AC4）**：`artifacts/foundation-verification.json` 仅含 `{schema_version, generated_at, revision, tools:{go,node,pnpm,make}, phases:[{name, command, status, duration_ms}]}`——零 env 值、零 DSN、零 Token/Cookie、零用户数据；命令字符串为静态 make/go 目标名。
10. **验收命令路径勘误预防（沿 #104 教训）**：`TestPlatformAPIProcess` 在 `./cmd/platform-api`；计划矩阵命令一律要求非空执行留痕（`-v` 或 ok 行），防 "no tests to run" 假绿；正则形态命令需先 `-list` 验证非零匹配。

## 环境事实（所有 subagent 必须遵守）

- Go 工具链：`PATH=/tmp/issue100-task2-go1267-retry.E59JCp/go/bin:$PATH GOTOOLCHAIN=local`（go1.26.7；`/opt/homebrew/bin/go` 已知损坏勿用）。模块下载：`GOPROXY=https://goproxy.cn,direct GOSUMDB=off`。
- **禁用 `env -u VAR cmd` 形式**（本机 `~/.local/bin/env` 损坏会静默 no-op 假绿）；用 `unset VAR` + 裸命令。
- pnpm 11.7.0（web/package.json packageManager 钉死；系列计划写 11.1.2 记为版本偏差，以仓库为准）、node v26.7.0、`/usr/bin/make`、docker 可用（postgres:18 本地已有先例）。
- 本 worktree `web/node_modules` 缺失：FRONTEND 相位前需手动 `pnpm --dir web install`（web/.gitignore 覆盖产物，不脏树）。
- 端口：占用勿触 3000/3030/6379/9000-9001/9100-9101/15432/50051（WeKnora）；F05 分配：**impl-2 临时 PG=55446**、Task 2 规格审=55448、质量审=55449、终审=55450（用前核实空闲，用毕拆除容器）。
- gh 已认证（只读用途：许可证文本、compare API）；带 `--repo 1123786563/myqypt`。
- 工作目录：本 worktree。禁止：push/merge/关闭/评论 GitHub、派生 subagent、读主仓库 `.superpowers/` 台账、改动计划外文件（`web/`、`api/openapi/`、`internal/transport/http/`、`deploy/`、`cmd/`、`db/`、系列计划文件）。
- 主仓库 main 工作区有用户自己的未提交 AGENTS.md 编辑（与 Issue 无关，不触碰）。

## 任务拆分

### Task 0（controller）：本计划

- 提交本文件：`docs(plan): add issue 105 f05 implementation plan`。

### Task 1：来源证据（provenance/LICENSES/NOTICES）（impl-1，全新 subagent）

- RED：先写 `internal/architecture/provenance_test.go`（表驱动，每条 `PROV-*` 规则至少一正一反用例 + 真实 manifest/LICENSES 的 happy-path 全绿用例），跑 `go test ./internal/architecture -run Provenance -count=1` 确认红（包不存在）。
- GREEN：`provenance.go` 实现校验器（裁定 2 规则集；错误信息含规则 ID 与定位字段）；`docs/upstream/provenance.yaml`、`LICENSES/*.txt`（gh api 逐字节核对上游）、`THIRD_PARTY_NOTICES.md` go-admin 段。
- 门禁（全部 verbatim 留痕）：①聚焦 Provenance ②`go test ./... -count=1`（无 DB）③同前 `-race` ④`go vet ./...` ⑤`gofmt -l .` 空 ⑥`go build ./...` ⑦`go mod tidy -diff` 空 ⑧`git diff --stat` 确认零 web//api/openapi//internal/transport/http//deploy/ 改动。
- 单提交 `docs: record upstream extraction provenance`。

### Task 2：架构门禁与统一验证入口（impl-2，全新 subagent）

- RED：夹具 `testdata/dependency-policy/`（每规则一文件：gorm/casbin/swaggo/go-admin(含 common/global 路径)/两种 jwt import；default-credentials 与 host-tenant 内容各一；豁免路径一）+ `dependency_policy_test.go`（断言每夹具报出 文件+ruleID；真实仓库源零违规；豁免逻辑参数化验证；`_test.go` 内容豁免验证）→ 红。`scripts/check-frontend-policy.mjs --fixture` 夹具（clerk 依赖 + clerk 源引用）→ exit 1 红。`scripts/review-upstream-update.sh` 无 --record → exit 1 红。
- GREEN：`dependency_policy.go` 扫描器（go/parser 提 import + 逐行内容规则；裁定 3/4/5）；`check-frontend-policy.mjs`（裁定 6）；`Makefile` 四目标（裁定 7）；`review-upstream-update.sh`（裁定 8）；`verify-foundation.sh`（裁定 9）+ `.gitignore`/`artifacts/.gitkeep`。
- 门禁（全部 verbatim 留痕）：①`go test ./internal/architecture -count=1` ②`go test ./... -count=1`（无 DB）③同前 `-race` ④vet/gofmt/build/tidy -diff ⑤`make generate-check` ⑥`make policy-check` ⑦临时 PG（55446）上 `make test-foundation`（三相位分别报告）⑧`make verify-foundation` 无 TEST_DATABASE_URL → INTEGRATION FAIL 且 JSON 落盘 ⑨同前带临时 PG → 全相位 PASS ⑩`node --check` 两个 .mjs ⑪`bash -n` 两个 .sh ⑫TestPlatformAPIProcess（`./cmd/platform-api`，#104 勘误沿用）⑬禁止依赖篡改黑盒：/tmp scratch 克隆注入 `gorm.io/gorm` import → make policy-check exit 1 且输出 ARCH-GORM+文件行号 ⑭`git diff --stat` 零 web/ 改动、artifacts/ 仅 .gitkeep 入库。
- 黑盒证据：`docs/evidence/2026-08-26-issue-105-foundation-gates.md`（⑦⑧⑨⑬ verbatim + 容器拆除记录；日期以实际执行日为准）。
- 单提交 `build: add foundation architecture gates`。

### 全部任务后：最终全分支审查（最强可用模型，全新独立上下文）

- 矩阵 16 条逐条重跑（见下）；AC1-AC5 逐项映射证据；规格 §§3/5.4/6.4/11 覆盖核对；F01-F04 零回归。

## 验收矩阵（最终审查逐条重跑）

| # | 条件 | 命令 / 证据 |
| --- | --- | --- |
| 1 | Provenance 聚焦测试 PASS（先红后绿） | `go test ./internal/architecture -run Provenance -count=1` |
| 2 | manifest 两上游钉死 40-hex commit | `docs/upstream/provenance.yaml` + 校验器 |
| 3 | LICENSES 文本与上游逐字节一致 | `gh api .../contents/LICENSE?ref=<commit>` diff 为空 |
| 4 | NOTICES 记录 go-admin commit/许可证/抽取范围/本地修改 | THIRD_PARTY_NOTICES.md go-admin 段 |
| 5 | 无 DB 全量测试 PASS | `go test ./... -count=1` |
| 6 | race 全量 PASS | `go test ./... -race -count=1` |
| 7 | vet/gofmt/build/tidy 干净 | 四命令 exit 0 / 输出空 |
| 8 | 架构门禁聚焦 PASS（每夹具断言文件+ruleID） | `go test ./internal/architecture -count=1` |
| 9 | 生成物干净 | `make generate-check` |
| 10 | 策略门禁 PASS | `make policy-check` |
| 11 | 三相位分别执行报告（带临时 PG） | `make test-foundation`（55446） |
| 12 | 真空验收被拒 | 无 TEST_DATABASE_URL `make verify-foundation` → INTEGRATION FAIL |
| 13 | 全相位 PASS + 证据 JSON | 带 PG `make verify-foundation`；artifacts/foundation-verification.json 含 revision/工具版本、零 secret |
| 14 | 上游更新评审门禁 | review-upstream-update.sh：无 record exit 1；全勾 record exit 0；全程只读 |
| 15 | 禁止依赖篡改被拦截 | /tmp scratch 注入 gorm import → exit 1 + ARCH-GORM + 文件:行号 |
| 16 | F01-F04 零回归 + 零越界改动 | `./cmd/platform-api -run TestPlatformAPIProcess`；`git diff --stat main..HEAD` 全部改动在计划内文件清单 |

## 全局约束（系列计划 Global Constraints，逐条落实）

- 固定 `shadcn-admin@e16c87f213a5ba5e45964e9b67c792105ec74d26` 与 `go-admin@1b7dcd843ce38fddc8c280fe3139e02735cf7574`；上游更新只走 review-upstream-update.sh 独立审查（裁定 8）。
- 复制的实质代码保留 MIT notice（LICENSES 双文件 + NOTICES 段；当前零逐字复制，门禁防未来引入时失记）。
- 证据不含 Token、Cookie、DSN 或用户数据（裁定 9；审计留痕含 grep 证明 JSON 无 env 值）。
- 门禁对 Clerk、JWT、Casbin、GORM、Swaggo、Host Tenant 与全局 runtime（go-admin common/global 前缀）失败（裁定 3/4/6）。
