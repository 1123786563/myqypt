# T72 Tenant Erasure Production Gate Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> 模板：[2026-08-25-p0-gate-template.md](2026-08-25-p0-gate-template.md)。本计划按模板重写（2026-08-25），替代旧版"目标句验收"写法。
> **建模修正（2026-08-25 审计 H4）**：旧版 `TenantErasureCommand` 缺失 TenantID，无法表达"擦除哪个租户"；本版命令以 TenantID 为必填主键，逐存储擦除由 Erasure Record 证据驱动。

**Goal:** 证明 Read-only Retention 结束后，指定 Tenant 的 Platform DB、Product DB、Object Storage、Vector Store、任务队列/缓存、Product 凭证、OpenFGA tuple、Product User/Membership Binding、Backup 过期计划全部完成擦除，且每个存储产出独立 Adapter 证据（Erasure Record），可重试、可审计、失败不产生半擦除状态。

**Gate 身份:** 基线 §25「Tenant Erasure」；ADR-0011、ADR-0031、ADR-0050；dossier T86.9（Erasure 证据）。**gate_id: t72-tenant-erasure**。

**Architecture:** 本 Gate 是**擦除验收套件**：driver 对每个存储执行擦除动作并独立断言（数据不可读 + Erasure Record 存在），验证重试/中断恢复与幂等。擦除由 ErasureTenantWorkflow（Temporal，基线 §15）驱动，但 **Workflow 成功 ≠ Erasure Record**（ADR-0011）；Gate 只认逐存储 Adapter 证据。harness 前置：tests/platformtest（#100/F01–F05）。

**Tech Stack:** Go test harness、Temporal（EraseTenantWorkflow）、PostgreSQL/Vector/Object Storage（数据存在性探测）、OpenFGA（tuple 删除）

**Spec:** [GitHub Issue #73](https://github.com/1123786563/myqypt/issues/73)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §17/§25、ADR-0011、ADR-0031、ADR-0050、2026-08-25-p0-gate-template.md

## Global Constraints

- Erase 命令主键：`TenantErasureCommand{TenantID string, ErasureOperationID string, IdempotencyKey string}`——TenantID 与 IdempotencyKey 均必填；缺失 TenantID 拒绝执行（H4 修复）。
- 擦除只允许在 Read-only Retention 结束后开始（T71 前置）；保留期内的擦除请求 ⇒ blocked。
- 每个存储必须产出独立 Erasure Record（含 store 类型、删除范围、校验和、时间、Adapter 证据）；缺失任一存储 ⇒ blocked。
- 数据存在性探测必须验证"不可读"而非"文件被删"：随机采样 + 直接寻址均不可读。
- 中断/失败后重试不得产生半擦除状态（幂等）；备份恢复不得复活已擦除数据（ADR-0037 交互，见 CASE-08）。

---

## Case Matrix

| case_id | 存储/条件 | 防御层/机制 | 具体动作 | 期望可观察结果 | 严重度 |
| --- | --- | --- | --- | --- | --- |
| CASE-01 | Platform PostgreSQL | Tenant 数据删除 | 擦除后以任意角色查询该 tenant 的行 | 0 行；absence_of [tenant_id 关联数据]；Erasure Record 存在 | P0 |
| CASE-02 | Product DB（WeKnora Cell） | Product DB 擦除 | 擦除后查询该 tenant 的 KB/文档/会话 | 0 行；无残留引用 | P0 |
| CASE-03 | Object Storage | 对象删除 + 校验 | 擦除后按原 key 与枚举方式访问 | 403/404；absence_of [对象内容]；含清单校验和 | P0 |
| CASE-04 | Vector Store | 向量删除 + 过滤 | 擦除后检索含该 tenant 向量的查询 | 0 命中；无孤儿向量 | P0 |
| CASE-05 | 任务队列/缓存 | 队列清空 + 缓存失效 | 擦除后枚举该 tenant 的 pending 任务与缓存 key | 0 任务；0 缓存条目；无延迟执行 | P0 |
| CASE-06 | Product 凭证 + OpenFGA tuple + Binding | 凭证吊销 + tuple 删除 + binding 清除 | 擦除后验证 | 凭证不可用；OpenFGA 查询返回 0 tuple；Product User/Membership Binding 0 行 | P0 |
| CASE-07 | Backup 过期计划 | 备份过期调度 | 擦除后检查备份保留计划 | 该 tenant 备份已列入过期/删除计划；无"从备份复活"路径 | P0 |
| CASE-08 | 恢复交互（ADR-0037） | tombstone + 重擦除义务 | 擦除完成后执行一次恢复演练（T85） | 恢复集不含已擦除 tenant；或恢复后触发重擦除；tombstone 存在 | P0 |
| CASE-09 | 中断/重试幂等 | Erasure Operation 恢复 | 擦除中途注入失败后重试 | 恢复执行；不重复、不残留半擦除；最终全存储 Record 齐 | P0 |
| CASE-10 | 保留期守卫 | Read-only Retention（T71） | 在保留期内尝试擦除 | 拒绝/blocked；不产生任何删除动作 | P0 |

## 机械判据

- 逐 case：存在性探测（status/absence_of/0 行/0 命中）、Erasure Record 存在性（store、sha256、时间）、幂等重试无重复副作用。
- Gate 级：CASE-01 至 CASE-10 任一 fail ⇒ blocked；任一存储缺 Erasure Record ⇒ blocked；Workflow 成功不替代 Record（ADR-0011）。
- 证据不得含客户内容（擦除后证据只含 store 类型/哈希/时间，不含被删数据本身）。

## 证据与批准

- `docs/evidence/gates/t72-tenant-erasure/<run-id>.yaml`：含 fingerprint（postgres/vector/objectstore 版本 digest）与逐 case 结果 + 每存储 Erasure Record 摘要。
- 四方批准（ADR-0044）：四角色各自 reviewer + approved + rationale + manifest_sha256；审批人 ≠ 证据生成者。

## Fail-Closed 语义

- 任一存储探测工具不可用 ⇒ 该 case blocked，不推断为"已删除"；
- Temporal/OpenFGA 不可用 ⇒ blocked；
- 保留期未结束 ⇒ blocked（CASE-10）。

---

## File Structure

- Create `tests/production-gates/scenarios/t72-tenant-erasure.yaml`（10 case 场景契约）
- Create `tests/production-gates/drivers/t72.go` + `t72_test.go`（擦除执行 + 逐存储断言）
- Create `docs/evidence/gates/t72-tenant-erasure/README.md`（Erasure Record 目录约定）

### Task 1: 场景契约与 driver 骨架

- [ ] **Step 1:** 写失败用例：`go test ./tests/production-gates/drivers -run T72 -count=1` → FAIL。
- [ ] **Step 2:** 场景 YAML：10 case；每 case 定义存储、fixture 数据、探测动作、期望（0 行/0 命中/absence_of/Record 存在）。
- [ ] **Step 3:** driver：以 TenantID + ErasureOperationID + IdempotencyKey 驱动；逐存储执行擦除与存在性探测；写入 Erasure Record 证据；支持中断恢复与幂等重试。
- [ ] **Step 4:** 提交：git commit -m "test(gates): t72 tenant erasure driver with per-store records"

### Task 2: 受控环境全量运行

- [ ] **Step 1:** 起含全部存储的受控环境（Postgres/Vector/ObjectStore/OpenFGA/Temporal），记录 fingerprint。
- [ ] **Step 2:** 运行全 10 case（含 CASE-08 恢复交互、CASE-09 中断重试、CASE-10 保留期拒绝）。
- [ ] **Step 3:** 修复缺陷（擦除链路缺陷回退 T70/T71 对应 ticket）。
- [ ] **Step 4:** 产出并提交证据：git commit -m "evidence(gates): t72 run-<run-id> all cases pass"

### Task 3: 四方批准

- [ ] **Step 1:** 生成 manifest_sha256。
- [ ] **Step 2:** 四方在 issue #73 评论 approve/block + rationale（审批人 ≠ 证据生成者）。
- [ ] **Step 3:** 全通过 ⇒ 填 approval 提交；否则 blocked 并回退。

## Self-Review Record

- Spec coverage: 10 case 覆盖基线 §17 全部存储 + 恢复交互 + 幂等 + 保留期守卫。
- Placeholder scan: 无目标句 expect；每 case 有具体存在性探测与 Record 断言。
- Type consistency: TenantID 必填（H4 修复）；Erasure Record 字段与模板一致。
- Right-sizing: Gate 为擦除验收；Workflow 成功不算证据（ADR-0011）。
