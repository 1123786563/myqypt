# T78 Prompt Registry PoC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 独立验证 Prompt 版本、授权、可见性、回滚和 Audit。

**Architecture:** Implement this Ticket as one vertical slice in `internal/registry` and prove it through the Provider/Adapter conformance case under `tests/conformance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, Nacos 3.2.3 GA, Java 17, AI Registry Provider

**Spec:** [GitHub Issue #79](https://github.com/1123786563/myqypt/issues/79), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0051-include-nacos-in-the-day-1-platform.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #9 — T08 OpenFGA Grant Projection - #75 — T74 Nacos Baseline 与 AI Registry Provider

---

## File Structure

- Create `internal/registry/prompt-registry-poc/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/registry/prompt-registry-poc/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/conformance/scenarios/t78-prompt-registry-poc.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/conformance/t78_prompt_registry_poc_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T78 as one testable vertical slice

**Files:**
- Create: `internal/registry/prompt-registry-poc/service.go`
- Create: `internal/registry/prompt-registry-poc/service_test.go`
- Create: `tests/conformance/scenarios/t78-prompt-registry-poc.yaml`
- Create: `tests/conformance/t78_prompt_registry_poc_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `PromptRegistryPocCommand{ContractVersion string, PromptVersion string, IdempotencyKey string}`, `NewPromptRegistryPocService(tx Tx, port PromptRegistryPocPort, evidence EvidenceSink) *PromptRegistryPocService`, and `(*PromptRegistryPocService).Execute(ctx context.Context, cmd PromptRegistryPocCommand) (PromptRegistryPocResult, error)`.
- Guarantees: idempotency key and `ContractVersion` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package promptregistrypoc_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/registry/prompt-registry-poc"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.PromptRegistryPocCommand) (feature.PromptRegistryPocResult, error) {
    p.calls++
    return feature.PromptRegistryPocResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestPromptRegistryPocRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewPromptRegistryPocService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.PromptRegistryPocCommand{
        ContractVersion: "",
        IdempotencyKey: "t78-guard",
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

Run: `go test ./internal/registry/prompt-registry-poc -run TestPromptRegistryPocRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewPromptRegistryPocService`, `PromptRegistryPocCommand`, and `ErrContractVersionRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package promptregistrypoc

import (
    "context"
    "errors"
)

var (
    ErrContractVersionRequired = errors.New("contract version is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type PromptRegistryPocCommand struct {
    ContractVersion string
    PromptVersion string
    IdempotencyKey string
}

type PromptRegistryPocResult struct {
    ResourceID string
    Outcome    string
}

type PromptRegistryPocPort interface {
    Apply(context.Context, PromptRegistryPocCommand) (PromptRegistryPocResult, error)
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
type PromptRegistryPocService struct {
    tx       Tx
    port     PromptRegistryPocPort
    evidence EvidenceSink
}

func NewPromptRegistryPocService(tx Tx, port PromptRegistryPocPort, evidence EvidenceSink) *PromptRegistryPocService {
    return &PromptRegistryPocService{tx: tx, port: port, evidence: evidence}
}

func (s *PromptRegistryPocService) Execute(ctx context.Context, cmd PromptRegistryPocCommand) (result PromptRegistryPocResult, err error) {
    if cmd.ContractVersion == "" {
        return PromptRegistryPocResult{}, ErrContractVersionRequired
    }
    if cmd.IdempotencyKey == "" {
        return PromptRegistryPocResult{}, ErrIdempotencyKeyRequired
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

The concrete `PromptRegistryPocPort.Apply` implementation in this file must enforce the Ticket invariant: **独立验证 Prompt 版本、授权、可见性、回滚和 Audit。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/registry/prompt-registry-poc -run 'PromptRegistryPoc' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t78-prompt-registry-poc
issue: 79
batch: P6
seam: Provider/Adapter conformance case
scope:
  contract_version: stage1-v1
idempotency_key: t78-acceptance
normal:
  expect: "独立验证 Prompt 版本、授权、可见性、回滚和 Audit。"
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

func TestT78PromptRegistryPoc(t *testing.T) {
    report := platformtest.Run(t, "tests/conformance/scenarios/t78-prompt-registry-poc.yaml")
    if !report.Passed {
        t.Fatalf("T78 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/conformance -run TestT78PromptRegistryPoc -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t78/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/registry/prompt-registry-poc ./tests/conformance -count=1`

Expected: PASS with no skipped T78 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/registry/prompt-registry-poc/service.go internal/registry/prompt-registry-poc/service_test.go tests/conformance/scenarios/t78-prompt-registry-poc.yaml tests/conformance/t78_prompt_registry_poc_test.go
git commit -m "feat(registry): deliver T78 prompt-registry-poc"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #79 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `PromptRegistryPocCommand`, `PromptRegistryPocResult`, `PromptRegistryPocPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
