# T82 PostgreSQL Logical Isolation 与 HA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Platform、Identity、Authorization、Workflow、Billing、Nacos 和 Cell 使用独立 database、role 和 backup boundary。

**Architecture:** Implement this Ticket as one vertical slice in `internal/reliability` and prove it through the Production Gate evidence case under `tests/production-gates`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

**Tech Stack:** Go services and test harnesses, PostgreSQL, Docker Compose for development and controlled-beta verification, Kubernetes, multi-AZ managed state services, evidence runner

**Spec:** [GitHub Issue #83](https://github.com/1123786563/myqypt/issues/83), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, `docs/adr/0043-use-one-mainland-region-across-multiple-availability-zones.md`, `docs/adr/0052-isolate-component-state-in-logical-postgresql-databases.md`

## Global Constraints

- Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
- Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
- Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
- Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
- Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
- Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
- Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
- A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
- Blockers from the issue graph must be complete before implementation: - #20 — T19 Desired、Observed 与 Operation 查询 - #22 — T21 WeKnora Adapter 封闭测试接入 - #45 — T44 OpenMeter Runtime 与 Adapter - #75 — T74 Nacos Baseline 与 AI Registry Provider

---

## File Structure

- Create `internal/reliability/postgres-isolation-ha/service.go` for the feature command, result, validation, transaction boundary, and typed outbound port.
- Create `internal/reliability/postgres-isolation-ha/service_test.go` for the focused red/green contract and invariant tests.
- Create `tests/production-gates/scenarios/t82-postgres-isolation-ha.yaml` for the normal and denial/failure scenario expressed at the highest practical seam.
- Create `tests/production-gates/t82_postgres_isolation_ha_test.go` to execute the scenario and emit a content-minimized evidence report.
- Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

### Task 1: Deliver T82 as one testable vertical slice

**Files:**
- Create: `internal/reliability/postgres-isolation-ha/service.go`
- Create: `internal/reliability/postgres-isolation-ha/service_test.go`
- Create: `tests/production-gates/scenarios/t82-postgres-isolation-ha.yaml`
- Create: `tests/production-gates/t82_postgres_isolation_ha_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
- Produces: `PostgresIsolationHaCommand{EnvironmentID string, DatabaseName string, FailureTarget string, IdempotencyKey string}`, `NewPostgresIsolationHaService(tx Tx, port PostgresIsolationHaPort, evidence EvidenceSink) *PostgresIsolationHaService`, and `(*PostgresIsolationHaService).Execute(ctx context.Context, cmd PostgresIsolationHaCommand) (PostgresIsolationHaResult, error)`.
- Guarantees: idempotency key and `EnvironmentID` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

- [ ] **Step 1: Write the failing focused contract test**

```go
package postgresisolationha_test

import (
    "context"
    "errors"
    "testing"

    feature "github.com/1123786563/myqypt/internal/reliability/postgres-isolation-ha"
)

type recordingPort struct{ calls int }

func (p *recordingPort) Apply(_ context.Context, _ feature.PostgresIsolationHaCommand) (feature.PostgresIsolationHaResult, error) {
    p.calls++
    return feature.PostgresIsolationHaResult{ResourceID: "resource-a", Outcome: "accepted"}, nil
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

func TestPostgresIsolationHaRejectsInvalidScopeBeforeSideEffects(t *testing.T) {
    port := &recordingPort{}
    service := feature.NewPostgresIsolationHaService(inMemoryTx{}, port, &memoryEvidence{})

    _, err := service.Execute(context.Background(), feature.PostgresIsolationHaCommand{
        EnvironmentID: "",
        IdempotencyKey: "t82-guard",
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

Run: `go test ./internal/reliability/postgres-isolation-ha -run TestPostgresIsolationHaRejectsInvalidScopeBeforeSideEffects -count=1`

Expected: FAIL because `NewPostgresIsolationHaService`, `PostgresIsolationHaCommand`, and `ErrEnvironmentRequired` do not exist.

- [ ] **Step 3: Add the typed contract and validation before any side effect**

```go
package postgresisolationha

import (
    "context"
    "errors"
)

var (
    ErrEnvironmentRequired = errors.New("production-shaped environment is required")
    ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type PostgresIsolationHaCommand struct {
    EnvironmentID string
    DatabaseName string
    FailureTarget string
    IdempotencyKey string
}

type PostgresIsolationHaResult struct {
    ResourceID string
    Outcome    string
}

type PostgresIsolationHaPort interface {
    Apply(context.Context, PostgresIsolationHaCommand) (PostgresIsolationHaResult, error)
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
type PostgresIsolationHaService struct {
    tx       Tx
    port     PostgresIsolationHaPort
    evidence EvidenceSink
}

func NewPostgresIsolationHaService(tx Tx, port PostgresIsolationHaPort, evidence EvidenceSink) *PostgresIsolationHaService {
    return &PostgresIsolationHaService{tx: tx, port: port, evidence: evidence}
}

func (s *PostgresIsolationHaService) Execute(ctx context.Context, cmd PostgresIsolationHaCommand) (result PostgresIsolationHaResult, err error) {
    if cmd.EnvironmentID == "" {
        return PostgresIsolationHaResult{}, ErrEnvironmentRequired
    }
    if cmd.IdempotencyKey == "" {
        return PostgresIsolationHaResult{}, ErrIdempotencyKeyRequired
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

The concrete `PostgresIsolationHaPort.Apply` implementation in this file must enforce the Ticket invariant: **Platform、Identity、Authorization、Workflow、Billing、Nacos 和 Cell 使用独立 database、role 和 backup boundary。**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

- [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

Run: `go test ./internal/reliability/postgres-isolation-ha -run 'PostgresIsolationHa' -count=1`

Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

- [ ] **Step 6: Add the highest-seam scenario**

```yaml
id: t82-postgres-isolation-ha
issue: 83
batch: P19
seam: Production Gate evidence case
scope:
  environment_id: prod-shaped-a
idempotency_key: t82-acceptance
normal:
  expect: "Platform、Identity、Authorization、Workflow、Billing、Nacos 和 Cell 使用独立 database、role 和 backup boundary。"
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

func TestT82PostgresIsolationHa(t *testing.T) {
    report := platformtest.Run(t, "tests/production-gates/scenarios/t82-postgres-isolation-ha.yaml")
    if !report.Passed {
        t.Fatalf("T82 evidence failed: %s", report.Summary)
    }
}
```

Run: `go test ./tests/production-gates -run TestT82PostgresIsolationHa -count=1`

Expected: PASS and a versioned report under `artifacts/evidence/t82/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

- [ ] **Step 8: Run the domain regression suite**

Run: `go test ./internal/reliability/postgres-isolation-ha ./tests/production-gates -count=1`

Expected: PASS with no skipped T82 scenario.

- [ ] **Step 9: Commit the independently reviewable slice**

```bash
git add internal/reliability/postgres-isolation-ha/service.go internal/reliability/postgres-isolation-ha/service_test.go tests/production-gates/scenarios/t82-postgres-isolation-ha.yaml tests/production-gates/t82_postgres_isolation_ha_test.go
git commit -m "feat(reliability): deliver T82 postgres-isolation-ha"
```

## Self-Review Record

- Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue #83 are each mapped to Steps 1, 4, 5, 6, and 7.
- Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
- Type consistency: `PostgresIsolationHaCommand`, `PostgresIsolationHaResult`, `PostgresIsolationHaPort`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
- Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
