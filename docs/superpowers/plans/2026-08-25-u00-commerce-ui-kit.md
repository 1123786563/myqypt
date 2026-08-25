# U00 Commerce UI Kit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 Portal 商业页面共用的展示组件：金额（CNY 分）、状态徽章（Observed State 全集）、用量进度、空状态，并证明其格式与边界正确。

**Architecture:** 纯展示组件，位于 `web/src/components/commerce/`，不持有业务状态、不发请求；金额组件只负责"分 → ¥ 字符串"的确定转换（不运算），状态徽章只接受 Observed State 枚举，未知值渲染为 `processing` 且记录错误边界。全部组件通过浏览器组件测试锁定格式。

**Tech Stack:** React 19、TypeScript、Vite、Tailwind 4、shadcn、Vitest Browser、Playwright

**Spec:** [GitHub Issue 待创建（U00，#122 起，见 portal-ui-index）](https://github.com/1123786563/myqypt/issues)、`docs/architecture/architecture-baseline-risk-assessment-v1.1.md` §4/§13/§17、`docs/superpowers/specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md` §5.2

## Global Constraints

- 金额以 CNY 分 int64 传输；组件输入为 `number`（分），输出 `"¥1,234.56"` 字符串；负数渲染 `-¥1,234.56`。
- 禁止浮点运算：组件内部只做整数取整（分/100 商 + 余数补零），不执行任何加减乘除后展示。
- 状态徽章只接受 Observed State 枚举（absent/provisioning/active/degraded/suspended/erasing/erased，基线 §4），未知值编译期报错或渲染 processing。
- 组件不得 import 任何数据层、store、路由或 i18n 文本表；文案由调用页传入。
- 测试不得包含真实客户数据。

---

## File Structure

- Create `web/src/components/commerce/currency.ts`：`formatCnyFen(fen: number): string`
- Create `web/src/components/commerce/currency.test.ts`
- Create `web/src/components/commerce/status-badge.tsx`：`StatusBadge({ state, label }: { state: ObservedState; label?: string })`
- Create `web/src/components/commerce/status-badge.test.tsx`
- Create `web/src/components/commerce/usage-progress.tsx`：`UsageProgress({ used, allowance, unit })`（含超用警示态）
- Create `web/src/components/commerce/usage-progress.test.tsx`
- Create `web/src/components/commerce/empty-state.tsx`：`EmptyState({ title, action? })`
- Create `web/src/components/commerce/empty-state.test.tsx`

### Task 1: 金额格式化与状态徽章

**Interfaces:**
- Produces: `formatCnyFen` 与 `StatusBadge`；`StatusBadge` 的 `state` 枚举类型从 `api/openapi` 生成的 TS 类型导入。

- [ ] **Step 1: 写失败用例（先锁格式）**

```ts
import { describe, expect, it } from "vitest";
import { formatCnyFen } from "./currency";

describe("formatCnyFen", () => {
  it("formats integer yuan with two decimals", () => {
    expect(formatCnyFen(123456)).toBe("¥1,234.56");
  });
  it("pads zero fen and zero jiao", () => {
    expect(formatCnyFen(100)).toBe("¥1.00");
  });
  it("renders negative amounts with minus sign", () => {
    expect(formatCnyFen(-50)).toBe("-¥0.50");
  });
  it("does not emit floating point artifacts", () => {
    expect(formatCnyFen(199)).toBe("¥1.99");
    expect(formatCnyFen(200)).toBe("¥2.00");
  });
});
```

Expected: FAIL（`currency.ts` 不存在）。

- [ ] **Step 2: 实现 `formatCnyFen`（纯整数转换，无业务运算）**

```ts
const nf = new Intl.NumberFormat("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
export function formatCnyFen(fen: number): string {
  if (!Number.isInteger(fen)) throw new TypeError("fen must be an integer");
  const sign = fen < 0 ? "-" : "";
  const abs = Math.abs(fen);
  const yuan = Math.floor(abs / 100);
  const fenPart = abs % 100;
  return sign + "¥" + nf.format(yuan + fenPart / 100);
}
```

注意：`fenPart / 100` 只用于取整后两位小数，不参与业务运算；`Math.abs` 对 `Number.MIN_SAFE_INTEGER` 以下输入由 `Number.isInteger` + 显式上限守卫兜底（Step 3 覆盖）。

- [ ] **Step 3: 跑测试并补边界**：`npx vitest run web/src/components/commerce/currency.test.ts`；补 `formatCnyFen(Number.MAX_SAFE_INTEGER)` 不抛错、非整数抛 TypeError 两个用例。

- [ ] **Step 4: 状态徽章（组件测试锁定状态映射）**

```tsx
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { StatusBadge } from "./status-badge";

describe("StatusBadge", () => {
  it.each(["active", "degraded", "suspended", "provisioning", "erasing", "erased", "absent"] as const)(
    "renders a deterministic badge for %s",
    (state) => {
      render(<StatusBadge state={state} />);
      expect(screen.getByRole("status")).toHaveAttribute("data-state", state);
    },
  );
});
```

Expected: FAIL。实现 `StatusBadge`：按 `state` 映射 Tailwind 类（active=绿、degraded=黄、suspended=灰、provisioning/erasing=蓝 + 动画、absent/erased=无色描边），`role="status"` + `data-state`，label 缺省用枚举的中文展示文案。

- [ ] **Step 5: 提交**：`git commit -m "feat(web): commerce currency and status badge components"`

### Task 2: 用量进度与空状态

- [ ] **Step 1: 失败用例（超用警示 + 除零）**

```tsx
it("shows warning when used exceeds allowance", () => {
  render(<UsageProgress used={110} allowance={100} unit="tokens" />);
  expect(screen.getByRole("progressbar")).toHaveAttribute("data-over", "true");
});
it("renders full allowance without dividing by zero", () => {
  render(<UsageProgress used={0} allowance={0} unit="tokens" />);
  expect(screen.getByText(/0\/0 tokens/)).toBeTruthy();
});
```

- [ ] **Step 2: 实现 `UsageProgress`**：`role="progressbar"`、`aria-valuenow/max`、`data-over`（used > allowance）、文案 `used/allowance unit`；allowance=0 且 used=0 显示 `0/0 unit`，allowance=0 且 used>0 恒 `data-over`。
- [ ] **Step 3: 空状态**：`EmptyState` 渲染 title + 可选 action 按钮，测试断言 action 只渲染一次、不渲染时无多余 DOM。
- [ ] **Step 4: 提交**：`git commit -m "feat(web): usage progress and empty state components"`

## Self-Review Record

- Spec coverage: 金额/状态/进度/空态四类共享展示件齐全，边界（负数、除零、未知状态）有机械断言。
- Placeholder scan: 全部步骤含可运行测试与具体断言，无"invariant 目标句"。
- Type consistency: `ObservedState` 来自 OpenAPI 生成类型，组件不重复定义枚举。
- Right-sizing: 纯展示无状态，为 U01–U08 提供共享依赖，不提前引入图表库。
