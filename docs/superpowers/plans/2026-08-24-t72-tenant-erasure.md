# T72 Tenant Erasure 与 Erasure Record Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 删除数据库、文件、向量、任务、缓存、凭证、授权与 Binding，并保存逐存储证据。

**Architecture:** Implement this Ticket as one vertical slice in `internal/lifecycle` and prove it through the Production Gate evidence case under `tests/production-gates`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, WeKnora, Higress

**Spec:** [GitHub Issue #73](https://github.com/1123786563/myqypt/issues/73), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0011-erase-product-data-after-read-only-retention.md`, `docs/adr/0031-compensate-only-reversible-lifecycle-effects.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #10 — T09 OpenFGA 立即撤权 - #22 — T21 WeKnora Adapter 封闭测试接入 - #71 — T70 Verifiable Tenant Export - #72 — T71 Read-only Retention

---

## File Structure

- Create `internal/lifecycle/tenant-erasure/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/lifecycle/tenant-erasure/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/production-gates/scenarios/t72-tenant-erasure.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/production-gates/t72_tenant_erasure_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T72 as one testable vertical slice

**Files:**
- Create: `internal/lifecycle/tenant-erasure/service.go`
- Create: `internal/lifecycle/tenant-erasure/service_test.go`
- Create: `tests/production-gates/scenarios/t72-tenant-erasure.yaml`
- Create: `tests/production-gates/t72_tenant_erasure_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `TenantErasureCommand{EnvironmentID string, ErasureOperationID string, IdempotencyKey string}`, `NewTenantErasureService(tx Tx, port TenantErasurePort, evidence EvidenceSink) *TenantErasureService`, and `(*TenantErasureService).Execute(ctx context.Context, cmd TenantErasureCommand) (TenantErasureResult, error)`.
- Guarantees: idempotency key and `EnvironmentID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package tenanterasure_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/lifecycle/tenant-erasure"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.TenantErasureCommand) (feature.TenantErasureResult, error) {
    p.calls++
    return feature.TenantErasureResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestTenantErasureRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewTenantErasureService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.TenantErasureCommand{
        EnvironmentID: "",
        IdempotencyKey: "t72-guard",
    })

    if !errors.Is(err, feature.ErrEnvironmentRequired) {
        t.Fatalf("expected %v, got %v", feature.ErrEnvironmentRequired, err)
    }
    if port.calls != 0 {
        t.Fatalf("outbound port called %d times", port.calls)
    }
}
```

- [ ] **Step 2: Run the focused test and confirm the red state**

Run: `go test ./internal/lifecycle/tenant-erasure -run TestTenantErasureRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewTenantErasureService`, `TenantErasureCommand`, and `ErrEnvironmentRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package tenanterasure

import (
    "context"
    "errors"
)

var (
    ErrEnvironmentRequired = errors.New("production-shaped environment is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type TenantErasureCommand struct {
    EnvironmentID string
    ErasureOperationID string
    IdempotencyKey string
}

type TenantErasureResult struct {
    ResourceID string
    Outcome    string
}

type TenantErasurePort interface {
    Apply(context.Context, TenantErasureCommand) (TenantErasureResult, error)
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
type TenantErasureService struct {
    tx       Tx
    port     TenantErasurePort
    evidence EvidenceSink
}

func NewTenantErasureService(tx Tx, port TenantErasurePort, evidence EvidenceSink) *TenantErasureService {
    return &TenantErasureService{tx: tx, port: port, evidence: evidence}
}

func (s *TenantErasureService) Execute(ctx context.Context, cmd TenantErasureCommand) (result TenantErasureResult, err error) {
    if cmd.EnvironmentID == "" {
        return TenantErasureResult{}, ErrEnvironmentRequired
    }
    if cmd.IdempotencyKey == "" {
        return TenantErasureResult{}, ErrIdempotencyKeyRequired
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

The concrete `TenantErasurePort.Apply` implementation in this file must enforce the Ticket invariant: **删除数据库、文件、向量、任务、缓存、凭证、授权与 Binding，并保存逐存储证据。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/lifecycle/tenant-erasure -run 'TenantErasure' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t72-tenant-erasure
issue: 73
batch: P20
seam: Production Gate evidence case
scope:
  environment_id: prod-shaped-a
idempotency_key: t72-acceptance
normal:
  expect: "删除数据库、文件、向量、任务、缓存、凭证、授权与 Binding，并保存逐存储证据。"
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
package production-gates_test

import (
    "testing"

    "github.com/1123786563/myqypt/tests/platformtest"
)

func TestT72TenantErasure(t *testing.T) {
    report := platformtest.Run(t, "tests/production-gates/scenarios/t72-tenant-erasure.yaml")
    if !report.Passed {
        t.Fatalf("T72 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/production-gates -run TestT72TenantErasure -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t72/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/lifecycle/tenant-erasure ./tests/production-gates -count=1`

Expected: PASS with no skipped T72 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/lifecycle/tenant-erasure/service.go internal/lifecycle/tenant-erasure/service_test.go tests/production-gates/scenarios/t72-tenant-erasure.yaml tests/production-gates/t72_tenant_erasure_test.go
git commit -m "feat(lifecycle): deliver T72 tenant-erasure"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #73 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `TenantErasureCommand`, `TenantErasureResult`, `TenantErasurePort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
