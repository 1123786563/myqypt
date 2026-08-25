# U07 Member Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付成员管理页（仅 Business Tenant，T04）：邀请成员（T05）、分配/变更 Platform Role（T06）、授予/撤销 Product Access（T08/T09）、查看成员变更审计（T07）——Owner/Admin 可用，Member 只读自己的入口；Personal Tenant 不显示本页。

**Architecture:** 页面 = /app/members + BFF 端点：GET /portal-api/members（成员 + Membership 状态 + Role + Product Access）、POST /portal-api/members/invitations（T05，返回邀请链接/凭据）、POST /portal-api/members/{id}/role（T06）、POST /portal-api/members/{id}/product-access（T08/T09 授权或撤销）。邀请交付依赖出站通知通道（ADR-0056 proposed，2026-08-25 已有初稿）——本计划将邀请链接以「复制链接」方式交付；短信/邮件通道在 ADR-0056 转正并通过外部确认（external-confirmations「Notification channel」行）后接入，不阻塞邀请链接功能。撤销 Product Access 需立即生效（T09 语义：DB revoke + 异步删 tuple），页面展示 revoking → revoked，不做「撤销成功」假象。

**Tech Stack:** React 19、TypeScript、Vite、Tailwind 4、TanStack Query、Vitest Browser、Playwright、Go BFF

**Spec:** [GitHub Issue 待创建（U07，#122 起，见 portal-ui-index）](https://github.com/1123786563/myqypt/issues)、docs/architecture/architecture-baseline-risk-assessment-v1.1.md §4.1/§4.2/§9、ADR-0009、ADR-0022、docs/superpowers/plans/2026-08-24-t04-business-tenant.md、2026-08-24-t05-membership-invitation.md、2026-08-24-t06-platform-roles.md、2026-08-24-t08-openfga-grant-projection.md、2026-08-24-t09-openfga-immediate-revoke.md

## Global Constraints

- 只有 Owner/Admin 可见邀请、改角色、授撤销操作；Member 仅见自己的状态。操作权限由 BFF 依 F12 授权判定，前端隐藏不等于鉴权。
- 撤销 Product Access 展示 revoking（数据库已拒绝）→ revoked（tuple 已删），不得立即显示「已撤销」。
- 邀请状态机（pending/accepted/expired/revoked）来自 T05；链接有效期由后端返回。
- Owner 不能被降级/移除（页面不提供该操作）；至少保留一名 Owner。
- Personal Tenant 返回 404 或隐藏入口，不渲染成员管理。
- 测试用内存 Membership/OpenFGA 双端；不得包含真实用户个人信息。

---

## File Structure

- Create web/src/routes/app/members.tsx + members.test.tsx
- Create web/src/features/members/invite-dialog.tsx + tests
- Create web/src/features/members/access-table.tsx + tests
- Create internal/transport/http/members_handler.go + members_handler_test.go
- Create tests/acceptance/scenarios/u07-members.yaml + tests/acceptance/u07_members_test.go
- Create e2e/members.spec.ts

### Task 1: BFF 成员端点

**Interfaces:**
- Produces: GET /portal-api/members → { members: [{ user_id, display_name?, membership_id, role, state, product_access: [{ product_id, state }] }], inviter_role: "owner"|"admin"|"member" }；POST /portal-api/members/invitations { email?, role, product_ids?, idempotency_key } → { invitation_id, link, expires_at }；POST /portal-api/members/{membership_id}/role { role }；POST /portal-api/members/{membership_id}/product-access { product_id, action: "grant"|"revoke" }。

- [ ] **Step 1: 契约测试（Go）**

```go
func TestMembersScopedToBusinessTenantAndRole(t *testing.T) {
    // given: Business Tenant + Owner/Admin 调用 → 200；Member 调用 → 仅返回自身或 403 依据角色语义
    // Personal Tenant → 404
}
func TestInvitationIsIdempotentAndExpiring(t *testing.T) {
    // when: 同 key 重复邀请同一邮箱
    // then: 返回同一 invitation，且 expires_at 由后端设置
}
func TestRoleChangeProtectsOwner(t *testing.T) {
    // when: 试图降级最后一名 Owner
    // then: 409，且不产生 Membership 变更
}
func TestRevokeIsImmediateDenyWithAsyncCleanup(t *testing.T) {
    // when: revoke
    // then: 数据库立即拒绝（T09），页面视图状态为 revoking；OpenFGA tuple 删除异步完成 → revoked
}
```

- [ ] **Step 2: 实现 handler**：成员列表聚合 Membership + OpenFGA projection；邀请创建走 T05；角色与授权走 T06/T08/T09 端口。
- [ ] **Step 3: 跑测试**：go test ./internal/transport/http -run Members -count=1。

### Task 2: 成员页面与授权交互

- [ ] **Step 1: 组件测试**

```tsx
it("hides admin actions for member role", async () => {
  // stub: inviter_role:"member" → 无「邀请成员」「变更角色」按钮
});
it("shows revoking then revoked after revoke", async () => {
  // stub: 点击撤销 → 状态 revoking（按钮禁用）→ 异步更新后 revoked
});
it("does not offer role change on the owner row", () => {
  // Owner 行无角色下拉
});
```

- [ ] **Step 2: 实现页面**：成员表（F13 PlatformDataTable：分页/排序/筛选）→ 邀请 Dialog（角色 + 可选 Product Access + 生成链接复制）→ 行内角色下拉（T06 枚举）→ Product Access 授权/撤销（确认弹窗）。顶部提示「邀请链接有效期至 …」。
- [ ] **Step 3: 浏览器测试全绿**。

### Task 3: Playwright smoke 与黑盒场景

- [ ] **Step 1: e2e/members.spec.ts**：① Owner 邀请并授予 Access；② Member 无管理入口；③ 撤销后立即 403（T09 fail closed）；④ 会话过期。
- [ ] **Step 2: tests/acceptance/scenarios/u07-members.yaml**：正常流、最后 Owner 保护、撤销即时性、跨 Tenant 隔离。
- [ ] **Step 3: 提交**：git commit -m "feat(web): member management with invitations and product access"

## Self-Review Record

- Spec coverage: 角色可见性、Owner 保护、撤销 revoking→revoked、邀请幂等均有机械断言。
- Placeholder scan: 无目标句；每步有具体输入输出。
- Type consistency: role 与 product_access.state 来自后端枚举。
- Right-sizing: 邀请通知通道显式标注待 ADR，不阻塞链接功能。
