# F10 Keycloak 浏览器 Session BFF Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 由 Go BFF 完成 Keycloak Authorization Code + PKCE 流程，浏览器只持有不透明 HttpOnly Session Cookie。

**Architecture:** Session Application Module 依赖 OIDC provider、Identity Binder（#101）和 SessionStore ports。Keycloak token 只在服务端验证/使用；React 通过同源 `/portal-api/session` 获取最小会话视图。

**Tech Stack:** Go 1.26.7, coreos/go-oidc 3.20.0, OAuth2, React 19, PostgreSQL

**Spec:** [Issue #110](https://github.com/1123786563/myqypt/issues/110), ADR-0001, ADR-0007, extraction design §7.1

## Global Constraints

- 必须校验 state、nonce、PKCE、issuer、audience、签名和过期时间。
- Cookie 固定 `HttpOnly; Secure; SameSite=Lax; Path=/portal-api`; 存储的是 256-bit 随机句柄的哈希。
- Session 响应不含 access/refresh/id token；登出撤销服务端 session。

---

## File Structure

- Create `internal/application/session/{ports,service}.go` and tests.
- Create `internal/adapter/keycloak/oidc.go`; create `internal/adapter/postgres/session_store.go` and migration `000002_browser_sessions.sql`.
- Extend OpenAPI with `/portal-api/auth/login`, `/callback`, `/session`, `/logout`; create strict handler.
- Create `web/src/features/session/{api,provider}.tsx` and tests; create login callback route.

### Task 1: Complete and persist a verified login

**Interfaces:**

```go
type OIDCProvider interface {
    AuthorizationURL(state, nonce, challenge, redirectURI string) string
    Exchange(context.Context, string, string, string) (VerifiedIdentity, error)
}
type IdentityBinder interface { Bind(context.Context, VerifiedIdentity) (User, error) }
type SessionStore interface { Create(context.Context, Session) error; Find(context.Context, [32]byte) (Session, error); Delete(context.Context, [32]byte) error }
```

- [ ] Write service tests for start-login state/nonce/verifier creation, one-time callback, binder invocation only after verification, hashed session handle, expiration, replay, wrong state and provider failure.
- [ ] Run `go test ./internal/application/session -count=1`; confirm red.
- [ ] Implement ports/service and append-only migration with `handle_hash bytea primary key`, `user_id`, `expires_at`, `created_at`, `revoked_at`; never persist tokens.
- [ ] Add Keycloak adapter with discovery URL and exact client/redirect configuration; map verification failures to stable `invalid_identity` without raw claims.
- [ ] Run unit tests plus PostgreSQL store integration tests.
- [ ] Commit: `git commit -m "feat(auth): add verified browser sessions"`.

### Task 2: Expose BFF routes and React session state

- [ ] Add OpenAPI schemas `SessionView{authenticated,user{id,display_name}}` and empty 204 logout; generate Go/TS and assert clean diff.
- [ ] Write HTTP tests for secure cookie flags, no tokens in body/header, unauthenticated 401 Problem, CSRF Origin check on logout, and cookie deletion.
- [ ] Write React tests for loading/authenticated/anonymous, login redirect, logout and no browser storage keys containing token.
- [ ] Implement strict handlers and `SessionProvider`; use `credentials:'include'` and invalidate session query after callback/logout.
- [ ] Run `go test ./... -race -count=1`, frontend tests/typecheck, and Playwright with a fake OIDC adapter.
- [ ] Commit: `git commit -m "feat(portal): add keycloak session bff"`.

## Self-Review Record

- Spec coverage: OIDC verification, identity binding, opaque server session, cookie security, logout and React integration are covered.
- Placeholder scan: ports, columns, cookie flags, routes and failure cases are explicit.
- Type consistency: `VerifiedIdentity` comes from #101; browser receives only `SessionView`.
