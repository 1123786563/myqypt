# F20 Higress 路由与身份 Header 清洗 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 配置并验证 Higress 将静态站、Portal BFF、Public API 和 webhooks 分流到正确后端，同时清除所有客户端伪造的内部身份 Header。

**Architecture:** 路由优先级由精确前缀决定；CDN 提供公开页和 `/app` fallback，Go 提供 `/portal-api`, `/api/v1`, `/webhooks`。边缘先删除 `X-Platform-*`/内部用户租户头，再由可信认证插件或 Go 重建内部上下文。

**Tech Stack:** Higress Gateway API/Ingress, Kubernetes manifests, conformance shell/Go tests

**Spec:** [Issue #120](https://github.com/1123786563/myqypt/issues/120), ADR-0022, ADR-0024, extraction design §4

## Global Constraints

- `/api/v1` 不得落到 SPA；`/portal-api` 不得缓存；`/app/*` 未命中文件时只回退 `/app/index.html`。
- 删除 `X-User-ID`, `X-Tenant-ID` 的内部变体、`X-Platform-*`, `X-Forwarded-Client-Cert` 等外部输入；公开 API 的契约 `X-Tenant-ID` 保留为不可信业务输入并由 F12 校验。
- webhooks 保留原始 body，按 provider 限制方法/大小/超时，不经过 SPA fallback。

---

## File Structure

- Create `deploy/higress/{routes,header-sanitization,cache-policy}.yaml` and README.
- Create `tests/gateway/conformance_test.go` with fake CDN/API/Portal/Webhook upstreams.
- Create `scripts/verify-higress.sh` and fixtures.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: platform-api
  annotations:
    higress.io/request-header-control-remove: "X-User-ID,X-Platform-User,X-Platform-Tenant,X-Forwarded-Client-Cert"
spec:
  ingressClassName: higress
  rules:
    - host: platform.example.com
      http:
        paths:
          - path: /portal-api/
            pathType: Prefix
            backend: { service: { name: platform-api, port: { number: 8080 } } }
          - path: /api/v1/
            pathType: Prefix
            backend: { service: { name: platform-api, port: { number: 8080 } } }
          - path: /webhooks/
            pathType: Prefix
            backend: { service: { name: platform-api, port: { number: 8080 } } }
```

### Task 1: Define deterministic routing and caching

- [ ] Write conformance cases for `/`, product/pricing assets, hashed asset, `/app`, `/app/deep/link`, `/portal-api/session`, `/api/v1/catalog/products`, `/webhooks/provider`, unknown API and traversal path.
- [ ] Assert selected upstream, rewritten path, cache headers, method preservation, body preservation and API 404 without SPA HTML.
- [ ] Add separate API and static-CDN Ingress resources; rely on Kubernetes longest-prefix matching, and configure the CDN origin—not an API rewrite—to serve `/app/index.html` only for missing `/app/*` objects. Add CDN cache rules matching F17 and validate manifests with the pinned Higress/Kubernetes schema tool.
- [ ] Run local gateway fixture and `go test ./tests/gateway -run Routing -count=1`.
- [ ] Commit: `git commit -m "feat(gateway): route static portal and api traffic"`.

### Task 2: Prove identity headers cannot be spoofed

```go
func TestSpoofedIdentityHeadersAreRemoved(t *testing.T) {
    for _, name := range []string{"X-User-ID", "X-Platform-User", "X-Forwarded-Client-Cert"} {
        got := sendThroughGateway(t, map[string]string{name: "attacker"})
        require.NotContains(t, got.UpstreamHeaders, name)
    }
}
```

- [ ] Send every case with mixed-case/duplicate spoofed internal headers; assert fake upstream never receives them.
- [ ] Assert public API receives only the contract `X-Tenant-ID`, Go marks it untrusted, and trusted Platform Context can only be added after authentication.
- [ ] Test CORS preflight, cookie forwarding only to Portal BFF, request/body size limits, timeouts and correlation ID propagation.
- [ ] Run `scripts/verify-higress.sh`; save redacted route/status evidence and scan it for token/cookie values.
- [ ] Commit: `git commit -m "test(gateway): enforce trusted edge boundary"`.

## Self-Review Record

- Spec coverage: all route classes, SPA fallback isolation, caching, header stripping, cookies, webhooks and conformance evidence are covered.
- Placeholder scan: paths, headers, upstreams, assertions and commands are concrete.
- Type consistency: public Tenant header remains an input to F12; internal Platform Context is a distinct trusted type.
