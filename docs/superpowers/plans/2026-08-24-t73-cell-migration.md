# T73 Cell Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Product Binding 经 Export、验证、切换和回滚窗口迁移到另一 Cell。

**Architecture:** Implement this Ticket as one vertical slice in `internal/lifecycle` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, WeKnora, Higress

**Spec:** [GitHub Issue #74](https://github.com/1123786563/myqypt/issues/74), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0034-reserve-multidimensional-cell-capacity-before-placement.md`, `docs/adr/0050-require-verifiable-tenant-export-from-shared-products.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #42 — T41 Capacity Exhaustion 与新 Cell Placement - #69 — T68 Backward-compatible Canary Upgrade - #70 — T69 Forward-only/Destructive Restore Rehearsal - #71 — T70 Verifiable Tenant Export

---

## File Structure

- Create `internal/lifecycle/cell-migration/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/lifecycle/cell-migration/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t73-cell-migration.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t73_cell_migration_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T73 as one testable vertical slice

**Files:**
- Create: `internal/lifecycle/cell-migration/service.go`
- Create: `internal/lifecycle/cell-migration/service_test.go`
- Create: `tests/acceptance/scenarios/t73-cell-migration.yaml`
- Create: `tests/acceptance/t73_cell_migration_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `CellMigrationCommand{TenantID string, ProductBindingID string, SourceCellID string, TargetCellID string, IdempotencyKey string}`, `NewCellMigrationService(tx Tx, port CellMigrationPort, evidence EvidenceSink) *CellMigrationService`, and `(*CellMigrationService).Execute(ctx context.Context, cmd CellMigrationCommand) (CellMigrationResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package cellmigration_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/lifecycle/cell-migration"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.CellMigrationCommand) (feature.CellMigrationResult, error) {
    p.calls++
    return feature.CellMigrationResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestCellMigrationRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewCellMigrationService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.CellMigrationCommand{
        TenantID: "",
        IdempotencyKey: "t73-guard",
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

Run: `go test ./internal/lifecycle/cell-migration -run TestCellMigrationRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewCellMigrationService`, `CellMigrationCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package cellmigration

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type CellMigrationCommand struct {
    TenantID string
    ProductBindingID string
    SourceCellID string
    TargetCellID string
    IdempotencyKey string
}

type CellMigrationResult struct {
    ResourceID string
    Outcome    string
}

type CellMigrationPort interface {
    Apply(context.Context, CellMigrationCommand) (CellMigrationResult, error)
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
type CellMigrationService struct {
    tx       Tx
    port     CellMigrationPort
    evidence EvidenceSink
}

func NewCellMigrationService(tx Tx, port CellMigrationPort, evidence EvidenceSink) *CellMigrationService {
    return &CellMigrationService{tx: tx, port: port, evidence: evidence}
}

func (s *CellMigrationService) Execute(ctx context.Context, cmd CellMigrationCommand) (result CellMigrationResult, err error) {
    if cmd.TenantID == "" {
        return CellMigrationResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return CellMigrationResult{}, ErrIdempotencyKeyRequired
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

The concrete `CellMigrationPort.Apply` implementation in this file must enforce the Ticket invariant: **Product Binding 经 Export、验证、切换和回滚窗口迁移到另一 Cell。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/lifecycle/cell-migration -run 'CellMigration' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t73-cell-migration
issue: 74
batch: P22
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t73-acceptance
normal:
  expect: "Product Binding 经 Export、验证、切换和回滚窗口迁移到另一 Cell。"
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

func TestT73CellMigration(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t73-cell-migration.yaml")
    if !report.Passed {
        t.Fatalf("T73 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT73CellMigration -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t73/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/lifecycle/cell-migration ./tests/acceptance -count=1`

Expected: PASS with no skipped T73 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/lifecycle/cell-migration/service.go internal/lifecycle/cell-migration/service_test.go tests/acceptance/scenarios/t73-cell-migration.yaml tests/acceptance/t73_cell_migration_test.go
git commit -m "feat(lifecycle): deliver T73 cell-migration"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #74 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `CellMigrationCommand`, `CellMigrationResult`, `CellMigrationPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
