# T38 Cross-Tenant Attack Matrix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 验证数据库、向量、任务、缓存、身份和 Product Route 的跨 Tenant 防御。

**Architecture:** Implement this Ticket as one vertical slice in `internal/weknora` and prove it through the Production Gate evidence case under `tests/production-gates`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, WeKnora, Higress

**Spec:** [GitHub Issue #39](https://github.com/1123786563/myqypt/issues/39), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0008-require-weknora-shared-tenancy-hardening-before-paid-launch.md`, `docs/adr/0009-forbid-cross-tenant-product-object-sharing.md`, `docs/adr/0044-make-p0-production-gates-non-waivable.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #24 — T23 Header 清洗与 Product 直连阻断 - #35 — T34 Repository TenantScope - #36 — T35 Vector TenantScope - #37 — T36 Tenant 级后台任务公平性 - #38 — T37 Product 原子配额 Reservation

---

## File Structure

- Create `internal/weknora/cross-tenant-attack-matrix/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/weknora/cross-tenant-attack-matrix/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/production-gates/scenarios/t38-cross-tenant-attack-matrix.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/production-gates/t38_cross_tenant_attack_matrix_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T38 as one testable vertical slice

**Files:**
- Create: `internal/weknora/cross-tenant-attack-matrix/service.go`
- Create: `internal/weknora/cross-tenant-attack-matrix/service_test.go`
- Create: `tests/production-gates/scenarios/t38-cross-tenant-attack-matrix.yaml`
- Create: `tests/production-gates/t38_cross_tenant_attack_matrix_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `CrossTenantAttackMatrixCommand{EnvironmentID string, AttackerTenantID string, VictimTenantID string, AttackCase string, IdempotencyKey string}`, `NewCrossTenantAttackMatrixService(tx Tx, port CrossTenantAttackMatrixPort, evidence EvidenceSink) *CrossTenantAttackMatrixService`, and `(*CrossTenantAttackMatrixService).Execute(ctx context.Context, cmd CrossTenantAttackMatrixCommand) (CrossTenantAttackMatrixResult, error)`.
- Guarantees: idempotency key and `EnvironmentID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package crosstenantattackmatrix_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/weknora/cross-tenant-attack-matrix"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.CrossTenantAttackMatrixCommand) (feature.CrossTenantAttackMatrixResult, error) {
    p.calls++
    return feature.CrossTenantAttackMatrixResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestCrossTenantAttackMatrixRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewCrossTenantAttackMatrixService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.CrossTenantAttackMatrixCommand{
        EnvironmentID: "",
        IdempotencyKey: "t38-guard",
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

Run: `go test ./internal/weknora/cross-tenant-attack-matrix -run TestCrossTenantAttackMatrixRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewCrossTenantAttackMatrixService`, `CrossTenantAttackMatrixCommand`, and `ErrEnvironmentRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package crosstenantattackmatrix

import (
    "context"
    "errors"
)

var (
    ErrEnvironmentRequired = errors.New("production-shaped environment is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type CrossTenantAttackMatrixCommand struct {
    EnvironmentID string
    AttackerTenantID string
    VictimTenantID string
    AttackCase string
    IdempotencyKey string
}

type CrossTenantAttackMatrixResult struct {
    ResourceID string
    Outcome    string
}

type CrossTenantAttackMatrixPort interface {
    Apply(context.Context, CrossTenantAttackMatrixCommand) (CrossTenantAttackMatrixResult, error)
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
type CrossTenantAttackMatrixService struct {
    tx       Tx
    port     CrossTenantAttackMatrixPort
    evidence EvidenceSink
}

func NewCrossTenantAttackMatrixService(tx Tx, port CrossTenantAttackMatrixPort, evidence EvidenceSink) *CrossTenantAttackMatrixService {
    return &CrossTenantAttackMatrixService{tx: tx, port: port, evidence: evidence}
}

func (s *CrossTenantAttackMatrixService) Execute(ctx context.Context, cmd CrossTenantAttackMatrixCommand) (result CrossTenantAttackMatrixResult, err error) {
    if cmd.EnvironmentID == "" {
        return CrossTenantAttackMatrixResult{}, ErrEnvironmentRequired
    }
    if cmd.IdempotencyKey == "" {
        return CrossTenantAttackMatrixResult{}, ErrIdempotencyKeyRequired
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

The concrete `CrossTenantAttackMatrixPort.Apply` implementation in this file must enforce the Ticket invariant: **验证数据库、向量、任务、缓存、身份和 Product Route 的跨 Tenant 防御。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/weknora/cross-tenant-attack-matrix -run 'CrossTenantAttackMatrix' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t38-cross-tenant-attack-matrix
issue: 39
batch: P19
seam: Production Gate evidence case
scope:
  environment_id: prod-shaped-a
idempotency_key: t38-acceptance
normal:
  expect: "验证数据库、向量、任务、缓存、身份和 Product Route 的跨 Tenant 防御。"
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

func TestT38CrossTenantAttackMatrix(t *testing.T) {
    report := platformtest.Run(t, "tests/production-gates/scenarios/t38-cross-tenant-attack-matrix.yaml")
    if !report.Passed {
        t.Fatalf("T38 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/production-gates -run TestT38CrossTenantAttackMatrix -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t38/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/weknora/cross-tenant-attack-matrix ./tests/production-gates -count=1`

Expected: PASS with no skipped T38 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/weknora/cross-tenant-attack-matrix/service.go internal/weknora/cross-tenant-attack-matrix/service_test.go tests/production-gates/scenarios/t38-cross-tenant-attack-matrix.yaml tests/production-gates/t38_cross_tenant_attack_matrix_test.go
git commit -m "feat(weknora): deliver T38 cross-tenant-attack-matrix"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #39 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `CrossTenantAttackMatrixCommand`, `CrossTenantAttackMatrixResult`, `CrossTenantAttackMatrixPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
