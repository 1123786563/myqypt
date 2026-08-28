// Package productcatalog delivers the T13 vertical slice (Issue #14):
// Owner 查看内部策展 Product 及其可用状态。The slice is the Stage-1
// shape of the internally curated Product Catalog (ADR-0003): typed
// command/result contracts with the availability carried across the
// seam, validation before any side effect, a transaction boundary
// around the single outbound effect plus one content-minimized evidence
// record, and an in-process curated-catalog adapter whose catalog is a
// closed code literal — no external publisher path exists — with
// idempotency-key registration that makes delivery replays converge
// onto one access effect.
package productcatalog

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Classified sentinel errors. The first three are input-shaped and are
// returned before the outbound port is ever touched; ErrProductNotCurated
// is the denial class and ErrCatalogUnavailable the retryable failure
// class, both raised by the port.
var (
	ErrTenantRequired         = errors.New("tenant context is required")
	ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
	ErrProductRequired        = errors.New("product is required")
	ErrProductNotCurated      = errors.New("product is not in the internally curated catalog")
	ErrCatalogUnavailable     = errors.New("product catalog unavailable")
)

// availabilities is the closed availability vocabulary of the curated
// catalog: every catalog entry's 可用状态 is exactly one of these.
var availabilities = map[string]struct{}{
	"available":   {},
	"unavailable": {},
}

// curatedCatalog is the internally curated Stage-1 catalog (ADR-0003):
// a closed set of Platform-curated product ids, each with its current
// availability. It is a code literal — only the Platform's internal
// team curates it, and no external publisher or runtime addition path
// exists in this slice by construction.
var curatedCatalog = map[string]string{
	"product-alpha": "available",
	"product-beta":  "unavailable",
}

// ProductCatalogCommand is the feature command: the Tenant whose
// boundary the browse runs under, the curated product being viewed,
// and the idempotency key that makes delivery replays converge.
type ProductCatalogCommand struct {
	TenantID       string
	ProductID      string
	IdempotencyKey string
}

// ProductCatalogResult is the feature result: the catalog access
// entry id, the product's availability status carried across the seam
// (the ticket's 「及其可用状态」), and the outcome token ("accepted" for
// the first delivery, "duplicate" for a replayed delivery).
type ProductCatalogResult struct {
	ResourceID   string
	Availability string
	Outcome      string
}

// ProductCatalogPort is the typed outbound boundary to the curated
// catalog. The Platform side never mutates catalog entries — a browse
// observes the curated product and its availability, nothing else.
type ProductCatalogPort interface {
	Apply(context.Context, ProductCatalogCommand) (ProductCatalogResult, error)
}

// Tx is the transaction boundary: the outbound effect and the evidence
// record commit together or not at all.
type Tx interface {
	Run(context.Context, func(context.Context) error) error
}

// EvidenceSink records exactly one content-minimized evidence row per
// accepted or replayed delivery: idempotency key, access entry id,
// outcome token. No secret material, no customer content — ever.
type EvidenceSink interface {
	Record(context.Context, string, string, string) error
}

// ProductCatalogService validates, applies, and evidences one curated
// catalog view delivery.
type ProductCatalogService struct {
	tx       Tx
	port     ProductCatalogPort
	evidence EvidenceSink
}

// NewProductCatalogService assembles the service. The Stage-1
// in-process seam (see NewInProcessCatalogPort and the acceptance
// journey) wires real collaborators; production catalog adapters plug
// in behind the same ports.
func NewProductCatalogService(tx Tx, port ProductCatalogPort, evidence EvidenceSink) *ProductCatalogService {
	return &ProductCatalogService{tx: tx, port: port, evidence: evidence}
}

// Execute validates the command (TenantID, idempotency key, and
// product mandatory — every rejection happens before the outbound port
// and leaves zero evidence), then applies the catalog view and records
// the evidence row inside one transaction.
func (s *ProductCatalogService) Execute(ctx context.Context, cmd ProductCatalogCommand) (result ProductCatalogResult, err error) {
	if cmd.TenantID == "" {
		return ProductCatalogResult{}, ErrTenantRequired
	}
	if cmd.IdempotencyKey == "" {
		return ProductCatalogResult{}, ErrIdempotencyKeyRequired
	}
	if cmd.ProductID == "" {
		return ProductCatalogResult{}, ErrProductRequired
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
// evidence record are function-scoped, so an applyErr skips the
// evidence write entirely — the rollback semantics the Tx interface
// reserves for the real database transaction.
type InProcessTx struct{}

// Run executes fn directly; the function boundary is the transaction.
func (InProcessTx) Run(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// catalogAccess is one content-minimized catalog access entry: the
// append sequence, the Tenant boundary, the curated product, and the
// availability observed at view time. No secrets, no customer content
// — by construction, the struct has no field for them.
type catalogAccess struct {
	sequence     int
	tenantID     string
	productID    string
	availability string
}

// inProcessCatalogPort is the Stage-1 curated-catalog adapter
// stand-in: the boundary shape every future catalog adapter (the
// Platform's curated store per ADR-0003) implements. Its Apply
// registers the access entry id under the idempotency key BEFORE any
// retryable work continues, so a delivery replay answers with the same
// entry id and the store-side effect happens exactly once per key.
// Catalog entries themselves are read-only: the curated set is a code
// literal with no mutation path.
type inProcessCatalogPort struct {
	mu       sync.Mutex
	applied  map[string]string
	accesses map[string]catalogAccess
	views    int
}

// NewInProcessCatalogPort returns a catalog port over the internally
// curated catalog. Access entry ids are opaque sequential tokens
// ("catalog-1", "catalog-2", …) — stable within the port instance,
// carrying no secret or customer material.
func NewInProcessCatalogPort() ProductCatalogPort {
	return &inProcessCatalogPort{
		applied:  make(map[string]string),
		accesses: make(map[string]catalogAccess),
	}
}

// Apply views and answers idempotently: the first delivery of a key
// performs the one catalog access effect and answers "accepted" with
// the product's availability; every replay of the same key answers the
// registered entry id as "duplicate" with no second effect; a product
// outside the curated set is denied with ErrProductNotCurated before
// any effect.
func (p *inProcessCatalogPort) Apply(_ context.Context, cmd ProductCatalogCommand) (ProductCatalogResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entryID, ok := p.applied[cmd.IdempotencyKey]; ok {
		access := p.accesses[entryID]
		return ProductCatalogResult{ResourceID: entryID, Availability: access.availability, Outcome: "duplicate"}, nil
	}
	availability, curated := curatedCatalog[cmd.ProductID]
	if !curated {
		return ProductCatalogResult{}, ErrProductNotCurated
	}
	p.views++
	entryID := fmt.Sprintf("catalog-%d", p.views)
	// The entry id is registered before any retryable work could
	// continue past this point — replays converge here.
	p.applied[cmd.IdempotencyKey] = entryID
	p.accesses[entryID] = catalogAccess{
		sequence:     p.views,
		tenantID:     cmd.TenantID,
		productID:    cmd.ProductID,
		availability: availability,
	}
	return ProductCatalogResult{ResourceID: entryID, Availability: availability, Outcome: "accepted"}, nil
}
