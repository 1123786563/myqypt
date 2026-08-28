// T13 acceptance journey: Owner 查看内部策展 Product 及其可用状态 —
// proven through the real service, the concrete in-process
// curated-catalog port, and a recording evidence sink at the
// platformtest seam. The ticket has no HTTP contract, so the journey
// runs in-process with no stack gating.
package acceptance

import (
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

func TestT13ProductCatalog(t *testing.T) {
	report := platformtest.Run(t, "scenarios/t13-product-catalog.yaml")
	if !report.Passed {
		t.Fatalf("T13 evidence failed: %s", report.Summary)
	}
}
