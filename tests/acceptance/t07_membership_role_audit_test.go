// T07 acceptance journey: 邀请、激活、拒绝、角色变更和移除产生不可变
// Audit Event — proven through the real service, the concrete
// in-process audit ledger, and a recording evidence sink at the
// platformtest seam. The ticket has no HTTP contract, so the journey
// runs in-process with no stack gating.
package acceptance

import (
	"testing"

	"github.com/1123786563/myqypt/tests/platformtest"
)

func TestT07MembershipRoleAudit(t *testing.T) {
	report := platformtest.Run(t, "scenarios/t07-membership-role-audit.yaml")
	if !report.Passed {
		t.Fatalf("T07 evidence failed: %s", report.Summary)
	}
}
