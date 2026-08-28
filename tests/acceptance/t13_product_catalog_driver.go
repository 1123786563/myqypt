// T13 acceptance journey driver (Issue #14): proves the curated-catalog
// browse invariant end to end at the highest practical seam — the real
// service, the concrete in-process curated-catalog port, and a
// recording evidence sink, executed through the platformtest harness.
// The ticket has no HTTP contract (the source plan runs platformtest
// without a stack), so this in-process journey is the named acceptance
// seam.
package acceptance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	productcatalog "github.com/1123786563/myqypt/internal/catalog/product-catalog"
	"github.com/1123786563/myqypt/tests/platformtest"
)

const seamProductCatalog = "lighthouse-product-catalog"

// t13AssertionNames is the exact set declared by
// scenarios/t13-product-catalog.yaml, in declaration order. The harness
// reconciles by name and rejects any drift, so this list and the YAML
// must move together.
var t13AssertionNames = []string{
	"reject_missing_tenant",
	"reject_missing_idempotency_key",
	"reject_missing_product",
	"curated_product_availability_visible",
	"unavailable_product_status_visible",
	"unknown_product_denied",
	"replay_converges_single_effect",
	"port_failure_no_partial_then_retry_converges",
	"evidence_content_minimized",
}

// t13FakeSensitiveHalves assembles the journey's fake sensitive
// material in memory only. Neither half — nor the assembled value — is
// a real credential or customer content, and the split keeps the
// assembled literal out of every committed file. The catalog slice
// must never carry material of this shape into evidence.
func t13FakeSensitiveHalves() (string, string) {
	return "T13-Journey-Fake-", "Api-Key-8d4c$$"
}

// t13JourneyEvidenceSink records every evidence triple for the
// content-minimization assertion.
type t13JourneyEvidenceSink struct{ triples [][3]string }

func (s *t13JourneyEvidenceSink) Record(_ context.Context, key, resourceID, outcome string) error {
	s.triples = append(s.triples, [3]string{key, resourceID, outcome})
	return nil
}

// t13FlakyPort wraps a catalog port and fails the first deliveries
// with the classified retryable error before delegating — the injected
// catalog failure of the failure-path assertion.
type t13FlakyPort struct {
	inner     productcatalog.ProductCatalogPort
	failFirst int
	calls     int
}

func (p *t13FlakyPort) Apply(ctx context.Context, cmd productcatalog.ProductCatalogCommand) (productcatalog.ProductCatalogResult, error) {
	p.calls++
	if p.calls <= p.failFirst {
		return productcatalog.ProductCatalogResult{}, productcatalog.ErrCatalogUnavailable
	}
	return p.inner.Apply(ctx, cmd)
}

func init() {
	platformtest.Register(seamProductCatalog, t13ProductCatalogDriver{})
}

type t13ProductCatalogDriver struct{}

func (t13ProductCatalogDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	input := func(key string) string {
		value, _ := scenario.Inputs[key].(string)
		return value
	}
	tenantID := input("tenant_id")
	idempotencyKey := input("idempotency_key")
	if tenantID == "" || idempotencyKey == "" {
		return t13FailedReport("scenario inputs tenant_id/idempotency_key are required"), nil
	}

	results := map[string]platformtest.AssertionResult{}
	record := func(name string, passed bool, details string) {
		results[name] = platformtest.AssertionResult{Name: name, Passed: passed, Details: details}
	}

	fakeFirstHalf, fakeSecondHalf := t13FakeSensitiveHalves()
	fakeSensitive := fakeFirstHalf + fakeSecondHalf

	command := func(productID, key string) productcatalog.ProductCatalogCommand {
		return productcatalog.ProductCatalogCommand{
			TenantID:       tenantID,
			ProductID:      productID,
			IdempotencyKey: key,
		}
	}

	// The three input-shaped rejections happen before the outbound
	// port and leave zero evidence.
	rejections := []struct {
		name string
		cmd  productcatalog.ProductCatalogCommand
		want error
	}{
		{"reject_missing_tenant", func() productcatalog.ProductCatalogCommand {
			cmd := command("product-alpha", idempotencyKey+"-reject")
			cmd.TenantID = ""
			return cmd
		}(), productcatalog.ErrTenantRequired},
		{"reject_missing_idempotency_key", command("product-alpha", ""), productcatalog.ErrIdempotencyKeyRequired},
		{"reject_missing_product", command("", idempotencyKey+"-reject"), productcatalog.ErrProductRequired},
	}
	rejectionEvidence := &t13JourneyEvidenceSink{}
	rejectionService := productcatalog.NewProductCatalogService(productcatalog.InProcessTx{}, productcatalog.NewInProcessCatalogPort(), rejectionEvidence)
	for _, rejection := range rejections {
		_, err := rejectionService.Execute(ctx, rejection.cmd)
		record(rejection.name,
			errors.Is(err, rejection.want) && len(rejectionEvidence.triples) == 0,
			fmt.Sprintf("error_class=%t zero_evidence=%t", errors.Is(err, rejection.want), len(rejectionEvidence.triples) == 0))
	}

	// The curated catalog is browsed with each product's availability
	// carried across the seam: the available product answers available,
	// and the unavailable curated product is equally viewable with its
	// true unavailable status (visible, not filtered).
	acceptPort := productcatalog.NewInProcessCatalogPort()
	acceptEvidence := &t13JourneyEvidenceSink{}
	acceptService := productcatalog.NewProductCatalogService(productcatalog.InProcessTx{}, acceptPort, acceptEvidence)
	alpha, alphaErr := acceptService.Execute(ctx, command("product-alpha", idempotencyKey+"-alpha"))
	record("curated_product_availability_visible",
		alphaErr == nil && alpha.Outcome == "accepted" && alpha.Availability == "available" && alpha.ResourceID != "" && len(acceptEvidence.triples) == 1,
		fmt.Sprintf("outcome=%s availability=%s evidence_rows=%d", alpha.Outcome, alpha.Availability, len(acceptEvidence.triples)))

	beta, betaErr := acceptService.Execute(ctx, command("product-beta", idempotencyKey+"-beta"))
	record("unavailable_product_status_visible",
		betaErr == nil && beta.Outcome == "accepted" && beta.Availability == "unavailable" && beta.ResourceID != "",
		fmt.Sprintf("outcome=%s availability=%s", beta.Outcome, beta.Availability))

	// A product outside the internally curated set (ADR-0003: only the
	// Platform's internal team curates) is denied with the classified
	// error and leaves zero evidence.
	_, ghostErr := acceptService.Execute(ctx, command("product-ghost", idempotencyKey+"-ghost"))
	record("unknown_product_denied",
		errors.Is(ghostErr, productcatalog.ErrProductNotCurated) && len(acceptEvidence.triples) == 2,
		fmt.Sprintf("denied=%t evidence_rows_still=%d", errors.Is(ghostErr, productcatalog.ErrProductNotCurated), len(acceptEvidence.triples)))

	// The replay converges: same access entry id, outcome duplicate,
	// one more evidence row for the delivery — and no second effect
	// (the entry id is unchanged).
	replayed, replayErr := acceptService.Execute(ctx, command("product-alpha", idempotencyKey+"-alpha"))
	record("replay_converges_single_effect",
		replayErr == nil && replayed.Outcome == "duplicate" && replayed.ResourceID == alpha.ResourceID && replayed.Availability == alpha.Availability,
		fmt.Sprintf("same_entry=%t outcome=%s availability_stable=%t", replayed.ResourceID == alpha.ResourceID, replayed.Outcome, replayed.Availability == alpha.Availability))

	// The failure path: an injected catalog failure records zero
	// evidence, and the retry under the same key converges onto the
	// single accepted effect.
	flaky := &t13FlakyPort{inner: productcatalog.NewInProcessCatalogPort(), failFirst: 1}
	failureEvidence := &t13JourneyEvidenceSink{}
	failureService := productcatalog.NewProductCatalogService(productcatalog.InProcessTx{}, flaky, failureEvidence)
	_, failureErr := failureService.Execute(ctx, command("product-alpha", idempotencyKey+"-failure"))
	zeroPartial := errors.Is(failureErr, productcatalog.ErrCatalogUnavailable) && len(failureEvidence.triples) == 0
	retried, retryErr := failureService.Execute(ctx, command("product-alpha", idempotencyKey+"-failure"))
	record("port_failure_no_partial_then_retry_converges",
		zeroPartial && retryErr == nil && retried.Outcome == "accepted" && retried.Availability == "available" && len(failureEvidence.triples) == 1,
		fmt.Sprintf("failure_classified=%t zero_evidence_after_failure=%t retry_outcome=%s evidence_rows=%d",
			errors.Is(failureErr, productcatalog.ErrCatalogUnavailable), zeroPartial, retried.Outcome, len(failureEvidence.triples)))

	// Evidence minimization across every flow: the recorded triples are
	// exactly (idempotency key, access entry id, outcome token) and
	// none of them — nor any split half — carries the fake sensitive
	// material.
	allTriples := append(append([][3]string{}, rejectionEvidence.triples...), acceptEvidence.triples...)
	allTriples = append(allTriples, failureEvidence.triples...)
	minimized := true
	for _, triple := range allTriples {
		joined := strings.Join(triple[:], " ")
		for _, needle := range []string{fakeSensitive, fakeFirstHalf, fakeSecondHalf} {
			if strings.Contains(joined, needle) {
				minimized = false
			}
		}
	}
	record("evidence_content_minimized",
		minimized && len(allTriples) == 4,
		fmt.Sprintf("rows=%d sensitive_material_hits=%t", len(allTriples), !minimized))

	ordered := make([]platformtest.AssertionResult, 0, len(t13AssertionNames))
	passed := true
	for _, name := range t13AssertionNames {
		result, ok := results[name]
		if !ok {
			return t13FailedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}
	summary := fmt.Sprintf("rejects=%d alpha=%s beta=%s replay=%s evidence_rows=%d",
		len(rejections), alpha.Availability, beta.Availability, replayed.Outcome, len(allTriples))
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// t13FailedReport builds a failing report whose assertion set matches
// the declared T13 names (all failed), keeping the harness
// reconciliation valid.
func t13FailedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(t13AssertionNames))
	for _, name := range t13AssertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}
