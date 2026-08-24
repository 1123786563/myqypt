# T47 Usage Reservation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 可变成本问答执行前预留最大可消费金额。

**Architecture:** Implement this Ticket as one vertical slice in `internal/usage` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, Kafka, OpenMeter, ClickHouse

**Spec:** [GitHub Issue #48](https://github.com/1123786563/myqypt/issues/48), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0017-use-append-only-adjustments-and-pre-execution-reservations.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #34 — T33 发起知识库问答 - #46 — T45 Included Allowance Consumption

---

## File Structure

- Create `internal/usage/usage-reservation/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/usage/usage-reservation/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t47-usage-reservation.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t47_usage_reservation_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T47 as one testable vertical slice

**Files:**
- Create: `internal/usage/usage-reservation/service.go`
- Create: `internal/usage/usage-reservation/service_test.go`
- Create: `tests/acceptance/scenarios/t47-usage-reservation.yaml`
- Create: `tests/acceptance/t47_usage_reservation_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `UsageReservationCommand{TenantID string, ReservationID string, MaximumAmountFen int64, IdempotencyKey string}`, `NewUsageReservationService(tx Tx, port UsageReservationPort, evidence EvidenceSink) *UsageReservationService`, and `(*UsageReservationService).Execute(ctx context.Context, cmd UsageReservationCommand) (UsageReservationResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package usagereservation_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/usage/usage-reservation"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.UsageReservationCommand) (feature.UsageReservationResult, error) {
    p.calls++
    return feature.UsageReservationResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestUsageReservationRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewUsageReservationService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.UsageReservationCommand{
        TenantID: "",
        IdempotencyKey: "t47-guard",
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

Run: `go test ./internal/usage/usage-reservation -run TestUsageReservationRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewUsageReservationService`, `UsageReservationCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package usagereservation

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type UsageReservationCommand struct {
    TenantID string
    ReservationID string
    MaximumAmountFen int64
    IdempotencyKey string
}

type UsageReservationResult struct {
    ResourceID string
    Outcome    string
}

type UsageReservationPort interface {
    Apply(context.Context, UsageReservationCommand) (UsageReservationResult, error)
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
type UsageReservationService struct {
    tx       Tx
    port     UsageReservationPort
    evidence EvidenceSink
}

func NewUsageReservationService(tx Tx, port UsageReservationPort, evidence EvidenceSink) *UsageReservationService {
    return &UsageReservationService{tx: tx, port: port, evidence: evidence}
}

func (s *UsageReservationService) Execute(ctx context.Context, cmd UsageReservationCommand) (result UsageReservationResult, err error) {
    if cmd.TenantID == "" {
        return UsageReservationResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return UsageReservationResult{}, ErrIdempotencyKeyRequired
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

The concrete `UsageReservationPort.Apply` implementation in this file must enforce the Ticket invariant: **可变成本问答执行前预留最大可消费金额。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/usage/usage-reservation -run 'UsageReservation' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t47-usage-reservation
issue: 48
batch: P20
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t47-acceptance
normal:
  expect: "可变成本问答执行前预留最大可消费金额。"
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

func TestT47UsageReservation(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t47-usage-reservation.yaml")
    if !report.Passed {
        t.Fatalf("T47 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT47UsageReservation -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t47/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/usage/usage-reservation ./tests/acceptance -count=1`

Expected: PASS with no skipped T47 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/usage/usage-reservation/service.go internal/usage/usage-reservation/service_test.go tests/acceptance/scenarios/t47-usage-reservation.yaml tests/acceptance/t47_usage_reservation_test.go
git commit -m "feat(usage): deliver T47 usage-reservation"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #48 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `UsageReservationCommand`, `UsageReservationResult`, `UsageReservationPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
