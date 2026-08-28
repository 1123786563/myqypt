# Issue #26 [T25][P1] Secret Reference Provider — Implementation Plan

- Issue: https://github.com/1123786563/myqypt/issues/26 （OPEN / ready-for-agent / 0 评论；唯一 blocked_by #2 已 CLOSED）
- 源计划：`docs/superpowers/plans/2026-08-24-t25-secret-reference.md`（Issue 正文内嵌，作者拆分）
- Branch: `codex/issue-26-t25-secret-reference` ← base `main@a02c4d6`
- Worktree: `.superpowers/worktrees/issue-26-t25-secret-reference`（本会话沙箱仅允许写会话工作区，标准 worktrees 目录不可写——环境裁定见台账）

## Goal

Platform 只保存 secret_ref，开发环境也不把 Secret 提交到仓库。

## Scope（一个垂直切片）

- 新包 `internal/security/secret-reference`：`SecretReferenceCommand{TenantID, SecretRef, IdempotencyKey}`、`SecretReferenceResult{ResourceID, Outcome}`、端口 `SecretReferencePort`/`Tx`/`EvidenceSink`、构造 `NewSecretReferenceService`、`Execute`（校验先于任何副作用；事务内一次 port 效应 + 一条最小化证据）。
- 具体端口实现（同文件，沿源计划 Step 4 指示）：进程内 managed-secrets 适配器边界，维护幂等键→ResourceID 登记表（重放收敛单效应；外部 ID 先于可重试工作持久化）。
- 旅程三件套：`tests/acceptance/scenarios/t25-secret-reference.yaml` + `t25_secret_reference_driver.go`（新 seam `lighthouse-secret-reference` 经 `platformtest.Register`）+ `t25_secret_reference_test.go`。

## Non-goals

- 不加迁移/schema、不改 `api/openapi`（零契约变更、零 regen）、零 `web/` 改动、不引入 KMS/OpenBao 真适配（ADR-0026：Stage 1 托管密钥先行，本切片只立参考形状）、不触 tenancy/identity 既有代码。

## Design rulings

1. **包位置与新域**：`internal/security/secret-reference` 为应用层新域包，仅依赖标准库（依赖方向单向，policy-check 架构策略必须通过）。
2. **最高可行接缝 = platformtest 旅程（进程内）**：本票无 HTTP 契约（源计划自证：Step 7 直接 `platformtest.Run` 不起栈）——旅程驱动真实服务+具体端口+记录证据 sink，以 `lighthouse-secret-reference` seam 注册；不起 compose 栈。
3. **引用语法守卫（票面不变量的可执行化）**：`SecretRef` 必须匹配 `^[a-z0-9][a-z0-9._/-]{0,199}$`（提供方中立的不透明引用 token）；空/越界/含空白或引号或 base64 团块的输入判 `ErrSecretRefInvalid`（分类错误、先于端口、零证据）。原始 Secret 值天然不满足该语法 → 「Platform 只保存 secret_ref」由契约形状强制。
4. **幂等语义**：同键重放返回同一 ResourceID、提供方效应恰一次（进程内登记表）；异键=新效应。端口失败（`ErrProviderUnavailable` 分类）后重试收敛到单效应、零重复证据。
5. **事务边界**：`Tx.Run` 包裹 port 效应 + 证据写入；任一失败整体回滚语义（进程内 Tx = 直接执行；接口形状为将来真实 DB 事务预留，沿源计划）。
6. **证据最小化**：`EvidenceSink.Record(ctx, idempotencyKey, resourceID, outcome)` 三字符串；旅程断言证据内容零秘密材料/零客户内容（#100 双脱敏在 force）。
7. **仓库卫生断言（票面第二子句可执行化）**：旅程用受控假 Secret 材料（一次性、非真实凭据、仅存在于内存与断言）验证：①被引用语法守卫拒绝且零证据；②对 `git ls-files` 跟踪文件全文扫描零命中（开发环境 Secret 不入库的活体自检）；③旅程自身 evidence JSON 零命中。
8. **错误词表**：`ErrTenantRequired`/`ErrIdempotencyKeyRequired`/`ErrSecretRefInvalid`（先于端口）；`ErrProviderUnavailable`（可重试，来自端口）。sentinel 风格同构 tenancy 包先例。
9. **旅程断言集（YAML name ↔ driver 双锚，harness 强制对账）**：`reject_missing_tenant`、`reject_missing_idempotency_key`、`reject_raw_secret_value`、`accepted_apply_once`、`replay_converges_single_effect`、`port_failure_no_partial_then_retry_converges`、`evidence_content_minimized`、`repo_hygiene_zero_secret_committed`（8 条）。
10. **零 schema/契约变更**：无迁移文件、无 openapi 改动、go.mod 零触碰。

## Task breakdown

- **Task 0（controller）**：本计划，提交 `docs(plan): add issue 26 t25 implementation plan`。
- **Task 1（实施者——本会话为 controller fallback，独立性披露见台账；仍守 RED→green、恰一提交纪律）**：focused 契约测试先红（`service_test.go` 引用不存在词汇 build fail，RED 证据落 `artifacts/evidence/task1/`）→ 实现 `service.go`（含具体端口）→ focused 全绿（校验先于端口零调用/单效应单证据/重放收敛/失败无半成品/引用语法拒/证据最小化）→ 旅程三件套 → 域回归 → 13 门禁。
- **Task 1 双审**：规格符合性 + 代码质量（独立 subagent；本会话不可用→controller 双轴报告 + 醒目披露）。
- **终审**：最强可用模型全分支审查（本会话=controller 自身，披露）。

## Acceptance matrix（13 门禁；审查/终审逐条独立重跑）

环境：`GOTOOLCHAIN=local`、`/Users/wuyongjun/.local/go1.26.7/bin/go`、`GOPROXY=https://goproxy.cn,direct GOSUMDB=off`、禁 `env -u`、`TestPlatformAPIProcess` 在场时 `-p 1`、WeKnora 端口勿触、临时 PG 用 55xxx 毕拆。**本会话沙箱适配**：worktree 位于 `.superpowers/worktrees/`（会话工作区内；标准 worktrees 目录被沙箱拒写）；全部 go 命令 `GOCACHE=/tmp/t25-gocache`（宿主缓存新建文件被拒写，T06 轨道亲证）；需树写的验证在 worktree 内直接可行（工作区内）。

1. `go test ./internal/security/secret-reference -count=1` — focused 全绿。
2. `go test -race -count=1 ./internal/security/secret-reference` — 无竞争。
3. `go test ./tests/acceptance -run TestT25SecretReference -count=1 -v` — 旅程 PASS（无栈门控、不 skip），evidence JSON 落 `artifacts/evidence/t25-secret-reference/`。
4. `go test ./... -count=1 -p 1`（无 DB env） — 全仓绿、T25 旅程运行不 skip。
5. `go vet ./...`、`gofmt -l .`（除 web/）空、`go build ./...`、`go mod tidy -diff` 空。
6. `make generate-check` — 零漂移（本切片零 codegen 触碰的自证）。
7. `make policy-check` — 新包过依赖策略。
8. `bash scripts/verify-foundation.sh`（TEST_DATABASE_URL=临时 PG 55485）— 七相位全 PASS（web/node_modules 以 cp -c 预热）。
9. 旅程证据审计 — JSON passed=true、8/8 断言、details 双脱敏、假 Secret 材料零命中。
10. 提交卫生 — 恰 2 提交（plan+slice）、树净、`git diff --check` 干净。
11. RED 独立复现 — /tmp scratch：base + 仅新测试 → 恰红于新词汇边界；对照基线全绿。
12. 秘密扫描 — 全新增 diff 扫 token/secret/password 模式：仅命中文档散文与公开测试标识符。
13. 零回归 — gates 4/8 + diff 审计（不触任何既有文件；唯一共享接缝 = tests/acceptance 新增三文件，无修改既有文件）。

## Global constraints（Issue #26 原文，逐条守）

- Stage 1 规模包络/单 Region；Tenant 硬边界（命令携带 TenantID，校验先于副作用）；billing 1:1；Product Domain Objects Product 拥有（本切片不涉）；**秘密不入 logs/traces/metrics/Audit/Usage/fixtures/evidence**（ruling 3/6/7 可执行化）；compose 限开发/CI/受控 beta；99.9%/RPO/RTO 目标（本切片不涉）；聚焦单测不替代具名验收接缝（ruling 2 旅程=具名接缝）；依赖图完备（#2 CLOSED 亲验）。
