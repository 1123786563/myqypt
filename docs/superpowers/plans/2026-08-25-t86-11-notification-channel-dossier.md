# [T86.11] Notification Channel 与 ADR-0056 转正 Dossier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 由可承担责任的法务/运维审核者确认出站通知通道事实（大陆短信实名制与模板审核、邮件可达性、防刷边界、验证码合规），并据此裁决 ADR-0056（通知网关）转正或阻断。

**Architecture:** Treat this Issue as an accountable evidence product, not as an engineering opinion. Primary-source evidence, scope, effective dates, contradictions, expiry/renewal triggers, paid-launch consequence, and the reviewer decision are stored separately so a later Production Gate can verify the dossier without copying sensitive source material. The dossier doubles as the ADR-0056 acceptance record: its decision.yaml names the ADR and the resulting status flip.

**Tech Stack:** Markdown, YAML, Go evidence-schema tests, GitHub Issue approval record

**Spec:** [GitHub Issue #131](https://github.com/1123786563/myqypt/issues/131), `docs/architecture/external-confirmations.md`（Notification channel 行）, `docs/adr/0056-use-a-notification-gateway-for-outbound-communication.md`, `docs/adr/0015-run-kafka-and-clickhouse-as-openmeter-core-dependencies.md`

## Global Constraints

- Use current primary sources or an accountable licensed professional; record retrieval date, version/effective date, jurisdiction, and exact scope.
- Do not infer legal, provider, or deliverability conclusions from architecture preference.
- Unknown, contradictory, expired, or unapproved evidence produces `blocked`, never an implicit approval; ADR-0056 stays `proposed`.
- Evidence files contain no Secret, raw payment payload, Prompt, document body, or sensitive personal information.
- An accountable reviewer must record identity, timestamp, rationale, and the SHA-256 digest of the reviewed source manifest.
- This Issue is `ready-for-human`; an agent may prepare and validate the dossier but cannot invent the approval.

---

## File Structure

- Create `docs/evidence/dossiers/notification-channel/README.md` for the bounded question, scope, findings, contradictions, expiry, and paid-launch consequence.
- Create `docs/evidence/dossiers/notification-channel/sources.yaml` for machine-verifiable source metadata and evidence digests.
- Create `docs/evidence/dossiers/notification-channel/decision.yaml` for the accountable approve/block decision, the ADR-0056 status outcome, and the renewal trigger.
- Create `tests/evidence/notification_channel_test.go` for schema, freshness, digest, and approval-state validation.

### Task 1: Produce one reviewable Notification Channel 与 ADR-0056 转正 dossier

**Files:**
- Create: `docs/evidence/dossiers/notification-channel/README.md`
- Create: `docs/evidence/dossiers/notification-channel/sources.yaml`
- Create: `docs/evidence/dossiers/notification-channel/decision.yaml`
- Create: `tests/evidence/notification_channel_test.go`

**Interfaces:**
- Consumes: the external-confirmation question (Notification channel row) and ADR-0056/ADR-0015/ADR-0041/ADR-0048/ADR-0054.
- Produces: `sources.yaml` with `source_id`, `authority`, `url`, `retrieved_at`, `effective_at`, `jurisdiction`, `scope`, and `sha256`; `decision.yaml` with `status`, `reviewer`, `reviewed_at`, `rationale`, `source_manifest_sha256`, `expires_at`, `renewal_trigger`, and `adr_0056_outcome`（accepted | blocked）.

- [ ] **Step 1: Write the failing evidence contract test**

```go
package evidence_test

import (
    "os"
    "testing"

    "gopkg.in/yaml.v3"
)

type notificationDecision struct {
    Status              string `yaml:"status"`
    Reviewer            string `yaml:"reviewer"`
    ReviewedAt          string `yaml:"reviewed_at"`
    Rationale           string `yaml:"rationale"`
    SourceManifestSHA256 string `yaml:"source_manifest_sha256"`
    ExpiresAt           string `yaml:"expires_at"`
    RenewalTrigger      string `yaml:"renewal_trigger"`
    Adr0056Outcome      string `yaml:"adr_0056_outcome"`
}

func TestNotificationChannelDecisionSchema(t *testing.T) {
    raw, err := os.ReadFile("../../docs/evidence/dossiers/notification-channel/decision.yaml")
    if err != nil {
        t.Fatal(err)
    }
    var d notificationDecision
    if err := yaml.Unmarshal(raw, &d); err != nil {
        t.Fatal(err)
    }
    if d.Status != "approved" && d.Status != "blocked" {
        t.Fatalf("status=%s", d.Status)
    }
    if d.Reviewer == "" || d.Rationale == "" || d.SourceManifestSHA256 == "" {
        t.Fatal("reviewer, rationale and manifest digest are mandatory")
    }
    if d.Adr0056Outcome != "accepted" && d.Adr0056Outcome != "blocked" {
        t.Fatalf("adr_0056_outcome=%s", d.Adr0056Outcome)
    }
}
```

- [ ] **Step 2:** Collect bounded questions into README.md: 大陆短信服务商实名制/签名/模板审核要求；邮件服务商与送达率承诺；验证码 TTL/频控/防刷的行业与监管边界； Emergency 通知（ADR-0048）与发票通知（ADR-0054）的模板合规。
- [ ] **Step 3:** Fill sources.yaml with primary sources (provider docs, regulator notices) and sha256 digests; contradictions and expiry go to README.md.
- [ ] **Step 4:** Reviewer decision: approve/block + `adr_0056_outcome`; run the evidence test green.
- [ ] **Step 5:** Commit: git add docs/evidence/dossiers/notification-channel tests/evidence && git commit -m "evidence(dossier): notification channel and adr-0056 acceptance record"

### Task 2: Apply the ADR-0056 outcome

- [ ] **Step 1:** If `adr_0056_outcome=accepted`: flip ADR-0056 status to accepted, register it in ADR-INDEX, and note the dossier link in the ADR tail sentence.
- [ ] **Step 2:** If blocked: record blocking facts on issue #131 and keep ADR-0056 proposed; U07 invitation stays link-copy only.
- [ ] **Step 3:** Update external-confirmations Notification channel row with the dossier link.

## Self-Review Record

- Spec coverage: 通道合规四问（实名制/模板、邮件、防刷、模板合规）+ ADR-0056 转正输出均有 schema 字段与机械断言。
- Placeholder scan: 测试与 schema 具体，无目标句。
- Type consistency: decision.yaml 字段与 t86-x dossier 约定一致并新增 adr_0056_outcome。
- Right-sizing: 证据产品 + 转正载体合一，不实现通道本身。
