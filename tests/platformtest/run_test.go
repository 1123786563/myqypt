package platformtest

import (
	"context"
	"encoding/json"
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

func TestRunRedactsSensitiveValuesFromDriverOutputAndEvidence(t *testing.T) {
	seam := "test-redaction-driver-output"
	Register(seam, stubDriver{
		report: Report{
			Passed:  false,
			Summary: "driver saw super-secret-value",
			Assertions: []AssertionResult{
				{
					Name:    "details",
					Passed:  false,
					Details: "driver saw super-secret-value",
				},
			},
		},
	})
	t.Cleanup(func() {
		drivers.Delete(seam)
	})

	path := writeScenario(t, `id: t01-2
seam: test-redaction-driver-output
inputs:
  api_token: super-secret-value
`)

	report := Run(t, path)
	if strings.Contains(report.Summary, "super-secret-value") {
		t.Fatalf("summary leaked secret: %+v", report)
	}
	if strings.Contains(report.Assertions[0].Details, "super-secret-value") {
		t.Fatalf("details leaked secret: %+v", report.Assertions)
	}

	payload, err := os.ReadFile(report.EvidencePath)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	if strings.Contains(string(payload), "super-secret-value") {
		t.Fatalf("evidence leaked secret: %s", string(payload))
	}

	var stored Report
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if got := stored.Scenario["inputs"].(map[string]any)["api_token"]; got != "[REDACTED]" {
		t.Fatalf("stored input=%v want redacted", got)
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
	report Report
	err    error
}

func (s stubDriver) Execute(context.Context, Scenario) (Report, error) {
	return s.report, s.err
}
