# Portal 商业 UI 系列 U00–U08 Implementation Plan Index

> 日期：2026-08-25
> 状态：新增系列，填补 Stage-1 计划语料的 Portal 商业页面缺口
> 触发来源：2026-08-25 架构与设计审计（P1-1：100 份 T 计划零前端工作；T87 Lighthouse Journey 的 9 个 blocker 全是后端计划，主链路 UI 无人交付）

**Goal：** 为风险基线 §10 定义的 Portal 职责（App Center、购买、成员、Usage、Billing、Settings）补齐可独立审阅、可执行的商业页面垂直切片，使 T87 黑盒旅程与 T88 付费上线 Gate 有可验收的 UI。

**Spec:** [GitHub Issue #88 T87 Lighthouse Journey](https://github.com/1123786563/myqypt/issues/88)、`docs/architecture/architecture-baseline-risk-assessment-v1.1.md` §10/§12/§13/§14/§17、`docs/superpowers/specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md` §5/§7/§8/§9

## 为什么需要这个系列（审计依据）

- 风险基线 §10："Portal 拥有 App Center、购买、成员、Usage、Billing 和 Settings"；
- 但 100 份 T 计划零前端文件（`web/`、`src/`、`*.tsx` 0 命中），F01–F21 只交付工程基础（AppShell、DataTable、公共页预渲染、Session）；
- T87 依赖的 9 个 blocker 全部是后端计划；无任何计划建设 T87 要验收的页面 → 主链路无法闭环。

## 执行地图

| ID | Issue | Plan | Direct prerequisites（计划/issue） | 消费的后端能力 |
| --- | --- | --- | --- | --- |
| U00 | 待创建（#122+） | [Commerce UI Kit](2026-08-25-u00-commerce-ui-kit.md) | F07、F09、F12 | —（纯前端共享组件） |
| U01 | 待创建 | [App Center](2026-08-25-u01-app-center.md) | U00、F07、F12 | T13 Catalog、T19 Lifecycle 状态查询、T24 SSO 入口、T64 购买启用 |
| U02 | 待创建 | [Product Checkout](2026-08-25-u02-product-checkout.md) | U00、F12、F14、F15 | T14/T15 版本与 Offer、T16/T17/T56/T57/T58 支付、T18/T20 履约启用、T64 |
| U03 | 待创建 | [Balance Recharge](2026-08-25-u03-balance-recharge.md) | U00、F12 | T16/T17/T56/T57/T58 支付、T59 Credit Lot 履约、T60 消费展示 |
| U04 | 待创建 | [Usage View](2026-08-25-u04-usage-view.md) | U00、F12、F13 | T42–T44 用量管线、T45 Allowance、T46/T48 计价结算、T52 可解释账单、T60 |
| U05 | 待创建 | [Billing View](2026-08-25-u05-billing-view.md) | U00、F12、F13 | T19、T52、T60、T61 退款、T65 套餐变更 |
| U06 | 待创建 | [Invoice View](2026-08-25-u06-invoice-view.md) | U00、F12 | T62 对账事实、T63 税号与发票请求 |
| U07 | 待创建 | [Member Management](2026-08-25-u07-member-management.md) | U00、F12、F13 | T04 企业租户、T05 邀请、T06 角色、T08/T09 授权、T24 SSO |
| U08 | 待创建 | [Settings](2026-08-25-u08-settings.md) | U00、F12 | T03 租户上下文、T63 税号、T70 导出、T71 只读保留、T72 擦除状态 |

> Issue 创建约定：本系列 issue 在 main 推送后创建（编号 #122 起），创建后回填各计划 **Spec** 行的 Issue 链接，并按 issue-tracker.md 把 U01–U08 登记为 T87（#88）与 T88（#89）的 blocker。sync_stage1_plans_to_github.rb 需随 F 系列一起改造后才可再运行，本系列不依赖该脚本。

## Cross-Plan Invariants（本系列所有计划必须遵守）

1. **浏览器永不持有 Token**：所有页面数据只经 `/portal-api` BFF 聚合端点；BFF 复用与 Public API 相同的 Application Module 与授权规则（抽取设计 §7.3、§8.1），浏览器禁止直连 T 系列后端 API。
2. **Tenant 访问**：页面数据请求只携带 Session；Tenant Scope 由 BFF 依据 PostgreSQL active Membership + OpenFGA allow 生成（F11/F12），客户端从不提交 `X-Tenant-ID` 等身份头。
3. **金额展示**：所有金额以 CNY 分（int64）传输，前端仅渲染为 ¥ + 两位小数字符串；禁止在 JS 中做金额运算或比较。
4. **状态语义**：页面展示 Observed State（T19），不展示 Desired State 冒充现状；未知状态渲染"处理中"并轮询，不渲染为成功或失败。
5. **幂等与重试**：购买/充值/开票等 POST 只由用户显式动作触发；通用前端拦截器不得无条件重试写接口（抽取设计 §9）；支付状态用只读轮询。
6. **错误处理**：401/403/404/500/503 走 F08 RouteErrorBoundary；RFC Problem Details 的稳定 code 映射为页面文案，不展示内部错误原文。
7. **无演示数据**：不引入假产品、假账单、假用户；空状态必须有可操作引导。
8. **证据**：每个页面至少一条 Playwright smoke（正常流 + 权限不足 + 会话过期）与浏览器组件测试，测试与证据按 platformtest 约定记录，且不得包含客户内容。
9. **失败边界**：任一后端依赖不可用时页面必须 fail closed 或明确降级为"暂不可用"，不得渲染误导性成功状态。

## 与本系列冲突的既有决策

- 本系列页面全部位于 `/app/*`（SPA fallback），不进入公共预渲染路由（F15 只覆盖公开产品/价格页）。
- App Center 进入 Product 原生 UI 一律走 OIDC SSO（T24），禁止 iframe（ADR 0035）。
