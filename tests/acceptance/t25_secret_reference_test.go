// T25 acceptance journey: the Platform stores only a secret_ref and the
// development environment commits no Secret to the repository — proven
// through the real service, the concrete in-process provider port, and a
// recording evidence sink at the platformtest seam. The ticket has no
// HTTP contract, so the journey runs in-process with no stack gating.
package acceptance

import (
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

func TestT25SecretReference(t *testing.T) {
	report := platformtest.Run(t, "scenarios/t25-secret-reference.yaml")
	if !report.Passed {
		t.Fatalf("T25 evidence failed: %s", report.Summary)
	}
}
