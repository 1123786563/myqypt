# U02 Product Checkout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付购买结账流程：Offer 选择 → 创建 Payment Order → 渲染微信/支付宝支付凭据（二维码/跳转）→ 只读轮询支付状态 → paid 后展示履约状态（enabling/active）→ 成功后进入 App Center。覆盖 created → awaiting_payment → paid → fulfilled 全链路 UI（基线 §14.2）。

**Architecture:** 结账页位于 /app/checkout?offer_id=…。浏览器经 BFF POST /portal-api/checkout/orders 创建 Payment Order（幂等键），GET /portal-api/checkout/orders/{id} 只读轮询状态；BFF 复用支付 Provider Adapter（T56 契约）与履约流程（T18/T20），支付状态机完全在后端，前端不构造任何支付意图。支付凭据渲染：Provider 返回 QR content 或跳转 URL（T57/T58 契约字段），UI 仅负责展示；paid 未 fulfilled 时显示「正在开通，请勿重复支付」，轮询至 fulfilled 后跳 App Center。禁止前端拦截器重试 POST（抽取设计 §9）。

**Tech Stack:** React 19、TypeScript、Vite、Tailwind 4、TanStack Query、qrcode 渲染（shadcn 兼容、本地化）、Vitest Browser、Playwright、Go BFF

**Spec:** [GitHub Issue #124](https://github.com/1123786563/myqypt/issues)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §13/§14、ADR-0010、ADR-0018、ADR-0021、docs/superpowers/plans/2026-08-24-t56-payment-provider-conformance.md、2026-08-24-t57-wechat-paid-path.md、2026-08-24-t58-alipay-paid-path.md、2026-08-24-t64-paid-weknora-purchase.md

## Global Constraints

- 支付状态机（created/awaiting_payment/paid/fulfilled）只存在于后端；前端只读轮询，永不 POST 确认或取消支付。
- 创建订单必须携带 IdempotencyKey（页面会话生成一次）；重复点击「去支付」返回同一订单，不创建新单。
- paid → fulfilled 之间只能展示中间态「正在开通」并继续轮询；不允许 UI 触发再次支付或取消。
- 支付凭据（二维码）仅在订单处于 awaiting_payment 时渲染；paid/fulfilled/失败后清空，防止泄露重放。
- Provider 原始错误不得透传（抽取设计 §9）；展示稳定 code 文案。
- 金额展示用 U00 formatCnyFen；价格来源为 F14 已发布 Catalog 快照，页面不自行定价。
- 测试用内存支付 Adapter（T56 conformance harness 的 fake），不触真实 Provider。

---

## File Structure

- Create web/src/routes/app/checkout.tsx + checkout.test.tsx
- Create web/src/features/checkout/queries.ts（useCreateOrder、useOrder 轮询）
- Create web/src/features/checkout/payment-qr.tsx（凭据渲染）+ 测试
- Create internal/transport/http/checkout_handler.go（BFF：创建订单 + 查询订单）+ checkout_handler_test.go
- Create tests/acceptance/scenarios/u02-checkout.yaml + tests/acceptance/u02_checkout_test.go
- Create e2e/checkout.spec.ts

### Task 1: BFF 订单创建与查询端点

**Interfaces:**
- Consumes: TenantScope、Catalog 快照（F14）、Offer（T15）、Payment Provider Adapter（T56）、Fulfillment 流程状态（T18/T20 的 Desired/Observed）。
- Produces: POST /portal-api/checkout/orders { offer_id, idempotency_key } → { order_id, state, amount_fen, payment: { channel, qr_content?|redirect_url? } }；GET /portal-api/checkout/orders/{id} → 同构状态视图。

- [ ] **Step 1: 契约测试（Go）**

```go
func TestCheckoutCreateIsIdempotent(t *testing.T) {
    // when: 同 idempotency_key 连续创建两次
    // then: 返回同一 order_id，且只产生一条 Payment Order
}
func TestCheckoutNeverExposesProviderError(t *testing.T) {
    // when: fake Provider 返回签名失败/网络错
    // then: 响应为稳定 Problem Details code，不含 Provider 内部错误
}
func TestCheckoutStateTransitionIsServerOwned(t *testing.T) {
    // when: 查询 order
    // then: state 只来自后端状态机；paid 后即使 Provider 重复回调也仍为单次 fulfilled
}
```

Expected: FAIL（handler 不存在）。

- [ ] **Step 2: 实现 handler**：创建时校验 Offer 在已发布 Catalog 快照内、amount 以分返回；支付凭据字段按 T56 契约映射；查询端点聚合 Payment Order 与 Fulfillment 状态为 UI 视图。
- [ ] **Step 3: 跑测试**：go test ./internal/transport/http -run Checkout -count=1。

### Task 2: 结账页面与状态机 UI

- [ ] **Step 1: 组件测试**

```tsx
it("creates one order per idempotency key on repeated clicks", async () => {
  // stub: useCreateOrder 记录调用次数
  // when: 连点两次「去支付」
  // then: 调用次数 === 1（hook 内按 key 去重），页面只渲染一次二维码
});
it("renders QR only while awaiting_payment", async () => {
  // stub: {state:"awaiting_payment", payment:{qr_content:"weixin://..."}}
  render(<CheckoutPage />);
  expect(await screen.findByTestId("payment-qr")).toBeTruthy();
  // stub 切换为 {state:"paid"} 后：二维码消失，出现「正在开通」文案，无「再次支付」按钮
});
it("redirects to app center after fulfilled", async () => {
  // stub: {state:"fulfilled"} → 断言跳转 /app
});
```

- [ ] **Step 2: 实现页面**：Offer 摘要（名称/价格/含额 Allowance）→ 创建订单 → awaiting_payment 渲染二维码/跳转按钮（带「支付完成后自动跳转」倒计时）→ 轮询（2s 间隔，最多 10 分钟，超时给「支付结果确认中」而非失败）→ paid 显示「正在开通」→ fulfilled 跳 /app。取消按钮在 paid 后禁用。
- [ ] **Step 3: 支付凭据组件**：payment-qr.tsx 只接收 qr_content / redirect_url 与 channel；不在页面保存凭据状态；离开页面即卸载。
- [ ] **Step 4: 浏览器测试全绿**。

### Task 3: Playwright smoke 与黑盒场景

- [ ] **Step 1: e2e/checkout.spec.ts**：① 完整流（stub Provider：创建→awaiting_payment→paid→fulfilled→跳 /app）；② paid 后重复轮询不出现「再次支付」；③ 会话过期中途 → 登录页。
- [ ] **Step 2: tests/acceptance/scenarios/u02-checkout.yaml**：正常流、重复创建幂等、Provider 失败稳定错误码、paid 后无二次支付四类断言。
- [ ] **Step 3: 提交**：git commit -m "feat(web): product checkout with server-owned payment states"

## Self-Review Record

- Spec coverage: created→awaiting_payment→paid→fulfilled 四态 UI、幂等创建、paid 后无二次支付、凭据生命周期均有机械断言。
- Placeholder scan: 契约测试与组件测试均有具体输入输出；无 invariant 目标句。
- Type consistency: 状态与支付订单视图来自 BFF 单一契约；前端不定义支付状态机。
- Right-sizing: 不实现支付逻辑，只做状态展示与跳转；二维码渲染为本地化小组件。
