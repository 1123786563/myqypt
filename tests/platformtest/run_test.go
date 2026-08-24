package platformtest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
