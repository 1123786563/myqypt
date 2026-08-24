# T23 Header 清洗与 Product 直连阻断 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 伪造内部 Header、external Tenant ID 或绕过 Platform Edge 的请求均被拒绝。

**Architecture:** Implement this Ticket as one vertical slice in `internal/product` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, WeKnora, Higress

**Spec:** [GitHub Issue #24](https://github.com/1123786563/myqypt/issues/24), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0028-issue-platform-context-only-at-the-trusted-edge.md`, `docs/adr/0035-retain-native-product-uis-behind-the-platform-edge.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #23 — T22 Platform Context 签发

---

## File Structure

- Create `internal/product/trusted-edge-boundary/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/product/trusted-edge-boundary/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t23-trusted-edge-boundary.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t23_trusted_edge_boundary_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T23 as one testable vertical slice

**Files:**
- Create: `internal/product/trusted-edge-boundary/service.go`
- Create: `internal/product/trusted-edge-boundary/service_test.go`
- Create: `tests/acceptance/scenarios/t23-trusted-edge-boundary.yaml`
- Create: `tests/acceptance/t23_trusted_edge_boundary_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `TrustedEdgeBoundaryCommand{TenantID string, RequestHeaders map[string]string, ExternalTenantID string, IdempotencyKey string}`, `NewTrustedEdgeBoundaryService(tx Tx, port TrustedEdgeBoundaryPort, evidence EvidenceSink) *TrustedEdgeBoundaryService`, and `(*TrustedEdgeBoundaryService).Execute(ctx context.Context, cmd TrustedEdgeBoundaryCommand) (TrustedEdgeBoundaryResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package trustededgeboundary_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/product/trusted-edge-boundary"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.TrustedEdgeBoundaryCommand) (feature.TrustedEdgeBoundaryResult, error) {
    p.calls++
    return feature.TrustedEdgeBoundaryResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
}

type inMemoryTx struct{}

func (inMemoryTx) Run(ctx context.Context, fn func(context.Context) error) error {
    return fn(ctx)
}

type memoryEvidence struct{ records int }

func (m *memoryEvidence) Record(_ context.Context, _, _, _ string) error {
    m.records++
    return nil
}

func TestTrustedEdgeBoundaryRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewTrustedEdgeBoundaryService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.TrustedEdgeBoundaryCommand{
        TenantID: "",
        IdempotencyKey: "t23-guard",
    })

    if !errors.Is(err, feature.ErrTenantRequired) {
        t.Fatalf("expected %v, got %v", feature.ErrTenantRequired, err)
    }
    if port.calls != 0 {
        t.Fatalf("outbound port called %d times", port.calls)
    }
}
```

- [ ] **Step 2: Run the focused test and confirm the red state**

Run: `go test ./internal/product/trusted-edge-boundary -run TestTrustedEdgeBoundaryRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewTrustedEdgeBoundaryService`, `TrustedEdgeBoundaryCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package trustededgeboundary

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type TrustedEdgeBoundaryCommand struct {
    TenantID string
    RequestHeaders map[string]string
    ExternalTenantID string
    IdempotencyKey string
}

type TrustedEdgeBoundaryResult struct {
    ResourceID string
    Outcome    string
}

type TrustedEdgeBoundaryPort interface {
    Apply(context.Context, TrustedEdgeBoundaryCommand) (TrustedEdgeBoundaryResult, error)
}

type Tx interface {
    Run(context.Context, func(context.Context) error) error
}

type EvidenceSink interface {
    Record(context.Context, string, string, string) error
}
```

- [ ] **Step 4: Implement the minimal transactional service**

```go
type TrustedEdgeBoundaryService struct {
    tx       Tx
    port     TrustedEdgeBoundaryPort
    evidence EvidenceSink
}

func NewTrustedEdgeBoundaryService(tx Tx, port TrustedEdgeBoundaryPort, evidence EvidenceSink) *TrustedEdgeBoundaryService {
    return &TrustedEdgeBoundaryService{tx: tx, port: port, evidence: evidence}
}

func (s *TrustedEdgeBoundaryService) Execute(ctx context.Context, cmd TrustedEdgeBoundaryCommand) (result TrustedEdgeBoundaryResult, err error) {
    if cmd.TenantID == "" {
        return TrustedEdgeBoundaryResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return TrustedEdgeBoundaryResult{}, ErrIdempotencyKeyRequired
    }
    err = s.tx.Run(ctx, func(txCtx context.Context) error {
        applied, applyErr := s.port.Apply(txCtx, cmd)
        if applyErr != nil {
            return applyErr
        }
        result = applied
        return s.evidence.Record(txCtx, cmd.IdempotencyKey, result.ResourceID, result.Outcome)
    })
    return result, err
}
```

The concrete `TrustedEdgeBoundaryPort.Apply` implementation in this file must enforce the Ticket invariant: **伪造内部 Header、external Tenant ID 或绕过 Platform Edge 的请求均被拒绝。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/product/trusted-edge-boundary -run 'TrustedEdgeBoundary' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t23-trusted-edge-boundary
issue: 24
batch: P12
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t23-acceptance
normal:
  expect: "伪造内部 Header、external Tenant ID 或绕过 Platform Edge 的请求均被拒绝。"
  side_effect_count: 1
  evidence_content_minimized: true
guard:
  mutation: remove_required_scope_or_inject_dependency_failure
  expect_error_class: denied_or_retryable
  side_effect_count: 0
replay:
  deliveries: 2
  final_business_effect_count: 1
```

- [ ] **Step 7: Run the named seam and preserve evidence**

```go
package acceptance_test

import (
    "testing"

    "github.com/1123786563/myqypt/tests/platformtest"
)

func TestT23TrustedEdgeBoundary(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t23-trusted-edge-boundary.yaml")
    if !report.Passed {
        t.Fatalf("T23 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT23TrustedEdgeBoundary -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t23/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/product/trusted-edge-boundary ./tests/acceptance -count=1`

Expected: PASS with no skipped T23 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/product/trusted-edge-boundary/service.go internal/product/trusted-edge-boundary/service_test.go tests/acceptance/scenarios/t23-trusted-edge-boundary.yaml tests/acceptance/t23_trusted_edge_boundary_test.go
git commit -m "feat(product): deliver T23 trusted-edge-boundary"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #24 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `TrustedEdgeBoundaryCommand`, `TrustedEdgeBoundaryResult`, `TrustedEdgeBoundaryPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
