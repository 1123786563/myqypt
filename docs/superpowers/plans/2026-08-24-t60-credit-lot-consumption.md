# T60 Credit Lot 最早到期优先消费 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 多来源 Credit Lot 按明确 expiry 顺序消费并保留余额解释。

**Architecture:** Implement this Ticket as one vertical slice in `internal/commerce` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, WeChat Pay / Alipay Provider adapters, OpenMeter

**Spec:** [GitHub Issue #61](https://github.com/1123786563/myqypt/issues/61), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0053-preserve-credit-source-lots-for-consumption-and-refund.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #46 — T45 Included Allowance Consumption - #60 — T59 Paid Fulfillment 与 Credit Lots

---

## File Structure

- Create `internal/commerce/credit-lot-consumption/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/commerce/credit-lot-consumption/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t60-credit-lot-consumption.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t60_credit_lot_consumption_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T60 as one testable vertical slice

**Files:**
- Create: `internal/commerce/credit-lot-consumption/service.go`
- Create: `internal/commerce/credit-lot-consumption/service_test.go`
- Create: `tests/acceptance/scenarios/t60-credit-lot-consumption.yaml`
- Create: `tests/acceptance/t60_credit_lot_consumption_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `CreditLotConsumptionCommand{TenantID string, AmountFen int64, IdempotencyKey string}`, `NewCreditLotConsumptionService(tx Tx, port CreditLotConsumptionPort, evidence EvidenceSink) *CreditLotConsumptionService`, and `(*CreditLotConsumptionService).Execute(ctx context.Context, cmd CreditLotConsumptionCommand) (CreditLotConsumptionResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package creditlotconsumption_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/commerce/credit-lot-consumption"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.CreditLotConsumptionCommand) (feature.CreditLotConsumptionResult, error) {
    p.calls++
    return feature.CreditLotConsumptionResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestCreditLotConsumptionRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewCreditLotConsumptionService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.CreditLotConsumptionCommand{
        TenantID: "",
        IdempotencyKey: "t60-guard",
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

Run: `go test ./internal/commerce/credit-lot-consumption -run TestCreditLotConsumptionRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewCreditLotConsumptionService`, `CreditLotConsumptionCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package creditlotconsumption

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type CreditLotConsumptionCommand struct {
    TenantID string
    AmountFen int64
    IdempotencyKey string
}

type CreditLotConsumptionResult struct {
    ResourceID string
    Outcome    string
}

type CreditLotConsumptionPort interface {
    Apply(context.Context, CreditLotConsumptionCommand) (CreditLotConsumptionResult, error)
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
type CreditLotConsumptionService struct {
    tx       Tx
    port     CreditLotConsumptionPort
    evidence EvidenceSink
}

func NewCreditLotConsumptionService(tx Tx, port CreditLotConsumptionPort, evidence EvidenceSink) *CreditLotConsumptionService {
    return &CreditLotConsumptionService{tx: tx, port: port, evidence: evidence}
}

func (s *CreditLotConsumptionService) Execute(ctx context.Context, cmd CreditLotConsumptionCommand) (result CreditLotConsumptionResult, err error) {
    if cmd.TenantID == "" {
        return CreditLotConsumptionResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return CreditLotConsumptionResult{}, ErrIdempotencyKeyRequired
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

The concrete `CreditLotConsumptionPort.Apply` implementation in this file must enforce the Ticket invariant: **多来源 Credit Lot 按明确 expiry 顺序消费并保留余额解释。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/commerce/credit-lot-consumption -run 'CreditLotConsumption' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t60-credit-lot-consumption
issue: 61
batch: P20
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t60-acceptance
normal:
  expect: "多来源 Credit Lot 按明确 expiry 顺序消费并保留余额解释。"
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

func TestT60CreditLotConsumption(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t60-credit-lot-consumption.yaml")
    if !report.Passed {
        t.Fatalf("T60 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT60CreditLotConsumption -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t60/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/commerce/credit-lot-consumption ./tests/acceptance -count=1`

Expected: PASS with no skipped T60 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/commerce/credit-lot-consumption/service.go internal/commerce/credit-lot-consumption/service_test.go tests/acceptance/scenarios/t60-credit-lot-consumption.yaml tests/acceptance/t60_credit_lot_consumption_test.go
git commit -m "feat(commerce): deliver T60 credit-lot-consumption"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #61 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `CreditLotConsumptionCommand`, `CreditLotConsumptionResult`, `CreditLotConsumptionPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
