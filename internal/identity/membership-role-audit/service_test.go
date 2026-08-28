package membershiproleaudit_test

import (
	"context"
	"errors"
	"testing"

	feature "github.com/1123786563/myqypt/internal/identity/membership-role-audit"
)

// recordingPort counts Apply calls and can inject a classified retryable
// failure for the first failCalls deliveries before succeeding. The
// outcome token reflects successful effects only: the first success is
// "accepted", later successes are "duplicate" replays.
type recordingPort struct {
	calls      int
	successes  int
	failCalls  int
	resourceID string
}

func (p *recordingPort) Apply(_ context.Context, _ feature.MembershipRoleAuditCommand) (feature.MembershipRoleAuditResult, error) {
	p.calls++
	if p.calls <= p.failCalls {
		return feature.MembershipRoleAuditResult{}, feature.ErrAuditUnavailable
	}
	p.successes++
	outcome := "accepted"
	if p.successes > 1 {
		outcome = "duplicate"
	}
	return feature.MembershipRoleAuditResult{ResourceID: p.resourceID, Outcome: outcome}, nil
}

type inMemoryTx struct{ runs int }

func (t *inMemoryTx) Run(ctx context.Context, fn func(context.Context) error) error {
	t.runs++
	return fn(ctx)
}

// memoryEvidence captures every record as an exact triple for the
// content-minimization assertions.
type memoryEvidence struct{ triples [][3]string }

func (m *memoryEvidence) Record(_ context.Context, key, resourceID, outcome string) error {
	m.triples = append(m.triples, [3]string{key, resourceID, outcome})
	return nil
}

func validCommand() feature.MembershipRoleAuditCommand {
	return feature.MembershipRoleAuditCommand{
		TenantID:       "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e",
		MembershipID:   "membership-1",
		Action:         "invite",
		IdempotencyKey: "t07-acceptance",
	}
}

// TestMembershipRoleAuditRejectsMissingTenantBeforePort proves the
// first guarantee limb: TenantID is mandatory and the rejection happens
// before any outbound port call or evidence record.
func TestMembershipRoleAuditRejectsMissingTenantBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "audit-1"}
	evidence := &memoryEvidence{}
	service := feature.NewMembershipRoleAuditService(&inMemoryTx{}, port, evidence)

	cmd := validCommand()
	cmd.TenantID = ""
	_, err := service.Execute(context.Background(), cmd)

	if !errors.Is(err, feature.ErrTenantRequired) {
		t.Fatalf("Execute error = %v, want ErrTenantRequired", err)
	}
	if port.calls != 0 {
		t.Fatalf("outbound port calls = %d, want 0 (rejected before the port)", port.calls)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records = %d, want 0", len(evidence.triples))
	}
}

// TestMembershipRoleAuditRejectsMissingIdempotencyKeyBeforePort proves
// the second guarantee limb: the idempotency key is mandatory and
// rejected before any side effect.
func TestMembershipRoleAuditRejectsMissingIdempotencyKeyBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "audit-1"}
	evidence := &memoryEvidence{}
	service := feature.NewMembershipRoleAuditService(&inMemoryTx{}, port, evidence)

	cmd := validCommand()
	cmd.IdempotencyKey = ""
	_, err := service.Execute(context.Background(), cmd)

	if !errors.Is(err, feature.ErrIdempotencyKeyRequired) {
		t.Fatalf("Execute error = %v, want ErrIdempotencyKeyRequired", err)
	}
	if port.calls != 0 {
		t.Fatalf("outbound port calls = %d, want 0", port.calls)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records = %d, want 0", len(evidence.triples))
	}
}

// TestMembershipRoleAuditRejectsMissingMembershipBeforePort proves the
// third guarantee limb: an audit event without its membership subject
// is rejected before the outbound port — an event must name what it is
// about.
func TestMembershipRoleAuditRejectsMissingMembershipBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "audit-1"}
	evidence := &memoryEvidence{}
	service := feature.NewMembershipRoleAuditService(&inMemoryTx{}, port, evidence)

	cmd := validCommand()
	cmd.MembershipID = ""
	_, err := service.Execute(context.Background(), cmd)

	if !errors.Is(err, feature.ErrMembershipRequired) {
		t.Fatalf("Execute error = %v, want ErrMembershipRequired", err)
	}
	if port.calls != 0 {
		t.Fatalf("outbound port calls = %d, want 0", port.calls)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records = %d, want 0", len(evidence.triples))
	}
}

// TestMembershipRoleAuditRejectsUnknownActionBeforePort enforces the
// closed five-action vocabulary (invite, activate, reject, role_change,
// remove — invitation, activation, rejection, role change, removal): any
// other action token, including the empty one, is rejected before the
// outbound port with the stable classified error.
func TestMembershipRoleAuditRejectsUnknownActionBeforePort(t *testing.T) {
	for _, action := range []string{"", "escalate", "INVITE"} {
		port := &recordingPort{resourceID: "audit-1"}
		evidence := &memoryEvidence{}
		service := feature.NewMembershipRoleAuditService(&inMemoryTx{}, port, evidence)

		cmd := validCommand()
		cmd.Action = action
		_, err := service.Execute(context.Background(), cmd)

		if !errors.Is(err, feature.ErrActionInvalid) {
			t.Fatalf("action %q: Execute error = %v, want ErrActionInvalid", action, err)
		}
		if port.calls != 0 {
			t.Fatalf("action %q: outbound port calls = %d, want 0", action, port.calls)
		}
		if len(evidence.triples) != 0 {
			t.Fatalf("action %q: evidence records = %d, want 0", action, len(evidence.triples))
		}
	}
}

// TestMembershipRoleAuditRejectsIncompleteRoleChangeBeforePort: a
// role_change event must carry both the role before and the role after
// — the transition is the auditable fact. Anything less is rejected
// before the outbound port with the same classified action error.
func TestMembershipRoleAuditRejectsIncompleteRoleChangeBeforePort(t *testing.T) {
	for name, cmd := range map[string]feature.MembershipRoleAuditCommand{
		"missing_before": {TenantID: validCommand().TenantID, MembershipID: "membership-1", Action: "role_change", RoleAfter: "admin", IdempotencyKey: "t07-guard"},
		"missing_after":  {TenantID: validCommand().TenantID, MembershipID: "membership-1", Action: "role_change", RoleBefore: "member", IdempotencyKey: "t07-guard"},
	} {
		port := &recordingPort{resourceID: "audit-1"}
		evidence := &memoryEvidence{}
		service := feature.NewMembershipRoleAuditService(&inMemoryTx{}, port, evidence)

		_, err := service.Execute(context.Background(), cmd)

		if !errors.Is(err, feature.ErrActionInvalid) {
			t.Fatalf("%s: Execute error = %v, want ErrActionInvalid", name, err)
		}
		if port.calls != 0 {
			t.Fatalf("%s: outbound port calls = %d, want 0", name, port.calls)
		}
		if len(evidence.triples) != 0 {
			t.Fatalf("%s: evidence records = %d, want 0", name, len(evidence.triples))
		}
	}
}

// TestMembershipRoleAuditAcceptedAppliesOnceAndRecordsEvidence proves
// the accepted path: exactly one outbound effect inside one transaction
// and one content-minimized evidence row keyed by the idempotency key.
func TestMembershipRoleAuditAcceptedAppliesOnceAndRecordsEvidence(t *testing.T) {
	port := &recordingPort{resourceID: "audit-1"}
	evidence := &memoryEvidence{}
	tx := &inMemoryTx{}
	service := feature.NewMembershipRoleAuditService(tx, port, evidence)

	result, err := service.Execute(context.Background(), validCommand())

	if err != nil {
		t.Fatalf("Execute error = %v, want nil", err)
	}
	if result.Outcome != "accepted" || result.ResourceID != "audit-1" {
		t.Fatalf("result = %+v, want {audit-1 accepted}", result)
	}
	if port.calls != 1 || tx.runs != 1 {
		t.Fatalf("port calls = %d, tx runs = %d, want 1 and 1", port.calls, tx.runs)
	}
	if len(evidence.triples) != 1 {
		t.Fatalf("evidence records = %d, want 1", len(evidence.triples))
	}
	want := [3]string{"t07-acceptance", "audit-1", "accepted"}
	if evidence.triples[0] != want {
		t.Fatalf("evidence triple = %v, want %v", evidence.triples[0], want)
	}
}

// TestMembershipRoleAuditReplayConvergesToSingleEvent proves the
// idempotency semantics: a replayed delivery under the same key answers
// the same event id as a duplicate — never a second event — while each
// delivery still leaves its own evidence row.
func TestMembershipRoleAuditReplayConvergesToSingleEvent(t *testing.T) {
	port := &recordingPort{resourceID: "audit-1"}
	evidence := &memoryEvidence{}
	service := feature.NewMembershipRoleAuditService(&inMemoryTx{}, port, evidence)

	first, firstErr := service.Execute(context.Background(), validCommand())
	second, secondErr := service.Execute(context.Background(), validCommand())

	if firstErr != nil || secondErr != nil {
		t.Fatalf("execute errors = %v / %v, want nil", firstErr, secondErr)
	}
	if first.Outcome != "accepted" || second.Outcome != "duplicate" {
		t.Fatalf("outcomes = %s then %s, want accepted then duplicate", first.Outcome, second.Outcome)
	}
	if second.ResourceID != first.ResourceID {
		t.Fatalf("replay resource id = %s, want the original %s", second.ResourceID, first.ResourceID)
	}
	if len(evidence.triples) != 2 {
		t.Fatalf("evidence records = %d, want 2 (one per delivery)", len(evidence.triples))
	}
}

// TestMembershipRoleAuditPortFailureLeavesNoPartialState proves the
// failure path: a classified retryable port failure records zero
// evidence (no partial state), and the retry under the same key
// converges onto the single accepted event.
func TestMembershipRoleAuditPortFailureLeavesNoPartialState(t *testing.T) {
	port := &recordingPort{resourceID: "audit-1", failCalls: 1}
	evidence := &memoryEvidence{}
	service := feature.NewMembershipRoleAuditService(&inMemoryTx{}, port, evidence)

	_, failureErr := service.Execute(context.Background(), validCommand())
	if !errors.Is(failureErr, feature.ErrAuditUnavailable) {
		t.Fatalf("failure error = %v, want ErrAuditUnavailable", failureErr)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records after failure = %d, want 0", len(evidence.triples))
	}

	retried, retryErr := service.Execute(context.Background(), validCommand())
	if retryErr != nil {
		t.Fatalf("retry error = %v, want nil", retryErr)
	}
	if retried.Outcome != "accepted" {
		t.Fatalf("retry outcome = %s, want accepted", retried.Outcome)
	}
	if len(evidence.triples) != 1 {
		t.Fatalf("evidence records after retry = %d, want 1", len(evidence.triples))
	}
}

// TestMembershipRoleAuditFiveActionsEachAcceptedAndEvidenced proves the
// ticket invariant's action coverage through the service contract: the
// invitation, activation, rejection, role change, and removal actions
// each pass validation, apply once, and leave exactly one evidence row.
func TestMembershipRoleAuditFiveActionsEachAcceptedAndEvidenced(t *testing.T) {
	commands := []feature.MembershipRoleAuditCommand{
		{TenantID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e", MembershipID: "membership-1", Action: "invite", IdempotencyKey: "t07-invite"},
		{TenantID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e", MembershipID: "membership-1", Action: "activate", IdempotencyKey: "t07-activate"},
		{TenantID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e", MembershipID: "membership-1", Action: "reject", IdempotencyKey: "t07-reject"},
		{TenantID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e", MembershipID: "membership-1", Action: "role_change", RoleBefore: "member", RoleAfter: "admin", IdempotencyKey: "t07-role-change"},
		{TenantID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e", MembershipID: "membership-1", Action: "remove", IdempotencyKey: "t07-remove"},
	}
	for _, cmd := range commands {
		port := &recordingPort{resourceID: "audit-1"}
		evidence := &memoryEvidence{}
		service := feature.NewMembershipRoleAuditService(&inMemoryTx{}, port, evidence)

		result, err := service.Execute(context.Background(), cmd)

		if err != nil {
			t.Fatalf("action %q: Execute error = %v, want nil", cmd.Action, err)
		}
		if result.Outcome != "accepted" {
			t.Fatalf("action %q: outcome = %s, want accepted", cmd.Action, result.Outcome)
		}
		if len(evidence.triples) != 1 {
			t.Fatalf("action %q: evidence records = %d, want 1", cmd.Action, len(evidence.triples))
		}
	}
}
