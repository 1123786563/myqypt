# T33 发起知识库问答 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Member 通过批准模型路径得到基于知识库的回答。

**Architecture:** Implement this Ticket as one vertical slice in `internal/weknora` and prove it through the black-box journey slice under `tests/acceptance`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, WeKnora, Higress

**Spec:** [GitHub Issue #34](https://github.com/1123786563/myqypt/issues/34), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0008-require-weknora-shared-tenancy-hardening-before-paid-launch.md`, `docs/adr/0045-use-platform-funded-models-only-through-higress.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #29 — T28 Higress Approved Model Call - #33 — T32 文档上传与索引

---

## File Structure

- Create `internal/weknora/knowledge-base-qa/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/weknora/knowledge-base-qa/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/acceptance/scenarios/t33-knowledge-base-qa.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/acceptance/t33_knowledge_base_qa_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T33 as one testable vertical slice

**Files:**
- Create: `internal/weknora/knowledge-base-qa/service.go`
- Create: `internal/weknora/knowledge-base-qa/service_test.go`
- Create: `tests/acceptance/scenarios/t33-knowledge-base-qa.yaml`
- Create: `tests/acceptance/t33_knowledge_base_qa_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `KnowledgeBaseQaCommand{TenantID string, KnowledgeBaseID string, RequestID string, IdempotencyKey string}`, `NewKnowledgeBaseQaService(tx Tx, port KnowledgeBaseQaPort, evidence EvidenceSink) *KnowledgeBaseQaService`, and `(*KnowledgeBaseQaService).Execute(ctx context.Context, cmd KnowledgeBaseQaCommand) (KnowledgeBaseQaResult, error)`.
- Guarantees: idempotency key and `TenantID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package knowledgebaseqa_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/weknora/knowledge-base-qa"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.KnowledgeBaseQaCommand) (feature.KnowledgeBaseQaResult, error) {
    p.calls++
    return feature.KnowledgeBaseQaResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestKnowledgeBaseQaRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewKnowledgeBaseQaService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.KnowledgeBaseQaCommand{
        TenantID: "",
        IdempotencyKey: "t33-guard",
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

Run: `go test ./internal/weknora/knowledge-base-qa -run TestKnowledgeBaseQaRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewKnowledgeBaseQaService`, `KnowledgeBaseQaCommand`, and `ErrTenantRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package knowledgebaseqa

import (
    "context"
    "errors"
)

var (
    ErrTenantRequired = errors.New("tenant context is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type KnowledgeBaseQaCommand struct {
    TenantID string
    KnowledgeBaseID string
    RequestID string
    IdempotencyKey string
}

type KnowledgeBaseQaResult struct {
    ResourceID string
    Outcome    string
}

type KnowledgeBaseQaPort interface {
    Apply(context.Context, KnowledgeBaseQaCommand) (KnowledgeBaseQaResult, error)
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
type KnowledgeBaseQaService struct {
    tx       Tx
    port     KnowledgeBaseQaPort
    evidence EvidenceSink
}

func NewKnowledgeBaseQaService(tx Tx, port KnowledgeBaseQaPort, evidence EvidenceSink) *KnowledgeBaseQaService {
    return &KnowledgeBaseQaService{tx: tx, port: port, evidence: evidence}
}

func (s *KnowledgeBaseQaService) Execute(ctx context.Context, cmd KnowledgeBaseQaCommand) (result KnowledgeBaseQaResult, err error) {
    if cmd.TenantID == "" {
        return KnowledgeBaseQaResult{}, ErrTenantRequired
    }
    if cmd.IdempotencyKey == "" {
        return KnowledgeBaseQaResult{}, ErrIdempotencyKeyRequired
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

The concrete `KnowledgeBaseQaPort.Apply` implementation in this file must enforce the Ticket invariant: **Member 通过批准模型路径得到基于知识库的回答。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/weknora/knowledge-base-qa -run 'KnowledgeBaseQa' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t33-knowledge-base-qa
issue: 34
batch: P17
seam: black-box journey slice
scope:
  tenant_id: tenant-a
idempotency_key: t33-acceptance
normal:
  expect: "Member 通过批准模型路径得到基于知识库的回答。"
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

func TestT33KnowledgeBaseQa(t *testing.T) {
    report := platformtest.Run(t, "tests/acceptance/scenarios/t33-knowledge-base-qa.yaml")
    if !report.Passed {
        t.Fatalf("T33 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/acceptance -run TestT33KnowledgeBaseQa -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t33/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/weknora/knowledge-base-qa ./tests/acceptance -count=1`

Expected: PASS with no skipped T33 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/weknora/knowledge-base-qa/service.go internal/weknora/knowledge-base-qa/service_test.go tests/acceptance/scenarios/t33-knowledge-base-qa.yaml tests/acceptance/t33_knowledge_base_qa_test.go
git commit -m "feat(weknora): deliver T33 knowledge-base-qa"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #34 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `KnowledgeBaseQaCommand`, `KnowledgeBaseQaResult`, `KnowledgeBaseQaPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
