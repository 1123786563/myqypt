// Package acceptance hosts black-box acceptance journeys that compose the
// merged sub-issue deliverables against a real docker compose stack
// (platform-api + postgres + casdoor) and prove them end to end.
package acceptance

import (
	"os"
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// stackEnv gates the T01 journey: the scenario only runs when the operator
// explicitly opted in with T01_ACCEPTANCE_STACK=1, so a stack-less
// `go test ./...` stays green. Anything else skips with the exact commands:
// one to bring the stack up, and the volume-dropping reset every rerun
// needs — the journey proves the FIRST bind (201), so a warm stack whose
// platform-postgres-data named volume survived a previous run fails the
// stale-state precheck. Because the #100 harness redacts all driver-side
// text from reports and evidence, this skip message is the only channel
// that can tell the operator any of this.
const stackEnv = "T01_ACCEPTANCE_STACK"

func TestT01IdentityBinding(t *testing.T) {
	if os.Getenv(stackEnv) != "1" {
		t.Skipf("%s not set; skipping T01 acceptance journey.\n"+
			"The journey proves the FIRST bind (201) and needs a clean platform database: reset any previous stack before (re)running.\n"+
			"Start the stack:\n%s\n"+
			"Reset before each rerun (the named postgres volume survives a plain down/up):\n%s",
			stackEnv, stackStartupCommand, stackResetCommand)
	}

	report := platformtest.Run(t, "scenarios/t01-identity-binding.yaml")
	if !report.Passed {
		t.Fatalf("t01-identity-binding failed: reason=%s summary=%s evidence=%s",
			report.FailureReason, report.Summary, report.EvidencePath)
	}
	t.Logf("t01-identity-binding passed: evidence=%s", report.EvidencePath)
}
