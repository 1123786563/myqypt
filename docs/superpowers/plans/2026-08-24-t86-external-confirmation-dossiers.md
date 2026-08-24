# T86 External Confirmation Evidence Dossiers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 汇总十个独立、可审批的外部确认 Dossier；任一缺失、过期或被阻断时，父 Gate 必须 Fail Closed。

**Architecture:** Issue #87 is an aggregator over ten native sub-Issues, not the place to repeat their research. A machine-readable manifest points at each child decision, and one Production Gate validates completeness, approval, freshness, and source-manifest digest before emitting the parent evidence result.

**Tech Stack:** YAML evidence manifest, Go Production Gate runner, GitHub native sub-Issues

**Spec:** [GitHub Issue #87](https://github.com/1123786563/myqypt/issues/87), `docs/architecture/external-confirmations.md`, child Issues [#90 privacy-data-governance](https://github.com/1123786563/myqypt/issues/90), [#91 tax-electronic-invoice](https://github.com/1123786563/myqypt/issues/91), [#92 wechat-alipay-merchant](https://github.com/1123786563/myqypt/issues/92), [#93 product-version-license](https://github.com/1123786563/myqypt/issues/93), [#94 model-provider-terms](https://github.com/1123786563/myqypt/issues/94), [#95 mainland-cloud-capabilities](https://github.com/1123786563/myqypt/issues/95), [#96 nacos-production-baseline](https://github.com/1123786563/myqypt/issues/96), [#97 valkey-openmeter-compatibility](https://github.com/1123786563/myqypt/issues/97), [#98 weknora-shared-security](https://github.com/1123786563/myqypt/issues/98), [#99 openmeter-commerce-chain](https://github.com/1123786563/myqypt/issues/99)

## Global Constraints

- All ten child Dossiers must be `approved`; missing, `blocked`, stale, expired, or digest-mismatched evidence blocks paid launch.
- Agents may validate evidence but may not invent the accountable human approval.
- The aggregate report contains digests and references only, never Secret, raw payment payload, Prompt, document body, or sensitive personal information.
- Production Gate output is immutable and versioned by source revision and Dossier manifest digest.

---

### Task 1: Aggregate and validate all T86 Dossiers

**Files:**
- Create: `docs/evidence/dossiers/stage1/manifest.yaml`
- Create: `tests/production-gates/t86_external_dossiers_test.go`
- Create: `runbooks/external-dossier-renewal.md`

**Interfaces:**
- Consumes: child decision files from Issues #90-#99.
- Produces: `ValidateDossierSet(path string, now time.Time) (Report, error)` and a versioned T86 evidence report.

- [ ] **Step 1: Write a failing table test that requires every child approval**

```go
func TestT86ExternalDossiers(t *testing.T) {
    report, err := gates.ValidateDossierSet("../../docs/evidence/dossiers/stage1/manifest.yaml", time.Now())
    if err != nil {
        t.Fatal(err)
    }
    if report.Approved != 10 || len(report.Blockers) != 0 {
        t.Fatalf("approved=%d blockers=%v", report.Approved, report.Blockers)
    }
}
```

- [ ] **Step 2: Run the test and confirm it fails before child approvals exist**

Run: `go test ./tests/production-gates -run TestT86ExternalDossiers -count=1`

Expected: FAIL with each missing, unapproved, expired, or digest-mismatched child identified by Issue number.

- [ ] **Step 3: Create the exact aggregate manifest**

```yaml
dossier_set: stage1-paid-launch
required_count: 10
dossiers:
      - issue: 90
        dossier: privacy-data-governance
        decision: docs/evidence/dossiers/privacy-data-governance/decision.yaml
      - issue: 91
        dossier: tax-electronic-invoice
        decision: docs/evidence/dossiers/tax-electronic-invoice/decision.yaml
      - issue: 92
        dossier: wechat-alipay-merchant
        decision: docs/evidence/dossiers/wechat-alipay-merchant/decision.yaml
      - issue: 93
        dossier: product-version-license
        decision: docs/evidence/dossiers/product-version-license/decision.yaml
      - issue: 94
        dossier: model-provider-terms
        decision: docs/evidence/dossiers/model-provider-terms/decision.yaml
      - issue: 95
        dossier: mainland-cloud-capabilities
        decision: docs/evidence/dossiers/mainland-cloud-capabilities/decision.yaml
      - issue: 96
        dossier: nacos-production-baseline
        decision: docs/evidence/dossiers/nacos-production-baseline/decision.yaml
      - issue: 97
        dossier: valkey-openmeter-compatibility
        decision: docs/evidence/dossiers/valkey-openmeter-compatibility/decision.yaml
      - issue: 98
        dossier: weknora-shared-security
        decision: docs/evidence/dossiers/weknora-shared-security/decision.yaml
      - issue: 99
        dossier: openmeter-commerce-chain
        decision: docs/evidence/dossiers/openmeter-commerce-chain/decision.yaml
policy:
  required_status: approved
  require_reviewer: true
  require_source_manifest_sha256: true
  fail_on_expiry: true
  fail_on_missing: true
```

- [ ] **Step 4: Implement deterministic validation and renewal reporting**

```go
func ValidateDossierSet(path string, now time.Time) (Report, error) {
    manifest, err := loadManifest(path)
    if err != nil {
        return Report{}, err
    }
    report := Report{Required: manifest.RequiredCount}
    for _, item := range manifest.Dossiers {
        decision, readErr := loadDecision(item.Decision)
        if readErr != nil || decision.Status != "approved" || decision.Reviewer == "" || decision.SourceManifestSHA256 == "" || decision.Expired(now) {
            report.Blockers = append(report.Blockers, item.Issue)
            continue
        }
        report.Approved++
    }
    sort.Ints(report.Blockers)
    return report, nil
}
```

`loadManifest`, `loadDecision`, and `Decision.Expired` must reject unknown YAML fields, malformed timestamps, absent renewal triggers, and source-manifest digest mismatches. Never downgrade a blocker to a warning.

- [ ] **Step 5: Run the aggregate Gate after Issues #90-#99 are approved**

Run: `go test ./tests/production-gates -run TestT86ExternalDossiers -count=1`

Expected: PASS with exactly 10 approved Dossiers and zero blockers; attach the aggregate manifest digest and report to Issue #87.

- [ ] **Step 6: Commit the aggregator**

```bash
git add docs/evidence/dossiers/stage1/manifest.yaml tests/production-gates/t86_external_dossiers_test.go runbooks/external-dossier-renewal.md
git commit -m "test(gates): aggregate T86 external dossiers"
```

## Self-Review Record

- Spec coverage: all ten categories named by T86 map one-to-one to Issues #90-#99.
- Placeholder scan: missing or pending decisions are explicit blockers and cannot pass.
- Type consistency: the aggregate manifest and child `decision.yaml` keys match the validation interface.
- Right-sizing: child research/approval runs in parallel; the parent performs only deterministic aggregation.
