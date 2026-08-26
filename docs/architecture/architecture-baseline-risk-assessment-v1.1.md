# 企业级多语言 AI SaaS Platform 架构基线与风险评估 v1.1

> 日期：2026-08-24  
> 状态：访谈定稿，可进入实施规划；尚未授权实施  
> 替代：v1.0

## 1. 执行摘要

本平台正式定位为面向中国大陆大众用户和小型企业的公共多租户 AI Application SaaS。平台不是 WeKnora、RAGFlow 或某种 Agent Framework 的企业发行版，而是围绕独立 AI Product 提供统一身份、入口、商业、用量、授权和生命周期的 Platform。

首发采用：

- 一个中国大陆 Region、多个 Availability Zone；
- Platform 运营的公共 SaaS；
- WeKnora 作为 Lighthouse Product；
- Shared Product Instance 采用容量受限 Cell；
- 平台统一身份、Product Access、订阅、余额、用量和生命周期；
- Product Domain Object、Product Role 和 Product UI 仍由各 Product 所有；
- 内部团队策展 Product Catalog，不开放第三方 Marketplace；
- 平台管理并支付所有模型 Provider，只允许经 Higress 的模型访问；
- 微信支付和支付宝作为首发 Payment Provider；
- 订阅包含 Product/Meter 级额度，超额消费使用 Tenant 级 CNY 预付余额；
- OpenFGA、Temporal、Nacos、OpenMeter、Kafka、ClickHouse 均进入 Day-1；
- OpenChoreo、OpenBao、Harbor、Crossplane 继续延后并采用证据触发。

最关键的架构结论是：

> Platform 必须拥有自己的 Tenant、Product、Product Binding、Canonical Usage、Payment Journal、授权业务关系和 Lifecycle 状态；第三方基础设施只在清晰的 Provider/Adapter 边界之后提供能力。

## 2. 商业产品边界

### 2.1 首发客户

- 大众个人用户；
- 小型企业；
- 中国大陆单区域正式运营；
- 不承诺境外付款、跨境数据、跨区域数据驻留、私有化部署或 Dedicated Cluster；
- 金融、医疗等强监管行业不是 Stage 1 的默认承诺。

### 2.2 首发交付形态

开发、CI、集成测试和最多 10 个受控 Tenant 的封闭测试使用 Docker Compose。付费生产使用单区域、多节点 Kubernetes 和多可用区的 HA 或托管状态服务。

单机 Docker Compose 不承担 99.9% 付费生产承诺。

### 2.3 Lighthouse Journey

首个必须被真实验证的客户旅程为：

```text
企业管理员购买 WeKnora
→ 成员登录
→ 创建知识库
→ 发起问答
→ 产生可核对 Usage
→ 账单可解释
→ 套餐变更不中断
→ Product Version 升级不丢数据
→ 可导出、可停用、可删除
```

RAGFlow 不与 Lighthouse Journey 同时全面铺开；它作为第二个兼容性挑战者，用来推翻错误的通用抽象。

## 3. Stage 1 约束

| 约束 | 目标 |
| --- | --- |
| 封闭测试 | 16 周 |
| 付费上线 | 24 周 |
| 付费 Tenant | 100 |
| 月活 User | 1,000 |
| 并发 AI 请求 | 100 |
| Control Plane 峰值 | 50 RPS |
| Control Plane / Gateway 可用性 | 月度 99.9% |
| Platform metadata / billing fact RPO | 15 分钟以内 |
| Product data RPO | 1 小时以内 |
| 整体 RTO | 4 小时以内 |
| 固定基础设施成本 | 每月 CNY 30,000 以内，不含模型与客户文件存储 |
| 稳定期非模型基础设施成本 | 净收入 20% 以内 |

建议最小团队为 8 人：3 名平台/后端、2 名前端/BFF、1 名基础设施/SRE、1 名测试/安全、1 名产品/设计。少于 6 名全职人员时应删除范围，而不是降低安全与验收门槛。

## 4. 核心领域模型

### 4.1 客户与租户

```text
Billing Customer 1 ─── 1 Tenant
Tenant           1 ─── N Membership
User             1 ─── N Membership
Tenant           1 ─── N Product Binding
```

- Billing Customer 是付款责任主体，可以是个人或小企业；
- Tenant 是硬安全、数据和计费边界；
- Personal Tenant 在个人自助注册时创建；
- Business Tenant 由 Owner 创建并邀请成员；
- 一个 User 可以拥有 Personal Tenant，并同时加入多个 Business Tenant；
- 删除 Tenant 不能级联删除仍属于其他 Tenant 的全局 User；
- Organization 不进入通用 Platform 契约。

### 4.2 平台角色

| Platform Role | 责任 |
| --- | --- |
| Owner | 所有权转移、Tenant 删除、付款与完整访问策略 |
| Admin | 成员、Product 购买、配置与 Product Access |
| Billing Member | 充值、订阅、用量和账单，无 Product Domain Object 访问 |
| Member | 使用已显式授予 Product Access 的 Product |

Personal Tenant 的 Owner 自动获得 Product Access。Business Tenant 购买 Product 后，Owner 自动获得；其他 User 必须显式授权。

Product 内部 Role 不提升为 Platform Role。

### 4.3 Product 模型

```text
Product
  └── Product Version
        └── Product Instance / Cell
              └── Product Binding
```

- Product 是 Catalog 身份；
- Product Version 不可变绑定 upstream、Adapter、Patch Set、schema 和 image digest；
- Product Instance 是容量受限 Shared Cell；
- Product Binding 是 Tenant 到 Product Instance 和 external tenant 的服务器端映射；
- external tenant/user ID 永远不接受客户端提交；
- Tenant、Namespace 和 Product Instance 不是同义词。

Shared Cell 同时限制 Tenant 数、存储、向量、后台任务、模型并发、ingest rate 和数据库规模。容量不足时停止分配新 Tenant，通过显式 Cell Migration Workflow 搬迁。

### 4.4 Product User 映射

```text
Product User Binding:
platform_user_id + product_instance_id → external_user_id

Product Membership Binding:
platform_membership_id + product_binding_id
→ external_membership_id / external_role
```

同一 Platform User 在一个 Shared WeKnora Cell 中只建立一个 Product User，但可以拥有多个相互隔离的 Tenant membership。禁止用 email、手机号或用户名匹配外部身份。

## 5. 总体架构

```text
Customer / User
      │
      ▼
Portal / BFF / Platform API
      │
      ▼
Higress ─────────────── Controlled Egress Proxy
      │                           │
      ├── Casdoor                  └── Approved external targets
      ├── OpenFGA
      ├── Platform Control Plane
      ├── Platform Commerce
      ├── Usage Ingest
      ├── OpenMeter
      └── Product Routes
               │
               ▼
       WeKnora Shared Cells

Durable orchestration: Temporal
AI runtime registry: Nacos
Usage transport: Kafka
Usage analytics/rating store: ClickHouse
Transactional state: PostgreSQL
Persistent dedupe: Valkey
Files/archive/backups: Object Storage
Secrets: Managed Secret Manager + KMS
Observability: OpenTelemetry-compatible stack
```

## 6. Day-1 组件

### 6.1 Platform Application

- Portal；
- BFF；
- SaaS Control Plane；
- Platform Commerce；
- Usage Ingest；
- Immutable Archive Consumer；
- Product Adapter Worker；
- Temporal Worker。

### 6.2 Edge、Identity 与 Authorization

- Higress；
- Controlled Egress Proxy；
- Casdoor；
- OpenFGA；
- Kubernetes Network Policy。

### 6.3 AI Control Plane

- Nacos 3.2.3 GA（截至 2026-08-24 的核查基线）；
- Java 17；
- 至少 3 个 Server node，位于内部 VIP/SLB 后；
- 外置关系数据库与独立备份恢复；
- 独立 Console，只开放管理网络；
- AI Registry Provider/Adapter。

Platform PostgreSQL 仍然拥有 Product 级 AI Runtime Asset metadata。Nacos 只负责运行时注册、版本、发现和分发；业务模型不得依赖 Nacos 内部表。

MCP Registry、Agent Registry、Skill Registry 和 Prompt Registry 在 3.2.x 中的能力与客户端成熟度并不相同。四者进入 Day-1 旁路 PoC，但不得成为“购买 → 登录 → 建库 → 问答 → 用量 → 账单 → 升级”主链路的前置依赖；分别通过版本、鉴权、可见性、下载缓存、回滚和故障模式验收后再转正。

Nacos 保持内网化。生产显式开启 Server/Admin/Console auth，所有节点共享受管的 server identity 和 JWT secret。内置鉴权只作为基础保护，不能代替 Platform Edge、OpenFGA 或网络隔离。3.3.0-BETA 不进入生产基线。

官方核查依据：[Quick Start](https://nacos.io/en/docs/latest/quickstart/quick-start/)、[Nacos 3.2.3 Release](https://github.com/alibaba/nacos/releases/tag/3.2.3)、[Deployment Best Practices](https://nacos.io/docs/latest/manual/admin/deployment/deployment-best-practices/)、[Cluster Deployment](https://nacos.io/en/docs/latest/manual/admin/deployment/deployment-cluster/)、[AI Registry Overview](https://nacos.io/en/docs/latest/manual/user/ai/ai-registry-overview/)。

### 6.4 Billing

常驻 OpenMeter 进程：API、Sink Worker、Balance Worker、Billing Worker。

定时任务：subscription sync、invoice collect、invoice advance、advance-charges。

关闭 Svix 和独立 Notification Service。支付入站 Webhook 由 Platform Commerce 处理。

### 6.5 State 与基础设施

- PostgreSQL；
- Kafka；
- ClickHouse；
- Valkey；
- Object Storage；
- Managed Secret Manager + KMS；
- Managed Private Registry；
- Kubernetes；
- Temporal。

### 6.6 Observability 与供应链

- OpenTelemetry；
- Prometheus-compatible Metrics；
- Grafana；
- Loki-compatible Logs；
- Tempo-compatible Traces；
- License Scan、SBOM、CVE Scan、Cosign signing 和 Admission verification。

## 7. PostgreSQL 隔离

Stage 1 使用一个托管 HA PostgreSQL 服务，但为 Platform、Casdoor、OpenFGA、Temporal、OpenMeter、Nacos 和每个 WeKnora Cell 创建独立 database、role、migration owner、credential、monitoring 和 backup boundary。

禁止跨数据库 Join、共享表或复用迁移账号。Billing 或 Product Cell 达到连接、存储、负载或故障隔离阈值时迁入独立服务。

Nacos 3.2.x 已正式支持 PostgreSQL；必须使用匹配版本的 schema、独立 database 和最小权限账号，并在升级前验证 migration、backup 和 restore。内置 Derby 只用于开发或临时验证，不得用于生产。

## 8. Identity 与 Tenant Context

Casdoor 拥有 Credential、MFA、外部 IdP 和稳定 subject。Platform User 通过 `identity_provider + subject` 建立 Identity Binding。

```text
客户端选择 tenant_id
→ Platform Edge 验证 Membership == active
→ OpenFGA Check == allowed
→ 删除客户端内部身份 Header
→ Gateway 签发短期、audience-bound Platform Context
→ 通过受保护网络访问 Product
```

Platform Context 至少包含：

```text
platform_user_id
tenant_id
product_id
product_instance_id
product_binding_id
audience
issued_at
expires_at
jti
```

Product 不公开直接入口，不信任客户端 `X-Tenant-ID`，不接收长寿命角色 Token。

## 9. OpenFGA

Platform PostgreSQL 是 Membership、Platform Role 和 Product Access 的业务事实源。OpenFGA 是 Authorization Projection 和授权求值器。

```text
Grant:
DB pending → OpenFGA tuple → DB active

Revoke:
DB revoked / immediate deny → async delete tuple
```

访问必须同时满足 `business relationship == active AND OpenFGA == allowed`。

OpenFGA 仅覆盖 Tenant membership/ownership、Platform Role、Product Access、Product Instance administration 和 Billing visibility，不覆盖 Product 内部知识库、会话、文件和 Role。

故障时保护请求 Fail Closed，不缓存 allow。Authorization Model 不可变、版本化，先 Shadow Check 和 Tuple Migration，再切换 Model ID 并保留回滚窗口。

## 10. Product UI 与 API

Portal 拥有 App Center、购买、成员、Usage、Billing 和 Settings。Product 保留原生上游 UI，经 Higress Product route、OIDC SSO 和 Platform Context 访问。

Stage 1 禁止 iframe 和大规模 Product 前端 Fork。

Platform API 只暴露 Tenant、Membership、Catalog、Product Binding、Subscription、Usage、Billing 和 Lifecycle。Product API 保留原生语义，通过 Product-specific SDK 访问。

Capability Contract 只有在多个 Product 证明相同、稳定的业务含义后才能提升为 Platform 契约。

## 11. WeKnora Shared 安全门槛

当前静态审计结论是：WeKnora 部分具备应用层多租户，但当前快照不能安全作为大众 SaaS Shared 生产实例。主要阻断项：

- Repository 存在由 caller 保证隔离的 ID-only 访问；
- PostgreSQL 没有统一 Tenant Scope/RLS；
- 向量记录与过滤缺少 tenant_id；
- 后台任务缺少 Tenant 级公平调度与并发预算；
- Tenant 删除不是完整可验证 Erasure；
- 配额检查缺少原子 Reservation；
- RBAC 可配置为只记录不拒绝；
- System Admin 与 Platform Key 爆炸半径过大。

Paid launch 前必须：

1. 强制 TenantScope，禁止普通 ID-only Repository；
2. 增加 RLS 或等效数据库级隔离；
3. 向量写、查、删全部加入 Tenant filter；
4. 增加 Tenant 级限流、公平调度和原子配额预留；
5. 实现可重试、可审计 Erasure；
6. 生产禁止 RBAC logging-only；
7. 加固 System Admin/Platform Key；
8. 通过真实跨 Tenant 攻击矩阵。

封闭测试可为不超过 10 个 Tenant 使用临时独立实例。Week 12 仍未通过 Shared Gate 时，推迟付费上线或更换 Lighthouse Product，禁止安全豁免。

修复优先提交 upstream，并维护与 `upstream_version + adapter_version + patchset_version` 绑定的最小 Patch Queue。

## 12. Canonical Usage

### 12.1 权威来源

| Meter | Usage Authority |
| --- | --- |
| 模型 Token、模型请求 | Higress / trusted Model Gateway |
| 知识库查询、索引、解析 | Product Adapter server-side collector |
| 订阅、价格、余额 | Billing Control Plane |
| Browser / external client | 永远不是计费事实源 |

### 12.2 Event Schema

```text
event_id
schema_version

tenant_id
product_id
product_instance_id

subject_type
subject_id
actor_user_id?
resource_type?
resource_id?

meter
quantity
unit

occurred_at
ingested_at

source_type
source_id

request_id
trace_id
correction_of?
metadata
```

删除 `organization_id`，不写 `billing_customer_id`。actor 不是计费边界。Metadata 使用白名单，禁止 Secret、Prompt、文档内容和个人敏感信息。

支付金额使用 CNY 分的 int64；Meter quantity 和定价计算使用固定精度 Decimal，禁止浮点数。

### 12.3 平台事实流

```text
Gateway / Product Adapter
          ↓
Platform Usage Ingest
          ↓
Platform-owned Kafka Topic
          ├── Immutable Archive Consumer
          └── OpenMeter Adapter
                    ↓
              OpenMeter Ingest API
                    ↓
              OpenMeter Kafka
                    ↓
              ClickHouse
```

Platform 与 OpenMeter 可共享 Kafka Cluster，但不得共享 Topic 所有权、Schema 和 retention contract。禁止直接写 OpenMeter 内部 Topic。

### 12.4 修正、迟到和定价

- 原 Event 不更新、不删除；错误通过 `correction_of + quantity_delta` 的 Usage Adjustment 修正；
- 24 小时内迟到事件按 `occurred_at` 的历史 Price Version 自动计价；
- 超过 24 小时进入 Reconciliation Review；
- 时钟偏差超过 5 分钟标记异常；
- Rated Result 保存实际 Price Version 与 rounding rule；
- 历史重放不能应用当前价格。

### 12.5 Authorization、Reservation 与 Settlement

```text
Authorize
→ Reserve maximum cost
→ Execute
→ Trusted final Usage Event
→ Settle actual cost
→ Release remainder
```

断流按 Gateway/Provider 最终事实结算。缺少最终事件的 Reservation 由恢复流程核对，不永久占用也不自动释放造成免费消费。

## 13. Subscription、Balance 与 Product Offer

一个 Tenant 可以订阅多个 Product-specific Offer。平台统一结账、CNY 余额和账单视图，但不制造万能套餐。

```text
Product Offer
= price
+ entitlements
+ Product/Meter-specific Included Allowance
+ Data Processing Profile
```

Prepaid Usage Balance 归 Tenant 所有，可跨 Product 付款；Included Allowance 不跨 Meter 兑换；User Budget 只是 Tenant 内部消费护栏，不是独立资金。

Credit Lot 必须保存来源、Payment Order、原始与剩余金额、币种、到期和 refundability。消费顺序采用显式“最早到期优先”。

## 14. Payment、Refund 与电子发票

### 14.1 Source of Truth

Platform Commerce 拥有：Payment Order、Provider Transaction、Refund Order、Webhook Inbox、Immutable Payment Journal 和 Invoice Request。

OpenMeter 拥有 Subscription、Entitlement、Credit/Balance、Usage Pricing 和 Billing。真实资金事实不能只存在 OpenMeter。

### 14.2 Payment 状态

```text
created
→ awaiting_payment
→ paid
→ fulfilled
```

`paid` 表示微信/支付宝确认资金成功；`fulfilled` 表示购买价值已幂等进入 OpenMeter 商业状态。Paid 后 Fulfillment 失败必须重试，不能要求客户再次支付。

Provider 成功回调必须验证签名/证书、App/Merchant、Platform order、唯一 Provider Transaction、CNY 金额、Provider status 和合法状态转换。Webhook、主动查询和对账进入同一个幂等转换。

### 14.3 Refund

退款先原子预留具体可退款 Credit Lot，再调用原支付渠道。Provider 确认后写不可变冲正；失败重试或释放 Reservation。

可退款：尚未消费的现金充值 Lot。不可退款：Included Allowance、促销赠送、已消费金额。

### 14.4 电子发票

Stage 1 支持 Billing Customer Tax Profile 和电子 Invoice Request。开票、红冲和作废必须关联已确认、已履约的 Payment/Provider Transaction。税率、发票类型、时点和法定保留期由中国大陆财税专业意见确认。

## 15. Temporal Lifecycle

Platform PostgreSQL 保存 Desired State、Observed State 和 Lifecycle Operation，是 Portal/API 的事实源。Temporal 保存 Workflow history、retry、timer、signal 和 compensation，不作为业务查询数据库。

Stage 1 Workflow：

- EnableProductWorkflow；
- SuspendProductWorkflow；
- ResumeProductWorkflow；
- UpgradeProductWorkflow；
- EraseTenantWorkflow；
- PaymentFulfillmentWorkflow；
- RefundFulfillmentWorkflow；
- SubscriptionChangeWorkflow；
- CellMigrationWorkflow。

禁止把 API 请求、Usage Event、OpenFGA Check、模型流式响应、普通页面操作和 Provider 签名验证放入 Temporal。

Activity 使用稳定业务幂等键并立即保存 external ID。只补偿安全可逆副作用；已确认支付、已发生 Usage、不可逆迁移和已完成 Erasure 只能通过退款、Adjustment、Restore 或人工处置等前向动作处理。

## 16. Product Version、Upgrade 与 Restore

每个 Product Version 声明：

```text
migration_class
backup_required
restore_tested
rollback_supported
minimum_source_version
maximum_source_version
estimated_downtime
```

Migration Class：none、backward_compatible、forward_only、destructive。

```text
Compatibility Test
→ Backup
→ Restore Rehearsal
→ Drain
→ Deploy
→ Migrate
→ Smoke
→ Observation
→ Promote
```

Shared Cell 从内部测试 Cell、Canary Cell 到批量生产 Cell 推进。Forward-only/destructive 失败从已验证 Backup Restore，不以旧镜像启动冒充 rollback。

完整 Cell recovery set 包括 Product DB、Object manifest、Vector snapshot、configuration、Product Version、Product Binding mapping、Secret refs、Gateway 和 identity configuration。

每次破坏性升级前 Restore Rehearsal；每月完整恢复演练并记录真实 RPO/RTO。

## 17. Tenant Export 与 Erasure

Shared Product Adapter 必须提供 Tenant Export：Product Version、Schema Version、Object checksums、时间和 completeness evidence。Stage 1 不承诺跨 Product import。

Subscription 取消后继续服务到付费周期结束，再进入 30 天 Read-only Retention；允许查看、导出和重新订阅，禁止新增高成本任务。

Retention 结束后 Temporal 执行 Tenant Erasure，覆盖 Platform DB、Product DB、Object Storage、Vector Store、Task queue/cache、Product credential、OpenFGA tuple、Product User/Membership Binding 和 Backup expiration schedule。

完成必须生成 per-store Adapter evidence 的 Erasure Record。Temporal Workflow 成功不等于删除证明。不支持可验证 Export/Erasure 的 Product 不能进入 Shared 生产 Catalog。

## 18. Model、Data Processing 与 Egress

Stage 1 所有模型 Key、费用和 Provider 合同由 Platform 管理；不支持 BYOK。

Product 只能通过 Higress Model Gateway 调用模型，Kubernetes Network Policy 禁止绕过。Product 普通 Internet Egress 默认拒绝，通过受控 Egress Proxy 执行域名/协议策略、DNS 复检、私网与 Metadata 拦截以及大小/时限约束。

每个 Product Offer 绑定 Data Processing Profile：approved providers、processing region、content retention、training-use、supported data classes 和 subprocessors。Higress 不允许把流量自动 Failover 到数据条款不同的 Provider。

## 19. Supply Chain 与 License

```text
Pinned source
→ Isolated build
→ License Report
→ SBOM
→ CVE scan
→ Cosign sign
→ Managed private Registry
→ Admission verification
→ Deploy by digest
```

License Gate 按 Product Version 运行，覆盖源代码、前端资产、Plugin、模型权重、依赖、基础镜像、字体和数据集。未知、自定义、网络 Copyleft 或商业托管权不明确时禁止生产，必须由可承担责任的法律审核者批准。

禁止工程管理员绕过 P0 License Gate，禁止生产直接拉取 Public Registry image。

## 20. Observability、Audit 与 Operator Access

Logs/Traces 可保存受控的 tenant_id、product_binding_id、request_id 和 trace_id，但禁止 Prompt、文档正文、Secret 和原始支付载荷。

Prometheus Metrics 禁止 tenant_id/user_id 高基数标签。Per-Tenant Usage 进入 Canonical Usage/ClickHouse，不冒充 Metrics。

Audit 是独立不可变流，默认保留 12 个月，覆盖 Membership、Role、Product Access、Payment、Refund、Adjustment、Lifecycle、Secret reference/rotation、OpenFGA model/tuple、管理员跨 Tenant 操作、Export 和 Erasure。

Platform operator 无长期跨 Tenant 数据访问。Support 访问要求 case、reason、Tenant Owner consent、MFA、JIT、最短期限、只读优先、Audit、session marking/recording 和自动到期。Emergency Access 只用于正在发生的安全事故，并要求事后通知和复核。

Kill Switch 进入 Quarantine：停止新流量、任务、授权、Provision 和 Upgrade，但保留数据和证据；删除必须走独立 Erasure。

## 21. Reconciliation

每小时 Payment Reconciliation：

```text
WeChat / Alipay
↔ Platform Payment Journal
↔ OpenMeter Fulfillment / Credit
```

每日 Usage Reconciliation：

```text
Platform Kafka / Immutable Archive
↔ OpenMeter Ingest
↔ ClickHouse Rated Usage
↔ Credit / Billing Ledger
```

差异生成 Reconciliation Case。金额冲突、重复 Fulfillment 或无法解释的负余额冻结相关资金动作。修复只能使用新的 Adjustment/Refund/Fulfillment Event。

## 22. Availability 与 Disaster Recovery

Stage 1 在一个中国大陆 Region 内跨多个 Availability Zone：

- 多节点 Kubernetes；
- HA/托管 PostgreSQL、Kafka、ClickHouse 和 Valkey；
- 加密 Backup 位于独立故障域；
- 不做跨 Region Active-Active。

Casdoor、Higress、OpenFGA、Temporal、Nacos、OpenMeter API/Workers 和 Platform Services 均不得以单实例承担生产流量。

## 23. Deferred Components 与引入条件

| Component | Day-1 | Evidence trigger |
| --- | --- | --- |
| Nacos | 是 | 3.2.3 GA 基础集群；AI Registry 四件套逐项旁路 PoC 后转正 |
| OpenChoreo | 否 | 至少 3 个 Product 或 10 个 Cell，并证明重复 lifecycle 成本 |
| OpenBao | 否 | 托管 Secret 无法满足合规、私有化或 PKI |
| Harbor | 否 | 复制、离线、私有化或高级镜像治理需求 |
| Crossplane | 否 | 多云或大规模自助基础设施交付 |

任何新增组件必须先完成 PoC、Provider Contract 和退出方案。

## 24. P0/P1 风险

### P0

1. Shared Tenant 越权；
2. License/SaaS 商业托管权；
3. Usage 丢失、重复、错价和错误重放；
4. Payment、Credit、Refund 对不上；
5. WeKnora Patch/Upstream 升级破坏；
6. Product DB/Vector migration 无法恢复；
7. Secret 或 Platform Context 泄露；
8. 第三方代码和镜像供应链攻击；
9. Erasure/Export 不完整；
10. OpenFGA projection 延迟导致授权不一致。

### P1

1. OpenMeter、Nacos 等依赖版本与接口快速变化；
2. Higress、Casdoor、OpenFGA、Kafka、ClickHouse、Temporal 的集中故障；
3. Shared Cell noisy neighbor；
4. 多组件运维负担超出 8 人团队；
5. 模型 Provider 数据条款与 Fallback 冲突；
6. Managed service 可替换性和成本漂移；
7. 单 Region 灾难。

## 25. 不可豁免 Production Gates

```text
Tenant Isolation
Cross-Tenant Security
License
Payment Reconciliation
Usage Replay
Usage Reconciliation
Backup / Restore
Tenant Export
Tenant Erasure
Secret Rotation
Image Signature
Product Adapter Compatibility
Verified Deduplication Backend (Valkey compatibility or approved Redis fallback)
```

> 2026-08-25 与 ADR-0057 对齐：Nacos Production PoC 移出付费上线不可豁免清单，改为 AI Registry 能力从旁路 PoC 转正的前置门（失败仅降级该能力，不阻塞上线，见 ADR-0051）；ADR-0044 的 11 项枚举由 ADR-0057 修订补齐 Tenant Export 与 Verified Deduplication Backend，以上 13 项为唯一权威清单（ADR-0057，accepted 2026-08-25，amends 0044）。

Platform engineering、安全、财务/计费和产品四方共同批准可复现证据。健康检查、单元测试、静态审计、Workflow 成功或小范围 Smoke Test 不能替代完整验收。

## 26. 外部确认事项

下列事项不是当前架构会议可以自行证明的事实，必须在实施前获得外部或实时证据：

1. 中国大陆隐私、跨境、数据删除和未成年人条款；
2. 电子发票税率、类型、时点、红冲和法定保存期；
3. 微信支付、支付宝商户资质、回调、退款和对账规则；
4. 所有 Product Version 的 License Report 与法律批准；
5. 模型 Provider 的地域、训练使用、内容保留和 subprocessors；
6. 实施时重新确认 Nacos 稳定版本，并完成 Agent/MCP/Skill/Prompt Registry 旁路 PoC；
7. Valkey 对 OpenMeter 当前 Redis 使用方式的真实兼容性；
8. 选定云厂商在中国大陆 Region 的 KMS、Secret、Kafka、ClickHouse、PostgreSQL、Registry 和多 AZ 能力；
9. WeKnora Shared hardening 的真实运行、攻击和恢复验收；
10. OpenMeter 微信/支付宝资金链的 Provider sandbox 与真实商户验收。

## 27. v1.0 → v1.1 的关键变化

1. 从“企业与 SaaS 全覆盖”收紧为中国大陆大众与小企业公共 SaaS；
2. 从默认 Dedicated Namespace 改为付费生产 Shared Cell；
3. 明确 WeKnora 当前 Shared 安全性不足，Paid launch 前必须加固；
4. 删除通用 Organization 层，固定 Billing Customer、Tenant、User 和 Membership 基数；
5. 平台 Role 与 Product Role 分离；
6. Product User 与 Product Membership 映射分离；
7. OpenFGA、Temporal 和 Nacos 进入 Day-1；
8. Platform 自有 Usage Kafka Topic 与 Immutable Archive，OpenMeter 不是原始事实源；
9. Platform Commerce 拥有 Payment Journal，OpenMeter 拥有 Credit/Billing；
10. 引入 Credit Lot、Reservation、Settlement、Adjustment 和 Reconciliation Case；
11. 引入 Shared Cell、Cell Capacity Reservation、Cell Migration 和爆炸半径控制；
12. 明确 Product UI/API 保持上游语义，不做 iframe 和万能 API；
13. OpenBao、Harbor、OpenChoreo 与 Crossplane 采用证据触发而非默认部署；
14. 把所有 P0 Gate 设为不可豁免。

## 28. 最终判断

架构方向可行，但首发风险仍为中高。最大风险不是 Kubernetes 或组件选型，而是：

- 当前 WeKnora 的 Shared Tenant 防御深度不足；
- 资金事实、Usage 事实和 OpenMeter 派生状态必须持续对账；
- OpenFGA、Temporal、Nacos、OpenMeter、Kafka 和 ClickHouse 同时进入 Day-1，使运营复杂度逼近团队上限；
- 24 周上线目标只有在严格控制 Product、Region、Isolation Mode 和 Provider 数量时才可信。

允许进入实施规划的前提是继续保持：

> Platform 业务事实归 Platform；第三方系统提供可替换能力；任何安全、资金、删除或恢复结论必须由可复现证据证明。
