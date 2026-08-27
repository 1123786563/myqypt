// T04 acceptance journey: the authenticated user explicitly creates a
// business tenant through the public contract endpoint and becomes its
// only owner — atomic bundle (tenant + 1:1 billing customer + single
// active owner membership), same-key replay convergence, different-key
// second entity, immediate T03 list/select visibility, and refusal
// paths that never leak an oracle or a stray write — through the same
// compose stack as T01/T02/T03 (platform-api + postgres + casdoor).
package acceptance

import (
	"os"
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// t04StackEnv gates the T04 journey: the scenario only runs when the
// operator explicitly opted in with T04_ACCEPTANCE_STACK=1, so a
// stack-less `go test ./...` stays green. Anything else skips with the
// exact commands: one to bring the stack up (note the audience override:
// the T04 application mints aud=t04-acceptance while the compose default
// is T01's), and the volume-dropping reset every rerun needs — the
// journey proves a fresh first bind and a fresh first creation (201)
// before anything else, so a warm stack whose platform-postgres-data
// named volume survived a previous run fails the stale-state precheck.
const t04StackEnv = "T04_ACCEPTANCE_STACK"

func TestT04BusinessTenant(t *testing.T) {
	if os.Getenv(t04StackEnv) != "1" {
		t.Skipf("%s not set; skipping T04 acceptance journey.\n"+
			"The journey proves fresh first binds and fresh business tenant creations (201) and needs a clean platform database: reset any previous stack before (re)running.\n"+
			"Start the stack (note the audience override: the T04 application mints aud=t04-acceptance while the compose default is T01's):\n%s\n"+
			"Reset before each rerun (the named postgres volume survives a plain down/up):\n%s",
			t04StackEnv, t04StackStartupCommand, stackResetCommand)
	}

	report := platformtest.Run(t, "scenarios/t04-business-tenant.yaml")
	if !report.Passed {
		t.Fatalf("t04-business-tenant failed: reason=%s summary=%s evidence=%s",
			report.FailureReason, report.Summary, report.EvidencePath)
	}
	t.Logf("t04-business-tenant passed: evidence=%s", report.EvidencePath)
}
