package membershiproleaudit

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

// t07Command builds a valid command with the given action and key.
func t07Command(action, key string) MembershipRoleAuditCommand {
	return MembershipRoleAuditCommand{
		TenantID:       "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e",
		MembershipID:   "membership-1",
		Action:         action,
		IdempotencyKey: key,
	}
}

// TestInProcessAuditPortFiveActionsAppendDistinctEvents proves the
// ticket invariant at the store: the invitation, activation,
// rejection, role change, and removal actions each append exactly one
// distinct content-minimized event under the stable authority axis.
func TestInProcessAuditPortFiveActionsAppendDistinctEvents(t *testing.T) {
	port := &inProcessAuditPort{applied: make(map[string]string), events: make(map[string]auditEvent)}
	actions := []string{"invite", "activate", "reject", "role_change", "remove"}

	seen := map[string]bool{}
	for i, action := range actions {
		cmd := t07Command(action, "t07-"+action)
		if action == "role_change" {
			cmd.RoleBefore, cmd.RoleAfter = "member", "admin"
		}
		result, err := port.Apply(context.Background(), cmd)
		if err != nil {
			t.Fatalf("action %q: Apply error = %v", action, err)
		}
		wantID := fmt.Sprintf("audit-%d", i+1)
		if result.ResourceID != wantID || result.Outcome != "accepted" {
			t.Fatalf("action %q: result = %+v, want {%s accepted}", action, result, wantID)
		}
		if seen[result.ResourceID] {
			t.Fatalf("action %q: event id %s reused", action, result.ResourceID)
		}
		seen[result.ResourceID] = true
		event := port.events[result.ResourceID]
		if event.action != action || event.authority != auditAuthority || event.tenantID != cmd.TenantID {
			t.Fatalf("action %q: event axes = %+v", action, event)
		}
	}
	if port.appends != 5 || len(port.events) != 5 {
		t.Fatalf("appends = %d, events = %d, want 5 and 5", port.appends, len(port.events))
	}
}

// TestInProcessAuditPortReplayNeverOverwritesStoredEvent is the
// immutability proof at the content level (the seam can only observe
// ids and outcomes; the store must also keep the appended bytes): a
// divergent payload replayed under an existing key converges onto the
// original event — same id, duplicate outcome, stored content
// byte-identical — and consumes no phantom append.
func TestInProcessAuditPortReplayNeverOverwritesStoredEvent(t *testing.T) {
	port := &inProcessAuditPort{applied: make(map[string]string), events: make(map[string]auditEvent)}

	first := t07Command("role_change", "t07-immutable")
	first.RoleBefore, first.RoleAfter = "member", "admin"
	firstResult, err := port.Apply(context.Background(), first)
	if err != nil || firstResult.Outcome != "accepted" {
		t.Fatalf("first apply = %+v, %v", firstResult, err)
	}

	divergent := t07Command("role_change", "t07-immutable")
	divergent.RoleBefore, divergent.RoleAfter = "member", "owner"
	replayResult, err := port.Apply(context.Background(), divergent)
	if err != nil {
		t.Fatalf("divergent replay error = %v", err)
	}
	if replayResult.ResourceID != firstResult.ResourceID || replayResult.Outcome != "duplicate" {
		t.Fatalf("divergent replay = %+v, want {%s duplicate}", replayResult, firstResult.ResourceID)
	}

	want := auditEvent{
		sequence:     1,
		authority:    auditAuthority,
		tenantID:     first.TenantID,
		membershipID: first.MembershipID,
		action:       "role_change",
		roleBefore:   "member",
		roleAfter:    "admin",
	}
	if got := port.events[firstResult.ResourceID]; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored event = %+v, want the untouched original %+v", got, want)
	}

	// The divergent replay consumed no phantom append: the next fresh
	// key takes exactly the next sequence number.
	next, err := port.Apply(context.Background(), t07Command("remove", "t07-next"))
	if err != nil || next.ResourceID != "audit-2" || next.Outcome != "accepted" {
		t.Fatalf("next fresh key = %+v, %v — want {audit-2 accepted} (no phantom append)", next, err)
	}
	if port.appends != 2 {
		t.Fatalf("appends = %d, want 2", port.appends)
	}
}

// TestInProcessAuditPortConcurrentAppliesConverge gives the race
// detector real concurrent work on the registry: many goroutines
// delivering the same key must converge onto exactly one append and
// one shared event id.
func TestInProcessAuditPortConcurrentAppliesConverge(t *testing.T) {
	port := &inProcessAuditPort{applied: make(map[string]string), events: make(map[string]auditEvent)}
	const deliveries = 32

	var wg sync.WaitGroup
	ids := make([]string, deliveries)
	for i := 0; i < deliveries; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := port.Apply(context.Background(), t07Command("invite", "t07-concurrent"))
			if err != nil {
				t.Errorf("delivery %d: Apply error = %v", i, err)
				return
			}
			ids[i] = result.ResourceID
		}(i)
	}
	wg.Wait()

	for _, id := range ids {
		if id != "audit-1" {
			t.Fatalf("concurrent delivery ids diverged: %v", ids)
		}
	}
	if port.appends != 1 {
		t.Fatalf("appends = %d, want 1 (all deliveries converged)", port.appends)
	}
}
