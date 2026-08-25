# T68 Canary Upgrade Production Gate Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> 模板：[2026-08-25-p0-gate-template.md](2026-08-25-p0-gate-template.md)。本计划按模板重写（2026-08-25），替代旧版"目标句验收"写法。

**Goal:** 证明 Product Version 升级在内部测试 Cell → Canary Cell → 批量生产 Cell 的推进中不丢数据、不中断访问；forward-only/destructive 失败从已验证 Backup Restore（不以旧镜像冒充 rollback）；逐阶段记录 Desired/Observed 状态与兼容性证据。

**Gate 身份:** 基线 §25「Product Adapter Compatibility」+ §16 升级/回滚契约；ADR-0032、ADR-0033；dossier T86.9（升级证据）。**gate_id: t68-canary-upgrade**。

**Architecture:** 本 Gate 是**升级路径验收套件**：driver 按迁移契约（T14 migration_class/rollback_supported 字段）选择升级编排，在 Cell 级执行 兼容性测试 → Backup → Restore Rehearsal → Drain → Deploy → Migrate → Smoke → Observation → Promote（基线 §16），每阶段断言 Observed State（T19）与数据完整性，产出逐 case 证据。harness 前置：tests/platformtest（#100/F01–F05）。

**Tech Stack:** Go test harness、Temporal（UpgradeProductWorkflow）、PostgreSQL/Vector（数据校验）、WeKnora Shared Cell、Object Storage（备份）

**Spec:** [GitHub Issue #69](https://github.com/1123786563/myqypt/issues/69)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §16/§25、ADR-0032、ADR-0033、2026-08-25-p0-gate-template.md

## Global Constraints

- 升级只按 migration_class 允许的路径推进：backward_compatible 可 Canary；forward_only/destructive 必须 Backup + Restore Rehearsal 证据齐备且失败时 Restore（基线 §16）。
- 每个 case 断言 Observed State 而非 Desired State（T19）；升级中失败必须被分类为 Lifecycle Operation 的失败类型（基线 §4）。
- 兼容性矩阵（T14 minimum/maximum_source_version + T67 Adapter 兼容）不满足 ⇒ 升级 blocked。
- 证据含 fingerprint（product_version digest、adapter、patchset、migration_class）；缺失即 blocked。

---

## Case Matrix

| case_id | 条件/场景 | 防御层/机制 | 具体动作 | 期望可观察结果 | 严重度 |
| --- | --- | --- | --- | --- | --- |
| CASE-01 | migration_class=backward_compatible | Canary 推进 | 内部测试 Cell 升级 → Canary Cell → 批量 Cell | 每阶段 Observed=active；KB 问答（T33）可用；无数据丢失校验通过 | P0 |
| CASE-02 | migration_class=forward_only | Backup + Rehearsal 前置 | 升级前执行 Backup + Restore Rehearsal（T69）后升级 | 前置证据存在；升级成功；失败路径见 CASE-03 | P0 |
| CASE-03 | forward_only/destructive 失败 | Restore 而非旧镜像回滚 | 注入迁移失败 | 从已验证 Backup Restore；Observed 恢复 active；禁止"以旧镜像启动冒充 rollback" | P0 |
| CASE-04 | 版本兼容矩阵不满足 | T14/T67 兼容校验 | 用 minimum/maximum_source_version 之外的源版本升级 | 升级拒绝；Lifecycle Operation 记录 compatibility 失败 | P0 |
| CASE-05 | Canary 观察期失败 | Promote 阻断 | Canary Cell 冒烟/观测失败 | 不 Promote；批量 Cell 不升级；失败分类记录 | P0 |
| CASE-06 | 升级中访问连续性 | Drain + 双读/无中断窗口 | 升级期间发起 KB 问答 | 请求成功（或返回明确 maintenance 503）；无数据损坏 | P1 |
| CASE-07 | Desired/Observed 状态一致性 | T19 状态机 | 升级全过程查询状态 | Observed 只出现合法迁移序列；无"Desired=active 而 Observed 未知" | P1 |
| CASE-08 | rollback_supported 声明验证 | 版本契约 | 对声明 rollback_supported 的版本执行回滚路径 | 回滚只经 Restore（forward-only/destructive）或契约允许的降级；无数据丢失 | P0 |

## 机械判据

- 逐 case：Observed State 枚举断言、数据完整性（KB 数量 + 抽样问答结果一致）、升级操作结果（成功/失败分类）、max_ms（CASE-06 时延上界）。
- Gate 级：CASE-01/03/04/05/08 任一 fail ⇒ blocked；CASE-02 的前置证据（backup + rehearsal digest）缺失 ⇒ blocked。
- 证据不得含客户文档正文/Prompt（基线 §20）。

## 证据与批准

- `docs/evidence/gates/t68-canary-upgrade/<run-id>.yaml`，含 fingerprint（product_version、adapter、patchset、migration_class）与逐 case 结果。
- 四方批准（ADR-0044）：四角色各自 reviewer + approved + rationale + manifest_sha256；审批人 ≠ 证据生成者。

## Fail-Closed 语义

- Backup/Restore Rehearsal 证据缺失或过期 ⇒ 升级类 case blocked；
- Temporal 不可用 ⇒ blocked，不跳过升级验证；
- 任一 Cell 阶段未达 active ⇒ 该 case fail。

---

## File Structure

- Create `tests/production-gates/scenarios/t68-canary-upgrade.yaml`（8 case 场景契约）
- Create `tests/production-gates/drivers/t68.go`（升级路径执行器）+ `t68_test.go`
- Create `docs/evidence/gates/t68-canary-upgrade/README.md`

### Task 1: 场景契约与 driver 骨架

- [ ] **Step 1:** 写失败用例：`go test ./tests/production-gates/drivers -run T68 -count=1` → FAIL。
- [ ] **Step 2:** 场景 YAML：8 case，含源/目标版本、migration_class、注入失败点、期望 Observed State 序列。
- [ ] **Step 3:** driver：按 migration_class 编排升级；每阶段查询 T19 状态、跑 KB 数据完整性校验、断言失败分类。
- [ ] **Step 4:** 提交：git commit -m "test(gates): t68 canary upgrade driver and scenario"

### Task 2: 受控环境全量运行

- [ ] **Step 1:** 起内部测试 Cell + Canary Cell + 批量 Cell（Compose/受控环境），记录 fingerprint。
- [ ] **Step 2:** 运行全 8 case（含 CASE-03 注入迁移失败、CASE-04 兼容拒绝）。
- [ ] **Step 3:** 修复缺陷（升级链路缺陷回退 T67/T20 对应 ticket）。
- [ ] **Step 4:** 产出并提交证据：git commit -m "evidence(gates): t68 run-<run-id> all cases pass"

### Task 3: 四方批准

- [ ] **Step 1:** 生成 manifest_sha256。
- [ ] **Step 2:** 四方在 issue #69 评论 approve/block + rationale（审批人 ≠ 证据生成者）。
- [ ] **Step 3:** 全通过 ⇒ 填 approval 提交；否则 blocked 并回退。

## Self-Review Record

- Spec coverage: 8 case 覆盖基线 §16 全路径（兼容/备份/Rehearsal/Canary/失败恢复/兼容拒绝/连续性/回滚契约）。
- Placeholder scan: 无目标句 expect；每 case 有具体状态序列与数据断言。
- Type consistency: Observed State 与 T19 一致；migration_class 与 T14 一致。
- Right-sizing: Gate 为升级路径验收；未引入 Command 服务。
