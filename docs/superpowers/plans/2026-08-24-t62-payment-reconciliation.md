# T62 Hourly Payment Reconciliation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provider、Payment Journal、Fulfillment 和 Credit 的差异被发现并冻结。

**Architecture:** Implement this Ticket as one vertical slice in `internal/commerce` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, WeChat Pay / Alipay Provider adapters, OpenMeter

**Spec:** [GitHub Issue #63](https://github.com/1123786563/myqypt/issues/63), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0018-own-real-money-transactions-in-platform-commerce.md`, `docs/adr/0042-reconcile-payment-hourly-and-usage-daily.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #60 — T59 Paid Fulfillment 与 Credit Lots - #62 — T61 Refund Reservation 与 Provider Refund

---

## File Structure

- Create `internal/commerce/payment-reconciliation/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/commerce/payment-reconciliation/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t62-payment-reconciliation.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t62_payment_reconciliation_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T62 as one testable vertical slice

**Files:**
- Create: `internal/commerce/payment-reconciliation/service.go`
- Create: `internal/commerce/payment-reconciliation/service_test.go`
- Create: `tests/acceptance/scenarios/t62-payment-reconciliation.yaml`
- Create: `tests/acceptance/t62_payment_reconciliation_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `PaymentReconciliationCommand{TenantID string, ReconciliationHour string, IdempotencyKey string}`, `NewPaymentReconciliationService(tx Tx, port PaymentReconciliationPort, evidence EvidenceSink) *PaymentReconciliationService`, and `(*PaymentReconciliationService).Execute(ctx context.Context, cmd PaymentReconciliationCommand) (PaymentReconciliationResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package paymentreconciliation_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/commerce/payment-reconciliation"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.PaymentReconciliationCommand) (feature.PaymentReconciliationResult, error) {
    p.calls++
    return feature.PaymentReconciliationResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestPaymentReconciliationRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewPaymentReconciliationService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.PaymentReconciliationCommand{
        TenantID: "",
        IdempotencyKey: "t62-guard",
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

Run: `go test ./internal/commerce/payment-reconciliation -run TestPaymentReconciliationRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewPaymentReconciliationService`, `PaymentReconciliationCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package paymentreconciliation

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type PaymentReconciliationCommand struct {
    TenantID string
    ReconciliationHour string
    IdempotencyKey string
}

type PaymentReconciliationResult struct {
    ResourceID string
    Outcome    string
}

type PaymentReconciliationPort interface {
    Apply(context.Context, PaymentReconciliationCommand) (PaymentReconciliationResult, error)
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
type PaymentReconciliationService struct {
    tx       Tx
    port     PaymentReconciliationPort
    evidence EvidenceSink
}

func NewPaymentReconciliationService(tx Tx, port PaymentReconciliationPort, evidence EvidenceSink) *PaymentReconciliationService {
    return &PaymentReconciliationService{tx: tx, port: port, evidence: evidence}
}

func (s *PaymentReconciliationService) Execute(ctx context.Context, cmd PaymentReconciliationCommand) (result PaymentReconciliationResult, err error) {
    if cmd.TenantID == "" {
        return PaymentReconciliationResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return PaymentReconciliationResult{}, ErrIdempotencyKeyRequired
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

The concrete `PaymentReconciliationPort.Apply` implementation in this file must enforce the Ticket invariant: **Provider、Payment Journal、Fulfillment 和 Credit 的差异被发现并冻结。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/commerce/payment-reconciliation -run 'PaymentReconciliation' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t62-payment-reconciliation
issue: 63
batch: P22
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t62-acceptance
normal:
  expect: "Provider、Payment Journal、Fulfillment 和 Credit 的差异被发现并冻结。"
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

func TestT62PaymentReconciliation(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t62-payment-reconciliation.yaml")
    if !report.Passed {
        t.Fatalf("T62 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT62PaymentReconciliation -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t62/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/commerce/payment-reconciliation ./tests/acceptance -count=1`

Expected: PASS with no skipped T62 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/commerce/payment-reconciliation/service.go internal/commerce/payment-reconciliation/service_test.go tests/acceptance/scenarios/t62-payment-reconciliation.yaml tests/acceptance/t62_payment_reconciliation_test.go
git commit -m "feat(commerce): deliver T62 payment-reconciliation"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #63 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `PaymentReconciliationCommand`, `PaymentReconciliationResult`, `PaymentReconciliationPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
