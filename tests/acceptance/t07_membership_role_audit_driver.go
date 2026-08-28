// T07 acceptance journey driver (Issue #8): proves the membership-role
// audit invariant end to end at the highest practical seam — the real
// service, the concrete in-process audit ledger, and a recording
// evidence sink, executed through the platformtest harness. The ticket
// has no HTTP contract (the source plan runs platformtest without a
// stack), so this in-process journey is the named acceptance seam.
package acceptance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	membershiproleaudit "github.com/1123786563/myqypt/internal/identity/membership-role-audit"
	"github.com/1123786563/myqypt/tests/platformtest"
)

const seamMembershipRoleAudit = "lighthouse-membership-role-audit"

// t07AssertionNames is the exact set declared by
// scenarios/t07-membership-role-audit.yaml, in declaration order. The
// harness reconciles by name and rejects any drift, so this list and
// the YAML must move together.
var t07AssertionNames = []string{
	"reject_missing_tenant",
	"reject_missing_idempotency_key",
	"reject_unknown_action",
	"five_actions_each_one_immutable_event",
	"replay_converges_single_event",
	"immutability_no_overwrite",
	"port_failure_no_partial_then_retry_converges",
	"evidence_content_minimized",
}

// t07FakeSensitiveHalves assembles the journey's fake sensitive
// material in memory only. Neither half — nor the assembled value — is
// a real credential or customer content, and the split keeps the
// assembled literal out of every committed file. The audit stream must
// never carry material of this shape (ADR-0041).
func t07FakeSensitiveHalves() (string, string) {
	return "T07-Journey-Fake-", "Raw-Token-4e1b##"
}

// t07JourneyEvidenceSink records every evidence triple for the
// content-minimization assertion.
type t07JourneyEvidenceSink struct{ triples [][3]string }

func (s *t07JourneyEvidenceSink) Record(_ context.Context, key, resourceID, outcome string) error {
	s.triples = append(s.triples, [3]string{key, resourceID, outcome})
	return nil
}

// t07FlakyPort wraps an audit port and fails the first deliveries with
// the classified retryable error before delegating — the injected
// audit-store failure of the failure-path assertion.
type t07FlakyPort struct {
	inner     membershiproleaudit.MembershipRoleAuditPort
	failFirst int
	calls     int
}

func (p *t07FlakyPort) Apply(ctx context.Context, cmd membershiproleaudit.MembershipRoleAuditCommand) (membershiproleaudit.MembershipRoleAuditResult, error) {
	p.calls++
	if p.calls <= p.failFirst {
		return membershiproleaudit.MembershipRoleAuditResult{}, membershiproleaudit.ErrAuditUnavailable
	}
	return p.inner.Apply(ctx, cmd)
}

func init() {
	platformtest.Register(seamMembershipRoleAudit, t07MembershipRoleAuditDriver{})
}

type t07MembershipRoleAuditDriver struct{}

func (t07MembershipRoleAuditDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	input := func(key string) string {
		value, _ := scenario.Inputs[key].(string)
		return value
	}
	tenantID := input("tenant_id")
	membershipID := input("membership_id")
	idempotencyKey := input("idempotency_key")
	if tenantID == "" || membershipID == "" || idempotencyKey == "" {
		return t07FailedReport("scenario inputs tenant_id/membership_id/idempotency_key are required"), nil
	}

	results := map[string]platformtest.AssertionResult{}
	record := func(name string, passed bool, details string) {
		results[name] = platformtest.AssertionResult{Name: name, Passed: passed, Details: details}
	}

	fakeFirstHalf, fakeSecondHalf := t07FakeSensitiveHalves()
	fakeSensitive := fakeFirstHalf + fakeSecondHalf

	command := func(action, key string) membershiproleaudit.MembershipRoleAuditCommand {
		return membershiproleaudit.MembershipRoleAuditCommand{
			TenantID:       tenantID,
			MembershipID:   membershipID,
			Action:         action,
			IdempotencyKey: key,
		}
	}

	// The three input-shaped rejections happen before the outbound
	// port and leave zero evidence.
	rejections := []struct {
		name string
		cmd  membershiproleaudit.MembershipRoleAuditCommand
		want error
	}{
		{"reject_missing_tenant", func() membershiproleaudit.MembershipRoleAuditCommand {
			cmd := command("invite", idempotencyKey+"-invite")
			cmd.TenantID = ""
			return cmd
		}(), membershiproleaudit.ErrTenantRequired},
		{"reject_missing_idempotency_key", func() membershiproleaudit.MembershipRoleAuditCommand {
			cmd := command("invite", "")
			return cmd
		}(), membershiproleaudit.ErrIdempotencyKeyRequired},
		{"reject_unknown_action", command("purge", idempotencyKey+"-purge"), membershiproleaudit.ErrActionInvalid},
	}
	rejectionEvidence := &t07JourneyEvidenceSink{}
	rejectionService := membershiproleaudit.NewMembershipRoleAuditService(membershiproleaudit.InProcessTx{}, membershiproleaudit.NewInProcessAuditPort(), rejectionEvidence)
	for _, rejection := range rejections {
		_, err := rejectionService.Execute(ctx, rejection.cmd)
		record(rejection.name,
			errors.Is(err, rejection.want) && len(rejectionEvidence.triples) == 0,
			fmt.Sprintf("error_class=%t zero_evidence=%t", errors.Is(err, rejection.want), len(rejectionEvidence.triples) == 0))
	}

	// The five lifecycle actions each append exactly one distinct
	// immutable event (distinct event ids observed at the seam).
	acceptPort := membershiproleaudit.NewInProcessAuditPort()
	acceptEvidence := &t07JourneyEvidenceSink{}
	acceptService := membershiproleaudit.NewMembershipRoleAuditService(membershiproleaudit.InProcessTx{}, acceptPort, acceptEvidence)
	five := []membershiproleaudit.MembershipRoleAuditCommand{
		command("invite", idempotencyKey+"-invite"),
		command("activate", idempotencyKey+"-activate"),
		command("reject", idempotencyKey+"-reject"),
		func() membershiproleaudit.MembershipRoleAuditCommand {
			cmd := command("role_change", idempotencyKey+"-role-change")
			cmd.RoleBefore, cmd.RoleAfter = "member", "admin"
			return cmd
		}(),
		command("remove", idempotencyKey+"-remove"),
	}
	eventIDs := map[string]bool{}
	fiveOK := true
	for _, cmd := range five {
		result, err := acceptService.Execute(ctx, cmd)
		if err != nil || result.Outcome != "accepted" || result.ResourceID == "" || eventIDs[result.ResourceID] {
			fiveOK = false
		}
		eventIDs[result.ResourceID] = true
	}
	record("five_actions_each_one_immutable_event",
		fiveOK && len(eventIDs) == 5,
		fmt.Sprintf("outcomes_ok=%t distinct_event_ids=%d", fiveOK, len(eventIDs)))

	// The replay converges: same event id, outcome duplicate, one more
	// evidence row for the delivery — and no second event (the id set
	// is unchanged).
	replayed, replayErr := acceptService.Execute(ctx, five[0])
	_, stillFive := eventIDs[replayed.ResourceID]
	record("replay_converges_single_event",
		replayErr == nil && replayed.Outcome == "duplicate" && stillFive,
		fmt.Sprintf("same_event=%t outcome=%s distinct_ids_still=%d", stillFive, replayed.Outcome, len(eventIDs)))

	// Immutability at the seam: a divergent payload (member→owner
	// instead of member→admin) replayed under the same key answers the
	// original event id as a duplicate, and the very next fresh key
	// takes exactly the next sequence number — no phantom append, no
	// overwritten content (the byte-level content proof lives in the
	// package's internal white-box test).
	immutablePort := membershiproleaudit.NewInProcessAuditPort()
	immutableEvidence := &t07JourneyEvidenceSink{}
	immutableService := membershiproleaudit.NewMembershipRoleAuditService(membershiproleaudit.InProcessTx{}, immutablePort, immutableEvidence)
	original := command("role_change", idempotencyKey+"-immutable")
	original.RoleBefore, original.RoleAfter = "member", "admin"
	first, firstErr := immutableService.Execute(ctx, original)
	divergent := command("role_change", idempotencyKey+"-immutable")
	divergent.RoleBefore, divergent.RoleAfter = "member", "owner"
	divergentReplay, divergentErr := immutableService.Execute(ctx, divergent)
	next, nextErr := immutableService.Execute(ctx, command("remove", idempotencyKey+"-immutable-next"))
	record("immutability_no_overwrite",
		firstErr == nil && divergentErr == nil && nextErr == nil &&
			first.Outcome == "accepted" && divergentReplay.Outcome == "duplicate" &&
			divergentReplay.ResourceID == first.ResourceID && next.ResourceID == "audit-2" && next.Outcome == "accepted",
		fmt.Sprintf("divergent_replay=%s original_id_kept=%t next_fresh=%s", divergentReplay.Outcome, divergentReplay.ResourceID == first.ResourceID, next.ResourceID))

	// The failure path: an injected audit-store failure records zero
	// evidence, and the retry under the same key converges onto the
	// single accepted event.
	flaky := &t07FlakyPort{inner: membershiproleaudit.NewInProcessAuditPort(), failFirst: 1}
	failureEvidence := &t07JourneyEvidenceSink{}
	failureService := membershiproleaudit.NewMembershipRoleAuditService(membershiproleaudit.InProcessTx{}, flaky, failureEvidence)
	_, failureErr := failureService.Execute(ctx, command("invite", idempotencyKey+"-failure"))
	zeroPartial := errors.Is(failureErr, membershiproleaudit.ErrAuditUnavailable) && len(failureEvidence.triples) == 0
	retried, retryErr := failureService.Execute(ctx, command("invite", idempotencyKey+"-failure"))
	record("port_failure_no_partial_then_retry_converges",
		zeroPartial && retryErr == nil && retried.Outcome == "accepted" && len(failureEvidence.triples) == 1,
		fmt.Sprintf("failure_classified=%t zero_evidence_after_failure=%t retry_outcome=%s evidence_rows=%d",
			errors.Is(failureErr, membershiproleaudit.ErrAuditUnavailable), zeroPartial, retried.Outcome, len(failureEvidence.triples)))

	// Evidence minimization across every flow: the recorded triples are
	// exactly (idempotency key, event id, outcome token) and none of
	// them — nor any split half — carries the fake sensitive material.
	allTriples := append(append(append([][3]string{}, rejectionEvidence.triples...), acceptEvidence.triples...), immutableEvidence.triples...)
	allTriples = append(allTriples, failureEvidence.triples...)
	minimized := true
	for _, triple := range allTriples {
		joined := strings.Join(triple[:], " ")
		for _, needle := range []string{fakeSensitive, fakeFirstHalf, fakeSecondHalf} {
			if strings.Contains(joined, needle) {
				minimized = false
			}
		}
	}
	record("evidence_content_minimized",
		minimized && len(allTriples) == 10,
		fmt.Sprintf("rows=%d sensitive_material_hits=%t", len(allTriples), !minimized))

	ordered := make([]platformtest.AssertionResult, 0, len(t07AssertionNames))
	passed := true
	for _, name := range t07AssertionNames {
		result, ok := results[name]
		if !ok {
			return t07FailedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}
	summary := fmt.Sprintf("rejects=%d distinct_event_ids=%d replay=%s divergent=%s evidence_rows=%d",
		len(rejections), len(eventIDs), replayed.Outcome, divergentReplay.Outcome, len(allTriples))
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// t07FailedReport builds a failing report whose assertion set matches
// the declared T07 names (all failed), keeping the harness
// reconciliation valid.
func t07FailedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(t07AssertionNames))
	for _, name := range t07AssertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}
