package platformtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultTimeout = 30 * time.Second

const (
	redactedSummaryText          = "driver summary omitted from evidence"
	redactedAssertionDetailsText = "assertion details omitted from evidence"
)

type Driver interface {
	Execute(context.Context, Scenario) (Report, error)
}

var drivers sync.Map

func Register(seam string, driver Driver) {
	if seam == "" || driver == nil {
		panic("platformtest: seam and driver are required")
	}
	if _, loaded := drivers.LoadOrStore(seam, driver); loaded {
		panic("platformtest: duplicate seam " + seam)
	}
}

func Run(t *testing.T, scenarioPath string) Report {
	t.Helper()

	startedAt := time.Now().UTC()

	scenario, summary, reason := loadScenario(scenarioPath)
	if reason != "" {
		report := baseReport(scenarioIDFromPath(scenarioPath), scenario.Seam, startedAt, false, summary, reason)
		return finalizeReport(report, scenarioPath, scenario)
	}

	driver, ok := loadDriver(scenario.Seam)
	if !ok {
		report := baseReport(scenario.ID, scenario.Seam, startedAt, false, fmt.Sprintf("scenario rejected: unsupported seam %q", scenario.Seam), "unsupported_seam")
		return finalizeReport(report, scenarioPath, scenario)
	}

	timeout := defaultTimeout
	if scenario.Timeout != "" {
		parsed, err := time.ParseDuration(scenario.Timeout)
		if err != nil {
			report := baseReport(scenario.ID, scenario.Seam, startedAt, false, "scenario rejected: invalid timeout", "invalid_timeout")
			return finalizeReport(report, scenarioPath, scenario)
		}
		timeout = parsed
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	report, err := driver.Execute(ctx, scenario)
	if err != nil {
		summary := "scenario execution failed"
		reason := "driver_error"
		if errors.Is(err, context.DeadlineExceeded) {
			summary = "scenario execution timed out"
			reason = "driver_timeout"
		}
		report = baseReport(scenario.ID, scenario.Seam, startedAt, false, summary, reason)
	}

	if report.ScenarioID == "" {
		report.ScenarioID = scenario.ID
	}
	if report.Seam == "" {
		report.Seam = scenario.Seam
	}
	if report.StartedAt == "" {
		report.StartedAt = startedAt.Format(time.RFC3339Nano)
	}
	if report.FinishedAt == "" {
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if report.Duration == "" {
		finishedAt, parseErr := time.Parse(time.RFC3339Nano, report.FinishedAt)
		if parseErr == nil {
			report.Duration = finishedAt.Sub(startedAt).String()
		}
	}
	if report.Dependencies == nil {
		report.Dependencies = buildDependencies()
	}
	if report.Revision == "" {
		report.Revision = buildRevision()
	}
	if report.Summary == "" {
		report.Summary = "scenario completed"
	}

	return finalizeReport(report, scenarioPath, scenario)
}

func loadScenario(path string) (Scenario, string, string) {
	file, err := os.Open(path)
	if err != nil {
		return Scenario{}, "scenario rejected: unable to read scenario", "read_error"
	}
	defer file.Close()

	var scenario Scenario
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&scenario); err != nil {
		return Scenario{}, "scenario rejected: invalid scenario contract", "decode_error"
	}
	if strings.TrimSpace(scenario.ID) == "" {
		return Scenario{}, "scenario rejected: id is required", "missing_id"
	}
	if strings.TrimSpace(scenario.Seam) == "" {
		return scenario, "scenario rejected: seam is required", "missing_seam"
	}

	return scenario, "", ""
}

func loadDriver(seam string) (Driver, bool) {
	value, ok := drivers.Load(seam)
	if !ok {
		return nil, false
	}

	driver, ok := value.(Driver)
	return driver, ok
}

func baseReport(scenarioID, seam string, startedAt time.Time, passed bool, summary, reason string) Report {
	return Report{
		ScenarioID:    scenarioID,
		Seam:          seam,
		Passed:        passed,
		Summary:       summary,
		StartedAt:     startedAt.Format(time.RFC3339Nano),
		FinishedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Duration:      time.Since(startedAt).String(),
		Revision:      buildRevision(),
		Dependencies:  buildDependencies(),
		FailureReason: reason,
	}
}

func finalizeReport(report Report, scenarioPath string, scenario Scenario) Report {
	report.Summary = sanitizeSummary(report.Summary)
	report.Assertions = sanitizeAssertions(report.Assertions)
	report.Scenario = scenarioEvidenceMetadata(scenario)

	evidencePath, err := writeReport(scenarioPath, report)
	if err != nil {
		report.Passed = false
		report.Summary = "scenario completed but evidence persistence failed"
		report.FailureReason = "write_error"
		report.EvidencePath = ""
		return report
	}

	report.EvidencePath = evidencePath
	return report
}

func writeReport(scenarioPath string, report Report) (string, error) {
	root := repoRoot()
	scenarioID := report.ScenarioID
	if scenarioID == "" {
		scenarioID = scenarioIDFromPath(scenarioPath)
	}

	dir := filepath.Join(root, "artifacts", "evidence", scenarioID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	name := fmt.Sprintf("%s.json", time.Now().UTC().Format("20060102T150405.000000000Z"))
	path := filepath.Join(dir, name)

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", err
	}

	return path, nil
}

func scenarioEvidenceMetadata(scenario Scenario) map[string]any {
	metadata := map[string]any{
		"id":   scenario.ID,
		"seam": scenario.Seam,
	}
	if scenario.Timeout != "" {
		metadata["timeout"] = scenario.Timeout
	}
	if len(scenario.Assertions) > 0 {
		metadata["assertion_count"] = len(scenario.Assertions)
	}

	return metadata
}

func sanitizeSummary(summary string) string {
	if summary == "" {
		return "scenario completed"
	}
	if isSafeHarnessSummary(summary) {
		return summary
	}
	return redactedSummaryText
}

func sanitizeAssertions(assertions []AssertionResult) []AssertionResult {
	if len(assertions) == 0 {
		return nil
	}

	clean := make([]AssertionResult, 0, len(assertions))
	for _, assertion := range assertions {
		assertion.Details = sanitizeAssertionDetails(assertion.Details)
		clean = append(clean, assertion)
	}

	return clean
}

func sanitizeAssertionDetails(details string) string {
	if details == "" {
		return ""
	}
	return redactedAssertionDetailsText
}

func isSafeHarnessSummary(summary string) bool {
	return summary == "scenario completed" ||
		summary == "scenario execution failed" ||
		summary == "scenario execution timed out" ||
		summary == "scenario completed but evidence persistence failed" ||
		strings.HasPrefix(summary, "scenario rejected:")
}

func scenarioIDFromPath(path string) string {
	name := filepath.Base(path)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func repoRoot() string {
	workingDir, err := os.Getwd()
	if err != nil {
		return "."
	}

	current := workingDir
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return workingDir
		}
		current = parent
	}
}

func buildDependencies() map[string]string {
	dependencies := map[string]string{
		"go": runtime.Version(),
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		dependencies[info.Main.Path] = info.Main.Version
		for _, dep := range info.Deps {
			dependencies[dep.Path] = dep.Version
		}
	}

	return dependencies
}

func buildRevision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}

	return "unknown"
}
