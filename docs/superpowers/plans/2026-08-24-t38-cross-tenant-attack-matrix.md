# T38 Cross-Tenant Attack Matrix Production Gate Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> 模板：[2026-08-25-p0-gate-template.md](2026-08-25-p0-gate-template.md)。本计划按模板重写（2026-08-25），替代旧版"目标句验收"写法。

**Goal:** 以可执行攻击矩阵证明数据库、向量、后台任务、缓存、身份头、Product Route 与 Object Store 的跨 Tenant 防御全部 fail closed，并产出机械可判的逐 case 证据。

**Gate 身份:** 基线 §25「Tenant Isolation」「Cross-Tenant Security」；ADR-0008、ADR-0009、ADR-0044；dossier T86.9（WeKnora Shared Security）。**gate_id: t38-cross-tenant-security**。

**Architecture:** 本 Gate 是**测试资产 + 证据产品**，不是业务服务：`tests/production-gates/drivers/t38.go` 加载 YAML 场景矩阵，对每个 case 用 attacker/victim 两个真实 fixture Tenant 执行攻击动作，断言 §3.2 期望值，逐 case 记录证据（`docs/evidence/gates/t38-cross-tenant-security/<run-id>.yaml`）。harness 前置：`tests/platformtest`（issue #100 / F01–F05 重建）。

**Tech Stack:** Go test harness、PostgreSQL（RLS 校验）、WeKnora Shared Cell、Higress、Object Storage stub

**Spec:** [GitHub Issue #39](https://github.com/1123786563/myqypt/issues/39)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §11/§25、ADR-0008、ADR-0009、ADR-0044、2026-08-25-p0-gate-template.md

## Global Constraints

- 每个 case 使用独立 fixture：attacker Tenant A 与 victim Tenant B 各含知识库/向量/任务/缓存数据；case 之间不共享状态。
- 期望值全部机械可判（状态码、absence_of 字段、时延上界）；禁止自然语言 expect。
- 任一 case fail/blocked ⇒ Gate blocked；skipped 不计入通过（ADR-0044）。
- 证据必须含环境指纹（K8s/WeKnora/patchset/PostgreSQL 版本 digest）；缺失即 blocked。
- 不引入真实客户数据；fixture 内容为合成文档。

---

## Case Matrix

| case_id | 攻击/条件向量 | 防御层 | 具体动作 | 期望可观察结果 | 严重度 |
| --- | --- | --- | --- | --- | --- |
| CASE-01 | ID-only 仓储访问 | Repository TenantScope（T34） | A 以 victim B 的知识库全局 ID 调用列表/读取 API | 403/404；响应体 absence_of [B 的 kb_id, B 的任何对象] | P0 |
| CASE-02 | 数据库直查越权 | PostgreSQL RLS（基线 §11.2） | 以 A 的 DB 角色执行跨 tenant SELECT（含 B 的 tenant_id） | 0 行返回；不报错泄露列结构 | P0 |
| CASE-03 | 向量无 tenant 过滤 | Vector TenantScope（T35） | A 发起含 B 的向量 id / 无 tenant filter 的检索 | 结果 absence_of [B 的 chunk 文本]；空结果或仅 A 数据 | P0 |
| CASE-04 | 后台任务饥饿 | Tenant 级公平调度（T36） | A 提交 N 个大 ingest 任务，观察 B 的任务推进 | B 的任务在限定时间内完成/推进；A 不独占 worker 并发 | P1 |
| CASE-05 | 配额预留非原子 | 原子配额 Reservation（T37） | A 并发发起超过配额上限的预留请求 | 只有 ≤ 配额的成功数；无超额分配 | P0 |
| CASE-06 | 缓存键越权 | Tenant-scoped cache key（基线 §11.4） | A 读取以 B 的 key 构造的缓存条目 | miss 或返回 A 自己数据；absence_of [B 数据] | P1 |
| CASE-07 | 伪造身份头 | 可信边缘头清洗（T23） | A 伪造 X-Tenant-ID / 内部身份 Header 直达 Product Route | 头被剥离；请求按 A 的真实 TenantScope 处理或 403 | P0 |
| CASE-08 | Product Route 跨 binding | Platform Context audience 校验（T22/T23） | A 携带 B 的 product_binding_id 访问 Product Route | 403（audience/scope 不匹配）；无 B 数据 | P0 |
| CASE-09 | Object Store 键枚举 | Tenant-scoped 对象键 + 签名访问（抽取设计 §7.4） | A 猜测/枚举 B 的存储 key 并发起访问 | 403/404；absence_of [B 对象内容] | P1 |
| CASE-10 | RBAC 只记不拒 | 生产 RBAC enforce 模式（基线 §11.6） | 验证 WeKnora RBAC 配置非 logging-only | 配置断言：enforce=true；无"仅记录"路径 | P0 |
| CASE-11 | Platform Key 冒充用户 | System Admin/Key 爆炸半径（基线 §11.7） | 用 Platform Key 尝试以普通 User 身份执行租户操作 | 拒绝；无静默 impersonation；产生 Audit 事件 | P0 |
| CASE-12 | 撤销后访问残留 | 立即撤权（T09） | B 撤销 A 的 Product Access 后，A 立即重放请求 | 403（fail closed）；无 5 分钟宽限 | P0 |

## 机械判据

- 逐 case：`expect.status`、`expect.absence_of`（响应体/日志 JSON 路径级校验）、`expect.presence_of`（如 Audit 事件 id）、`max_ms`（CASE-04/12 时延上界）。
- Gate 级：CASE-01/02/03/05/07/08/11/12 任一 fail ⇒ blocked（P0）；CASE-04/06/09 任一 fail ⇒ blocked（P1 属 Gate 覆盖面，不允许跳过）。
- 日志/证据不得出现 victim 数据、Prompt、Secret（基线 §20）。

## 证据与批准

- 运行产出 `docs/evidence/gates/t38-cross-tenant-security/<run-id>.yaml`，字段按模板 §3.4（fingerprint 必填：k8s、weknora、patchset、postgres digest）。
- 四方批准（ADR-0044）：platform_engineering/security/finance_billing/product，各自 reviewer + approved + rationale + manifest_sha256；审批人不得是证据生成者。

## Fail-Closed 语义

- OpenFGA/Keycloak/PostgreSQL 任一依赖不可用 ⇒ 保护类 case blocked，不降级；
- 任一 case 证据引用缺失或 fingerprint 缺失 ⇒ blocked。

---

## File Structure

- Create `tests/production-gates/scenarios/t38-cross-tenant-attack-matrix.yaml`（12 个 case 的完整场景契约）
- Create `tests/production-gates/drivers/t38.go`（矩阵执行器：逐 case 攻击 → 断言 → 证据）
- Create `tests/production-gates/drivers/t38_test.go`（driver 自身契约测试）
- Create `docs/evidence/gates/t38-cross-tenant-security/README.md`（证据目录约定 + 生成说明）

### Task 1: 场景契约与 driver 骨架

- [ ] **Step 1: 写失败用例（场景文件不存在）**：`go test ./tests/production-gates/drivers -run T38 -count=1` → FAIL（driver 不存在）。
- [ ] **Step 2: 建场景 YAML**：按 Case Matrix 写 12 个 case；每个 case 含 attacker/victim fixture 描述、动作、expect（status/absence_of/presence_of/max_ms）。
- [ ] **Step 3: 实现 driver**：加载场景 → 对每个 case 执行动作 → 收集断言结果 → 写 `docs/evidence/gates/.../` 证据 YAML；断言失败即该 case fail。
- [ ] **Step 4: 提交**：git commit -m "test(gates): t38 cross-tenant attack matrix driver and scenario"

### Task 2: 在受控环境跑真实矩阵

- [ ] **Step 1: 环境准备**：Compose 起 attacker/victim 双租户 WeKnora Shared Cell + Higress + PostgreSQL（RLS 开启），记录 fingerprint。
- [ ] **Step 2: 全量运行**：`go test ./tests/production-gates/drivers -run T38 -count=1 -v`；修复真实缺陷（属 T23/T34/T35/T36/T37 的缺陷回退对应 ticket）。
- [ ] **Step 3: 产出证据**：确认 <run-id>.yaml 含 12 个 case 的 pass 记录 + fingerprint + 无敏感内容。
- [ ] **Step 4: 提交证据**：git add docs/evidence/gates/t38-cross-tenant-security && git commit -m "evidence(gates): t38 run-<run-id> all cases pass"

### Task 3: 四方批准

- [ ] **Step 1:** 生成 manifest_sha256（对证据目录所有文件求哈希）。
- [ ] **Step 2:** 四方在 issue #39 评论各自 approve/block + rationale（审批人 ≠ 证据生成者）。
- [ ] **Step 3:** 四方全通过 ⇒ 在证据 YAML 填 approval 字段并提交；否则按 blocked 记录并回退修复。

## Self-Review Record

- Spec coverage: 12 个 case 覆盖基线 §11 全部阻断项（ID-only/RLS/向量/任务/配额/缓存/头清洗/RBAC/Key 爆炸/撤权）。
- Placeholder scan: 无目标句 expect；每 case 有具体状态码/absence_of/presence_of。
- Type consistency: gate_id 与 T88 聚合键一致；证据 schema 遵循模板 §3.4。
- Right-sizing: Gate 是测试资产；未引入 Command 服务。
