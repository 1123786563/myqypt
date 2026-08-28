// Package membershiproleaudit delivers the T07 vertical slice (Issue
// #8): 邀请、激活、拒绝、角色变更和移除产生不可变 Audit Event。The
// slice is the Stage-1 shape of the content-minimized immutable audit
// stream (ADR-0041): typed command/result contracts with a closed
// five-action vocabulary, validation before any side effect, a
// transaction boundary around the single outbound effect plus one
// content-minimized evidence record, and an in-process append-only
// audit ledger whose idempotency-key registration precedes any
// retryable work so replays converge onto one immutable event.
package membershiproleaudit

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Classified sentinel errors. The first four are input-shaped and are
// returned before the outbound port is ever touched; ErrAuditUnavailable
// is the retryable audit-store failure class.
var (
	ErrTenantRequired         = errors.New("tenant context is required")
	ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
	ErrMembershipRequired     = errors.New("membership subject is required")
	ErrActionInvalid          = errors.New("action is not in the membership audit vocabulary")
	ErrAuditUnavailable       = errors.New("membership role audit store unavailable")
)

// auditAuthority is the stable, non-sensitive authority axis every
// event carries (ADR-0041): the platform membership domain itself.
const auditAuthority = "platform-membership"

// auditActions is the closed action vocabulary of the ticket invariant
// — 邀请 (invite)、激活 (activate)、拒绝 (reject)、角色变更
// (role_change)、移除 (remove). No other action token is auditable.
var auditActions = map[string]struct{}{
	"invite":      {},
	"activate":    {},
	"reject":      {},
	"role_change": {},
	"remove":      {},
}

// MembershipRoleAuditCommand is the feature command: the Tenant the
// event belongs to (the hard boundary), the membership the event is
// about, the action from the closed vocabulary, the role transition
// (role_change only), and the idempotency key that makes delivery
// replays converge.
type MembershipRoleAuditCommand struct {
	TenantID       string
	MembershipID   string
	Action         string
	RoleBefore     string
	RoleAfter      string
	IdempotencyKey string
}

// MembershipRoleAuditResult is the feature result: the audit store's
// event id and the outcome token ("accepted" for the first delivery,
// "duplicate" for a replayed delivery).
type MembershipRoleAuditResult struct {
	ResourceID string
	Outcome    string
}

// MembershipRoleAuditPort is the typed outbound boundary to the audit
// store. The Platform side never sees mutable entries — an event, once
// appended under its idempotency key, is answered unchanged forever.
type MembershipRoleAuditPort interface {
	Apply(context.Context, MembershipRoleAuditCommand) (MembershipRoleAuditResult, error)
}

// Tx is the transaction boundary: the outbound effect and the evidence
// record commit together or not at all.
type Tx interface {
	Run(context.Context, func(context.Context) error) error
}

// EvidenceSink records exactly one content-minimized evidence row per
// accepted or replayed delivery: idempotency key, event id, outcome
// token. No secret material, no customer content — ever.
type EvidenceSink interface {
	Record(context.Context, string, string, string) error
}

// MembershipRoleAuditService validates, applies, and evidences one
// membership-role audit delivery.
type MembershipRoleAuditService struct {
	tx       Tx
	port     MembershipRoleAuditPort
	evidence EvidenceSink
}

// NewMembershipRoleAuditService assembles the service. The Stage-1
// in-process seam (see NewInProcessAuditPort and the acceptance
// journey) wires real collaborators; production audit adapters plug in
// behind the same ports.
func NewMembershipRoleAuditService(tx Tx, port MembershipRoleAuditPort, evidence EvidenceSink) *MembershipRoleAuditService {
	return &MembershipRoleAuditService{tx: tx, port: port, evidence: evidence}
}

// Execute validates the command (TenantID, idempotency key, and
// membership subject mandatory; the action must belong to the closed
// five-action vocabulary, and a role_change must carry both sides of
// the transition — every rejection happens before the outbound port
// and leaves zero evidence), then appends the immutable audit event and
// records the evidence row inside one transaction.
func (s *MembershipRoleAuditService) Execute(ctx context.Context, cmd MembershipRoleAuditCommand) (result MembershipRoleAuditResult, err error) {
	if cmd.TenantID == "" {
		return MembershipRoleAuditResult{}, ErrTenantRequired
	}
	if cmd.IdempotencyKey == "" {
		return MembershipRoleAuditResult{}, ErrIdempotencyKeyRequired
	}
	if cmd.MembershipID == "" {
		return MembershipRoleAuditResult{}, ErrMembershipRequired
	}
	if !actionAuditable(cmd) {
		return MembershipRoleAuditResult{}, ErrActionInvalid
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

// actionAuditable reports whether the action token is one of the five
// membership lifecycle actions, and — for role_change — whether the
// transition carries both the role before and the role after. The
// transition is the auditable fact; anything less cannot be audited.
func actionAuditable(cmd MembershipRoleAuditCommand) bool {
	if _, ok := auditActions[cmd.Action]; !ok {
		return false
	}
	if cmd.Action == "role_change" && (cmd.RoleBefore == "" || cmd.RoleAfter == "") {
		return false
	}
	return true
}

// InProcessTx is the Stage-1 transaction stand-in: the effect and the
// evidence record are function-scoped, so an applyErr skips the
// evidence write entirely — the rollback semantics the Tx interface
// reserves for the real database transaction.
type InProcessTx struct{}

// Run executes fn directly; the function boundary is the transaction.
func (InProcessTx) Run(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// auditEvent is one content-minimized immutable audit entry (ADR-0041
// axes on stable non-sensitive identifiers): the authority, the Tenant
// boundary, the membership subject, the action, the role transition,
// and the append sequence. No secrets, no payloads, no customer
// content — by construction, the struct simply has no field for them.
type auditEvent struct {
	sequence     int
	authority    string
	tenantID     string
	membershipID string
	action       string
	roleBefore   string
	roleAfter    string
}

// inProcessAuditPort is the Stage-1 audit-store adapter stand-in: the
// boundary shape every future audit adapter (PostgreSQL append-only
// stream per ADR-0041) implements. Its Apply registers the event id
// under the idempotency key BEFORE any retryable work continues, so a
// delivery replay answers with the same event id and the store-side
// append happens exactly once per key. Entries are write-once: the
// registry has no mutation path, and a divergent payload replayed under
// an existing key converges onto the already-appended event.
type inProcessAuditPort struct {
	mu      sync.Mutex
	applied map[string]string
	events  map[string]auditEvent
	appends int
}

// NewInProcessAuditPort returns an empty audit port. Event ids are
// opaque sequential tokens ("audit-1", "audit-2", …) — stable within
// the port instance, carrying no secret or customer material.
func NewInProcessAuditPort() MembershipRoleAuditPort {
	return &inProcessAuditPort{
		applied: make(map[string]string),
		events:  make(map[string]auditEvent),
	}
}

// Apply appends and answers idempotently: the first delivery of a key
// performs the one append and answers "accepted"; every replay of the
// same key answers the registered event id as "duplicate" with no
// second append and no overwrite of the stored event; a different key
// is a new append.
func (p *inProcessAuditPort) Apply(_ context.Context, cmd MembershipRoleAuditCommand) (MembershipRoleAuditResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if eventID, ok := p.applied[cmd.IdempotencyKey]; ok {
		return MembershipRoleAuditResult{ResourceID: eventID, Outcome: "duplicate"}, nil
	}
	p.appends++
	eventID := fmt.Sprintf("audit-%d", p.appends)
	// The event id is registered before any retryable work could
	// continue past this point — replays converge here.
	p.applied[cmd.IdempotencyKey] = eventID
	p.events[eventID] = auditEvent{
		sequence:     p.appends,
		authority:    auditAuthority,
		tenantID:     cmd.TenantID,
		membershipID: cmd.MembershipID,
		action:       cmd.Action,
		roleBefore:   cmd.RoleBefore,
		roleAfter:    cmd.RoleAfter,
	}
	return MembershipRoleAuditResult{ResourceID: eventID, Outcome: "accepted"}, nil
}
