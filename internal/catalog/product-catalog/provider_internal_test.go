package productcatalog

import (
	"context"
	"sync"
	"testing"
)

// t13Command builds a valid command for the given product and key.
func t13Command(productID, key string) ProductCatalogCommand {
	return ProductCatalogCommand{
		TenantID:       "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e",
		ProductID:      productID,
		IdempotencyKey: key,
	}
}

// TestCuratedCatalogIsClosedAndVocabularyValid proves the ADR-0003
// shape at the source level: the curated set is a closed code literal
// whose every entry carries an availability from the closed vocabulary,
// and the port denies any product outside the set before any effect.
func TestCuratedCatalogIsClosedAndVocabularyValid(t *testing.T) {
	if len(curatedCatalog) != 2 {
		t.Fatalf("curated catalog size = %d, want the closed two-entry Stage-1 set", len(curatedCatalog))
	}
	for productID, availability := range curatedCatalog {
		if _, ok := availabilities[availability]; !ok {
			t.Fatalf("product %q availability %q outside the closed vocabulary", productID, availability)
		}
	}
	if _, ok := curatedCatalog["product-alpha"]; !ok || curatedCatalog["product-alpha"] != "available" {
		t.Fatalf("product-alpha missing or not available: %v", curatedCatalog)
	}
	if _, ok := curatedCatalog["product-beta"]; !ok || curatedCatalog["product-beta"] != "unavailable" {
		t.Fatalf("product-beta missing or not unavailable: %v", curatedCatalog)
	}

	port := &inProcessCatalogPort{applied: make(map[string]string), accesses: make(map[string]catalogAccess)}
	_, err := port.Apply(context.Background(), t13Command("product-ghost", "t13-ghost"))
	if err != ErrProductNotCurated {
		t.Fatalf("ghost product error = %v, want ErrProductNotCurated", err)
	}
	if port.views != 0 || len(port.accesses) != 0 {
		t.Fatalf("ghost product left effects: views=%d accesses=%d", port.views, len(port.accesses))
	}
}

// TestInProcessCatalogPortReplayConvergesWithoutPhantomViews proves the
// convergence semantics at the store: a replay answers the original
// entry id as a duplicate (with the availability observed at first
// view), consumes no phantom view, and the next fresh key takes
// exactly the next sequence number.
func TestInProcessCatalogPortReplayConvergesWithoutPhantomViews(t *testing.T) {
	port := &inProcessCatalogPort{applied: make(map[string]string), accesses: make(map[string]catalogAccess)}

	first, err := port.Apply(context.Background(), t13Command("product-alpha", "t13-converge"))
	if err != nil || first.Outcome != "accepted" || first.ResourceID != "catalog-1" || first.Availability != "available" {
		t.Fatalf("first view = %+v, %v", first, err)
	}
	replayed, err := port.Apply(context.Background(), t13Command("product-alpha", "t13-converge"))
	if err != nil || replayed.Outcome != "duplicate" || replayed.ResourceID != "catalog-1" || replayed.Availability != "available" {
		t.Fatalf("replay = %+v, %v — want {catalog-1 available duplicate}", replayed, err)
	}

	next, err := port.Apply(context.Background(), t13Command("product-beta", "t13-converge-next"))
	if err != nil || next.ResourceID != "catalog-2" || next.Outcome != "accepted" || next.Availability != "unavailable" {
		t.Fatalf("next fresh key = %+v, %v — want {catalog-2 unavailable accepted} (no phantom view)", next, err)
	}
	if port.views != 2 {
		t.Fatalf("views = %d, want 2", port.views)
	}
}

// TestInProcessCatalogPortConcurrentViewsConverge gives the race
// detector real concurrent work on the registry: many goroutines
// delivering the same key must converge onto exactly one view and one
// shared entry id.
func TestInProcessCatalogPortConcurrentViewsConverge(t *testing.T) {
	port := &inProcessCatalogPort{applied: make(map[string]string), accesses: make(map[string]catalogAccess)}
	const deliveries = 32

	var wg sync.WaitGroup
	ids := make([]string, deliveries)
	for i := 0; i < deliveries; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := port.Apply(context.Background(), t13Command("product-alpha", "t13-concurrent"))
			if err != nil {
				t.Errorf("delivery %d: Apply error = %v", i, err)
				return
			}
			ids[i] = result.ResourceID
		}(i)
	}
	wg.Wait()

	for _, id := range ids {
		if id != "catalog-1" {
			t.Fatalf("concurrent delivery ids diverged: %v", ids)
		}
	}
	if port.views != 1 {
		t.Fatalf("views = %d, want 1 (all deliveries converged)", port.views)
	}
}
