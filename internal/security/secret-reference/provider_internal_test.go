package secretreference

import (
	"context"
	"testing"
)

// TestInProcessProviderPortRegistersExternalIDBeforeRetriableWork proves
// the concrete adapter's idempotency semantics (design ruling 4): the
// external id is registered under the idempotency key before any
// retryable work continues, so a replay answers the same resource id,
// the provider effect happens exactly once per key, and a different key
// is a new effect with a fresh id.
func TestInProcessProviderPortRegistersExternalIDBeforeRetriableWork(t *testing.T) {
	port := NewInProcessProviderPort().(*inProcessProviderPort)
	ctx := context.Background()
	first := SecretReferenceCommand{TenantID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e", SecretRef: "provider-alpha/credential-1", IdempotencyKey: "t25-acceptance"}

	accepted, err := port.Apply(ctx, first)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if accepted.Outcome != "accepted" || accepted.ResourceID == "" {
		t.Fatalf("first Apply = %+v, want {non-empty accepted}", accepted)
	}
	if port.effects != 1 {
		t.Fatalf("provider effects = %d, want 1", port.effects)
	}

	replay, err := port.Apply(ctx, first)
	if err != nil {
		t.Fatalf("replay Apply: %v", err)
	}
	if replay.ResourceID != accepted.ResourceID || replay.Outcome != "duplicate" {
		t.Fatalf("replay Apply = %+v, want {%s duplicate}", replay, accepted.ResourceID)
	}
	if port.effects != 1 {
		t.Fatalf("provider effects after replay = %d, want 1 (replay converges)", port.effects)
	}

	second := first
	second.IdempotencyKey = "t25-acceptance-2"
	fresh, err := port.Apply(ctx, second)
	if err != nil {
		t.Fatalf("new-key Apply: %v", err)
	}
	if fresh.ResourceID == accepted.ResourceID || fresh.Outcome != "accepted" {
		t.Fatalf("new-key Apply = %+v, want a fresh accepted resource", fresh)
	}
	if port.effects != 2 {
		t.Fatalf("provider effects after new key = %d, want 2", port.effects)
	}
}
