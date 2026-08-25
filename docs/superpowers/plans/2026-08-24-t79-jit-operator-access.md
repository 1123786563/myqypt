# T79 JIT Operator Access Production Gate Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> 模板：[2026-08-25-p0-gate-template.md](2026-08-25-p0-gate-template.md)。本计划按模板重写（2026-08-25），替代旧版"目标句验收"写法。

**Goal:** 证明 Platform operator 不存在长期跨 Tenant 数据访问：Support/Emergency 访问必须满足 case + reason + Tenant Owner consent + MFA + JIT 最短期限 + 只读优先 + Audit + 会话标记 + 自动到期；Emergency Access 仅限正在发生的安全事故并事后通知与复核（ADR-0048）。

**Gate 身份:** 基线 §25「Secret Rotation」邻接的运营安全验收 + §20 Operator Access；ADR-0041（Audit）、ADR-0048（JIT）、ADR-0049（Quarantine 关系）。**gate_id: t79-jit-operator-access**。

**Architecture:** 本 Gate 是**访问控制验收套件**：driver 模拟 Support/Emergency 访问申请 → 授权 → 使用 → 到期 → 审计全链路，断言每一道闸门（case/reason/consent/MFA/JIT/只读/audit/expiry）机械可判；验证无 standing 访问、Emergency 的事后通知与复核、以及 fail closed（consent 缺失即拒绝）。harness 前置：tests/platformtest（#100/F01–F05）。

**Tech Stack:** Go test harness、Keycloak（MFA/会话）、OpenFGA（临时授权）、Audit 流、Temporal（到期）

**Spec:** [GitHub Issue #80](https://github.com/1123786563/myqypt/issues/80)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §20/§25、ADR-0041、ADR-0048、ADR-0049、2026-08-25-p0-gate-template.md

## Global Constraints

- 访问申请必须同时满足：case 编号 + 业务 reason + Tenant Owner consent + MFA 通过 + JIT 最短期限 + 只读优先（基线 §20）。
- 无 standing 跨 Tenant 访问；Emergency 只用于正在发生的安全事故（ADR-0048），并触发事后通知 + 复核 + Audit。
- 每次访问产生 content-minimized Audit 事件（ADR-0041），含 case、scope、起止、审批与标记；不含客户数据正文。
- 到期自动失效：过期后的任何操作被拒。
- 任一闸门缺失/证据缺失 ⇒ blocked；审批人不得是访问执行人。

---

## Case Matrix

| case_id | 条件/场景 | 防御层/机制 | 具体动作 | 期望可观察结果 | 严重度 |
| --- | --- | --- | --- | --- | --- |
| CASE-01 | 无 case/reason | case 门禁 | 无 case 编号的 Support 访问申请 | 拒绝；无授权；Audit 记录拒绝 | P0 |
| CASE-02 | 无 Owner consent | consent 门禁 | case 齐全但 Tenant Owner 未同意 | 拒绝；无授权；Audit 记录 | P0 |
| CASE-03 | 无 MFA | MFA 门禁 | 未过 MFA 的访问尝试 | 拒绝；无会话 | P0 |
| CASE-04 | 非 JIT/期限过长 | JIT 最短期限 | 申请超过最短期限的访问 | 拒绝或自动截短到期限上限；到期强制失效 | P0 |
| CASE-05 | 读写越权 | 只读优先 | Support 会话尝试写操作 | 写拒绝；仅允许批准范围内的只读 | P0 |
| CASE-06 | 到期后使用 | 自动到期 | 访问到期后重放同一会话 | 403；无数据返回；Audit 记录过期拒绝 | P0 |
| CASE-07 | 跨 Tenant 漂移 | scope 锁定 | 授权 tenant A 的会话访问 tenant B | 403；scope 不匹配即拒 | P0 |
| CASE-08 | Emergency 合规 | 事后通知 + 复核 | 模拟安全事故中的 Emergency 访问 | 有 incident id；事后通知 Tenant Owner；复核记录；Audit 完整；无 silent impersonation | P0 |
| CASE-09 | 无 standing 访问 | 零常驻特权 | 枚举 operator 身份 | 无长期跨 Tenant 数据访问角色/密钥；全部为 JIT | P0 |
| CASE-10 | Audit 完整性 | 不可变审计流（ADR-0041） | 校验本次访问链的 Audit 事件 | 申请/授权/使用/到期/复核全链路事件齐全且内容最小化 | P0 |

## 机械判据

- 逐 case：状态码/授权结果/Audit 事件存在性（case_id、scope、起止）、到期后 403、无 standing 角色枚举为 0。
- Gate 级：CASE-01 至 CASE-10 任一 fail ⇒ blocked；Audit 事件缺失 ⇒ blocked（ADR-0041）。
- 证据不得含客户数据正文/凭据（基线 §20）。

## 证据与批准

- `docs/evidence/gates/t79-jit-operator-access/<run-id>.yaml`：含 fingerprint（keycloak/openfga/audit 版本 digest）与逐 case 结果。
- 四方批准（ADR-0044）：四角色各自 reviewer + approved + rationale + manifest_sha256；审批人 ≠ 访问执行人。

## Fail-Closed 语义

- Keycloak/OpenFGA/Audit 任一不可用 ⇒ 拒绝访问类 case blocked，不降级为允许；
- consent/case 缺失 ⇒ 拒绝（CASE-01/02）；到期时间无法判定 ⇒ 拒绝。

---

## File Structure

- Create `tests/production-gates/scenarios/t79-jit-operator-access.yaml`（10 case 场景契约）
- Create `tests/production-gates/drivers/t79.go` + `t79_test.go`
- Create `docs/evidence/gates/t79-jit-operator-access/README.md`

### Task 1: 场景契约与 driver 骨架

- [ ] **Step 1:** 写失败用例：`go test ./tests/production-gates/drivers -run T79 -count=1` → FAIL。
- [ ] **Step 2:** 场景 YAML：10 case，含申请/授权/使用/到期/复核全链路动作与期望。
- [ ] **Step 3:** driver：模拟完整访问生命周期；断言每道闸门；校验 Audit 事件链；模拟 Emergency（incident + 事后通知 + 复核）。
- [ ] **Step 4:** 提交：git commit -m "test(gates): t79 jit operator access driver and scenario"

### Task 2: 受控环境全量运行

- [ ] **Step 1:** 起 Keycloak + OpenFGA + Audit 环境，记录 fingerprint。
- [ ] **Step 2:** 运行全 10 case（含 CASE-06 到期重放、CASE-08 Emergency、CASE-09 standing 枚举）。
- [ ] **Step 3:** 修复缺陷（回退 T07/T09/T24 对应 ticket）。
- [ ] **Step 4:** 产出并提交证据：git commit -m "evidence(gates): t79 run-<run-id> all cases pass"

### Task 3: 四方批准

- [ ] **Step 1:** 生成 manifest_sha256。
- [ ] **Step 2:** 四方在 issue #80 评论 approve/block + rationale（审批人 ≠ 访问执行人）。
- [ ] **Step 3:** 全通过 ⇒ 填 approval 提交；否则 blocked。

## Self-Review Record

- Spec coverage: 10 case 覆盖 ADR-0048 全部闸门（case/consent/MFA/JIT/只读/到期/scope/Emergency/standing/audit）。
- Placeholder scan: 无目标句 expect；每 case 有具体授权结果与 Audit 断言。
- Type consistency: 访问 scope 与 TenantScope 一致；Audit 事件 schema 遵循 ADR-0041。
- Right-sizing: Gate 为访问控制验收；无 Command 服务。
