# T88 Paid Launch Gates Production Gate Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> 模板：[2026-08-25-p0-gate-template.md](2026-08-25-p0-gate-template.md)。本计划按模板重写（2026-08-25），替代旧版"目标句验收"写法。
> **裁决结果（2026-08-25，ADR-0057 proposed）**：ADR-0044 与基线 §25 的清单分叉已由 ADR-0057 裁定——权威清单为 **13 项不可豁免 Gate**（0044 的 11 项 + Tenant Export + Verified Deduplication Backend）；**Nacos Production PoC 移出**付费上线清单，改为 AI Registry 旁路转正门（不阻塞上线，见 ADR-0051）。本 Gate 在 ADR-0057 转正（accepted）前不得执行。

**Goal:** 聚合全部不可豁免 Production Gate 的机器可读结果与四方批准，输出可追溯的 paid-launch 或 blocked 结论——本计划**只做聚合与放行裁决，不重新实现任何 Gate 逻辑**（各 Gate 由 T38/T51/T53/T62/T66/T67/T68/T69/T70/T72/T74/T79/T81/T85/T86.x 分别验收）。

**Gate 身份:** 基线 §25 全部 14 项 Gate 的聚合门；ADR-0044（不可豁免 + 四方批准）。**gate_id: t88-paid-launch-gates**。

**Architecture:** 本 Gate 是**聚合裁决器**：读取 `docs/evidence/gates/<gate_id>/<run-id>.yaml` 清单（每份含 fingerprint + 逐 case 结果 + approval），校验：① Gate 清单完整（14 项各有通过证据）；② 每份证据 schema 合法且 fingerprint 齐全；③ 四方批准全通过且审批人 ≠ 证据生成者；④ 无 skipped 计入通过。全部满足 ⇒ launch；任一不满足 ⇒ blocked 并列出缺失/失败清单。harness 前置：tests/platformtest（#100/F01–F05）与全部 Gate 计划完成。

**Tech Stack:** Go 聚合器（读取并校验证据清单）、YAML schema 校验

**Spec:** [GitHub Issue #89](https://github.com/1123786563/myqypt/issues/89)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §25/§26、ADR-0044、ADR-0051、2026-08-25-p0-gate-template.md

## Global Constraints

- Gate 清单以裁决后的唯一权威（基线 §25 14 项）为准；任何"降级/豁免"必须先行修订 ADR-0044（不可豁免语义，ADR-0044）。
- 聚合器只读证据清单，不执行任何 Gate 场景；Gate 逻辑归属各 Gate driver。
- skipped/partial ≠ passed；任一 gate blocked ⇒ 整体 blocked。
- 四方批准为硬条件：任何一方未批准或审批人 = 证据生成者 ⇒ blocked。
- 证据与批准记录不得含客户内容；release 结论可追溯至每份证据文件。

---

## Gate Inventory（ADR-0057 裁决后的权威清单：13 项）

| # | gate_id | 负责计划 | 证据来源 |
| --- | --- | --- | --- |
| 1 | tenant-isolation | T38 | docs/evidence/gates/t38-cross-tenant-security/ |
| 2 | cross-tenant-security | T38 | 同上 |
| 3 | license | T66 + T86.4 dossier | docs/evidence/gates/t66-license/ + dossiers/ |
| 4 | payment-reconciliation | T62 | docs/evidence/gates/t62-payment-reconciliation/ |
| 5 | usage-replay | T51 | docs/evidence/gates/t51-usage-replay/ |
| 6 | usage-reconciliation | T53 | docs/evidence/gates/t53-usage-reconciliation/ |
| 7 | backup-restore | T69 + T85 | docs/evidence/gates/t69-restore-rehearsal/ + t85-cell-dr/ |
| 8 | tenant-export | T70 | docs/evidence/gates/t70-tenant-export/ |
| 9 | tenant-erasure | T72 | docs/evidence/gates/t72-tenant-erasure/ |
| 10 | secret-rotation | T26 | docs/evidence/gates/t26-secret-rotation/ |
| 11 | image-signature | T66 | docs/evidence/gates/t66-image-signature/ |
| 12 | adapter-compatibility | T67 + T68 | docs/evidence/gates/t67-adapter-compat/ + t68-canary-upgrade/ |
| 13 | verified-dedupe-backend | T54 + T86.8 | docs/evidence/gates/t54-valkey-compat/ + dossiers/ |

> 旁路门（不属于本 Gate 聚合范围）：nacos-production-poc（T74 + T86.7）——AI Registry 能力从旁路 PoC 转为生产依赖的前置验收，失败仅降级该能力（ADR-0051 / ADR-0057）。

## 机械判据

- 完整性：14 项（或裁决后的权威清单）gate_id 每项至少一份 <run-id>.yaml，且 conclusion=launch；
- schema：每份证据含 fingerprint（全部必填字段）与逐 case result（无 skipped 计入通过）；
- 批准：四角色 approval.approved=true 且 reviewer ≠ 证据生成者、manifest_sha256 与证据目录哈希一致；
- 产出：`docs/evidence/gates/t88-paid-launch-gates/<run-id>.yaml`（release 结论 + 逐 gate 引用）；
- 任一条件不满足 ⇒ blocked，输出缺失/失败清单（机器可读）。

## 证据与批准

- 本 Gate 自身产出 release 证据：gate_id 聚合结果 + 四方批准（同一四方，各自 reviewer + approved + rationale + manifest_sha256）。
- 四方批准为最终放行依据（ADR-0044）；任何一方 blocked ⇒ release blocked。

## Fail-Closed 语义

- 任一 gate 证据缺失/过期/不合格 ⇒ blocked（不推断通过）；
- 四方批准缺失 ⇒ blocked；
- ADR-0057 未转正（accepted）⇒ PENDING，不进入 launch。

---

## File Structure

- Create `tests/production-gates/scenarios/t88-paid-launch-gates.yaml`（聚合契约：清单、schema、批准规则）
- Create `tests/production-gates/drivers/t88.go` + `t88_test.go`（聚合器：读取/校验/裁决）
- Create `docs/evidence/gates/t88-paid-launch-gates/README.md`

### Task 1: 先决裁决（已完成草案，待转正）

- [x] **Step 1:** 记录 P0 Gate 清单分叉（ADR-0044 11 vs 基线 §25 14 + Nacos 语义冲突）——见本文件头部与 2026-08-25 审计 H1。
- [x] **Step 2:** 裁决草案：ADR-0057（13 项权威清单；Nacos 移为旁路转正门）；基线 §25 与本 Inventory 已对齐。
- [ ] **Step 3:** 转正：架构 owner 接受 ADR-0057（status → accepted）+ 安全方对两项新增 Gate 复核；转正后在 ADR-INDEX 登记，并在 issue #89 评论记录裁决链接。

### Task 2: 聚合器

- [ ] **Step 1:** 写失败用例：`go test ./tests/production-gates/drivers -run T88 -count=1` → FAIL。
- [ ] **Step 2:** 聚合器：扫描 docs/evidence/gates/，校验清单完整性 + schema + approval + manifest 哈希，输出 release 或 blocked + 缺失清单。
- [ ] **Step 3:** 提交：git commit -m "test(gates): t88 paid launch gate aggregator"

### Task 3: 试运行与正式裁决

- [ ] **Step 1:** 用已完成的 Gate 证据试运行聚合器；对 PENDING/缺失项输出清单。
- [ ] **Step 2:** 补缺（回退对应 Gate 计划）直至清单完整。
- [ ] **Step 3:** 正式运行：`go test ./tests/production-gates/drivers -run T88 -count=1 -v`；四方批准；产出 release 证据并提交：git commit -m "evidence(gates): t88 release-<id> launch decision"

## Self-Review Record

- Spec coverage: 14 项 Gate 聚合、schema 校验、四方批准、无 skipped、先决裁决均有机械断言与显式步骤。
- Placeholder scan: 无目标句 expect；聚合判据全部机器可判。
- Type consistency: gate_id 与各 Gate 计划一致；证据 schema 遵循模板 §3.4。
- Right-sizing: 聚合器不重新实现 Gate 逻辑；先决裁决有独立步骤。
