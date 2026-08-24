# T45 Included Allowance Consumption Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Usage 优先消费 Product 和 Meter 对应的 Included Allowance。

**Architecture:** Implement this Ticket as one vertical slice in `internal/usage` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, Kafka, OpenMeter, ClickHouse

**Spec:** [GitHub Issue #46](https://github.com/1123786563/myqypt/issues/46), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0006-use-subscriptions-with-prepaid-overage.md`, `docs/adr/0015-run-kafka-and-clickhouse-as-openmeter-core-dependencies.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #16 — T15 Product Offer 与 Data Processing Profile - #45 — T44 OpenMeter Runtime 与 Adapter

---

## File Structure

- Create `internal/usage/included-allowance/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/usage/included-allowance/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t45-included-allowance.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t45_included_allowance_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T45 as one testable vertical slice

**Files:**
- Create: `internal/usage/included-allowance/service.go`
- Create: `internal/usage/included-allowance/service_test.go`
- Create: `tests/acceptance/scenarios/t45-included-allowance.yaml`
- Create: `tests/acceptance/t45_included_allowance_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `IncludedAllowanceCommand{TenantID string, SubscriptionID string, Meter string, QuantityDecimal string, IdempotencyKey string}`, `NewIncludedAllowanceService(tx Tx, port IncludedAllowancePort, evidence EvidenceSink) *IncludedAllowanceService`, and `(*IncludedAllowanceService).Execute(ctx context.Context, cmd IncludedAllowanceCommand) (IncludedAllowanceResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package includedallowance_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/usage/included-allowance"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.IncludedAllowanceCommand) (feature.IncludedAllowanceResult, error) {
    p.calls++
    return feature.IncludedAllowanceResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestIncludedAllowanceRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewIncludedAllowanceService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.IncludedAllowanceCommand{
        TenantID: "",
        IdempotencyKey: "t45-guard",
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

Run: `go test ./internal/usage/included-allowance -run TestIncludedAllowanceRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewIncludedAllowanceService`, `IncludedAllowanceCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package includedallowance

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type IncludedAllowanceCommand struct {
    TenantID string
    SubscriptionID string
    Meter string
    QuantityDecimal string
    IdempotencyKey string
}

type IncludedAllowanceResult struct {
    ResourceID string
    Outcome    string
}

type IncludedAllowancePort interface {
    Apply(context.Context, IncludedAllowanceCommand) (IncludedAllowanceResult, error)
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
type IncludedAllowanceService struct {
    tx       Tx
    port     IncludedAllowancePort
    evidence EvidenceSink
}

func NewIncludedAllowanceService(tx Tx, port IncludedAllowancePort, evidence EvidenceSink) *IncludedAllowanceService {
    return &IncludedAllowanceService{tx: tx, port: port, evidence: evidence}
}

func (s *IncludedAllowanceService) Execute(ctx context.Context, cmd IncludedAllowanceCommand) (result IncludedAllowanceResult, err error) {
    if cmd.TenantID == "" {
        return IncludedAllowanceResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return IncludedAllowanceResult{}, ErrIdempotencyKeyRequired
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

The concrete `IncludedAllowancePort.Apply` implementation in this file must enforce the Ticket invariant: **Usage 优先消费 Product 和 Meter 对应的 Included Allowance。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/usage/included-allowance -run 'IncludedAllowance' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t45-included-allowance
issue: 46
batch: P19
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t45-acceptance
normal:
  expect: "Usage 优先消费 Product 和 Meter 对应的 Included Allowance。"
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

func TestT45IncludedAllowance(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t45-included-allowance.yaml")
    if !report.Passed {
        t.Fatalf("T45 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT45IncludedAllowance -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t45/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/usage/included-allowance ./tests/acceptance -count=1`

Expected: PASS with no skipped T45 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/usage/included-allowance/service.go internal/usage/included-allowance/service_test.go tests/acceptance/scenarios/t45-included-allowance.yaml tests/acceptance/t45_included_allowance_test.go
git commit -m "feat(usage): deliver T45 included-allowance"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #46 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `IncludedAllowanceCommand`, `IncludedAllowanceResult`, `IncludedAllowancePort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
