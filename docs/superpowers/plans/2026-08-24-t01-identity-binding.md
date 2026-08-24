# T01 User 注册与 Identity Binding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 组合 #100 的最小 Platform/测试 harness 与 #101 的 Keycloak Identity Binding，证明一次真实注册只创建一个稳定 Platform User 绑定。

**Architecture:** Issue #2 is an aggregator over two serial native sub-Issues. The parent adds no second identity implementation; it runs the black-box acceptance path against the composed Docker Compose stack and records the evidence needed by every downstream Ticket.

**Tech Stack:** Go 1.26, PostgreSQL, Keycloak OIDC, Docker Compose, black-box HTTP acceptance harness

**Spec:** [GitHub Issue #2](https://github.com/1123786563/myqypt/issues/2), [Issue #100](https://github.com/1123786563/myqypt/issues/100), [Issue #101](https://github.com/1123786563/myqypt/issues/101), `docs/adr/0024-separate-platform-users-from-keycloak-identities.md`

## Global Constraints

- Keycloak owns credentials and stable OIDC subject; Platform PostgreSQL owns User and Identity Binding.
- Identity key is exactly `identity_provider + subject`; email, phone, and username are profile attributes only.
- Duplicate callback or retry returns the same Platform User and does not create a second binding.
- Evidence contains stable test identifiers and dependency versions, never credentials, tokens, or personal profile values.

---

### Task 1: Run the composed T01 black-box acceptance

**Files:**
- Create: `tests/acceptance/scenarios/t01-identity-binding.yaml`
- Create: `tests/acceptance/t01_identity_binding_test.go`

**Interfaces:**
- Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report` from #100 and `POST /internal/v1/identity/callback` from #101.
- Produces: versioned T01 evidence proving unique issuer/subject binding, rejection of unverified claims, and duplicate-delivery idempotency.

- [ ] **Step 1: Write the failing acceptance scenario**

```yaml
id: t01-identity-binding
seam: lighthouse-black-box
request:
  method: POST
  path: /internal/v1/identity/callback
  verified_oidc_claims:
    issuer: http://keycloak:8080/realms/myqypt
    subject: subject-t01
expect:
  status: 201
  identity_key: http://keycloak:8080/realms/myqypt|subject-t01
  binding_count: 1
replay:
  deliveries: 2
  binding_count: 1
denial:
  remove_verified_claims: true
  status: 401
  binding_count: 0
```

- [ ] **Step 2: Confirm red before both children are complete**

Run: `go test ./tests/acceptance -run TestT01IdentityBinding -count=1`

Expected: FAIL until the #100 stack/harness and #101 callback are both available.

- [ ] **Step 3: Add the black-box test**

```go
func TestT01IdentityBinding(t *testing.T) {
    report := platformtest.Run(t, "scenarios/t01-identity-binding.yaml")
    if !report.Passed {
        t.Fatalf("T01 failed: %s", report.Summary)
    }
}
```

- [ ] **Step 4: Run focused and real integration evidence separately**

Run focused: `go test ./internal/identity/... -count=1`

Run integration: `docker compose -f deploy/compose/compose.yaml up -d --wait && go test ./tests/acceptance -run TestT01IdentityBinding -count=1`

Expected: both PASS; report the two results separately and attach the integration evidence digest to Issue #2.

- [ ] **Step 5: Commit the parent acceptance slice**

```bash
git add tests/acceptance/scenarios/t01-identity-binding.yaml tests/acceptance/t01_identity_binding_test.go
git commit -m "test(acceptance): prove T01 identity binding journey"
```

## Self-Review Record

- Spec coverage: scaffold, verified subject, immutable identity key, denial, replay, and real integration evidence are explicit.
- Placeholder scan: every step has an exact file, command, and expected result.
- Type consistency: the scenario and #101 endpoint use the same issuer/subject identity key.
- Right-sizing: #100 and #101 are independent review gates; #2 performs only their composed acceptance.
