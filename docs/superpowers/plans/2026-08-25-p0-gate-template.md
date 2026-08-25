# P0 Gate 计划模板（v1，2026-08-25）

> 适用：基线 §25 不可豁免 Production Gate 类 ticket（T38/T51/T53/T62/T66/T67/T68/T69/T70/T72/T74/T79/T81/T85/T86.x/T88 等）。
> 依据：ADR-0044（P0 Gate 不可豁免）、基线 v1.1 §25（14 项 Gate）、external-confirmations.md（证据规则）。
> 替代：旧式"把 Gate 套进 Command 服务 + 目标句 expect"的写法（2026-08-25 审计 H1/H2 判定为不可执行）。

## 1. 为什么需要本模板（审计依据）

旧式 Gate 计划把核心验收压缩成一句不可证伪的祈使句：

```text
The concrete XPort.Apply implementation must enforce the Ticket invariant: <目标句>.
```

且 YAML 场景的 expect 直接复制目标句原文、side_effect_count: 1。后果：

- 攻击向量、判据、状态机、表结构均未定义，绿灯可被空实现骗过；
- P0 Gate 是"测试资产 + 证据产品"，不是业务服务——套 Command/Port 模板语义错位；
- skipped/部分通过被当成通过，违反 ADR-0044 不可豁免语义。

本模板要求 Gate 计划必须包含：**Case Matrix（可执行用例矩阵）→ 机械判据 → 证据 schema → 四方批准 → fail-closed 语义**。

## 2. 前置依赖（所有 Gate 计划共同声明）

- `tests/platformtest` 证据 harness：归属 issue #100（F01–F05 重建，T01.1 已 superseded），Gate 执行前必须先就绪；
- 各 Gate 的 dossier（T86.x）与实现前置（T 系列 blocker）必须先完成；
- 环境指纹（K8s/组件版本、镜像 digest、patchset commit）必须在证据中记录。

## 3. 强制章节

### 3.1 Gate 身份与权威

声明本 Gate：属于基线 §25 哪一项、对应 ADR（0044 + 主题 ADR）、对应 dossier（T86.x）、被 T88 聚合的 gate_id。

### 3.2 Case Matrix（用例矩阵）

表格：`case_id | 攻击/条件向量 | 防御层 | 具体动作 | 期望可观察结果 | 严重度`

规则：

1. 每个 case 必须**独立可执行**：给定具体输入（attacker/victim、参数、数据），断言具体输出（状态码、返回体、日志、时序）；
2. 禁止"等等""and so on""其余类似"——矩阵必须穷尽本 Gate 覆盖面；
3. victim/attacker 在每个 case 内显式定义，跨 Tenant 类 case 必须用两个真实 fixture Tenant；
4. 每个 case 的期望可观察结果必须**机械可判**（状态码/字段存在性/计数/时延上限），不是自然语言描述。

### 3.3 机械判据（Mechanical Criteria）

- 逐 case 断言清单（status、absence_of 字段、presence_of 字段、时延上界）；
- Gate 级规则：**全部 case pass 才 pass；任一 case fail 或 blocked ⇒ Gate blocked**；不允许 skipped 计入通过（ADR-0044）；
- 不允许"降级为通过"：依赖不可用、证据缺失、版本未 pin ⇒ blocked。

### 3.4 证据 schema（机器可读结果清单）

每个 Gate 运行产出 `docs/evidence/gates/<gate_id>/<run-id>.yaml`：

```yaml
gate:
  id: <gate_id>              # 与 T88 聚合键一致
  release_id: <release>      # T88 聚合用
  environment: controlled-beta | production-like
  fingerprint:
    k8s: <version>
    platform_rev: <sha256>
    adapter_rev: <sha256>    # Product Adapter 版本
    patchset_rev: <sha256>   # WeKnora 等上游 patch queue
    product_version: <digest>
  cases:
    - case_id: CASE-01
      vector: <一句话>
      defense_layer: <层>
      action: <执行的攻击/条件动作>
      expect: { status: <int>, absence_of: [<字段>], presence_of: [<字段>], max_ms: <int>? }
      result: pass | fail | blocked
      evidence_ref: artifacts/evidence/<gate_id>/CASE-01.json
  run: { at: <iso8601>, by: <agent|human id> }
  approval:
    platform_engineering: { reviewer: <id>, approved: bool, rationale: <text>, manifest_sha256: <hex> }
    security: { ... }
    finance_billing: { ... }
    product: { ... }
  conclusion: launch | blocked
```

约束：证据文件不得包含客户内容、Prompt、文档正文、Secret、原始支付载荷（基线 §20）；case 级证据引用可复现 artifacts。

### 3.5 四方批准（ADR-0044）

Platform engineering、security、finance/billing、product 四方各自独立记录 reviewer + approved + rationale + 对证据清单的 manifest_sha256。任一方未批准 ⇒ 结论 blocked。审批人不得是证据生成者本人（防自证）。

### 3.6 Fail-Closed 语义

- OpenFGA/Keycloak/PostgreSQL/Temporal 任一依赖不可用 ⇒ 保护类 case blocked，不降级；
- 证据缺失（无 fingerprint、无 case 证据引用）⇒ blocked；
- 生产环境不得"只跑一半 Gate"；T88 聚合时任一 gate blocked ⇒ 上线 blocked。

## 4. 反模式清单（写计划时对照排除）

| ✗ 反模式 | ✓ 替代 |
| --- | --- |
| expect 为目标句原文 | expect 为具体状态码/字段/计数/时延 |
| 单条 side_effect 断言 | 完整 case 矩阵逐 case 断言 |
| skipped/部分通过=通过 | 任一非 pass ⇒ blocked |
| 把 Gate 写成 Command/Port 服务 | Gate 是场景驱动测试资产 + 证据产品 |
| 健康检查/单测/Workflow 成功代替 Gate | 必须跑真实场景矩阵 + 四方批准（ADR-0044） |
| 不记录环境指纹与版本 digest | fingerprint 必填，缺失即 blocked |
| 自然语言"验证…"收尾 | 以案例矩阵 + 判据收尾 |

## 5. 与 T88 的关系

- 每个 Gate 计划实现一个 driver（`tests/production-gates/drivers/<gate_id>.go`），执行场景并产出 §3.4 结果清单；
- T88 只做聚合：读取各 Gate 的结果清单 + 四方批准，输出 release 结论，**不重新实现 Gate 逻辑**；
- Gate 清单的**唯一权威**待裁决（ADR-0044 11 项 vs 基线 §25 14 项分叉，见 2026-08-25 ADR 审计 H1）——T88 计划必须先落定权威清单再实施。

## 6. 模板自检

写完一份 Gate 计划后逐项核对：□ Case Matrix 每行机械可判 □ 无目标句 expect □ 有 fingerprint 要求 □ 有四方批准结构 □ 有 fail-closed 语义 □ 未使用 Command 服务模板。
