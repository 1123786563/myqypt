# [T86.9] WeKnora Shared Security Dossier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 汇总 WeKnora Shared TenantScope、向量、任务、缓存、Upgrade、Export 与 Erasure 的攻击和恢复证据。

**Architecture:** Treat this Issue as an accountable evidence product, not as an engineering opinion. Primary-source evidence, scope, effective dates, contradictions, expiry/renewal triggers, paid-launch consequence, and the reviewer decision are stored separately so a later Production Gate can verify the dossier without copying sensitive source material.

**Tech Stack:** Markdown, YAML, Go evidence-schema tests, GitHub Issue approval record

**Spec:** [GitHub Issue #98](https://github.com/1123786563/myqypt/issues/98), `docs/architecture/external-confirmations.md`, `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `docs/adr/0008-require-weknora-shared-tenancy-hardening-before-paid-launch.md`, `docs/adr/0014-maintain-weknora-hardening-as-an-upstream-first-patch-queue.md`, `docs/adr/0037-verify-complete-cell-recovery-not-backup-presence.md`

## Global Constraints

- Use current primary sources or an accountable licensed professional; record retrieval date, version/effective date, jurisdiction, and exact scope.
- Do not infer legal, tax, Provider, licensing, or cloud capability conclusions from architecture preference.
- Unknown, contradictory, expired, or unapproved evidence produces `blocked`, never an implicit approval.
- Evidence files contain no Secret, raw payment payload, Prompt, document body, or sensitive personal information.
- An accountable reviewer must record identity, timestamp, rationale, and the SHA-256 digest of the reviewed source manifest.
- This Issue is `ready-for-human`; an agent may prepare and validate the dossier but cannot invent the approval.

---

## File Structure

- Create `docs/evidence/dossiers/weknora-shared-security/README.md` for the bounded question, scope, findings, contradictions, expiry, and paid-launch consequence.
- Create `docs/evidence/dossiers/weknora-shared-security/sources.yaml` for machine-verifiable source metadata and evidence digests.
- Create `docs/evidence/dossiers/weknora-shared-security/decision.yaml` for the accountable approve/block decision and renewal trigger.
- Create `tests/evidence/weknora_shared_security_test.go` for schema, freshness, digest, and approval-state validation.

### Task 1: Produce one reviewable WeKnora Shared Security Dossier dossier

**Files:**
- Create: `docs/evidence/dossiers/weknora-shared-security/README.md`
- Create: `docs/evidence/dossiers/weknora-shared-security/sources.yaml`
- Create: `docs/evidence/dossiers/weknora-shared-security/decision.yaml`
- Create: `tests/evidence/weknora_shared_security_test.go`

**Interfaces:**
- Consumes: the exact external-confirmation question in Issue #98 and the relevant ADRs above.
- Produces: `sources.yaml` with `source_id`, `authority`, `url`, `retrieved_at`, `effective_at`, `jurisdiction`, `scope`, and `sha256`; `decision.yaml` with `status`, `reviewer`, `reviewed_at`, `rationale`, `source_manifest_sha256`, `expires_at`, and `renewal_trigger`.

- [ ] **Step 1: Write the failing evidence contract test**

```go
package evidence_test

import (
    "os"
    "testing"

    "gopkg.in/yaml.v3"
)

type decision struct {
    Status               string `yaml:"status"`
    Reviewer             string `yaml:"reviewer"`
    ReviewedAt           string `yaml:"reviewed_at"`
    Rationale            string `yaml:"rationale"`
    SourceManifestSHA256 string `yaml:"source_manifest_sha256"`
    RenewalTrigger       string `yaml:"renewal_trigger"`
}

func TestWeknoraSharedSecurityDossierIsApproved(t *testing.T) {
    raw, err := os.ReadFile("../../docs/evidence/dossiers/weknora-shared-security/decision.yaml")
    if err != nil {
        t.Fatal(err)
    }
    var got decision
    if err := yaml.Unmarshal(raw, &got); err != nil {
        t.Fatal(err)
    }
    if got.Status != "approved" || got.Reviewer == "" || got.ReviewedAt == "" || got.Rationale == "" || got.SourceManifestSHA256 == "" || got.RenewalTrigger == "" {
        t.Fatalf("dossier is not accountable and approved: %+v", got)
    }
}
```

- [ ] **Step 2: Run the dossier test and confirm the red state**

Run: `go test ./tests/evidence -run TestWeknoraSharedSecurityDossierIsApproved -count=1`

Expected: FAIL because `docs/evidence/dossiers/weknora-shared-security/decision.yaml` does not exist or remains `blocked_pending_review`.

- [ ] **Step 3: Create the source manifest before collecting conclusions**

```yaml
dossier_id: weknora-shared-security
issue: 98
generated_at: 2026-08-24T00:00:00+08:00
sources: []
required_source_fields:
  - source_id
  - authority
  - url
  - retrieved_at
  - effective_at
  - jurisdiction
  - scope
  - sha256
```

Initialize `docs/evidence/dossiers/weknora-shared-security/decision.yaml` with `status: blocked_pending_review`; this is a real blocking state, not an approval placeholder.

- [ ] **Step 4: Collect and cross-check the bounded evidence**

Record evidence sufficient to decide: **汇总 WeKnora Shared TenantScope、向量、任务、缓存、Upgrade、Export 与 Erasure 的攻击和恢复证据。** For every claim in `README.md`, cite a `source_id`; record contradictions and unanswered questions as explicit blockers with owner and next review date. Store references and digests rather than confidential source payloads.

- [ ] **Step 5: Obtain the accountable human decision**

The responsible reviewer changes `status` to `approved` or `blocked`, records their real identity and timestamp, explains the decision, records the SHA-256 digest of `sources.yaml`, and sets an objective renewal trigger such as Provider contract change, Product Version change, regulation effective date, or 90-day freshness review.

- [ ] **Step 6: Run evidence validation**

Run: `go test ./tests/evidence -run TestWeknoraSharedSecurityDossierIsApproved -count=1`

Expected: PASS only for a complete `approved` dossier; a missing reviewer, stale/unknown source, digest mismatch, or `blocked` status must fail.

- [ ] **Step 7: Attach approval evidence and commit**

```bash
git add docs/evidence/dossiers/weknora-shared-security/README.md docs/evidence/dossiers/weknora-shared-security/sources.yaml docs/evidence/dossiers/weknora-shared-security/decision.yaml tests/evidence/weknora_shared_security_test.go
git commit -m "docs(evidence): add weknora-shared-security dossier"
```

Add the source-manifest digest, validation command, result, and decision status to Issue #98; do not paste confidential source content.

## Self-Review Record

- Spec coverage: source authority, freshness, jurisdiction, contradictions, expiry, paid-launch consequence, content minimization, and accountable approval are explicit.
- Placeholder scan: `blocked_pending_review` is a fail-closed workflow state; no incomplete dossier can pass as approved.
- Type consistency: the YAML keys match the Go evidence contract exactly.
- Right-sizing: this dossier has one accountable review boundary and may run in parallel with the other T86 dossier children.
