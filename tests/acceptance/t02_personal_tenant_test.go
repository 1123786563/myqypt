// T02 acceptance journey: the first identity bind provisions the new
// user's complete personal-tenant bundle through the same compose stack
// as T01 (platform-api + postgres + casdoor).
package acceptance

import (
	"os"
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// t02StackEnv gates the T02 journey: the scenario only runs when the
// operator explicitly opted in with T02_ACCEPTANCE_STACK=1, so a
// stack-less `go test ./...` stays green. Anything else skips with the
// exact commands: one to bring the stack up, and the volume-dropping
// reset every rerun needs — the journey proves the FIRST bind (201) and
// the fresh provisioning of the tenant bundle, so a warm stack whose
// platform-postgres-data named volume survived a previous run fails the
// stale-state precheck. Because the #100 harness redacts all
// driver-side text from reports and evidence, this skip message is the
// only channel that can tell the operator any of this.
const t02StackEnv = "T02_ACCEPTANCE_STACK"

func TestT02PersonalTenant(t *testing.T) {
	if os.Getenv(t02StackEnv) != "1" {
		t.Skipf("%s not set; skipping T02 acceptance journey.\n"+
			"The journey proves the FIRST bind (201) with fresh tenant provisioning and needs a clean platform database: reset any previous stack before (re)running.\n"+
			"Start the stack (note the audience override: the T02 application mints aud=t02-acceptance while the compose default is T01's):\n%s\n"+
			"Reset before each rerun (the named postgres volume survives a plain down/up):\n%s",
			t02StackEnv, t02StackStartupCommand, stackResetCommand)
	}

	report := platformtest.Run(t, "scenarios/t02-personal-tenant.yaml")
	if !report.Passed {
		t.Fatalf("t02-personal-tenant failed: reason=%s summary=%s evidence=%s",
			report.FailureReason, report.Summary, report.EvidencePath)
	}
	t.Logf("t02-personal-tenant passed: evidence=%s", report.EvidencePath)
}
