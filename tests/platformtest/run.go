package platformtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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

var safeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

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
		report := baseReport(reportScenarioID(scenarioPath, scenario.ID), scenario.Seam, startedAt, false, summary, reason)
		return finalizeReport(scenarioPath, report)
	}

	driver, ok := loadDriver(scenario.Seam)
	if !ok {
		report := baseReport(scenario.ID, scenario.Seam, startedAt, false, fmt.Sprintf("scenario rejected: unsupported seam %q", scenario.Seam), "unsupported_seam")
		return finalizeReport(scenarioPath, report)
	}

	timeout := defaultTimeout
	if scenario.Timeout != "" {
		parsed, err := time.ParseDuration(scenario.Timeout)
		if err != nil {
			report := baseReport(scenario.ID, scenario.Seam, startedAt, false, "scenario rejected: invalid timeout", "invalid_timeout")
			return finalizeReport(scenarioPath, report)
		}
		timeout = parsed
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	driverReport, err, finishedAt := executeDriver(ctx, driver, scenario)
	report := buildDriverReport(startedAt, finishedAt, scenario, driverReport, err)

	return finalizeReport(scenarioPath, report)
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
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Scenario{}, "scenario rejected: invalid scenario contract", "decode_error"
	}
	if strings.TrimSpace(scenario.ID) == "" {
		return Scenario{}, "scenario rejected: id is required", "missing_id"
	}
	if !isSafeIdentifier(scenario.ID) {
		return scenario, "scenario rejected: id must use only letters, numbers, dot, dash, or underscore", "invalid_id"
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

func finalizeReport(scenarioPath string, report Report) Report {
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

	dir, err := evidenceDirectory(root, scenarioID)
	if err != nil {
		return "", err
	}
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

func buildDriverReport(startedAt, finishedAt time.Time, scenario Scenario, driverReport Report, driverErr error) Report {
	report := Report{
		ScenarioID:   scenario.ID,
		Seam:         scenario.Seam,
		StartedAt:    startedAt.Format(time.RFC3339Nano),
		FinishedAt:   finishedAt.Format(time.RFC3339Nano),
		Duration:     finishedAt.Sub(startedAt).String(),
		Revision:     buildRevision(),
		Dependencies: buildDependencies(),
		Scenario:     scenarioEvidenceMetadata(scenario),
	}

	if driverErr != nil {
		report.Passed = false
		report.Summary = "scenario execution failed"
		report.FailureReason = "driver_error"
		if errors.Is(driverErr, context.DeadlineExceeded) {
			report.Summary = "scenario execution timed out"
			report.FailureReason = "driver_timeout"
		}
		return report
	}

	assertions, assertionState := reconcileAssertionResults(scenario.Assertions, driverReport.Assertions)
	if assertionState != assertionResultsValid {
		report.Passed = false
		report.Summary = "scenario assertion results invalid"
		report.FailureReason = "invalid_assertion_results"
		return report
	}

	report.Assertions = assertions

	switch {
	case hasFailedAssertion(assertions):
		report.Passed = false
		report.Summary = driverSummaryOrDefault(driverReport.Summary, "scenario assertions failed")
		report.FailureReason = "assertion_failed"
	case !driverReport.Passed:
		report.Passed = false
		report.Summary = driverSummaryOrDefault(driverReport.Summary, "scenario reported failure")
		report.FailureReason = "driver_reported_failure"
	default:
		report.Passed = true
		report.Summary = driverSummaryOrDefault(driverReport.Summary, "scenario completed")
	}

	return report
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

func driverSummaryOrDefault(summary, defaultSummary string) string {
	if summary == "" {
		return defaultSummary
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

func executeDriver(ctx context.Context, driver Driver, scenario Scenario) (Report, error, time.Time) {
	type driverResult struct {
		report     Report
		err        error
		finishedAt time.Time
	}

	resultCh := make(chan driverResult, 1)
	go func() {
		report, err := driver.Execute(ctx, scenario)
		resultCh <- driverResult{
			report:     report,
			err:        err,
			finishedAt: time.Now().UTC(),
		}
	}()

	select {
	case result := <-resultCh:
		if ctx.Err() != nil {
			return Report{}, context.DeadlineExceeded, time.Now().UTC()
		}
		return result.report, result.err, result.finishedAt
	case <-ctx.Done():
		return Report{}, context.DeadlineExceeded, time.Now().UTC()
	}
}

type assertionValidationState int

const (
	assertionResultsValid assertionValidationState = iota
	assertionResultsInvalid
)

func reconcileAssertionResults(declared []Assertion, actual []AssertionResult) ([]AssertionResult, assertionValidationState) {
	if len(declared) == 0 {
		if len(actual) > 0 {
			return nil, assertionResultsInvalid
		}
		return nil, assertionResultsValid
	}

	resultsByName := make(map[string]AssertionResult, len(actual))
	for _, result := range actual {
		if _, ok := resultsByName[result.Name]; ok {
			return nil, assertionResultsInvalid
		}
		resultsByName[result.Name] = result
	}

	reconciled := make([]AssertionResult, 0, len(declared))
	for index, assertion := range declared {
		result, ok := resultsByName[assertion.Name]
		if !ok {
			return nil, assertionResultsInvalid
		}
		delete(resultsByName, assertion.Name)
		reconciled = append(reconciled, AssertionResult{
			Name:    persistedAssertionName(assertion.Name, index),
			Passed:  result.Passed,
			Details: sanitizeAssertionDetails(result.Details),
		})
	}

	if len(resultsByName) > 0 {
		return nil, assertionResultsInvalid
	}

	return reconciled, assertionResultsValid
}

func hasFailedAssertion(assertions []AssertionResult) bool {
	for _, assertion := range assertions {
		if !assertion.Passed {
			return true
		}
	}
	return false
}

func persistedAssertionName(name string, index int) string {
	if isSafeIdentifier(name) {
		return name
	}
	return fmt.Sprintf("assertion_%d", index+1)
}

func evidenceDirectory(root, scenarioID string) (string, error) {
	if !isSafeIdentifier(scenarioID) {
		return "", fmt.Errorf("invalid scenario id %q", scenarioID)
	}

	evidenceRoot := filepath.Join(root, "artifacts", "evidence")
	dir := filepath.Join(evidenceRoot, scenarioID)

	absRoot, err := filepath.Abs(evidenceRoot)
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	prefix := absRoot + string(os.PathSeparator)
	if absDir != absRoot && !strings.HasPrefix(absDir, prefix) {
		return "", fmt.Errorf("evidence path escapes root: %s", absDir)
	}

	return absDir, nil
}

func reportScenarioID(path, decodedID string) string {
	if isSafeIdentifier(decodedID) {
		return decodedID
	}
	return scenarioIDFromPath(path)
}

func isSafeIdentifier(value string) bool {
	return safeIdentifierPattern.MatchString(value)
}

func scenarioIDFromPath(path string) string {
	name := filepath.Base(path)
	id := strings.TrimSuffix(name, filepath.Ext(name))
	if isSafeIdentifier(id) {
		return id
	}

	var builder strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}

	candidate := strings.Trim(builder.String(), "-.")
	if !isSafeIdentifier(candidate) {
		return "scenario"
	}

	return candidate
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
