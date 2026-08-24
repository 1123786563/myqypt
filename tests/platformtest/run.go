package platformtest

import (
	"context"
	"encoding/json"
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
		report = baseReport(scenario.ID, scenario.Seam, startedAt, false, "scenario execution failed", "driver_error")
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
	sensitiveValues := collectSensitiveValues(scenario)
	report.Summary = sanitizeText(report.Summary, sensitiveValues)
	report.Assertions = sanitizeAssertions(report.Assertions, sensitiveValues)
	report.Scenario = redactScenario(scenario)

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

func redactScenario(scenario Scenario) map[string]any {
	return redactAny(map[string]any{
		"id":         scenario.ID,
		"seam":       scenario.Seam,
		"timeout":    scenario.Timeout,
		"inputs":     scenario.Inputs,
		"assertions": scenario.Assertions,
		"metadata":   scenario.Metadata,
	}).(map[string]any)
}

func redactAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, child := range typed {
			if shouldRedact(key) {
				redacted[key] = "[REDACTED]"
				continue
			}
			redacted[key] = redactAny(child)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for i, child := range typed {
			redacted[i] = redactAny(child)
		}
		return redacted
	case []Assertion:
		redacted := make([]any, 0, len(typed))
		for _, assertion := range typed {
			redacted = append(redacted, map[string]any{
				"name": assertion.Name,
				"want": redactAny(assertion.Want),
			})
		}
		return redacted
	default:
		return typed
	}
}

func shouldRedact(key string) bool {
	lowered := strings.ToLower(key)
	return strings.Contains(lowered, "secret") ||
		strings.Contains(lowered, "token") ||
		strings.Contains(lowered, "prompt") ||
		strings.Contains(lowered, "document") ||
		strings.Contains(lowered, "payment_payload")
}

func sanitizeText(text string, sensitiveValues []string) string {
	if text == "" {
		return "scenario completed"
	}

	scrubbed := text
	for _, sensitiveValue := range sensitiveValues {
		if sensitiveValue == "" {
			continue
		}
		scrubbed = strings.ReplaceAll(scrubbed, sensitiveValue, "[REDACTED]")
	}

	return scrubbed
}

func sanitizeAssertions(assertions []AssertionResult, sensitiveValues []string) []AssertionResult {
	if len(assertions) == 0 {
		return nil
	}

	clean := make([]AssertionResult, 0, len(assertions))
	for _, assertion := range assertions {
		assertion.Details = sanitizeText(assertion.Details, sensitiveValues)
		clean = append(clean, assertion)
	}

	return clean
}

func collectSensitiveValues(scenario Scenario) []string {
	var values []string
	seen := map[string]struct{}{}
	collectSensitiveValuesFromAny(map[string]any{
		"inputs":     scenario.Inputs,
		"assertions": scenario.Assertions,
		"metadata":   scenario.Metadata,
	}, false, seen, &values)
	return values
}

func collectSensitiveValuesFromAny(value any, sensitive bool, seen map[string]struct{}, values *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			collectSensitiveValuesFromAny(child, sensitive || shouldRedact(key), seen, values)
		}
	case []any:
		for _, child := range typed {
			collectSensitiveValuesFromAny(child, sensitive, seen, values)
		}
	case []Assertion:
		for _, assertion := range typed {
			collectSensitiveValuesFromAny(map[string]any{
				"name": assertion.Name,
				"want": assertion.Want,
			}, sensitive, seen, values)
		}
	case string:
		if sensitive {
			addSensitiveValue(typed, seen, values)
		}
	}
}

func addSensitiveValue(value string, seen map[string]struct{}, values *[]string) {
	if value == "" {
		return
	}
	if _, ok := seen[value]; ok {
		return
	}
	seen[value] = struct{}{}
	*values = append(*values, value)
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
