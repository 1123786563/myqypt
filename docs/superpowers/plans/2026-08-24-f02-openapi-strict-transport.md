# F02 OpenAPI Strict Transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以 OpenAPI 3.1 为唯一 HTTP 契约，交付 oapi-codegen Gin strict status endpoint 和稳定 Problem Details。

**Architecture:** Generated types stay in Transport. A strict handler implements the generated interface; gin-middleware validates requests because strict generation does not perform request validation.

**Tech Stack:** Go 1.26.7, oapi-codegen 2.8.0, Gin 1.12.0, gin-middleware 1.1.0

**Spec:** [Issue #102](https://github.com/1123786563/myqypt/issues/102), `docs/superpowers/specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md`

## Global Constraints

- `api/openapi/platform.yaml` is the only public HTTP source of truth.
- Generate `gin-server`, `strict-server`, `models`, and `embedded-spec`.
- Generated types never become Domain or PostgreSQL models.
- Every error response uses `application/problem+json`, stable `code`, and correlation ID.

---

## File Structure

- Create `api/openapi/platform.yaml` and `api/openapi/oapi-codegen.yaml`.
- Create `internal/transport/http/api/generate.go`; generate `server.gen.go`.
- Create `internal/transport/http/status.go`, `problem.go`, and `status_test.go`.
- Modify `internal/transport/http/router.go` to install validation and strict routes.

### Task 1: Generate and serve the strict contract

**Files:**
- Create: `api/openapi/platform.yaml`
- Create: `api/openapi/oapi-codegen.yaml`
- Create: `internal/transport/http/api/generate.go`
- Generate: `internal/transport/http/api/server.gen.go`
- Create: `internal/transport/http/status.go`
- Test: `internal/transport/http/status_test.go`

**Interfaces:**
- Produces: `GET /api/v1/system/status` and `StatusHandler.GetSystemStatus(context.Context, api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error)`.

- [ ] **Step 1: Write the failing black-box test**

```go
func TestSystemStatusUsesGeneratedContract(t *testing.T) {
    handler := httptransport.NewRouter(httptransport.Dependencies{Version: "be4cc10"})
    request := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
    response := httptest.NewRecorder()
    handler.ServeHTTP(response, request)
    if response.Code != 200 || !strings.Contains(response.Body.String(), `"version":"be4cc10"`) {
        t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
    }
}
```

- [ ] **Step 2: Run and confirm red**

Run: `go test ./internal/transport/http -run TestSystemStatusUsesGeneratedContract -count=1`

Expected: FAIL because the contract does not exist.

- [ ] **Step 3: Add the exact operation and generator config**

```yaml
openapi: 3.1.0
info: { title: MyQYPT Platform API, version: 1.0.0 }
paths:
  /api/v1/system/status:
    get:
      operationId: getSystemStatus
      responses:
        '200':
          description: Platform process status
          content:
            application/json:
              schema: { $ref: '#/components/schemas/SystemStatus' }
components:
  schemas:
    SystemStatus:
      type: object
      additionalProperties: false
      required: [status, version]
      properties:
        status: { type: string, const: available }
        version: { type: string, minLength: 1 }
```

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.0/configuration-schema.json
package: api
generate: { gin-server: true, strict-server: true, models: true, embedded-spec: true }
output: internal/transport/http/api/server.gen.go
```

Pin the `go:generate` command to `github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0`.

- [ ] **Step 4: Implement and register the strict handler**

```go
type StatusHandler struct{ Version string }

var _ api.StrictServerInterface = (*StatusHandler)(nil)

func (h *StatusHandler) GetSystemStatus(context.Context, api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error) {
    return api.GetSystemStatus200JSONResponse{Status: api.Available, Version: h.Version}, nil
}
```

Register `ginmiddleware.OapiRequestValidator(swagger)` before `api.RegisterHandlers(router, api.NewStrictHandler(handler, nil))`.

- [ ] **Step 5: Generate and verify**

Run: `go generate ./internal/transport/http/api && go test ./internal/transport/http/... -count=1 && git diff --exit-code -- internal/transport/http/api/server.gen.go`

Expected: PASS and regeneration is clean.

- [ ] **Step 6: Commit**

```bash
git add api/openapi internal/transport/http go.mod go.sum
git commit -m "feat(api): add strict system status contract"
```

### Task 2: Standardize Problem Details

**Files:**
- Create: `internal/transport/http/problem.go`
- Modify: `internal/transport/http/router.go`
- Modify: `internal/transport/http/status_test.go`

**Interfaces:**
- Produces: `WriteProblem(*gin.Context, Problem)` and stable codes `invalid_request`, `not_found`, and `internal_error`.

- [ ] **Step 1: Add a failing invalid-request assertion**

Assert status 400, content type `application/problem+json`, code `invalid_request`, and non-empty `trace_id` for a request rejected by OpenAPI validation.

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./internal/transport/http -run Problem -count=1`

Expected: FAIL because Gin still emits its default error.

- [ ] **Step 3: Add the concrete response type and writer**

```go
type Problem struct {
    Type string `json:"type"`
    Title string `json:"title"`
    Status int `json:"status"`
    Code string `json:"code"`
    TraceID string `json:"trace_id"`
}

func WriteProblem(c *gin.Context, p Problem) {
    c.Header("Content-Type", "application/problem+json")
    c.AbortWithStatusJSON(p.Status, p)
}
```

Configure validator and generated strict request/response error hooks to call this writer without returning raw Go errors.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/transport/http/... -count=1`

```bash
git add internal/transport/http
git commit -m "feat(api): standardize problem details"
```

## Self-Review Record

- Spec coverage: OpenAPI 3.1, strict Gin, explicit validation, generated-staleness, transport-only types, and Problem Details are covered.
- Placeholder scan: contract, configuration, methods, tests, and commands are concrete.
- Type consistency: `GetSystemStatus`, `SystemStatus`, `Problem`, and `WriteProblem` are stable.
