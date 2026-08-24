# T15 Product Offer 与 Data Processing Profile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Owner 查看价格、Entitlement、Included Allowance 和数据处理条款。

**Architecture:** Implement this Ticket as one vertical slice in `internal/catalog` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification

**Spec:** [GitHub Issue #16](https://github.com/1123786563/myqypt/issues/16), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0006-use-subscriptions-with-prepaid-overage.md`, `docs/adr/0047-bind-model-routing-to-data-processing-profiles.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #14 — T13 Product Catalog 浏览

---

## File Structure

- Create `internal/catalog/product-offer-data-processing-profile/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/catalog/product-offer-data-processing-profile/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t15-product-offer-data-processing-profile.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t15_product_offer_data_processing_profile_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T15 as one testable vertical slice

**Files:**
- Create: `internal/catalog/product-offer-data-processing-profile/service.go`
- Create: `internal/catalog/product-offer-data-processing-profile/service_test.go`
- Create: `tests/acceptance/scenarios/t15-product-offer-data-processing-profile.yaml`
- Create: `tests/acceptance/t15_product_offer_data_processing_profile_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `ProductOfferDataProcessingProfileCommand{TenantID string, ProductOfferID string, IdempotencyKey string}`, `NewProductOfferDataProcessingProfileService(tx Tx, port ProductOfferDataProcessingProfilePort, evidence EvidenceSink) *ProductOfferDataProcessingProfileService`, and `(*ProductOfferDataProcessingProfileService).Execute(ctx context.Context, cmd ProductOfferDataProcessingProfileCommand) (ProductOfferDataProcessingProfileResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package productofferdataprocessingprofile_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/catalog/product-offer-data-processing-profile"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.ProductOfferDataProcessingProfileCommand) (feature.ProductOfferDataProcessingProfileResult, error) {
    p.calls++
    return feature.ProductOfferDataProcessingProfileResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestProductOfferDataProcessingProfileRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewProductOfferDataProcessingProfileService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.ProductOfferDataProcessingProfileCommand{
        TenantID: "",
        IdempotencyKey: "t15-guard",
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

Run: `go test ./internal/catalog/product-offer-data-processing-profile -run TestProductOfferDataProcessingProfileRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewProductOfferDataProcessingProfileService`, `ProductOfferDataProcessingProfileCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package productofferdataprocessingprofile

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type ProductOfferDataProcessingProfileCommand struct {
    TenantID string
    ProductOfferID string
    IdempotencyKey string
}

type ProductOfferDataProcessingProfileResult struct {
    ResourceID string
    Outcome    string
}

type ProductOfferDataProcessingProfilePort interface {
    Apply(context.Context, ProductOfferDataProcessingProfileCommand) (ProductOfferDataProcessingProfileResult, error)
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
type ProductOfferDataProcessingProfileService struct {
    tx       Tx
    port     ProductOfferDataProcessingProfilePort
    evidence EvidenceSink
}

func NewProductOfferDataProcessingProfileService(tx Tx, port ProductOfferDataProcessingProfilePort, evidence EvidenceSink) *ProductOfferDataProcessingProfileService {
    return &ProductOfferDataProcessingProfileService{tx: tx, port: port, evidence: evidence}
}

func (s *ProductOfferDataProcessingProfileService) Execute(ctx context.Context, cmd ProductOfferDataProcessingProfileCommand) (result ProductOfferDataProcessingProfileResult, err error) {
    if cmd.TenantID == "" {
        return ProductOfferDataProcessingProfileResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return ProductOfferDataProcessingProfileResult{}, ErrIdempotencyKeyRequired
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

The concrete `ProductOfferDataProcessingProfilePort.Apply` implementation in this file must enforce the Ticket invariant: **Owner 查看价格、Entitlement、Included Allowance 和数据处理条款。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/catalog/product-offer-data-processing-profile -run 'ProductOfferDataProcessingProfile' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t15-product-offer-data-processing-profile
issue: 16
batch: P3
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t15-acceptance
normal:
  expect: "Owner 查看价格、Entitlement、Included Allowance 和数据处理条款。"
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

func TestT15ProductOfferDataProcessingProfile(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t15-product-offer-data-processing-profile.yaml")
    if !report.Passed {
        t.Fatalf("T15 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT15ProductOfferDataProcessingProfile -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t15/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/catalog/product-offer-data-processing-profile ./tests/acceptance -count=1`

Expected: PASS with no skipped T15 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/catalog/product-offer-data-processing-profile/service.go internal/catalog/product-offer-data-processing-profile/service_test.go tests/acceptance/scenarios/t15-product-offer-data-processing-profile.yaml tests/acceptance/t15_product_offer_data_processing_profile_test.go
git commit -m "feat(catalog): deliver T15 product-offer-data-processing-profile"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #16 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `ProductOfferDataProcessingProfileCommand`, `ProductOfferDataProcessingProfileResult`, `ProductOfferDataProcessingProfilePort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
