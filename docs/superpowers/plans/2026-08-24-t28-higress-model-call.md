# T28 Higress Approved Model Call Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** WeKnora 模型请求只能经 Higress 使用 Platform 管理的 Provider Key。

**Architecture:** Implement this Ticket as one vertical slice in `internal/security` and prove it through the Provider/Adapter conformance case under `tests/conformance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, Higress, Kubernetes NetworkPolicy

**Spec:** [GitHub Issue #29](https://github.com/1123786563/myqypt/issues/29), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0045-use-platform-funded-models-only-through-higress.md`, `docs/adr/0047-bind-model-routing-to-data-processing-profiles.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #25 — T24 原生 WeKnora OIDC SSO - #28 — T27 Data Processing Profile Enforcement

---

## File Structure

- Create `internal/security/higress-model-call/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/security/higress-model-call/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/conformance/scenarios/t28-higress-model-call.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/conformance/t28_higress_model_call_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T28 as one testable vertical slice

**Files:**
- Create: `internal/security/higress-model-call/service.go`
- Create: `internal/security/higress-model-call/service_test.go`
- Create: `tests/conformance/scenarios/t28-higress-model-call.yaml`
- Create: `tests/conformance/t28_higress_model_call_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `HigressModelCallCommand{ContractVersion string, ProductBindingID string, ModelRoute string, MaximumAmountFen int64, IdempotencyKey string}`, `NewHigressModelCallService(tx Tx, port HigressModelCallPort, evidence EvidenceSink) *HigressModelCallService`, and `(*HigressModelCallService).Execute(ctx context.Context, cmd HigressModelCallCommand) (HigressModelCallResult, error)`.
- Guarantees: idempotency key and `ContractVersion` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package higressmodelcall_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/security/higress-model-call"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.HigressModelCallCommand) (feature.HigressModelCallResult, error) {
    p.calls++
    return feature.HigressModelCallResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestHigressModelCallRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewHigressModelCallService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.HigressModelCallCommand{
        ContractVersion: "",
        IdempotencyKey: "t28-guard",
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

Run: `go test ./internal/security/higress-model-call -run TestHigressModelCallRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewHigressModelCallService`, `HigressModelCallCommand`, and `ErrContractVersionRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package higressmodelcall

import (
    "context"
    "errors"
)

var (
    ErrContractVersionRequired = errors.New("contract version is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type HigressModelCallCommand struct {
    ContractVersion string
    ProductBindingID string
    ModelRoute string
    MaximumAmountFen int64
    IdempotencyKey string
}

type HigressModelCallResult struct {
    ResourceID string
    Outcome    string
}

type HigressModelCallPort interface {
    Apply(context.Context, HigressModelCallCommand) (HigressModelCallResult, error)
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
type HigressModelCallService struct {
    tx       Tx
    port     HigressModelCallPort
    evidence EvidenceSink
}

func NewHigressModelCallService(tx Tx, port HigressModelCallPort, evidence EvidenceSink) *HigressModelCallService {
    return &HigressModelCallService{tx: tx, port: port, evidence: evidence}
}

func (s *HigressModelCallService) Execute(ctx context.Context, cmd HigressModelCallCommand) (result HigressModelCallResult, err error) {
    if cmd.ContractVersion == "" {
        return HigressModelCallResult{}, ErrContractVersionRequired
    }
    if cmd.IdempotencyKey == "" {
        return HigressModelCallResult{}, ErrIdempotencyKeyRequired
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

The concrete `HigressModelCallPort.Apply` implementation in this file must enforce the Ticket invariant: **WeKnora 模型请求只能经 Higress 使用 Platform 管理的 Provider Key。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/security/higress-model-call -run 'HigressModelCall' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t28-higress-model-call
issue: 29
batch: P14
seam: Provider/Adapter conformance case
scope:
  contract_version: stage1-v1
idempotency_key: t28-acceptance
normal:
  expect: "WeKnora 模型请求只能经 Higress 使用 Platform 管理的 Provider Key。"
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

func TestT28HigressModelCall(t *testing.T) {
    report := platformtest.Run(t, "tests/conformance/scenarios/t28-higress-model-call.yaml")
    if !report.Passed {
        t.Fatalf("T28 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/conformance -run TestT28HigressModelCall -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t28/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/security/higress-model-call ./tests/conformance -count=1`

Expected: PASS with no skipped T28 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/security/higress-model-call/service.go internal/security/higress-model-call/service_test.go tests/conformance/scenarios/t28-higress-model-call.yaml tests/conformance/t28_higress_model_call_test.go
git commit -m "feat(security): deliver T28 higress-model-call"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #29 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `HigressModelCallCommand`, `HigressModelCallResult`, `HigressModelCallPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
