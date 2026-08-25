# U03 Balance Recharge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付余额充值页：展示 Prepaid Usage Balance 总览（Credit Lot 汇总，T59/T60），选择充值金额 → 创建 Payment Order → 渲染支付凭据 → 只读轮询 → paid 后展示 Credit Lot 履约结果。充值即"购买余额"，与 U02 共享支付 UI 骨架但订单语义不同（无 Offer，金额由用户选择）。

**Architecture:** 页面 = /app/billing/recharge + BFF 端点 GET /portal-api/billing/balance（汇总：总余额、各 Credit Lot 的来源/金额/剩余/到期/可退款，T59 字段）+ POST /portal-api/billing/orders（创建充值 Payment Order）。支付凭据渲染复用 U02 的 payment-qr 组件（作为 U00 扩展或本地组件）；金额选择只允许预设档位（后端白名单校验），前端不做金额计算。paid 未履约时显示「正在入账」，轮询至 Credit Lot 出现（T59 fulfillment 证据）。

**Tech Stack:** React 19、TypeScript、Vite、Tailwind 4、TanStack Query、Vitest Browser、Playwright、Go BFF

**Spec:** [GitHub Issue #125](https://github.com/1123786563/myqypt/issues)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §13/§14、ADR-0010、ADR-0053、docs/superpowers/plans/2026-08-24-t59-credit-lot-fulfillment.md、2026-08-24-t60-credit-lot-consumption.md

## Global Constraints

- 余额是 Tenant 级资产（基线 §13）；页面只展示当前 TenantScope 的 Credit Lot。
- 充值金额档位由后端配置白名单（如 50/100/200/500 CNY），前端只传档位 ID，不传任意金额。
- Credit Lot 必须展示来源、原始与剩余金额、币种、到期与可退款性（T59/T60 字段）；禁止把 Lot 汇总成无来源的总数后展示。
- 支付状态机与凭据生命周期规则同 U02（paid 后无二次支付、凭据仅 awaiting_payment 渲染）。
- 展示金额用 U00 formatCnyFen；到期按最早到期优先的消费顺序给出提示（T60）。
- 测试用内存支付/履约 Adapter；不得包含客户数据。

---

## File Structure

- Create web/src/routes/app/billing/recharge.tsx + recharge.test.tsx
- Create web/src/features/billing/queries.ts（useBalance、useCreateRechargeOrder）
- Create internal/transport/http/billing_handler.go（balance 汇总 + 充值订单）+ billing_handler_test.go
- Create tests/acceptance/scenarios/u03-recharge.yaml + tests/acceptance/u03_recharge_test.go
- Create e2e/recharge.spec.ts

### Task 1: BFF 余额汇总与充值订单端点

**Interfaces:**
- Produces: GET /portal-api/billing/balance → { total_fen, lots: [{ lot_id, source_order_id, original_fen, remaining_fen, currency: "CNY", expires_at?, refundable, consumed_fen }] }；POST /portal-api/billing/orders { amount_tier_id, idempotency_key } → 支付订单视图（同 U02 结构）。

- [ ] **Step 1: 契约测试（Go）**

```go
func TestBalanceIsTenantScoped(t *testing.T) {
    // given: tenant A 充值 3 个 Lot，tenant B 充值 1 个 Lot
    // when: 以 tenant A 调用
    // then: 只返回 tenant A 的 lots，且 total_fen == 三 Lot 剩余之和（服务端聚合）
}
func TestRechargeRejectsUnlistedAmountTier(t *testing.T) {
    // when: amount_tier_id 不在白名单
    // then: 400，且不创建 Payment Order
}
func TestRechargeCreatesLotOnlyAfterFulfillment(t *testing.T) {
    // when: 订单 paid 后重复查询
    // then: 视图显示单次履约，Credit Lot 只出现一次（T59 幂等）
}
```

- [ ] **Step 2: 实现 handler**：余额汇总在 BFF 按 Lot 聚合（服务端求和，前端不做金额运算）；充值订单创建复用 T56/T59 能力，档位白名单在 BFF 校验。
- [ ] **Step 3: 跑测试**：go test ./internal/transport/http -run Billing -count=1。

### Task 2: 充值页与 Lot 展示

- [ ] **Step 1: 组件测试**

```tsx
it("lists credit lots with source, expiry and refundability", async () => {
  // stub: lots: [{original_fen:10000, remaining_fen:6000, expires_at:"2026-12-31", refundable:true}]
  render(<RechargePage />);
  expect(await screen.findByText("¥100.00")).toBeTruthy();   // original
  expect(screen.getByText("¥60.00")).toBeTruthy();            // remaining
  expect(screen.getByText(/2026-12-31/)).toBeTruthy();
  expect(screen.getByText("可退款")).toBeTruthy();
});
it("does not render an arbitrary amount input", () => {
  // 只渲染白名单档位按钮，无自由金额输入框
  expect(screen.queryByRole("spinbutton")).toBeNull();
});
```

- [ ] **Step 2: 实现页面**：顶部余额总览（U00 金额格式化）→ Credit Lot 明细表（来源、原始/剩余、到期、可退款徽章）→ 档位选择 → 支付流程（复用 U02 状态机 UI：创建订单 → QR → 轮询 → paid「正在入账」→ Lot 出现即完成）。
- [ ] **Step 3: 浏览器测试全绿**。

### Task 3: Playwright smoke 与黑盒场景

- [ ] **Step 1: e2e/recharge.spec.ts**：① 完整充值流；② 未列出档位不可选；③ 会话过期。
- [ ] **Step 2: tests/acceptance/scenarios/u03-recharge.yaml**：正常流、跨 Tenant 隔离、档位白名单拒绝、重复回调单次入账。
- [ ] **Step 3: 提交**：git commit -m "feat(web): prepaid balance recharge with credit lot display"

## Self-Review Record

- Spec coverage: Lot 字段完整性、档位白名单、服务端聚合、幂等入账均有机械断言。
- Placeholder scan: 无目标句；每步有具体输入输出。
- Type consistency: 金额一律分；currency 固定 CNY 由 BFF 断言。
- Right-sizing: 复用 U02 支付状态机，不重复实现支付逻辑。
