// T03 acceptance journey: the user explicitly selects a tenant context
// through the public contract endpoints and switches between their
// tenants, with every selection validated against an active membership
// server-side, through the same compose stack as T01/T02 (platform-api +
// postgres + casdoor).
package acceptance

import (
	"os"
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// t03StackEnv gates the T03 journey: the scenario only runs when the
// operator explicitly opted in with T03_ACCEPTANCE_STACK=1, so a
// stack-less `go test ./...` stays green. Anything else skips with the
// exact commands: one to bring the stack up, and the volume-dropping
// reset every rerun needs — the journey proves fresh FIRST binds (201)
// before any selection, so a warm stack whose platform-postgres-data
// named volume survived a previous run fails the stale-state precheck.
// Because the #100 harness redacts all driver-side text from reports and
// evidence, this skip message is the only channel that can tell the
// operator any of this.
const t03StackEnv = "T03_ACCEPTANCE_STACK"

func TestT03TenantContext(t *testing.T) {
	if os.Getenv(t03StackEnv) != "1" {
		t.Skipf("%s not set; skipping T03 acceptance journey.\n"+
			"The journey proves fresh first binds (201) before the selection flow and needs a clean platform database: reset any previous stack before (re)running.\n"+
			"Start the stack (note the audience override: the T03 application mints aud=t03-acceptance while the compose default is T01's):\n%s\n"+
			"Reset before each rerun (the named postgres volume survives a plain down/up):\n%s",
			t03StackEnv, t03StackStartupCommand, stackResetCommand)
	}

	report := platformtest.Run(t, "scenarios/t03-tenant-context.yaml")
	if !report.Passed {
		t.Fatalf("t03-tenant-context failed: reason=%s summary=%s evidence=%s",
			report.FailureReason, report.Summary, report.EvidencePath)
	}
	t.Logf("t03-tenant-context passed: evidence=%s", report.EvidencePath)
}
