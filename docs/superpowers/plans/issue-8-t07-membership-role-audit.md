# Issue #8 [T07][P4] Membership 与 Role Audit — Implementation Plan

- Issue: https://github.com/1123786563/myqypt/issues/8 （OPEN / ready-for-agent / 0 评论；唯一 blocked_by #6 已 CLOSED 且已并入 main@fea557c/a02c4d6）
- 源计划：`docs/superpowers/plans/2026-08-24-t07-membership-role-audit.md`（Issue 正文内嵌，作者拆分；本文件为其在本轨道的落位与裁定）
- Branch: `codex/issue-8-t07-membership-role-audit` ← base `main@a02c4d6`
- Worktree: `.superpowers/worktrees/issue-8-t07-membership-role-audit`（会话沙箱仅允许写会话工作区——环境裁定见台账）

## Goal

邀请、激活、拒绝、角色变更和移除产生不可变 Audit Event。

## Scope（一个垂直切片）

- 新包 `internal/identity/membership-role-audit`：`MembershipRoleAuditCommand{TenantID, MembershipID, Action, RoleBefore, RoleAfter, IdempotencyKey}`、`MembershipRoleAuditResult{ResourceID, Outcome}`、端口 `MembershipRoleAuditPort`/`Tx`/`EvidenceSink`、构造 `NewMembershipRoleAuditService`、`Execute`（校验先于任何副作用；事务内一次 port 效应 + 一条最小化证据）。
- 具体端口实现（同文件，沿源计划 Step 4 指示）：进程内不可变 Audit 台账适配器（ADR-0041 的 Stage-1 形状），幂等键→事件登记表先于任何可重试工作持久化，重放收敛单事件。
- 旅程三件套：`tests/acceptance/scenarios/t07-membership-role-audit.yaml` + `t07_membership_role_audit_driver.go`（新 seam `lighthouse-membership-role-audit` 经 `platformtest.Register`）+ `t07_membership_role_audit_test.go`。
- 白盒效应测试 `provider_internal_test.go`（沿 t25 先例；台账不可变形的内部视角证明）。

## Non-goals

- 不加迁移/schema、不改 `api/openapi`（零契约变更、零 regen）、零 `web/` 改动、不接 OpenFGA/真实审计存储（T08–T11 后续票）、不触 tenancy/identity 既有代码、不改 T05 membership 行为（本切片是旁挂的 Audit 事件生产，不改写邀请/激活业务路径）。

## Design rulings

1. **包位置与新域**：`internal/identity/membership-role-audit` 为应用层新域包，仅依赖标准库（依赖方向单向，policy-check 架构策略必须通过）。命名沿源计划（`internal/identity` 顶层新目录，先例 = t25 的 `internal/security`）。
2. **最高可行接缝 = platformtest 旅程（进程内）**：本票无 HTTP 契约（源计划自证：Step 7 直接 `platformtest.Run` 不起栈）——旅程驱动真实服务+具体端口+记录证据 sink，以 `lighthouse-membership-role-audit` seam 注册；不起 compose 栈。
3. **五动作封闭词表（票面不变量的可执行化）**：`Action ∈ {invite, activate, reject, role_change, remove}`（邀请、激活、拒绝、角色变更、移除）。空 `MembershipID`、未知 `Action`、`role_change` 缺 RoleBefore/RoleAfter → 分类 sentinel、先于端口、零事件零证据。
4. **不可变性强制（ADR-0041 内容最小化不可变审计流）**：具体端口 = append-only 台账；同幂等键重放返回原事件 ID、outcome `duplicate`、**不覆写**；同键异载荷重放同样收敛于原事件（载荷分歧不产生第二事件、不改变已存内容——不可变性的黑盒证明）；台账类型不暴露任何变更路径（白盒测试证明存储内容在重放后逐字节不变）。事件字段最小化：authority/tenant/action/resource/decision 等轴以非敏感稳定标识表示，无 secrets/无原始载荷。
5. **幂等语义**：登记表幂等键→事件 ID 先于可重试工作持久化；首投 `accepted`（一事件、一证据行），重投 `duplicate`（零新事件）；异键=新事件。端口失败（`ErrAuditUnavailable` 分类）后零半成品，重试收敛单事件。
6. **事务边界**：`Tx.Run` 包裹 port 效应 + 证据写入；任一失败整体回滚语义（进程内 Tx = 直接执行；接口形状为将来真实 DB 事务预留，沿源计划）。
7. **证据最小化**：`EvidenceSink.Record(ctx, idempotencyKey, resourceID, outcome)` 三字符串；旅程断言证据内容零秘密材料/零客户内容。
8. **错误词表**：`ErrTenantRequired`/`ErrIdempotencyKeyRequired`/`ErrMembershipRequired`/`ErrActionInvalid`（先于端口）；`ErrAuditUnavailable`（可重试，来自端口）。sentinel 风格同构 tenancy/secret-reference 包先例。
9. **旅程断言集（YAML name ↔ driver 双锚，harness 强制对账）**：`reject_missing_tenant`、`reject_missing_idempotency_key`、`reject_unknown_action`、`five_actions_each_one_immutable_event`、`replay_converges_single_event`、`immutability_no_overwrite`、`port_failure_no_partial_then_retry_converges`、`evidence_content_minimized`（8 条）。
10. **零 schema/契约变更**：无迁移文件、无 openapi 改动、go.mod 零触碰。

## Task breakdown

- **Task 0（controller）**：本计划，提交 `docs(plan): add issue 8 t07 implementation plan`。
- **Task 1（实施者——本会话为 controller fallback，独立性披露见台账；仍守 RED→green、恰一提交纪律）**：focused 契约测试先红（`service_test.go` 引用不存在词汇 build fail，RED 证据落 `artifacts/evidence/task1/`）→ 实现 `service.go`（含具体端口）→ focused 全绿 → 旅程三件套 → 域回归 → 13 门禁。
- **Task 1 双审**：规格符合性 + 代码质量（独立 subagent；本会话不可用→controller 双轴报告 + 醒目披露）。
- **终审**：最强可用模型全分支审查（本会话=controller 自身，披露）。

## Acceptance matrix（13 门禁；审查/终审逐条独立重跑）

环境：`GOTOOLCHAIN=local`、`/Users/wuyongjun/.local/go1.26.7/bin/go`、`GOPROXY=https://goproxy.cn,direct GOSUMDB=off`、禁 `env -u`、`TestPlatformAPIProcess` 在场时 `-p 1`、WeKnora 端口勿触、临时 PG 用 55xxx 毕拆。**会话沙箱适配**：worktree 位于 `.superpowers/worktrees/`（会话工作区内）；全部 go 命令 `GOCACHE=/tmp/t07-gocache`；需树写的验证在 worktree 内直接可行。

1. `go test ./internal/identity/membership-role-audit -count=1` — focused 全绿。
2. `go test -race -count=1 ./internal/identity/membership-role-audit` — 无竞争。
3. `go test ./tests/acceptance -run TestT07MembershipRoleAudit -count=1 -v` — 旅程 PASS（无栈门控、不 skip），evidence JSON 落 `artifacts/evidence/t07-membership-role-audit/`。
4. `go test ./... -count=1 -p 1`（无 DB env） — 全仓绿、T07 旅程运行不 skip。
5. `go vet ./...`、`gofmt -l .`（除 web/）空、`go build ./...`、`go mod tidy -diff` 空。
6. `make generate-check` — 零漂移（本切片零 codegen 触碰的自证）。
7. `make policy-check` — 新包过依赖策略。
8. `bash scripts/verify-foundation.sh`（TEST_DATABASE_URL=临时 PG 55486）— 七相位全 PASS。
9. 旅程证据审计 — JSON passed=true、8/8 断言、details 双脱敏、伪造敏感材料零命中。
10. 提交卫生 — 恰 2 提交（plan+slice）、树净、`git diff --check` 干净。
11. RED 独立复现 — /tmp scratch：base + 仅新测试 → 恰红于新词汇边界；对照基线全绿。
12. 秘密扫描 — 全新增 diff 扫 token/secret/password 模式：仅命中文档散文与公开测试标识符。
13. 零回归 — gates 4/8 + diff 审计（不触任何既有文件；唯一共享接缝 = tests/acceptance 新增文件，无修改既有文件）。

## Global constraints（Issue #8 原文，逐条守）

- Stage 1 规模包络/单 Region；Tenant 硬边界（命令携带 TenantID，校验先于副作用）；billing 1:1；Product Domain Objects Product 拥有（本切片不涉）；**秘密/原始 prompt/文档体/原始支付载荷/敏感个人信息不入 logs/traces/metrics/Audit/Usage/fixtures/evidence**（ruling 4/7 可执行化，ADR-0041 在场）；compose 限开发/CI/受控 beta；99.9%/RPO/RTO 目标（本切片不涉）；聚焦单测不替代具名验收接缝（ruling 2 旅程=具名接缝）；依赖图完备（#6 CLOSED 亲验）；域词表与已接受 ADR 边界保持（ADR-0041 不可变最小化审计流即本票所立形状）。
