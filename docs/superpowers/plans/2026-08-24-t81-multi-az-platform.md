# T81 Multi-AZ Platform Production Gate Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> 模板：[2026-08-25-p0-gate-template.md](2026-08-25-p0-gate-template.md)。本计划按模板重写（2026-08-25），替代旧版"目标句验收"写法。

**Goal:** 证明 Higress、Portal、BFF、Platform API 和 Workers 跨 Availability Zone 运行，单节点/单 AZ 故障不影响 99.9% 月度可用性承诺，且关键有状态组件（PostgreSQL/Kafka/ClickHouse/Valkey）故障转移满足 RPO/RTO 上限（元数据 15min、Product 1h、整体 RTO 4h，ADR-0007）。

**Gate 身份:** 基线 §25「Backup / Restore」邻接的可用性验收 + §22 单 Region 多 AZ；ADR-0007、ADR-0043；dossier T86.6（云能力）。**gate_id: t81-multi-az-platform**。

**Architecture:** 本 Gate 是**故障注入验收套件**：driver 在 production-like 双 AZ 环境按清单注入故障（单实例终止、AZ 隔离、有状态组件切换），断言服务可用性、fail-closed 行为、RPO/RTO 实测值与恢复后的数据一致性。逐 case 记录证据（含故障注入前后 fingerprint 与时间戳）。harness 前置：tests/platformtest（#100/F01–F05）。

**Tech Stack:** Go test harness、Kubernetes（多 AZ）、托管 PostgreSQL/Kafka/ClickHouse/Valkey、Higress/Keycloak/OpenFGA/Temporal/Nacos/OpenMeter（多副本）

**Spec:** [GitHub Issue #82](https://github.com/1123786563/myqypt/issues/82)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §22/§25、ADR-0007、ADR-0043、2026-08-25-p0-gate-template.md

## Global Constraints

- 所有平台组件不得以单实例承担生产流量（基线 §22）；本 Gate 验证多副本 + 反亲和。
- 故障注入必须在 production-like 环境执行，不得在 Compose/开发环境冒充（ADR-0007）。
- RPO/RTO 为实测值（含时间戳与数据比对），不是配置声明（ADR-0037 精神）。
- 恢复后必须做数据一致性校验（元数据/计费事实不丢、不重复）。
- 任一关键组件故障时保护请求 fail closed，不降级为开放。

---

## Case Matrix

| case_id | 故障/条件 | 防御层/机制 | 具体动作 | 期望可观察结果 | 严重度 |
| --- | --- | --- | --- | --- | --- |
| CASE-01 | 单实例终止（无状态） | 多副本 + 反亲和 | 终止 Higress/Portal/BFF/Platform API/Workers 的单个 Pod | 服务继续可用；流量重路由；无 5xx 雪崩 | P0 |
| CASE-02 | 单 AZ 隔离 | 跨 AZ 部署 | 隔离 AZ-A（网络断开模拟） | 平台在 AZ-B 继续服务；可用性指标达标 | P0 |
| CASE-03 | PostgreSQL 故障转移 | 托管 HA | 触发主库切换 | 元数据 RPO ≤ 15min（实测）；切换后读写可用 | P0 |
| CASE-04 | Kafka/ClickHouse 故障转移 | 托管 HA | 触发集群成员切换 | 用量链路不丢事件；replay 后一致（T51 关联） | P0 |
| CASE-05 | Valkey 故障转移 | 多副本 + 去重恢复 | 触发主副本切换 | 去重不丢、不重；T54/T55 语义保持 | P0 |
| CASE-06 | OpenFGA/Temporal/Nacos/OpenMeter 单节点故障 | 多副本 | 终止各组件单个副本 | 服务继续；Workflow/授权/注册/计费 Worker 不中断 | P1 |
| CASE-07 | 部分故障下保护请求 | fail-closed | AZ 故障期间发起 Tenant 访问 | 403/503（明确降级），不返回错误授权 | P0 |
| CASE-08 | 恢复后数据一致性 | 一致性校验 | 各存储恢复后比对 | 元数据/计费事实不丢不重；Usage 与 ClickHouse 一致（T53 关联） | P0 |
| CASE-09 | 可用性实测 | 99.9% 承诺 | 按故障序列累计测量 | 月度可用性 ≥ 99.9%（采样证据）；RTO ≤ 4h 实测 | P0 |
| CASE-10 | 恢复演练（ADR-0037） | 完整恢复 | 执行一次完整恢复演练 | 恢复集完整；RPO/RTO 实测；无备份存在性冒充 | P0 |

## 机械判据

- 逐 case：可用性断言（请求成功率、错误码分布）、RPO/RTO 实测值（含时间戳）、数据一致性比对（计数/校验和）、fail-closed 状态码。
- Gate 级：CASE-01/02/03/07/08/09/10 任一 fail ⇒ blocked；RPO/RTO 为声明而非实测 ⇒ blocked。
- 证据含故障注入前后 fingerprint；不得含客户内容。

## 证据与批准

- `docs/evidence/gates/t81-multi-az-platform/<run-id>.yaml`：含拓扑 fingerprint（k8s、各组件版本、AZ 分布）与逐 case 实测结果。
- 四方批准（ADR-0044）：四角色各自 reviewer + approved + rationale + manifest_sha256；审批人 ≠ 证据生成者。

## Fail-Closed 语义

- 任一关键组件在故障期间暴露错误授权/丢事件 ⇒ 该 case fail；
- 无法测量 RPO/RTO ⇒ blocked（不允许"估计"）；
- 保护类请求在依赖不可用时必须 403/503，不允许放行。

---

## File Structure

- Create `tests/production-gates/scenarios/t81-multi-az-platform.yaml`（10 case 场景契约）
- Create `tests/production-gates/drivers/t81.go` + `t81_test.go`（故障注入 + 断言 + 测量）
- Create `docs/evidence/gates/t81-multi-az-platform/README.md`

### Task 1: 场景契约与 driver 骨架

- [ ] **Step 1:** 写失败用例：`go test ./tests/production-gates/drivers -run T81 -count=1` → FAIL。
- [ ] **Step 2:** 场景 YAML：10 case，含故障注入方法（Pod 终止/AZ 隔离/主库切换）、断言与测量窗口。
- [ ] **Step 3:** driver：执行故障注入 → 断言可用性与 fail-closed → 实测 RPO/RTO → 数据一致性比对 → 写证据。
- [ ] **Step 4:** 提交：git commit -m "test(gates): t81 multi-az failure injection driver and scenario"

### Task 2: production-like 环境全量运行

- [ ] **Step 1:** 起双 AZ production-like 环境（K8s + 托管状态服务），记录 fingerprint。
- [ ] **Step 2:** 运行全 10 case（含 CASE-02 AZ 隔离、CASE-03/04/05 有状态切换、CASE-10 完整恢复演练）。
- [ ] **Step 3:** 修复缺陷（回退 T82/T83/T84 对应 ticket）。
- [ ] **Step 4:** 产出并提交证据：git commit -m "evidence(gates): t81 run-<run-id> all cases pass"

### Task 3: 四方批准

- [ ] **Step 1:** 生成 manifest_sha256。
- [ ] **Step 2:** 四方在 issue #82 评论 approve/block + rationale（审批人 ≠ 证据生成者）。
- [ ] **Step 3:** 全通过 ⇒ 填 approval 提交；否则 blocked。

## Self-Review Record

- Spec coverage: 10 case 覆盖无状态/AZ/有状态/fail-closed/一致性/可用性/恢复演练。
- Placeholder scan: 无目标句 expect；每 case 有实测断言与测量要求。
- Type consistency: RPO/RTO 目标与 ADR-0007 一致；gate_id 与 T88 聚合键一致。
- Right-sizing: Gate 为故障注入验收；不引入 Command 服务。
