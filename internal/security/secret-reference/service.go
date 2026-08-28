// Package secretreference delivers the T25 vertical slice (Issue #26):
// the Platform stores only a secret_ref — a provider-neutral opaque
// reference — and never a Secret value, and the development environment
// commits no Secret to the repository. The slice is the Stage-1 shape of
// the managed-secrets boundary (ADR-0026: managed secrets before
// self-hosting OpenBao): typed command/result contracts, validation
// before any side effect, a transaction boundary around the single
// outbound effect plus one content-minimized evidence record, and an
// in-process provider adapter whose external-ID registration precedes
// any retryable work so replays converge onto one effect.
package secretreference

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
)

// Classified sentinel errors. The first three are input-shaped and are
// returned before the outbound port is ever touched; ErrProviderUnavailable
// is the retryable provider failure class.
var (
	ErrTenantRequired         = errors.New("tenant context is required")
	ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
	ErrSecretRefInvalid       = errors.New("secret reference is not a valid opaque reference")
	ErrProviderUnavailable    = errors.New("secret reference provider unavailable")
)

// secretRefGrammar is the closed reference grammar (design ruling 3): a
// provider-neutral opaque token — lowercase letters, digits, dot,
// underscore, slash, or dash, starting alphanumeric, at most 200 runes.
// Raw secret values (whitespace, punctuation payloads, mixed-case
// blobs) structurally fail this grammar, which is how "Platform 只保存
// secret_ref" is enforced by contract shape rather than by trusting the
// caller.
var secretRefGrammar = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,199}$`)

// SecretReferenceCommand is the feature command: the Tenant the
// reference belongs to (the hard boundary), the opaque secret_ref, and
// the idempotency key that makes delivery replays converge.
type SecretReferenceCommand struct {
	TenantID       string
	SecretRef      string
	IdempotencyKey string
}

// SecretReferenceResult is the feature result: the provider-assigned
// external resource id and the outcome token ("accepted" for the first
// delivery, "duplicate" for a replayed delivery).
type SecretReferenceResult struct {
	ResourceID string
	Outcome    string
}

// SecretReferencePort is the typed outbound boundary to the managed
// secrets provider. The Platform side never sees a Secret value — only
// the reference and the provider's opaque external id.
type SecretReferencePort interface {
	Apply(context.Context, SecretReferenceCommand) (SecretReferenceResult, error)
}

// Tx is the transaction boundary: the outbound effect and the evidence
// record commit together or not at all.
type Tx interface {
	Run(context.Context, func(context.Context) error) error
}

// EvidenceSink records exactly one content-minimized evidence row per
// accepted or replayed delivery: idempotency key, external resource id,
// outcome token. No secret material, no customer content — ever.
type EvidenceSink interface {
	Record(context.Context, string, string, string) error
}

// SecretReferenceService validates, applies, and evidences one secret
// reference delivery.
type SecretReferenceService struct {
	tx       Tx
	port     SecretReferencePort
	evidence EvidenceSink
}

// NewSecretReferenceService assembles the service. The Stage-1 in-process
// seam (see NewInProcessProviderPort and the acceptance journey) wires
// real collaborators; production adapters plug in behind the same ports.
func NewSecretReferenceService(tx Tx, port SecretReferencePort, evidence EvidenceSink) *SecretReferenceService {
	return &SecretReferenceService{tx: tx, port: port, evidence: evidence}
}

// Execute validates the command (TenantID and idempotency key mandatory,
// SecretRef must satisfy the reference grammar — every rejection happens
// before the outbound port and leaves zero evidence), then applies the
// reference and records the evidence record inside one transaction.
func (s *SecretReferenceService) Execute(ctx context.Context, cmd SecretReferenceCommand) (result SecretReferenceResult, err error) {
	if cmd.TenantID == "" {
		return SecretReferenceResult{}, ErrTenantRequired
	}
	if cmd.IdempotencyKey == "" {
		return SecretReferenceResult{}, ErrIdempotencyKeyRequired
	}
	if !secretRefGrammar.MatchString(cmd.SecretRef) {
		return SecretReferenceResult{}, ErrSecretRefInvalid
	}
	err = s.tx.Run(ctx, func(txCtx context.Context) error {
		applied, applyErr := s.port.Apply(txCtx, cmd)
		if applyErr != nil {
			return applyErr
		}
		result = applied
		return s.evidence.Record(txCtx, cmd.IdempotencyKey, result.ResourceID, result.Outcome)
	})
	return result, err
}

// InProcessTx is the Stage-1 transaction stand-in: the effect and the
// evidence record are function-scoped, so an applyErr skips the evidence
// write entirely — the rollback semantics the Tx interface reserves for
// the real database transaction.
type InProcessTx struct{}

// Run executes fn directly; the function boundary is the transaction.
func (InProcessTx) Run(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// inProcessProviderPort is the Stage-1 managed-secrets adapter
// stand-in: the boundary shape every future provider adapter (cloud
// KMS/SSM per ADR-0026) implements. Its Apply registers the external
// resource id under the idempotency key BEFORE any retryable work
// continues, so a delivery replay answers with the same resource id and
// the provider-side effect happens exactly once per key.
type inProcessProviderPort struct {
	mu      sync.Mutex
	applied map[string]string
	effects int
}

// NewInProcessProviderPort returns an empty provider port. Resource ids
// are opaque sequential tokens ("ext-1", "ext-2", …) — stable within the
// port instance, carrying no secret or customer material.
func NewInProcessProviderPort() SecretReferencePort {
	return &inProcessProviderPort{applied: make(map[string]string)}
}

// Apply registers and answers idempotently: the first delivery of a key
// performs the one provider effect and answers "accepted"; every replay
// of the same key answers the registered resource id as "duplicate"
// with no second effect; a different key is a new effect.
func (p *inProcessProviderPort) Apply(_ context.Context, cmd SecretReferenceCommand) (SecretReferenceResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if resourceID, ok := p.applied[cmd.IdempotencyKey]; ok {
		return SecretReferenceResult{ResourceID: resourceID, Outcome: "duplicate"}, nil
	}
	p.effects++
	resourceID := fmt.Sprintf("ext-%d", p.effects)
	// The external id is registered before any retryable work could
	// continue past this point — replays converge here.
	p.applied[cmd.IdempotencyKey] = resourceID
	return SecretReferenceResult{ResourceID: resourceID, Outcome: "accepted"}, nil
}
