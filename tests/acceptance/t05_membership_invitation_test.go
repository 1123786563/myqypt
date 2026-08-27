// T05 acceptance journey: the tenant's owner invites a bound user, the
// membership activates only after the invitee accepts — and until then
// the invited membership stays invisible and unusable at the
// tenant-context seam — through the same compose stack as T01-T04
// (platform-api + postgres + casdoor). The journey also proves replay
// convergence with the identical body, immediate list/select usability
// after acceptance, and every refusal path without an oracle or a
// stray write.
package acceptance

import (
	"os"
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// t05StackEnv gates the T05 journey: the scenario only runs when the
// operator explicitly opted in with T05_ACCEPTANCE_STACK=1, so a
// stack-less `go test ./...` stays green. Anything else skips with the
// exact commands: one to bring the stack up (note the audience
// override: the T05 application mints aud=t05-acceptance while the
// compose default is T01's), and the volume-dropping reset every rerun
// needs — the journey proves fresh first binds and a fresh first
// invitation (201) before anything else, so a warm stack whose
// platform-postgres-data named volume survived a previous run fails
// the stale-state precheck.
const t05StackEnv = "T05_ACCEPTANCE_STACK"

func TestT05MembershipInvitation(t *testing.T) {
	if os.Getenv(t05StackEnv) != "1" {
		t.Skipf("%s not set; skipping T05 acceptance journey.\n"+
			"The journey proves fresh first binds and a fresh first membership invitation (201) and needs a clean platform database: reset any previous stack before (re)running.\n"+
			"Start the stack (note the audience override: the T05 application mints aud=t05-acceptance while the compose default is T01's):\n%s\n"+
			"Reset before each rerun (the named postgres volume survives a plain down/up):\n%s",
			t05StackEnv, t05StackStartupCommand, stackResetCommand)
	}

	report := platformtest.Run(t, "scenarios/t05-membership-invitation.yaml")
	if !report.Passed {
		t.Fatalf("t05-membership-invitation failed: reason=%s summary=%s evidence=%s",
			report.FailureReason, report.Summary, report.EvidencePath)
	}
	t.Logf("t05-membership-invitation passed: evidence=%s", report.EvidencePath)
}
