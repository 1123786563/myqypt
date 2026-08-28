package secretreference_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	feature "github.com/1123786563/myqypt/internal/security/secret-reference"
)

// t25FakeRawSecret is deliberately NOT a real credential: it exists only
// as rejected-input material proving the reference grammar refuses raw
// secret values (the ticket invariant: Platform 只保存 secret_ref). The
// literal is split so the assembled value never appears verbatim in any
// committed file.
const t25FakeRawSecret = "T25-Fake-Raw-" + "Secret-Value-7f3a!!"

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

func (p *recordingPort) Apply(_ context.Context, _ feature.SecretReferenceCommand) (feature.SecretReferenceResult, error) {
	p.calls++
	if p.calls <= p.failCalls {
		return feature.SecretReferenceResult{}, feature.ErrProviderUnavailable
	}
	p.successes++
	outcome := "accepted"
	if p.successes > 1 {
		outcome = "duplicate"
	}
	return feature.SecretReferenceResult{ResourceID: p.resourceID, Outcome: outcome}, nil
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

func validCommand() feature.SecretReferenceCommand {
	return feature.SecretReferenceCommand{
		TenantID:       "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88e",
		SecretRef:      "provider-alpha/credential-1",
		IdempotencyKey: "t25-acceptance",
	}
}

// TestSecretReferenceRejectsMissingTenantBeforePort proves the first
// scaffold guarantee limb: TenantID is mandatory and the rejection
// happens before any outbound port call or evidence record.
func TestSecretReferenceRejectsMissingTenantBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "resource-a"}
	evidence := &memoryEvidence{}
	service := feature.NewSecretReferenceService(&inMemoryTx{}, port, evidence)

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

// TestSecretReferenceRejectsMissingIdempotencyKeyBeforePort proves the
// second guarantee limb: the idempotency key is mandatory and rejected
// before any side effect.
func TestSecretReferenceRejectsMissingIdempotencyKeyBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "resource-a"}
	evidence := &memoryEvidence{}
	service := feature.NewSecretReferenceService(&inMemoryTx{}, port, evidence)

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

// TestSecretReferenceRejectsRawSecretValueBeforePort enforces the ticket
// invariant at the contract shape: a raw secret value (whitespace,
// punctuation, secret-looking payload) is not a secret_ref and is
// rejected before the outbound port with a stable classified error. An
// empty reference is the same classified class.
func TestSecretReferenceRejectsRawSecretValueBeforePort(t *testing.T) {
	port := &recordingPort{resourceID: "resource-a"}
	evidence := &memoryEvidence{}
	service := feature.NewSecretReferenceService(&inMemoryTx{}, port, evidence)

	for name, ref := range map[string]string{
		"raw secret value": t25FakeRawSecret,
		"empty reference":  "",
		"uppercase token":  "Provider-Alpha/Credential",
		"quoted payload":   `provider-alpha/"credential-1"`,
	} {
		cmd := validCommand()
		cmd.SecretRef = ref
		_, err := service.Execute(context.Background(), cmd)
		if !errors.Is(err, feature.ErrSecretRefInvalid) {
			t.Fatalf("%s: Execute error = %v, want ErrSecretRefInvalid", name, err)
		}
	}
	if port.calls != 0 {
		t.Fatalf("outbound port calls = %d, want 0", port.calls)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records = %d, want 0", len(evidence.triples))
	}
}

// TestSecretReferenceAppliesOnceAndRecordsMinimizedEvidence proves the
// accepted path: exactly one outbound call inside the transaction, one
// evidence record carrying only the idempotency key, the external
// resource id, and the outcome token.
func TestSecretReferenceAppliesOnceAndRecordsMinimizedEvidence(t *testing.T) {
	port := &recordingPort{resourceID: "resource-a"}
	evidence := &memoryEvidence{}
	tx := &inMemoryTx{}
	service := feature.NewSecretReferenceService(tx, port, evidence)

	result, err := service.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.ResourceID != "resource-a" || result.Outcome != "accepted" {
		t.Fatalf("result = %+v, want {resource-a accepted}", result)
	}
	if port.calls != 1 {
		t.Fatalf("outbound port calls = %d, want 1", port.calls)
	}
	if tx.runs != 1 {
		t.Fatalf("transaction runs = %d, want 1", tx.runs)
	}
	if len(evidence.triples) != 1 {
		t.Fatalf("evidence records = %d, want 1", len(evidence.triples))
	}
	key, resourceID, outcome := evidence.triples[0][0], evidence.triples[0][1], evidence.triples[0][2]
	if key != "t25-acceptance" || resourceID != "resource-a" || outcome != "accepted" {
		t.Fatalf("evidence triple = (%q %q %q), want (t25-acceptance resource-a accepted)", key, resourceID, outcome)
	}
}

// TestSecretReferenceReplayDeliversSameResource proves the delivery
// replay shape at the service seam: every delivery reaches the port and
// records its own minimized evidence row while the provider-side effect
// convergence is the concrete port's guarantee (proven in the internal
// port test and the journey).
func TestSecretReferenceReplayDeliversSameResource(t *testing.T) {
	port := &recordingPort{resourceID: "resource-a"}
	evidence := &memoryEvidence{}
	service := feature.NewSecretReferenceService(&inMemoryTx{}, port, evidence)

	first, err := service.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	second, err := service.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("replay Execute: %v", err)
	}
	if first.ResourceID != second.ResourceID {
		t.Fatalf("replay resource = %q, want the first delivery's %q", second.ResourceID, first.ResourceID)
	}
	if second.Outcome != "duplicate" {
		t.Fatalf("replay outcome = %q, want duplicate", second.Outcome)
	}
	if len(evidence.triples) != 2 {
		t.Fatalf("evidence records = %d, want one per delivery (2)", len(evidence.triples))
	}
}

// TestSecretReferenceProviderFailureRecordsNoEvidenceThenRetryConverges
// proves the failure path: a classified retryable provider failure
// leaves zero evidence (the transaction boundary), and the retry
// converges onto the single accepted effect.
func TestSecretReferenceProviderFailureRecordsNoEvidenceThenRetryConverges(t *testing.T) {
	port := &recordingPort{failCalls: 1, resourceID: "resource-a"}
	evidence := &memoryEvidence{}
	service := feature.NewSecretReferenceService(&inMemoryTx{}, port, evidence)

	_, err := service.Execute(context.Background(), validCommand())
	if !errors.Is(err, feature.ErrProviderUnavailable) {
		t.Fatalf("failed Execute error = %v, want ErrProviderUnavailable", err)
	}
	if len(evidence.triples) != 0 {
		t.Fatalf("evidence records after failure = %d, want 0", len(evidence.triples))
	}

	retry, err := service.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("retry Execute: %v", err)
	}
	if retry.ResourceID != "resource-a" || retry.Outcome != "accepted" {
		t.Fatalf("retry result = %+v, want {resource-a accepted}", retry)
	}
	if port.calls != 2 {
		t.Fatalf("port calls = %d, want 2 (failed delivery + accepted retry)", port.calls)
	}
	if len(evidence.triples) != 1 {
		t.Fatalf("evidence records after retry = %d, want 1", len(evidence.triples))
	}
}

// TestSecretReferenceEvidenceCarriesNoSecretMaterial proves the evidence
// minimization guarantee across every flow: no recorded triple contains
// the raw secret material (or either split half) that the journey used,
// and nothing beyond the key/resource/outcome vocabulary is recorded.
func TestSecretReferenceEvidenceCarriesNoSecretMaterial(t *testing.T) {
	port := &recordingPort{resourceID: "resource-a"}
	evidence := &memoryEvidence{}
	service := feature.NewSecretReferenceService(&inMemoryTx{}, port, evidence)

	cmd := validCommand()
	cmd.SecretRef = t25FakeRawSecret
	if _, err := service.Execute(context.Background(), cmd); !errors.Is(err, feature.ErrSecretRefInvalid) {
		t.Fatalf("raw secret rejection error = %v, want ErrSecretRefInvalid", err)
	}
	if _, err := service.Execute(context.Background(), validCommand()); err != nil {
		t.Fatalf("valid Execute: %v", err)
	}

	forbidden := []string{t25FakeRawSecret, "T25-Fake-Raw-", "Secret-Value-7f3a"}
	for i, triple := range evidence.triples {
		joined := strings.Join(triple[:], " ")
		for _, needle := range forbidden {
			if strings.Contains(joined, needle) {
				t.Fatalf("evidence triple %d contains secret material %q: %q", i, needle, joined)
			}
		}
	}
	if len(evidence.triples) != 1 {
		t.Fatalf("evidence records = %d, want only the accepted delivery's 1", len(evidence.triples))
	}
}
