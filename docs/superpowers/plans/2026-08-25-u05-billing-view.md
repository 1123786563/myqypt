# U05 Billing View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付账单页：订阅列表（T65 套餐变更入口）、账单明细（T52）、Credit Lot 消费顺序与退款状态（T60/T61）、以及余额充值入口（U03）——Billing Member 的「我的账单」主页。

**Architecture:** 页面 = /app/billing + BFF 聚合端点 GET /portal-api/billing/summary。BFF 聚合：subscriptions（T65）、bills（T52）、lots（T60 消费/退款状态）、pending 退款（T61）。页面只读为主；写操作仅两个入口：套餐变更（跳 T65 引导流程）与退款申请（针对可退款 Lot 显式提交，携带原因）。所有金额用 U00 formatCnyFen；状态用 U00 StatusBadge。

**Tech Stack:** React 19、TypeScript、Vite、Tailwind 4、TanStack Query、Vitest Browser、Playwright、Go BFF

**Spec:** [GitHub Issue #127](https://github.com/1123786563/myqypt/issues)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §13/§14/§21、ADR-0018、ADR-0053、docs/superpowers/plans/2026-08-24-t52-explainable-bill.md、2026-08-24-t60-credit-lot-consumption.md、2026-08-24-t61-provider-refund.md、2026-08-24-t65-subscription-plan-change.md

## Global Constraints

- 页面展示 Observed 商业状态（订阅 active/suspended、退款 pending/confirmed/失败），不展示 Desired 状态冒充现状。
- 退款只能对可退款 Lot 发起（T61 预留在后端）；前端展示 refundable 标记，申请后展示 pending 状态与金额预留提示，禁止前端「即时退款成功」文案。
- 套餐变更入口只做跳转与引导（T65 后端负责计划与额度迁移），页面不重算价格。
- Credit Lot 按最早到期优先消费顺序展示（T60），已消费金额可折叠查看。
- 金额一律分；不做前端求和——BFF 返回的服务端聚合值直接展示。
- 测试用内存 Billing 数据源；不得包含客户数据。

---

## File Structure

- Create web/src/routes/app/billing.tsx + billing.test.tsx
- Create web/src/features/billing/billing-summary.tsx（聚合视图）+ 测试
- Create web/src/features/billing/refund-request.tsx（退款申请）+ 测试
- Create internal/transport/http/billing_handler.go（GET /portal-api/billing/summary、POST /portal-api/billing/lots/{id}/refund-requests）+ billing_handler_test.go
- Create tests/acceptance/scenarios/u05-billing.yaml + tests/acceptance/u05_billing_test.go
- Create e2e/billing.spec.ts

### Task 1: BFF 账单汇总与退款申请端点

**Interfaces:**
- Produces: GET /portal-api/billing/summary → { subscriptions: [{ product_name, plan_name, state, next_billing_at? }], bills: [{ bill_id, period, total_fen, status }], lots: [{ lot_id, remaining_fen, expires_at?, refundable, refund_state? }], pending_refunds: [{ refund_id, lot_id, amount_fen, state }] }；POST /portal-api/billing/lots/{id}/refund-requests { reason, idempotency_key }。

- [ ] **Step 1: 契约测试（Go）**

```go
func TestBillingSummaryIsTenantScoped(t *testing.T) {
    // then: 只返回当前 TenantScope 的订阅/账单/Lot；无 Billing 权限 → 403
}
func TestRefundRequestOnlyForRefundableLot(t *testing.T) {
    // when: 对不可退款 Lot 或已消费金额申请
    // then: 400/409，且不调用 Provider
}
func TestRefundRequestIsIdempotent(t *testing.T) {
    // when: 同 key 重复提交
    // then: 返回同一 refund_id
}
```

- [ ] **Step 2: 实现 handler**：summary 聚合各端口；退款申请走 T61 预留逻辑，返回 pending 状态。
- [ ] **Step 3: 跑测试**：go test ./internal/transport/http -run Billing -count=1。

### Task 2: 账单页与退款交互

- [ ] **Step 1: 组件测试**

```tsx
it("shows refundable lots with a request action and pending state after submit", async () => {
  // stub: lots: [{lot_id:"L1", remaining_fen:6000, refundable:true}, {lot_id:"L2", remaining_fen:500, refundable:false}]
  render(<BillingPage />);
  expect(await screen.findByText("¥60.00")).toBeTruthy();
  // L1 有「申请退款」，L2 无；提交后按钮变禁用 + 显示「退款处理中」
});
it("does not compute totals on the client", () => {
  // 渲染 BFF 返回的 total 值，页面源码不含 reduce 求和逻辑（code review 断言）
});
```

- [ ] **Step 2: 实现页面**：订阅卡（状态徽章 + 「变更套餐」入口）→ 账单表（点击可展开 T52 明细）→ Credit Lot 表（剩余、到期、消费顺序提示、退款操作）→ 退款申请 Dialog（原因必填、提交后 pending）。充值入口链接 U03。
- [ ] **Step 3: 浏览器测试全绿**。

### Task 3: Playwright smoke 与黑盒场景

- [ ] **Step 1: e2e/billing.spec.ts**：① Billing Member 全流程查看；② 对不可退款 Lot 无申请按钮；③ 权限不足 403；④ 会话过期。
- [ ] **Step 2: tests/acceptance/scenarios/u05-billing.yaml**：正常流、退款预留失败（余额不足）、幂等退款、跨 Tenant 隔离。
- [ ] **Step 3: 提交**：git commit -m "feat(web): billing view with subscriptions, bills and refund requests"

## Self-Review Record

- Spec coverage: 订阅/账单/Lot/退款四区、可退款守卫、幂等、无客户端求和均有机械断言。
- Placeholder scan: 无目标句；每步有具体输入输出。
- Type consistency: refund_state 枚举与后端一致；金额用分。
- Right-sizing: 套餐变更只做入口；退款预留逻辑全部在后端。
