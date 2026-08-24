# F04 HTTP 安全与可观测性 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为所有 Go HTTP 路由交付一致的请求 ID、安全响应头、受限 CORS、panic recovery、结构化日志和 OpenTelemetry 追踪。

**Architecture:** Middleware 按固定顺序安装在 Gin Transport；日志使用注入的 `*slog.Logger`，trace 使用注入的 `trace.TracerProvider`。Problem Details 从上下文读取同一 correlation ID。

**Tech Stack:** Go 1.26.7, Gin 1.12.0, slog, OpenTelemetry Go 1.45.0, otelgin 0.70.0

**Spec:** [Issue #104](https://github.com/1123786563/myqypt/issues/104), ADR-0036, ADR-0038

## Global Constraints

- 顺序固定为 request ID → security headers/CORS → tracing → access log → recovery → routes。
- 不记录 Cookie、Authorization、OIDC code、内部身份 Header 或响应正文。
- CORS origin 是显式配置列表，禁止凭证模式下的 `*`。

---

## File Structure

- Create `internal/transport/http/middleware/{requestid,security,accesslog,recovery}.go` and `_test.go` files.
- Create `internal/platform/observability/observability.go` and tests.
- Modify `internal/transport/http/router.go` and `problem.go`.

### Task 1: Enforce the HTTP middleware contract

**Interfaces:** `middleware.RequestID() gin.HandlerFunc`, `Security(SecurityConfig) gin.HandlerFunc`, `AccessLog(*slog.Logger) gin.HandlerFunc`, `Recovery(ProblemWriter) gin.HandlerFunc`.

```go
func TestRecoveryReturnsCorrelatedProblem(t *testing.T) {
    h := NewRouter(Dependencies{Routes: func(r *gin.Engine) { r.GET("/panic", func(*gin.Context) { panic("secret") }) }})
    req := httptest.NewRequest(http.MethodGet, "/panic", nil)
    req.Header.Set("X-Request-ID", "018f4f70-7c40-7c7e-9f0b-8c7a10b65211")
    res := httptest.NewRecorder()
    h.ServeHTTP(res, req)
    require.Equal(t, http.StatusInternalServerError, res.Code)
    require.NotContains(t, res.Body.String(), "secret")
    require.Contains(t, res.Body.String(), req.Header.Get("X-Request-ID"))
}
```

- [ ] Write black-box tests for generated request ID, preserved valid inbound request ID, HSTS/CSP/nosniff/referrer headers, allowed preflight, denied origin, and recovered panic returning `internal_error` with the same trace ID.
- [ ] Run `go test ./internal/transport/http/... -run 'RequestID|Security|CORS|Recovery' -count=1`; confirm red.
- [ ] Implement UUID validation/generation, exact origin membership, security headers, and recovery without stack/secret response leakage.
- [ ] Update `Problem` writer to use context request ID when `TraceID` is empty.
- [ ] Run focused tests, then `go test ./internal/transport/http/... -race -count=1`.
- [ ] Commit: `git commit -m "feat(api): harden http middleware"`.

### Task 2: Add explicit observability dependencies

**Interfaces:** `observability.New(context.Context, Config) (Resources, error)` where `Resources` contains `Logger`, `TracerProvider`, `MeterProvider`, and idempotent `Shutdown(context.Context) error`.

```go
type Resources struct {
    Logger *slog.Logger
    TracerProvider *sdktrace.TracerProvider
    MeterProvider *sdkmetric.MeterProvider
    Shutdown func(context.Context) error
}
```

- [ ] Write tests with an in-memory slog handler and OTel span recorder; assert route, method, status, duration, request ID, and trace ID exist while authorization/cookie values do not.
- [ ] Run `go test ./internal/platform/observability ./internal/transport/http/... -run Observability -count=1`; confirm red.
- [ ] Build Resource attributes `service.name`, `service.version`, `deployment.environment`; accept OTLP endpoint only through config; return a no-op exporter when disabled.
- [ ] Inject resources from `cmd/platform-api`; call shutdown after HTTP graceful shutdown.
- [ ] Run `go test ./... -race -count=1` and scan `rg -n 'Authorization|Cookie' internal/transport/http/middleware` to review every reference.
- [ ] Commit: `git commit -m "feat(platform): add slog and otel wiring"`.

## Self-Review Record

- Spec coverage: headers, CORS, panic handling, request correlation, minimized logs, tracing and shutdown are covered.
- Placeholder scan: middleware order, fields, prohibited values, tests and commands are exact.
- Type consistency: observability dependencies enter through composition root; Domain remains unaware of Gin and OTel.
