package productcatalog_test

import (
	"context"
	"errors"
	"testing"

	feature "github.com/1123786563/myqypt/internal/catalog/product-catalog"
)

// recordingPort counts Apply calls and can inject a classified retryable
// failure for the first failCalls deliveries before succeeding. The
// outcome token reflects successful effects only: the first success is
// "accepted", later successes are "duplicate" replays.
type recordingPort struct {
	calls        int
	successes    int
	failCalls    int
	resourceID   string
	availability string
}

func (p *recordingPort) Apply(_ context.Context, _ feature.ProductCatalogCommand) (feature.ProductCatalogResult, error) {
	p.calls++
	if p.calls <= p.failCalls {
		return feature.ProductCatalogResult{}, feature.ErrCatalogUnavailable
	}
	p.successes++
	outcome := "accepted"
	if p.successes > 1 {
		outcome = "duplicate"
	}
	return feature.ProductCatalogResult{ResourceID: p.resourceID, Availability: p.availability, Outcome: outcome}, nil
}

type inMemoryTx struct{ runs int }

func (t *inMemoryTx) Run(ctx context.Context, fn func(context.Context) error) error {
	t.runs++
	return fn(ctx)
}

// memoryEvidence captures every record as an exact triple for the
// content-minimization assertions.
type memoryEvidence struct{ triples [][3]string }

func (m *memoryEvidence) Record(_ context.Context, key, resourceID, outcome string) error {
	m.triples = append(m.triples, [3]string{key, resourceID, outcome})
	return nil
}

func validCommand() feature.ProductCatalogCommand {
	return feature.ProductCatalogCommand{
		TenantID:       "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e",
		ProductID:      "product-alpha",
		IdempotencyKey: "t13-acceptance",
	}
}

// TestProductCatalogRejectsMissingTenantBeforePort proves the first
// guarantee limb: TenantID is mandatory and the rejection happens
// before any outbound port call or evidence record.
func TestProductCatalogRejectsMissingTenantBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "catalog-1", availability: "available"}
	evidence := &memoryEvidence{}
	service := feature.NewProductCatalogService(&inMemoryTx{}, port, evidence)

	cmd := validCommand()
	cmd.TenantID = ""
	_, err := service.Execute(context.Background(), cmd)

	if !errors.Is(err, feature.ErrTenantRequired) {
		t.Fatalf("Execute error = %v, want ErrTenantRequired", err)
	}
	if port.calls != 0 {
		t.Fatalf("outbound port calls = %d, want 0 (rejected before the port)", port.calls)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records = %d, want 0", len(evidence.triples))
	}
}

// TestProductCatalogRejectsMissingIdempotencyKeyBeforePort proves the
// second guarantee limb: the idempotency key is mandatory and rejected
// before any side effect.
func TestProductCatalogRejectsMissingIdempotencyKeyBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "catalog-1", availability: "available"}
	evidence := &memoryEvidence{}
	service := feature.NewProductCatalogService(&inMemoryTx{}, port, evidence)

	cmd := validCommand()
	cmd.IdempotencyKey = ""
	_, err := service.Execute(context.Background(), cmd)

	if !errors.Is(err, feature.ErrIdempotencyKeyRequired) {
		t.Fatalf("Execute error = %v, want ErrIdempotencyKeyRequired", err)
	}
	if port.calls != 0 {
		t.Fatalf("outbound port calls = %d, want 0", port.calls)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records = %d, want 0", len(evidence.triples))
	}
}

// TestProductCatalogRejectsMissingProductBeforePort proves the third
// guarantee limb: a view without its product is rejected before the
// outbound port — a browse must name what it is browsing.
func TestProductCatalogRejectsMissingProductBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "catalog-1", availability: "available"}
	evidence := &memoryEvidence{}
	service := feature.NewProductCatalogService(&inMemoryTx{}, port, evidence)

	cmd := validCommand()
	cmd.ProductID = ""
	_, err := service.Execute(context.Background(), cmd)

	if !errors.Is(err, feature.ErrProductRequired) {
		t.Fatalf("Execute error = %v, want ErrProductRequired", err)
	}
	if port.calls != 0 {
		t.Fatalf("outbound port calls = %d, want 0", port.calls)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records = %d, want 0", len(evidence.triples))
	}
}

// TestProductCatalogAcceptedViewCarriesAvailabilityAndEvidence proves
// the accepted path: exactly one outbound effect inside one
// transaction, the product's availability carried across the seam, and
// one content-minimized evidence row keyed by the idempotency key.
func TestProductCatalogAcceptedViewCarriesAvailabilityAndEvidence(t *testing.T) {
	port := &recordingPort{resourceID: "catalog-1", availability: "available"}
	evidence := &memoryEvidence{}
	tx := &inMemoryTx{}
	service := feature.NewProductCatalogService(tx, port, evidence)

	result, err := service.Execute(context.Background(), validCommand())

	if err != nil {
		t.Fatalf("Execute error = %v, want nil", err)
	}
	if result.Outcome != "accepted" || result.ResourceID != "catalog-1" {
		t.Fatalf("result = %+v, want {catalog-1 accepted}", result)
	}
	if result.Availability != "available" {
		t.Fatalf("availability = %q, want available", result.Availability)
	}
	if port.calls != 1 || tx.runs != 1 {
		t.Fatalf("port calls = %d, tx runs = %d, want 1 and 1", port.calls, tx.runs)
	}
	if len(evidence.triples) != 1 {
		t.Fatalf("evidence records = %d, want 1", len(evidence.triples))
	}
	want := [3]string{"t13-acceptance", "catalog-1", "accepted"}
	if evidence.triples[0] != want {
		t.Fatalf("evidence triple = %v, want %v", evidence.triples[0], want)
	}
}

// TestProductCatalogReplayConvergesToSingleEffect proves the idempotency
// semantics: a replayed delivery under the same key answers the same
// access entry as a duplicate — never a second effect — while each
// delivery still leaves its own evidence row.
func TestProductCatalogReplayConvergesToSingleEffect(t *testing.T) {
	port := &recordingPort{resourceID: "catalog-1", availability: "available"}
	evidence := &memoryEvidence{}
	service := feature.NewProductCatalogService(&inMemoryTx{}, port, evidence)

	first, firstErr := service.Execute(context.Background(), validCommand())
	second, secondErr := service.Execute(context.Background(), validCommand())

	if firstErr != nil || secondErr != nil {
		t.Fatalf("execute errors = %v / %v, want nil", firstErr, secondErr)
	}
	if first.Outcome != "accepted" || second.Outcome != "duplicate" {
		t.Fatalf("outcomes = %s then %s, want accepted then duplicate", first.Outcome, second.Outcome)
	}
	if second.ResourceID != first.ResourceID {
		t.Fatalf("replay resource id = %s, want the original %s", second.ResourceID, first.ResourceID)
	}
	if second.Availability != first.Availability {
		t.Fatalf("replay availability = %s, want the original %s", second.Availability, first.Availability)
	}
	if len(evidence.triples) != 2 {
		t.Fatalf("evidence records = %d, want 2 (one per delivery)", len(evidence.triples))
	}
}

// TestProductCatalogPortFailureLeavesNoPartialState proves the failure
// path: a classified retryable port failure records zero evidence (no
// partial state), and the retry under the same key converges onto the
// single accepted effect.
func TestProductCatalogPortFailureLeavesNoPartialState(t *testing.T) {
	port := &recordingPort{resourceID: "catalog-1", availability: "available", failCalls: 1}
	evidence := &memoryEvidence{}
	service := feature.NewProductCatalogService(&inMemoryTx{}, port, evidence)

	_, failureErr := service.Execute(context.Background(), validCommand())
	if !errors.Is(failureErr, feature.ErrCatalogUnavailable) {
		t.Fatalf("failure error = %v, want ErrCatalogUnavailable", failureErr)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records after failure = %d, want 0", len(evidence.triples))
	}

	retried, retryErr := service.Execute(context.Background(), validCommand())
	if retryErr != nil {
		t.Fatalf("retry error = %v, want nil", retryErr)
	}
	if retried.Outcome != "accepted" {
		t.Fatalf("retry outcome = %s, want accepted", retried.Outcome)
	}
	if len(evidence.triples) != 1 {
		t.Fatalf("evidence records after retry = %d, want 1", len(evidence.triples))
	}
}
