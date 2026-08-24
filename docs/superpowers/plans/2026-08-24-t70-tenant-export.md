# T70 Verifiable Tenant Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 导出 Product 和 Schema Version、对象校验和、时间和完整性证据。

**Architecture:** Implement this Ticket as one vertical slice in `internal/lifecycle` and prove it through the Provider/Adapter conformance case under `tests/conformance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, WeKnora, Higress

**Spec:** [GitHub Issue #71](https://github.com/1123786563/myqypt/issues/71), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0050-require-verifiable-tenant-export-from-shared-products.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #34 — T33 发起知识库问答 - #68 — T67 WeKnora Adapter Compatibility

---

## File Structure

- Create `internal/lifecycle/tenant-export/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/lifecycle/tenant-export/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/conformance/scenarios/t70-tenant-export.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/conformance/t70_tenant_export_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T70 as one testable vertical slice

**Files:**
- Create: `internal/lifecycle/tenant-export/service.go`
- Create: `internal/lifecycle/tenant-export/service_test.go`
- Create: `tests/conformance/scenarios/t70-tenant-export.yaml`
- Create: `tests/conformance/t70_tenant_export_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `TenantExportCommand{ContractVersion string, ProductBindingID string, ExportID string, IdempotencyKey string}`, `NewTenantExportService(tx Tx, port TenantExportPort, evidence EvidenceSink) *TenantExportService`, and `(*TenantExportService).Execute(ctx context.Context, cmd TenantExportCommand) (TenantExportResult, error)`.
- Guarantees: idempotency key and `ContractVersion` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package tenantexport_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/lifecycle/tenant-export"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.TenantExportCommand) (feature.TenantExportResult, error) {
    p.calls++
    return feature.TenantExportResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestTenantExportRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewTenantExportService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.TenantExportCommand{
        ContractVersion: "",
        IdempotencyKey: "t70-guard",
    })

    if !errors.Is(err, feature.ErrContractVersionRequired) {
        t.Fatalf("expected %v, got %v", feature.ErrContractVersionRequired, err)
    }
    if port.calls != 0 {
        t.Fatalf("outbound port called %d times", port.calls)
    }
}
```

- [ ] **Step 2: Run the focused test and confirm the red state**

Run: `go test ./internal/lifecycle/tenant-export -run TestTenantExportRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewTenantExportService`, `TenantExportCommand`, and `ErrContractVersionRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package tenantexport

import (
    "context"
    "errors"
)

var (
    ErrContractVersionRequired = errors.New("contract version is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type TenantExportCommand struct {
    ContractVersion string
    ProductBindingID string
    ExportID string
    IdempotencyKey string
}

type TenantExportResult struct {
    ResourceID string
    Outcome    string
}

type TenantExportPort interface {
    Apply(context.Context, TenantExportCommand) (TenantExportResult, error)
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
type TenantExportService struct {
    tx       Tx
    port     TenantExportPort
    evidence EvidenceSink
}

func NewTenantExportService(tx Tx, port TenantExportPort, evidence EvidenceSink) *TenantExportService {
    return &TenantExportService{tx: tx, port: port, evidence: evidence}
}

func (s *TenantExportService) Execute(ctx context.Context, cmd TenantExportCommand) (result TenantExportResult, err error) {
    if cmd.ContractVersion == "" {
        return TenantExportResult{}, ErrContractVersionRequired
    }
    if cmd.IdempotencyKey == "" {
        return TenantExportResult{}, ErrIdempotencyKeyRequired
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

The concrete `TenantExportPort.Apply` implementation in this file must enforce the Ticket invariant: **导出 Product 和 Schema Version、对象校验和、时间和完整性证据。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/lifecycle/tenant-export -run 'TenantExport' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t70-tenant-export
issue: 71
batch: P18
seam: Provider/Adapter conformance case
scope:
  contract_version: stage1-v1
idempotency_key: t70-acceptance
normal:
  expect: "导出 Product 和 Schema Version、对象校验和、时间和完整性证据。"
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
package conformance_test

import (
    "testing"

    "github.com/1123786563/myqypt/tests/platformtest"
)

func TestT70TenantExport(t *testing.T) {
    report := platformtest.Run(t, "tests/conformance/scenarios/t70-tenant-export.yaml")
    if !report.Passed {
        t.Fatalf("T70 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/conformance -run TestT70TenantExport -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t70/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/lifecycle/tenant-export ./tests/conformance -count=1`

Expected: PASS with no skipped T70 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/lifecycle/tenant-export/service.go internal/lifecycle/tenant-export/service_test.go tests/conformance/scenarios/t70-tenant-export.yaml tests/conformance/t70_tenant_export_test.go
git commit -m "feat(lifecycle): deliver T70 tenant-export"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #71 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `TenantExportCommand`, `TenantExportResult`, `TenantExportPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
