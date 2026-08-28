// T25 acceptance journey driver (Issue #26): proves the secret-reference
// invariant end to end at the highest practical seam — the real service,
// the concrete in-process provider port, and a recording evidence sink,
// executed through the platformtest harness. The ticket has no HTTP
// contract (the scaffold plan itself runs platformtest without a stack),
// so this in-process journey is the named acceptance seam.
package acceptance

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/1123786563/myqypt/internal/security/secret-reference"
	"github.com/1123786563/myqypt/tests/platformtest"
)

const seamSecretReference = "lighthouse-secret-reference"

// t25AssertionNames is the exact set declared by
// scenarios/t25-secret-reference.yaml, in declaration order. The harness
// reconciles by name and rejects any drift, so this list and the YAML
// must move together.
var t25AssertionNames = []string{
	"reject_missing_tenant",
	"reject_missing_idempotency_key",
	"reject_raw_secret_value",
	"accepted_apply_once",
	"replay_converges_single_effect",
	"port_failure_no_partial_then_retry_converges",
	"evidence_content_minimized",
	"repo_hygiene_zero_secret_committed",
}

// t25FakeRawSecretHalves assembles the journey's fake secret material in
// memory only. Neither half — nor the assembled value — is ever a real
// credential, and the split keeps the assembled literal out of every
// committed file (the repo-hygiene assertion scans all tracked files for
// it, this source included).
func t25FakeRawSecretHalves() (string, string) {
	return "T25-Journey-Fake-", "Raw-Secret-9c2f!!"
}

// journeyEvidenceSink records every evidence triple for the
// content-minimization assertion.
type journeyEvidenceSink struct{ triples [][3]string }

func (s *journeyEvidenceSink) Record(_ context.Context, key, resourceID, outcome string) error {
	s.triples = append(s.triples, [3]string{key, resourceID, outcome})
	return nil
}

// flakyPort wraps a port and fails the first deliveries with the
// classified retryable error before delegating — the injected provider
// failure of the failure-path assertion.
type flakyPort struct {
	inner     secretreference.SecretReferencePort
	failFirst int
	calls     int
}

func (p *flakyPort) Apply(ctx context.Context, cmd secretreference.SecretReferenceCommand) (secretreference.SecretReferenceResult, error) {
	p.calls++
	if p.calls <= p.failFirst {
		return secretreference.SecretReferenceResult{}, secretreference.ErrProviderUnavailable
	}
	return p.inner.Apply(ctx, cmd)
}

func init() {
	platformtest.Register(seamSecretReference, secretReferenceDriver{})
}

type secretReferenceDriver struct{}

func (secretReferenceDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	input := func(key string) string {
		value, _ := scenario.Inputs[key].(string)
		return value
	}
	tenantID := input("tenant_id")
	idempotencyKey := input("idempotency_key")
	secretRef := input("secret_ref")
	if tenantID == "" || idempotencyKey == "" || secretRef == "" {
		return t25FailedReport("scenario inputs tenant_id/idempotency_key/secret_ref are required"), nil
	}

	results := map[string]platformtest.AssertionResult{}
	record := func(name string, passed bool, details string) {
		results[name] = platformtest.AssertionResult{Name: name, Passed: passed, Details: details}
	}

	fakeFirstHalf, fakeSecondHalf := t25FakeRawSecretHalves()
	fakeRawSecret := fakeFirstHalf + fakeSecondHalf

	// The three input-shaped rejections happen before the outbound port
	// and leave zero evidence.
	rejections := []struct {
		name string
		cmd  secretreference.SecretReferenceCommand
		want error
	}{
		{"reject_missing_tenant", secretreference.SecretReferenceCommand{TenantID: "", SecretRef: secretRef, IdempotencyKey: idempotencyKey}, secretreference.ErrTenantRequired},
		{"reject_missing_idempotency_key", secretreference.SecretReferenceCommand{TenantID: tenantID, SecretRef: secretRef, IdempotencyKey: ""}, secretreference.ErrIdempotencyKeyRequired},
		{"reject_raw_secret_value", secretreference.SecretReferenceCommand{TenantID: tenantID, SecretRef: fakeRawSecret, IdempotencyKey: idempotencyKey}, secretreference.ErrSecretRefInvalid},
	}
	rejectionPort := secretreference.NewInProcessProviderPort()
	rejectionEvidence := &journeyEvidenceSink{}
	rejectionService := secretreference.NewSecretReferenceService(secretreference.InProcessTx{}, rejectionPort, rejectionEvidence)
	for _, rejection := range rejections {
		_, err := rejectionService.Execute(ctx, rejection.cmd)
		record(rejection.name,
			errors.Is(err, rejection.want) && len(rejectionEvidence.triples) == 0,
			fmt.Sprintf("error_class=%t zero_evidence=%t", errors.Is(err, rejection.want), len(rejectionEvidence.triples) == 0))
	}

	// The accepted delivery: one provider effect, one minimized evidence
	// row, a non-empty external resource id.
	acceptPort := secretreference.NewInProcessProviderPort()
	acceptEvidence := &journeyEvidenceSink{}
	acceptService := secretreference.NewSecretReferenceService(secretreference.InProcessTx{}, acceptPort, acceptEvidence)
	accepted, err := acceptService.Execute(ctx, secretreference.SecretReferenceCommand{TenantID: tenantID, SecretRef: secretRef, IdempotencyKey: idempotencyKey})
	record("accepted_apply_once",
		err == nil && accepted.Outcome == "accepted" && accepted.ResourceID != "" && len(acceptEvidence.triples) == 1,
		fmt.Sprintf("outcome=%s resource_present=%t evidence_rows=%d", accepted.Outcome, accepted.ResourceID != "", len(acceptEvidence.triples)))

	// The replay converges: same resource id, outcome duplicate, still
	// exactly one evidence row per delivery — and the provider effect
	// count stays one (the concrete port's guarantee, observed through
	// a fresh apply of the same key).
	replayed, err := acceptService.Execute(ctx, secretreference.SecretReferenceCommand{TenantID: tenantID, SecretRef: secretRef, IdempotencyKey: idempotencyKey})
	record("replay_converges_single_effect",
		err == nil && replayed.ResourceID == accepted.ResourceID && replayed.Outcome == "duplicate" && len(acceptEvidence.triples) == 2,
		fmt.Sprintf("same_resource=%t outcome=%s evidence_rows=%d", replayed.ResourceID == accepted.ResourceID, replayed.Outcome, len(acceptEvidence.triples)))

	// The failure path: an injected provider failure records zero
	// evidence, and the retry converges onto the single accepted effect.
	flaky := &flakyPort{inner: secretreference.NewInProcessProviderPort(), failFirst: 1}
	failureEvidence := &journeyEvidenceSink{}
	failureService := secretreference.NewSecretReferenceService(secretreference.InProcessTx{}, flaky, failureEvidence)
	_, failureErr := failureService.Execute(ctx, secretreference.SecretReferenceCommand{TenantID: tenantID, SecretRef: secretRef, IdempotencyKey: idempotencyKey})
	zeroPartial := errors.Is(failureErr, secretreference.ErrProviderUnavailable) && len(failureEvidence.triples) == 0
	retried, retryErr := failureService.Execute(ctx, secretreference.SecretReferenceCommand{TenantID: tenantID, SecretRef: secretRef, IdempotencyKey: idempotencyKey})
	record("port_failure_no_partial_then_retry_converges",
		zeroPartial && retryErr == nil && retried.Outcome == "accepted" && len(failureEvidence.triples) == 1,
		fmt.Sprintf("failure_classified=%t zero_evidence_after_failure=%t retry_outcome=%s evidence_rows=%d",
			errors.Is(failureErr, secretreference.ErrProviderUnavailable), zeroPartial, retried.Outcome, len(failureEvidence.triples)))

	// Evidence minimization across every flow: the recorded triples are
	// exactly (idempotency key, external resource id, outcome token) and
	// none of them — nor any split half — carries the fake secret
	// material.
	allTriples := append(append(append([][3]string{}, rejectionEvidence.triples...), acceptEvidence.triples...), failureEvidence.triples...)
	minimized := true
	for _, triple := range allTriples {
		joined := strings.Join(triple[:], " ")
		for _, needle := range []string{fakeRawSecret, fakeFirstHalf, fakeSecondHalf} {
			if strings.Contains(joined, needle) {
				minimized = false
			}
		}
	}
	record("evidence_content_minimized",
		minimized && len(allTriples) == 3,
		fmt.Sprintf("rows=%d secret_material_hits=%t", len(allTriples), !minimized))

	// Repo hygiene (the ticket's dev-env clause, as a live self-check):
	// the assembled fake secret material appears in zero git-tracked
	// files — this driver's committed source included, because the
	// halves are split exactly so the assembled literal cannot be.
	gitHits, gitErr := t25GitTrackedHits(fakeRawSecret)
	record("repo_hygiene_zero_secret_committed",
		gitErr == nil && gitHits == 0,
		fmt.Sprintf("git_error=%t hits=%d", gitErr != nil, gitHits))

	ordered := make([]platformtest.AssertionResult, 0, len(t25AssertionNames))
	passed := true
	for _, name := range t25AssertionNames {
		result, ok := results[name]
		if !ok {
			return t25FailedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}
	summary := fmt.Sprintf("rejects=%d accepted=%s replay=%s evidence_rows=%d git_hits=%d",
		len(rejections), accepted.Outcome, replayed.Outcome, len(allTriples), gitHits)
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// t25GitTrackedHits counts git-tracked files containing the assembled
// fake secret material. Exit code 1 from git grep means zero matches.
func t25GitTrackedHits(needle string) (int, error) {
	output, err := exec.Command("git", "grep", "-I", "-l", "-F", "--", needle).Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok && len(output) == 0 {
			return 0, nil // git grep: no matches
		}
		return -1, fmt.Errorf("git grep unavailable: %w", err)
	}
	return len(strings.Split(strings.TrimSpace(string(output)), "\n")), nil
}

// t25FailedReport builds a failing report whose assertion set matches
// the declared T25 names (all failed), keeping the harness
// reconciliation valid.
func t25FailedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(t25AssertionNames))
	for _, name := range t25AssertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}
