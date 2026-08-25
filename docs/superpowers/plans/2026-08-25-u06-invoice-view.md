# U06 Invoice View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付发票页：Billing Customer Tax Profile 管理（T63）、对已确认且 fulfilled 的付款发起开票请求、发票历史与作废/红冲入口——仅当 T63 后端与财税外部确认（T86.2）就绪后启用。

**Architecture:** 页面 = /app/billing/invoices + BFF 端点 GET /portal-api/billing/invoices（历史与可开票付款清单）、POST /portal-api/billing/invoice-requests（开票）、POST /portal-api/billing/invoice-requests/{id}/void（作废/红冲）。可开票范围由 BFF 依据 T62 对账事实（已确认 + 已履约 + 未开票）计算；税率/类型/时点等字段只展示 T86.2 确认结果，前端不自行推断。该页为渐进增强：T63/T86.2 未就绪时整页返回「暂不可用」而非半成品表单。

**Tech Stack:** React 19、TypeScript、Vite、Tailwind 4、TanStack Query、Vitest Browser、Playwright、Go BFF

**Spec:** [GitHub Issue 待创建（U06，#122 起，见 portal-ui-index）](https://github.com/1123786563/myqypt/issues)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §14.4、ADR-0054、docs/superpowers/plans/2026-08-24-t63-tax-profile-invoice.md、2026-08-24-t86-2-tax-electronic-invoice.md

## Global Constraints

- 开票/作废/红冲只对已确认且 fulfilled 的 Payment/Provider Transaction 发起（基线 §14.4）；可开票清单由 BFF 依据对账事实计算，前端不筛选。
- 税号、发票类型、税率、开具时点以 T86.2 财税确认 + T63 后端为准；未确认前页面禁用开票动作。
- 作废/红冲为显式二次确认动作（输入原因），成功后展示法定保留提示，不做「撤销」。
- 发票状态机（requested/issued/voided/reversed）来自后端；前端只渲染。
- 测试用内存 Tax/Invoice Adapter；不得包含客户税号真实数据（用脱敏 fixture）。

---

## File Structure

- Create web/src/routes/app/billing/invoices.tsx + invoices.test.tsx
- Create web/src/features/invoices/queries.ts（useInvoiceList、useRequestInvoice、useVoidInvoice）
- Create internal/transport/http/invoice_handler.go + invoice_handler_test.go
- Create tests/acceptance/scenarios/u06-invoices.yaml + tests/acceptance/u06_invoices_test.go
- Create e2e/invoices.spec.ts

### Task 1: BFF 发票端点

**Interfaces:**
- Produces: GET /portal-api/billing/invoices → { eligible: [{ payment_order_id, amount_fen, occurred_at, already_invoiced: false }], invoices: [{ invoice_id, type, amount_fen, state, issued_at?, tax_profile_snapshot }] }；POST /portal-api/billing/invoice-requests { payment_order_id, idempotency_key }；POST /portal-api/billing/invoice-requests/{id}/void { reason, idempotency_key }。

- [ ] **Step 1: 契约测试（Go）**

```go
func TestEligibleListOnlyConfirmedAndFulfilled(t *testing.T) {
    // given: paid 未 fulfilled 的订单 + 已开票的订单
    // then: eligible 只含「confirmed + fulfilled + 未开票」的订单
}
func TestInvoiceRequestRejectsDuplicate(t *testing.T) {
    // when: 对已开票订单再次请求
    // then: 409，且不产生新发票
}
func TestVoidRequiresReasonAndStateGuard(t *testing.T) {
    // when: 无 reason 或发票已 voided
    // then: 400/409
}
func TestInvoicePageDisabledBeforeConfirmation(t *testing.T) {
    // when: T86.2 未确认（feature flag off）
    // then: 端点返回 503「暂不可用」，无半成品表单
}
```

- [ ] **Step 2: 实现 handler**：eligible 计算复用 T62 对账事实端口；开票/作废走 T63；feature flag（T86.2 确认）在 BFF 统一闸口。
- [ ] **Step 3: 跑测试**：go test ./internal/transport/http -run Invoice -count=1。

### Task 2: 发票页

- [ ] **Step 1: 组件测试**

```tsx
it("hides request actions while tax confirmation is pending", async () => {
  // stub: 503/disabled → 页面渲染「发票服务暂不可用」，无任何开票按钮
});
it("shows eligible payments and issues one invoice per explicit click", async () => {
  // stub: eligible 1 条 → 点击「申请开票」→ 出现 requested 状态；重复点击被幂等拦截
});
it("requires reason before void", async () => {
  // 点击「作废」→ 无原因时按钮禁用/校验错误
});
```

- [ ] **Step 2: 实现页面**：税号 Profile 摘要（脱敏展示，T63 提供）→ 可开票列表（金额/时间/「申请开票」）→ 发票历史（状态徽章、作废/红冲二次确认）。法定保留提示展示于作废/红冲成功后。
- [ ] **Step 3: 浏览器测试全绿**。

### Task 3: Playwright smoke 与黑盒场景

- [ ] **Step 1: e2e/invoices.spec.ts**：① 确认就绪后的完整开票流；② 未就绪时整页不可用；③ 权限不足 403；④ 会话过期。
- [ ] **Step 2: tests/acceptance/scenarios/u06-invoices.yaml**：正常流、重复开票拒绝、作废需原因、跨 Tenant 隔离。
- [ ] **Step 3: 提交**：git commit -m "feat(web): invoice requests with tax profile and void flow"

## Self-Review Record

- Spec coverage: 可开票范围、重复拒绝、作废守卫、feature flag 闸口均有机械断言。
- Placeholder scan: 无目标句；每步有具体输入输出。
- Type consistency: 发票状态机来自后端；前端不推断税率/类型。
- Right-sizing: 渐进增强，未确认前整页不可用，避免半成品。
