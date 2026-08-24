# T88 Paid-launch Production Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 运行全部不可豁免 Gate，收集四方批准，输出可追溯的上线或阻断结论。

**Architecture:** Implement this Ticket as one vertical slice in `internal/gates` and prove it through the Production Gate evidence case under `tests/production-gates`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, Kubernetes, multi-AZ managed state services, evidence runner

**Spec:** [GitHub Issue #89](https://github.com/1123786563/myqypt/issues/89), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0007-bound-the-stage-1-operating-envelope.md`, `docs/adr/0044-make-p0-production-gates-non-waivable.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #81 — T80 Quarantine Kill Switch - #86 — T85 Complete Cell Disaster Recovery - #87 — T86 External Confirmation Evidence Dossiers - #88 — T87 Full Lighthouse Journey

---

## File Structure

- Create `internal/gates/paid-launch-gates/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/gates/paid-launch-gates/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/production-gates/scenarios/t88-paid-launch-gates.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/production-gates/t88_paid_launch_gates_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T88 as one testable vertical slice

**Files:**
- Create: `internal/gates/paid-launch-gates/service.go`
- Create: `internal/gates/paid-launch-gates/service_test.go`
- Create: `tests/production-gates/scenarios/t88-paid-launch-gates.yaml`
- Create: `tests/production-gates/t88_paid_launch_gates_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `PaidLaunchGatesCommand{ReleaseID string, EvidenceManifestDigest string, IdempotencyKey string}`, `NewPaidLaunchGatesService(tx Tx, port PaidLaunchGatesPort, evidence EvidenceSink) *PaidLaunchGatesService`, and `(*PaidLaunchGatesService).Execute(ctx context.Context, cmd PaidLaunchGatesCommand) (PaidLaunchGatesResult, error)`.
- Guarantees: idempotency key and `ReleaseID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package paidlaunchgates_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/gates/paid-launch-gates"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.PaidLaunchGatesCommand) (feature.PaidLaunchGatesResult, error) {
    p.calls++
    return feature.PaidLaunchGatesResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestPaidLaunchGatesRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewPaidLaunchGatesService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.PaidLaunchGatesCommand{
        ReleaseID: "",
        IdempotencyKey: "t88-guard",
    })

    if !errors.Is(err, feature.ErrReleaseRequired) {
        t.Fatalf("expected %v, got %v", feature.ErrReleaseRequired, err)
    }
    if port.calls != 0 {
        t.Fatalf("outbound port called %d times", port.calls)
    }
}
```

- [ ] **Step 2: Run the focused test and confirm the red state**

Run: `go test ./internal/gates/paid-launch-gates -run TestPaidLaunchGatesRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewPaidLaunchGatesService`, `PaidLaunchGatesCommand`, and `ErrReleaseRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package paidlaunchgates

import (
    "context"
    "errors"
)

var (
    ErrReleaseRequired = errors.New("release is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type PaidLaunchGatesCommand struct {
    ReleaseID string
    EvidenceManifestDigest string
    IdempotencyKey string
}

type PaidLaunchGatesResult struct {
    ResourceID string
    Outcome    string
}

type PaidLaunchGatesPort interface {
    Apply(context.Context, PaidLaunchGatesCommand) (PaidLaunchGatesResult, error)
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
type PaidLaunchGatesService struct {
    tx       Tx
    port     PaidLaunchGatesPort
    evidence EvidenceSink
}

func NewPaidLaunchGatesService(tx Tx, port PaidLaunchGatesPort, evidence EvidenceSink) *PaidLaunchGatesService {
    return &PaidLaunchGatesService{tx: tx, port: port, evidence: evidence}
}

func (s *PaidLaunchGatesService) Execute(ctx context.Context, cmd PaidLaunchGatesCommand) (result PaidLaunchGatesResult, err error) {
    if cmd.ReleaseID == "" {
        return PaidLaunchGatesResult{}, ErrReleaseRequired
    }
    if cmd.IdempotencyKey == "" {
        return PaidLaunchGatesResult{}, ErrIdempotencyKeyRequired
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

The concrete `PaidLaunchGatesPort.Apply` implementation in this file must enforce the Ticket invariant: **运行全部不可豁免 Gate，收集四方批准，输出可追溯的上线或阻断结论。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/gates/paid-launch-gates -run 'PaidLaunchGates' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t88-paid-launch-gates
issue: 89
batch: P26
seam: Production Gate evidence case
scope:
  release_id: stage1-release-a
idempotency_key: t88-acceptance
normal:
  expect: "运行全部不可豁免 Gate，收集四方批准，输出可追溯的上线或阻断结论。"
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

func TestT88PaidLaunchGates(t *testing.T) {
    report := platformtest.Run(t, "tests/production-gates/scenarios/t88-paid-launch-gates.yaml")
    if !report.Passed {
        t.Fatalf("T88 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/production-gates -run TestT88PaidLaunchGates -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t88/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/gates/paid-launch-gates ./tests/production-gates -count=1`

Expected: PASS with no skipped T88 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/gates/paid-launch-gates/service.go internal/gates/paid-launch-gates/service_test.go tests/production-gates/scenarios/t88-paid-launch-gates.yaml tests/production-gates/t88_paid_launch_gates_test.go
git commit -m "feat(gates): deliver T88 paid-launch-gates"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #89 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `PaidLaunchGatesCommand`, `PaidLaunchGatesResult`, `PaidLaunchGatesPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
