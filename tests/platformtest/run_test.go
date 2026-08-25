package platformtest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunRejectsUnknownSeamWithoutLeakingInput(t *testing.T) {
	path := writeScenario(t, `id: t01-1
seam: unknown
secret: must-not-appear
`)

	report := Run(t, path)
	if report.Passed {
		t.Fatalf("report=%+v want failed", report)
	}
	if strings.Contains(report.Summary, "must-not-appear") {
		t.Fatalf("report leaked secret: %+v", report)
	}
}

func TestRunDoesNotPersistScenarioCustomerContent(t *testing.T) {
	seam := "test-scenario-minimization"
	Register(seam, stubDriver{
		report: Report{
			Passed: true,
		},
	})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	path := writeScenario(t, `id: t01-privacy
seam: test-scenario-minimization
inputs:
  message: hello from customer
  email: alice@example.com
metadata:
  customer_name: Alice Example
`)

	report := Run(t, path)
	cleanupEvidence(t, report.EvidencePath)

	payload, err := os.ReadFile(report.EvidencePath)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	for _, leaked := range []string{"hello from customer", "alice@example.com", "Alice Example"} {
		if strings.Contains(string(payload), leaked) {
			t.Fatalf("evidence leaked %q: %s", leaked, string(payload))
		}
	}

	var stored Report
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if _, ok := stored.Scenario["inputs"]; ok {
		t.Fatalf("stored scenario unexpectedly persisted inputs: %+v", stored.Scenario)
	}
	if _, ok := stored.Scenario["metadata"]; ok {
		t.Fatalf("stored scenario unexpectedly persisted metadata: %+v", stored.Scenario)
	}
	if got := stored.Scenario["id"]; got != "t01-privacy" {
		t.Fatalf("stored id=%v want t01-privacy", got)
	}
	if got := stored.Scenario["seam"]; got != seam {
		t.Fatalf("stored seam=%v want %s", got, seam)
	}
}

func TestRunRedactsDriverOnlySensitiveTextFromReturnedReportAndEvidence(t *testing.T) {
	seam := "test-redaction-driver-output"
	Register(seam, stubDriver{
		report: Report{
			Passed:  false,
			Summary: `prompt: summarize customer email alice@example.com`,
			Assertions: []AssertionResult{
				{
					Name:    "details",
					Passed:  false,
					Details: `payment payload: {"card":"4111111111111111"} document excerpt: Alice Example`,
				},
			},
		},
	})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	path := writeScenario(t, `id: t01-2
seam: test-redaction-driver-output
assertions:
  - name: details
`)

	report := Run(t, path)
	cleanupEvidence(t, report.EvidencePath)
	if report.Summary != redactedSummaryText {
		t.Fatalf("summary=%q want %q", report.Summary, redactedSummaryText)
	}
	if got := report.Assertions[0].Details; got != redactedAssertionDetailsText {
		t.Fatalf("details=%q want %q", got, redactedAssertionDetailsText)
	}

	payload, err := os.ReadFile(report.EvidencePath)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	for _, leaked := range []string{"alice@example.com", "4111111111111111", "Alice Example", "prompt:", "payment payload", "document excerpt"} {
		if strings.Contains(string(payload), leaked) {
			t.Fatalf("evidence leaked %q: %s", leaked, string(payload))
		}
	}
}

func TestRegisterPanicsOnDuplicateSeam(t *testing.T) {
	seam := "duplicate-seam"
	Register(seam, stubDriver{})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic on duplicate seam registration")
		}
		if recovered != "platformtest: duplicate seam "+seam {
			t.Fatalf("panic=%v", recovered)
		}
	}()

	Register(seam, stubDriver{})
}

func TestRunRecordsSuccessfulDriverExecution(t *testing.T) {
	seam := "test-success-driver"
	Register(seam, stubDriver{
		report: Report{
			Passed: true,
		},
	})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	report := Run(t, writeScenario(t, "id: t01-success\nseam: test-success-driver\n"))
	cleanupEvidence(t, report.EvidencePath)

	if !report.Passed {
		t.Fatalf("report=%+v want passed", report)
	}
	if report.Summary != "scenario completed" {
		t.Fatalf("summary=%q want scenario completed", report.Summary)
	}
	if report.EvidencePath == "" {
		t.Fatalf("report=%+v want evidence path", report)
	}
}

func TestRunRecordsTimeoutWhenDriverExceedsScenarioDeadline(t *testing.T) {
	seam := "test-timeout-driver"
	Register(seam, stubDriver{
		execute: func(ctx context.Context, scenario Scenario) (Report, error) {
			<-ctx.Done()
			return Report{}, ctx.Err()
		},
	})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	report := Run(t, writeScenario(t, "id: t01-timeout\nseam: test-timeout-driver\ntimeout: 1ms\n"))
	cleanupEvidence(t, report.EvidencePath)

	if report.Passed {
		t.Fatalf("report=%+v want failed", report)
	}
	if report.FailureReason != "driver_timeout" {
		t.Fatalf("failure_reason=%q want driver_timeout", report.FailureReason)
	}
	if report.Summary != "scenario execution timed out" {
		t.Fatalf("summary=%q want scenario execution timed out", report.Summary)
	}
}

func TestRunTimesOutDriverThatIgnoresContext(t *testing.T) {
	seam := "test-ignores-context"
	Register(seam, stubDriver{
		execute: func(context.Context, Scenario) (Report, error) {
			time.Sleep(50 * time.Millisecond)
			return Report{Passed: true}, nil
		},
	})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	startedAt := time.Now()
	report := Run(t, writeScenario(t, "id: t01-ignore-context\nseam: test-ignores-context\ntimeout: 1ms\n"))
	cleanupEvidence(t, report.EvidencePath)

	if elapsed := time.Since(startedAt); elapsed >= 40*time.Millisecond {
		t.Fatalf("elapsed=%s want timeout before driver sleep finishes", elapsed)
	}
	if report.Passed {
		t.Fatalf("report=%+v want failed", report)
	}
	if report.FailureReason != "driver_timeout" {
		t.Fatalf("failure_reason=%q want driver_timeout", report.FailureReason)
	}
}

func TestRunRejectsUnsafeScenarioID(t *testing.T) {
	report := Run(t, writeScenario(t, "id: ../../outside\nseam: test\n"))
	cleanupEvidence(t, report.EvidencePath)

	if report.Passed {
		t.Fatalf("report=%+v want failed", report)
	}
	if report.FailureReason != "invalid_id" {
		t.Fatalf("failure_reason=%q want invalid_id", report.FailureReason)
	}
	if report.ScenarioID != "scenario" {
		t.Fatalf("scenario_id=%q want fallback scenario", report.ScenarioID)
	}
	if strings.Contains(report.EvidencePath, "..") {
		t.Fatalf("evidence_path=%q contains traversal", report.EvidencePath)
	}
}

func TestRunRejectsTrailingYAMLDocuments(t *testing.T) {
	report := Run(t, writeScenario(t, "id: t01-trailing\nseam: known\n---\nextra: true\n"))
	cleanupEvidence(t, report.EvidencePath)

	if report.Passed {
		t.Fatalf("report=%+v want failed", report)
	}
	if report.FailureReason != "decode_error" {
		t.Fatalf("failure_reason=%q want decode_error", report.FailureReason)
	}
}

func TestRunBuildsHarnessOwnedPersistedReport(t *testing.T) {
	seam := "test-harness-owned-report"
	Register(seam, stubDriver{
		report: Report{
			ScenarioID:    "../../outside",
			Seam:          "driver-seam",
			Passed:        false,
			Summary:       "customer prompt: secret plan",
			Assertions:    []AssertionResult{{Name: "customer-email-alice@example.com", Passed: false, Details: "document excerpt"}},
			StartedAt:     "2000-01-01T00:00:00Z",
			FinishedAt:    "2000-01-01T00:00:00Z",
			Duration:      "999h",
			Revision:      "driver-revision-secret",
			Dependencies:  map[string]string{"driver-secret": "payment payload"},
			Scenario:      map[string]any{"customer_name": "Alice"},
			EvidencePath:  "/tmp/driver-secret",
			FailureReason: "driver-secret-reason",
		},
	})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	report := Run(t, writeScenario(t, `id: t01-owned-report
seam: test-harness-owned-report
assertions:
  - name: assertion_a
`))
	cleanupEvidence(t, report.EvidencePath)

	if report.ScenarioID != "t01-owned-report" {
		t.Fatalf("scenario_id=%q want t01-owned-report", report.ScenarioID)
	}
	if report.Seam != seam {
		t.Fatalf("seam=%q want %q", report.Seam, seam)
	}
	if report.Revision == "driver-revision-secret" {
		t.Fatalf("revision preserved driver value: %q", report.Revision)
	}
	if _, ok := report.Dependencies["driver-secret"]; ok {
		t.Fatalf("dependencies preserved driver value: %+v", report.Dependencies)
	}
	if report.EvidencePath == "/tmp/driver-secret" {
		t.Fatalf("evidence_path preserved driver value: %q", report.EvidencePath)
	}
	if report.FailureReason != "invalid_assertion_results" {
		t.Fatalf("failure_reason=%q want invalid_assertion_results", report.FailureReason)
	}
	if report.Assertions != nil {
		t.Fatalf("assertions=%+v want nil on invalid assertion results", report.Assertions)
	}

	payload, err := os.ReadFile(report.EvidencePath)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	for _, leaked := range []string{"driver-revision-secret", "payment payload", "/tmp/driver-secret", "driver-secret-reason", "alice@example.com", "customer prompt", "document excerpt"} {
		if strings.Contains(string(payload), leaked) {
			t.Fatalf("evidence leaked %q: %s", leaked, string(payload))
		}
	}
}

func TestRunRequiresAssertionResultsToMatchScenarioAssertions(t *testing.T) {
	tests := []struct {
		name    string
		results []AssertionResult
	}{
		{
			name: "missing result",
			results: []AssertionResult{
				{Name: "assertion_a", Passed: true},
			},
		},
		{
			name: "duplicate result",
			results: []AssertionResult{
				{Name: "assertion_a", Passed: true},
				{Name: "assertion_a", Passed: true},
			},
		},
		{
			name: "unexpected result",
			results: []AssertionResult{
				{Name: "assertion_a", Passed: true},
				{Name: "assertion_b", Passed: true},
				{Name: "unexpected_assertion", Passed: true},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seam := "assertion-match-" + strings.ReplaceAll(tc.name, " ", "-")
			Register(seam, stubDriver{
				report: Report{
					Passed:     true,
					Assertions: tc.results,
				},
			})
			t.Cleanup(func() {
				drivers.Delete(seam)
			})

			report := Run(t, writeScenario(t, `id: t01-assertions
seam: `+seam+`
assertions:
  - name: assertion_a
  - name: assertion_b
`))
			cleanupEvidence(t, report.EvidencePath)

			if report.Passed {
				t.Fatalf("report=%+v want failed", report)
			}
			if report.FailureReason != "invalid_assertion_results" {
				t.Fatalf("failure_reason=%q want invalid_assertion_results", report.FailureReason)
			}
		})
	}
}

func TestRunMapsAssertionResultsByDeclaredAssertionName(t *testing.T) {
	seam := "assertion-match-success"
	Register(seam, stubDriver{
		report: Report{
			Passed: true,
			Assertions: []AssertionResult{
				{Name: "assertion_b", Passed: true, Details: "raw secret"},
				{Name: "assertion_a", Passed: true},
			},
		},
	})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	report := Run(t, writeScenario(t, `id: t01-assertions-success
seam: assertion-match-success
assertions:
  - name: assertion_a
  - name: assertion_b
`))
	cleanupEvidence(t, report.EvidencePath)

	if !report.Passed {
		t.Fatalf("report=%+v want passed", report)
	}
	if len(report.Assertions) != 2 {
		t.Fatalf("assertions=%+v want 2 results", report.Assertions)
	}
	if report.Assertions[0].Name != "assertion_a" || report.Assertions[1].Name != "assertion_b" {
		t.Fatalf("assertion order=%+v want declared order", report.Assertions)
	}
	if report.Assertions[1].Details != redactedAssertionDetailsText {
		t.Fatalf("details=%q want %q", report.Assertions[1].Details, redactedAssertionDetailsText)
	}
}

func writeScenario(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	return path
}

type stubDriver struct {
	report  Report
	err     error
	execute func(context.Context, Scenario) (Report, error)
}

func (s stubDriver) Execute(ctx context.Context, scenario Scenario) (Report, error) {
	if s.execute != nil {
		return s.execute(ctx, scenario)
	}
	return s.report, s.err
}

func cleanupEvidence(t *testing.T, evidencePath string) {
	t.Helper()
	if evidencePath == "" {
		return
	}
	evidenceDir := filepath.Dir(evidencePath)
	t.Cleanup(func() {
		if err := os.RemoveAll(evidenceDir); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cleanup evidence dir: %v", err)
		}
		parent := filepath.Dir(evidenceDir)
		entries, err := os.ReadDir(parent)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(parent)
		}
	})
}
