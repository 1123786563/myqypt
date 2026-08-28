// T06 acceptance journey: Owner, Admin, Billing Member, and Member gain
// their own visible, non-escalating operations — observed through the
// same compose stack as T01-T05 (platform-api + postgres + casdoor).
// The journey proves the four exact, pairwise-distinct capability sets
// at the new contract endpoint, the member/billing_member invitation
// refusals without an oracle or a stray row, the invited/never-invited/
// unknown-tenant 404s, the 401 credential denials, the admin invite
// path end to end, and the byte-identical replay body.
package acceptance

import (
	"os"
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// t06StackEnv gates the T06 journey: the scenario only runs when the
// operator explicitly opted in with T06_ACCEPTANCE_STACK=1, so a
// stack-less `go test ./...` stays green. Anything else skips with the
// exact commands: one to bring the stack up (note the audience override
// — the T06 application mints aud=t06-acceptance while the compose
// default is T01's — and the port-override file remapping the published
// host ports so the stack never collides with the ports other projects
// hold on this machine), and the volume-dropping reset every rerun
// needs — the journey proves fresh first binds and fresh first
// invitations (201) before anything else, so a warm stack whose
// platform-postgres-data named volume survived a previous run fails the
// stale-state precheck.
const t06StackEnv = "T06_ACCEPTANCE_STACK"

func TestT06PlatformRoles(t *testing.T) {
	if os.Getenv(t06StackEnv) != "1" {
		t.Skipf("%s not set; skipping T06 acceptance journey.\n"+
			"The journey proves fresh first binds and fresh first invitations (201) and needs a clean platform database: reset any previous stack before (re)running.\n"+
			"Start the stack (note the audience override and the port-override file; create /tmp/t06-port-override.yaml remapping platform-api to 18080, postgres to 25432, and casdoor to 18000 if it is missing):\n%s\n"+
			"Reset before each rerun (the named postgres volume survives a plain down/up):\n%s",
			t06StackEnv, t06StackStartupCommand, t06StackResetCommand)
	}

	report := platformtest.Run(t, "scenarios/t06-platform-roles.yaml")
	if !report.Passed {
		t.Fatalf("t06-platform-roles failed: reason=%s summary=%s evidence=%s",
			report.FailureReason, report.Summary, report.EvidencePath)
	}
	t.Logf("t06-platform-roles passed: evidence=%s", report.EvidencePath)
}
