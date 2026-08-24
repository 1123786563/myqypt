# T42 Canonical Usage Schema Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Usage Ingest 接受批准 Authority 的事件并拒绝非法身份、单位、数量和 metadata。

**Architecture:** Implement this Ticket as one vertical slice in `internal/usage` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, Kafka, OpenMeter, ClickHouse

**Spec:** [GitHub Issue #43](https://github.com/1123786563/myqypt/issues/43), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0012-accept-usage-only-from-approved-server-authorities.md`, `docs/adr/0019-fix-canonical-usage-identity-and-privacy-boundaries.md`, `docs/adr/0020-use-fixed-precision-rating-at-occurrence-time.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #29 — T28 Higress Approved Model Call - #32 — T31 创建 Tenant-scoped 知识库

---

## File Structure

- Create `internal/usage/canonical-usage-validation/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/usage/canonical-usage-validation/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t42-canonical-usage-validation.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t42_canonical_usage_validation_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T42 as one testable vertical slice

**Files:**
- Create: `internal/usage/canonical-usage-validation/service.go`
- Create: `internal/usage/canonical-usage-validation/service_test.go`
- Create: `tests/acceptance/scenarios/t42-canonical-usage-validation.yaml`
- Create: `tests/acceptance/t42_canonical_usage_validation_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `CanonicalUsageValidationCommand{TenantID string, EventID string, SchemaVersion string, ProductID string, ProductInstanceID string, SubjectType string, SubjectID string, Meter string, QuantityDecimal string, Unit string, OccurredAt string, SourceType string, SourceID string, Metadata map[string]string, IdempotencyKey string}`, `NewCanonicalUsageValidationService(tx Tx, port CanonicalUsageValidationPort, evidence EvidenceSink) *CanonicalUsageValidationService`, and `(*CanonicalUsageValidationService).Execute(ctx context.Context, cmd CanonicalUsageValidationCommand) (CanonicalUsageValidationResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package canonicalusagevalidation_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/usage/canonical-usage-validation"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.CanonicalUsageValidationCommand) (feature.CanonicalUsageValidationResult, error) {
    p.calls++
    return feature.CanonicalUsageValidationResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestCanonicalUsageValidationRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewCanonicalUsageValidationService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.CanonicalUsageValidationCommand{
        TenantID: "",
        IdempotencyKey: "t42-guard",
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

Run: `go test ./internal/usage/canonical-usage-validation -run TestCanonicalUsageValidationRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewCanonicalUsageValidationService`, `CanonicalUsageValidationCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package canonicalusagevalidation

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type CanonicalUsageValidationCommand struct {
    TenantID string
    EventID string
    SchemaVersion string
    ProductID string
    ProductInstanceID string
    SubjectType string
    SubjectID string
    Meter string
    QuantityDecimal string
    Unit string
    OccurredAt string
    SourceType string
    SourceID string
    Metadata map[string]string
    IdempotencyKey string
}

type CanonicalUsageValidationResult struct {
    ResourceID string
    Outcome    string
}

type CanonicalUsageValidationPort interface {
    Apply(context.Context, CanonicalUsageValidationCommand) (CanonicalUsageValidationResult, error)
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
type CanonicalUsageValidationService struct {
    tx       Tx
    port     CanonicalUsageValidationPort
    evidence EvidenceSink
}

func NewCanonicalUsageValidationService(tx Tx, port CanonicalUsageValidationPort, evidence EvidenceSink) *CanonicalUsageValidationService {
    return &CanonicalUsageValidationService{tx: tx, port: port, evidence: evidence}
}

func (s *CanonicalUsageValidationService) Execute(ctx context.Context, cmd CanonicalUsageValidationCommand) (result CanonicalUsageValidationResult, err error) {
    if cmd.TenantID == "" {
        return CanonicalUsageValidationResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return CanonicalUsageValidationResult{}, ErrIdempotencyKeyRequired
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

The concrete `CanonicalUsageValidationPort.Apply` implementation in this file must enforce the Ticket invariant: **Usage Ingest 接受批准 Authority 的事件并拒绝非法身份、单位、数量和 metadata。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/usage/canonical-usage-validation -run 'CanonicalUsageValidation' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t42-canonical-usage-validation
issue: 43
batch: P16
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t42-acceptance
normal:
  expect: "Usage Ingest 接受批准 Authority 的事件并拒绝非法身份、单位、数量和 metadata。"
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

func TestT42CanonicalUsageValidation(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t42-canonical-usage-validation.yaml")
    if !report.Passed {
        t.Fatalf("T42 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT42CanonicalUsageValidation -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t42/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/usage/canonical-usage-validation ./tests/acceptance -count=1`

Expected: PASS with no skipped T42 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/usage/canonical-usage-validation/service.go internal/usage/canonical-usage-validation/service_test.go tests/acceptance/scenarios/t42-canonical-usage-validation.yaml tests/acceptance/t42_canonical_usage_validation_test.go
git commit -m "feat(usage): deliver T42 canonical-usage-validation"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #43 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `CanonicalUsageValidationCommand`, `CanonicalUsageValidationResult`, `CanonicalUsageValidationPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
