# T79 Consented JIT Operator Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support 访问要求 case、Owner consent、MFA、JIT、到期和完整 Audit。

**Architecture:** Implement this Ticket as one vertical slice in `internal/operations` and prove it through the Production Gate evidence case under `tests/production-gates`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification

**Spec:** [GitHub Issue #80](https://github.com/1123786563/myqypt/issues/80), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0041-maintain-a-content-minimized-immutable-audit-stream.md`, `docs/adr/0048-use-consented-jit-operator-access.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #8 — T07 Membership 与 Role Audit - #10 — T09 OpenFGA 立即撤权 - #25 — T24 原生 WeKnora OIDC SSO

---

## File Structure

- Create `internal/operations/jit-operator-access/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/operations/jit-operator-access/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/production-gates/scenarios/t79-jit-operator-access.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/production-gates/t79_jit_operator_access_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T79 as one testable vertical slice

**Files:**
- Create: `internal/operations/jit-operator-access/service.go`
- Create: `internal/operations/jit-operator-access/service_test.go`
- Create: `tests/production-gates/scenarios/t79-jit-operator-access.yaml`
- Create: `tests/production-gates/t79_jit_operator_access_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `JitOperatorAccessCommand{EnvironmentID string, CaseID string, ConsentID string, ExpiresAt string, IdempotencyKey string}`, `NewJitOperatorAccessService(tx Tx, port JitOperatorAccessPort, evidence EvidenceSink) *JitOperatorAccessService`, and `(*JitOperatorAccessService).Execute(ctx context.Context, cmd JitOperatorAccessCommand) (JitOperatorAccessResult, error)`.
- Guarantees: idempotency key and `EnvironmentID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package jitoperatoraccess_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/operations/jit-operator-access"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.JitOperatorAccessCommand) (feature.JitOperatorAccessResult, error) {
    p.calls++
    return feature.JitOperatorAccessResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestJitOperatorAccessRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewJitOperatorAccessService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.JitOperatorAccessCommand{
        EnvironmentID: "",
        IdempotencyKey: "t79-guard",
    })

    if !errors.Is(err, feature.ErrEnvironmentRequired) {
        t.Fatalf("expected %v, got %v", feature.ErrEnvironmentRequired, err)
    }
    if port.calls != 0 {
        t.Fatalf("outbound port called %d times", port.calls)
    }
}
```

- [ ] **Step 2: Run the focused test and confirm the red state**

Run: `go test ./internal/operations/jit-operator-access -run TestJitOperatorAccessRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewJitOperatorAccessService`, `JitOperatorAccessCommand`, and `ErrEnvironmentRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package jitoperatoraccess

import (
    "context"
    "errors"
)

var (
    ErrEnvironmentRequired = errors.New("production-shaped environment is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type JitOperatorAccessCommand struct {
    EnvironmentID string
    CaseID string
    ConsentID string
    ExpiresAt string
    IdempotencyKey string
}

type JitOperatorAccessResult struct {
    ResourceID string
    Outcome    string
}

type JitOperatorAccessPort interface {
    Apply(context.Context, JitOperatorAccessCommand) (JitOperatorAccessResult, error)
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
type JitOperatorAccessService struct {
    tx       Tx
    port     JitOperatorAccessPort
    evidence EvidenceSink
}

func NewJitOperatorAccessService(tx Tx, port JitOperatorAccessPort, evidence EvidenceSink) *JitOperatorAccessService {
    return &JitOperatorAccessService{tx: tx, port: port, evidence: evidence}
}

func (s *JitOperatorAccessService) Execute(ctx context.Context, cmd JitOperatorAccessCommand) (result JitOperatorAccessResult, err error) {
    if cmd.EnvironmentID == "" {
        return JitOperatorAccessResult{}, ErrEnvironmentRequired
    }
    if cmd.IdempotencyKey == "" {
        return JitOperatorAccessResult{}, ErrIdempotencyKeyRequired
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

The concrete `JitOperatorAccessPort.Apply` implementation in this file must enforce the Ticket invariant: **Support 访问要求 case、Owner consent、MFA、JIT、到期和完整 Audit。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/operations/jit-operator-access -run 'JitOperatorAccess' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t79-jit-operator-access
issue: 80
batch: P14
seam: Production Gate evidence case
scope:
  environment_id: prod-shaped-a
idempotency_key: t79-acceptance
normal:
  expect: "Support 访问要求 case、Owner consent、MFA、JIT、到期和完整 Audit。"
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
package production-gates_test

import (
    "testing"

    "github.com/1123786563/myqypt/tests/platformtest"
)

func TestT79JitOperatorAccess(t *testing.T) {
    report := platformtest.Run(t, "tests/production-gates/scenarios/t79-jit-operator-access.yaml")
    if !report.Passed {
        t.Fatalf("T79 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/production-gates -run TestT79JitOperatorAccess -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t79/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/operations/jit-operator-access ./tests/production-gates -count=1`

Expected: PASS with no skipped T79 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/operations/jit-operator-access/service.go internal/operations/jit-operator-access/service_test.go tests/production-gates/scenarios/t79-jit-operator-access.yaml tests/production-gates/t79_jit_operator_access_test.go
git commit -m "feat(operations): deliver T79 jit-operator-access"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #80 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `JitOperatorAccessCommand`, `JitOperatorAccessResult`, `JitOperatorAccessPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
