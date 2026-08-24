# T39 Cell Capacity Dimensions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cell 明确记录 Tenant、存储、向量、任务、模型、ingest 和数据库容量。

**Architecture:** Implement this Ticket as one vertical slice in `internal/capacity` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification

**Spec:** [GitHub Issue #40](https://github.com/1123786563/myqypt/issues/40), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0032-run-shared-products-in-capacity-bounded-cells.md`, `docs/adr/0034-reserve-multidimensional-cell-capacity-before-placement.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #22 — T21 WeKnora Adapter 封闭测试接入 - #38 — T37 Product 原子配额 Reservation

---

## File Structure

- Create `internal/capacity/cell-capacity-dimensions/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/capacity/cell-capacity-dimensions/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t39-cell-capacity-dimensions.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t39_cell_capacity_dimensions_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T39 as one testable vertical slice

**Files:**
- Create: `internal/capacity/cell-capacity-dimensions/service.go`
- Create: `internal/capacity/cell-capacity-dimensions/service_test.go`
- Create: `tests/acceptance/scenarios/t39-cell-capacity-dimensions.yaml`
- Create: `tests/acceptance/t39_cell_capacity_dimensions_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `CellCapacityDimensionsCommand{TenantID string, ProductInstanceID string, TenantLimit int64, StorageBytes int64, VectorLimit int64, JobConcurrency int64, ModelConcurrency int64, IngestPerSecond int64, DatabaseBytes int64, IdempotencyKey string}`, `NewCellCapacityDimensionsService(tx Tx, port CellCapacityDimensionsPort, evidence EvidenceSink) *CellCapacityDimensionsService`, and `(*CellCapacityDimensionsService).Execute(ctx context.Context, cmd CellCapacityDimensionsCommand) (CellCapacityDimensionsResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package cellcapacitydimensions_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/capacity/cell-capacity-dimensions"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.CellCapacityDimensionsCommand) (feature.CellCapacityDimensionsResult, error) {
    p.calls++
    return feature.CellCapacityDimensionsResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestCellCapacityDimensionsRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewCellCapacityDimensionsService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.CellCapacityDimensionsCommand{
        TenantID: "",
        IdempotencyKey: "t39-guard",
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

Run: `go test ./internal/capacity/cell-capacity-dimensions -run TestCellCapacityDimensionsRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewCellCapacityDimensionsService`, `CellCapacityDimensionsCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package cellcapacitydimensions

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type CellCapacityDimensionsCommand struct {
    TenantID string
    ProductInstanceID string
    TenantLimit int64
    StorageBytes int64
    VectorLimit int64
    JobConcurrency int64
    ModelConcurrency int64
    IngestPerSecond int64
    DatabaseBytes int64
    IdempotencyKey string
}

type CellCapacityDimensionsResult struct {
    ResourceID string
    Outcome    string
}

type CellCapacityDimensionsPort interface {
    Apply(context.Context, CellCapacityDimensionsCommand) (CellCapacityDimensionsResult, error)
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
type CellCapacityDimensionsService struct {
    tx       Tx
    port     CellCapacityDimensionsPort
    evidence EvidenceSink
}

func NewCellCapacityDimensionsService(tx Tx, port CellCapacityDimensionsPort, evidence EvidenceSink) *CellCapacityDimensionsService {
    return &CellCapacityDimensionsService{tx: tx, port: port, evidence: evidence}
}

func (s *CellCapacityDimensionsService) Execute(ctx context.Context, cmd CellCapacityDimensionsCommand) (result CellCapacityDimensionsResult, err error) {
    if cmd.TenantID == "" {
        return CellCapacityDimensionsResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return CellCapacityDimensionsResult{}, ErrIdempotencyKeyRequired
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

The concrete `CellCapacityDimensionsPort.Apply` implementation in this file must enforce the Ticket invariant: **Cell 明确记录 Tenant、存储、向量、任务、模型、ingest 和数据库容量。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/capacity/cell-capacity-dimensions -run 'CellCapacityDimensions' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t39-cell-capacity-dimensions
issue: 40
batch: P19
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t39-acceptance
normal:
  expect: "Cell 明确记录 Tenant、存储、向量、任务、模型、ingest 和数据库容量。"
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

func TestT39CellCapacityDimensions(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t39-cell-capacity-dimensions.yaml")
    if !report.Passed {
        t.Fatalf("T39 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT39CellCapacityDimensions -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t39/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/capacity/cell-capacity-dimensions ./tests/acceptance -count=1`

Expected: PASS with no skipped T39 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/capacity/cell-capacity-dimensions/service.go internal/capacity/cell-capacity-dimensions/service_test.go tests/acceptance/scenarios/t39-cell-capacity-dimensions.yaml tests/acceptance/t39_cell_capacity_dimensions_test.go
git commit -m "feat(capacity): deliver T39 cell-capacity-dimensions"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #40 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `CellCapacityDimensionsCommand`, `CellCapacityDimensionsResult`, `CellCapacityDimensionsPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
