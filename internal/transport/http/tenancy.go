package httptransport

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
	"github.com/1123786563/myqypt/internal/transport/http/api"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// tenancyUUID parses a canonical tenant id into the contract's uuid type.
// A repository that hands back non-uuid text has broken an internal
// invariant; the strict handler's error hook maps it to the opaque 500
// problem.
func tenancyUUID(tenantID string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(tenantID)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("tenancy: tenant id is not a canonical uuid: %w", err)
	}
	return parsed, nil
}

// TenancyDependencies wires the three tenant-context contract endpoints:
// the Verifier port authenticates the bearer token, the Repository port
// resolves the verified identity and persists the tenant context
// selection. Unlike the internal identity callback, these contract paths
// are always registered; a nil *TenancyDependencies (or a missing port)
// fails every request closed with 503 — the /readyz fail-closed
// precedent, applied per dependency at request time.
type TenancyDependencies struct {
	Verifier   identity.Verifier
	Repository tenancy.Repository
}

// TenancyHandler serves the tenant-context endpoints defined by the
// OpenAPI contract. It is embedded alongside StatusHandler in the
// aggregated strict-server implementation NewRouter registers.
type TenancyHandler struct {
	Dependencies *TenancyDependencies
}

// authenticateTenantUser authenticates one tenant-context request through
// the Verifier port. Only the Authorization: Bearer header is read — the
// principal always comes from the verified token. Status mapping (design
// ruling 2, along the identity callback's ruling 6): an unwired
// dependency fails closed with 503 first; missing or malformed
// credentials and ErrInvalidToken → 401 unauthorized;
// ErrProviderUnavailable → 503 dependency_unavailable; any other
// verification failure → 500 internal_error. When the second return is
// false the Problem response has already been written and the caller must
// stop without writing another response.
func (h TenancyHandler) authenticateTenantUser(c *gin.Context) (identity.VerifiedIdentity, bool) {
	if h.Dependencies == nil || h.Dependencies.Verifier == nil || h.Dependencies.Repository == nil {
		WriteProblem(c, newProblem(http.StatusServiceUnavailable, CodeDependencyUnavailable))
		return identity.VerifiedIdentity{}, false
	}
	rawToken, ok := bearerToken(c.Request.Header.Get("Authorization"))
	if !ok {
		WriteProblem(c, newProblem(http.StatusUnauthorized, CodeUnauthorized))
		return identity.VerifiedIdentity{}, false
	}
	verified, err := h.Dependencies.Verifier.Verify(c.Request.Context(), rawToken)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidToken):
			WriteProblem(c, newProblem(http.StatusUnauthorized, CodeUnauthorized))
		case errors.Is(err, identity.ErrProviderUnavailable):
			WriteProblem(c, newProblem(http.StatusServiceUnavailable, CodeDependencyUnavailable))
		default:
			WriteProblem(c, newProblem(http.StatusInternalServerError, CodeInternalError))
		}
		return identity.VerifiedIdentity{}, false
	}
	return verified, true
}

// tenancyServiceProblem maps an application tenancy error onto its stable
// Problem Details response: a selection without an active membership, an
// absent current selection, an unbound principal, an unauthorized
// inviter, and a missing invitation are indistinguishable 404s (no
// existence oracle), input-shaped rejections map to 400, and the
// provider-unavailable dependency to 503; everything else stays an
// opaque 500.
func tenancyServiceProblem(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancy.ErrNotAnActiveMember),
		errors.Is(err, tenancy.ErrNoTenantContext),
		errors.Is(err, tenancy.ErrUserNotBound),
		errors.Is(err, tenancy.ErrInviterNotAuthorized),
		errors.Is(err, tenancy.ErrInvitationNotFound):
		WriteProblem(c, newProblem(http.StatusNotFound, CodeNotFound))
	case errors.Is(err, tenancy.ErrUserRequired),
		errors.Is(err, tenancy.ErrTenantRequired),
		errors.Is(err, tenancy.ErrDisplayNameRequired),
		errors.Is(err, tenancy.ErrIdempotencyKeyRequired),
		errors.Is(err, tenancy.ErrInviteeSubjectRequired),
		errors.Is(err, tenancy.ErrRoleNotSupported):
		WriteProblem(c, newProblem(http.StatusBadRequest, CodeInvalidRequest))
	case errors.Is(err, identity.ErrProviderUnavailable):
		WriteProblem(c, newProblem(http.StatusServiceUnavailable, CodeDependencyUnavailable))
	default:
		WriteProblem(c, newProblem(http.StatusInternalServerError, CodeInternalError))
	}
}

// tenancyGinContext recovers the *gin.Context the generated strict
// handler passes as the method context, so Problem responses can be
// written mid-method. The generated wrapper always passes it; the error
// path is pure defensive hygiene and maps to the strict handler's 500.
func tenancyGinContext(ctx context.Context) (*gin.Context, error) {
	gc, ok := ctx.(*gin.Context)
	if !ok {
		return nil, errors.New("tenancy: strict handler context is not *gin.Context")
	}
	return gc, nil
}

// ListTenants serves GET /api/v1/tenants: the active memberships of the
// authenticated user, each with the tenant kind and the membership role.
func (h TenancyHandler) ListTenants(ctx context.Context, _ api.ListTenantsRequestObject) (api.ListTenantsResponseObject, error) {
	gc, err := tenancyGinContext(ctx)
	if err != nil {
		return nil, err
	}
	verified, ok := h.authenticateTenantUser(gc)
	if !ok {
		return nil, nil
	}
	tenants, err := tenancy.NewService(h.Dependencies.Repository).List(gc.Request.Context(), verified)
	if err != nil {
		tenancyServiceProblem(gc, err)
		return nil, nil
	}
	response := api.ListTenants200JSONResponse{Tenants: make([]api.TenantSummary, 0, len(tenants))}
	for _, summary := range tenants {
		tenantID, err := tenancyUUID(summary.TenantID)
		if err != nil {
			return nil, err
		}
		response.Tenants = append(response.Tenants, api.TenantSummary{
			TenantId: tenantID,
			Kind:     api.TenantSummaryKind(summary.Kind),
			Role:     api.TenantSummaryRole(summary.Role),
		})
	}
	return response, nil
}

// GetTenantContext serves GET /api/v1/tenant-context: the server-side
// re-validated current selection. A missing or invalidated selection
// answers 404 (design ruling 6).
func (h TenancyHandler) GetTenantContext(ctx context.Context, _ api.GetTenantContextRequestObject) (api.GetTenantContextResponseObject, error) {
	gc, err := tenancyGinContext(ctx)
	if err != nil {
		return nil, err
	}
	verified, ok := h.authenticateTenantUser(gc)
	if !ok {
		return nil, nil
	}
	current, err := tenancy.NewService(h.Dependencies.Repository).Current(gc.Request.Context(), verified)
	if err != nil {
		tenancyServiceProblem(gc, err)
		return nil, nil
	}
	currentID, err := tenancyUUID(current.TenantID)
	if err != nil {
		return nil, err
	}
	return api.GetTenantContext200JSONResponse{
		TenantId:   currentID,
		SelectedAt: current.SelectedAt,
	}, nil
}

// PutTenantContext serves PUT /api/v1/tenant-context: the explicit
// selection or switch, validated against an active membership before the
// write and idempotent on replay (a switch is last-write-wins; a
// singleton resource state answers 200, never 201).
func (h TenancyHandler) PutTenantContext(ctx context.Context, request api.PutTenantContextRequestObject) (api.PutTenantContextResponseObject, error) {
	gc, err := tenancyGinContext(ctx)
	if err != nil {
		return nil, err
	}
	verified, ok := h.authenticateTenantUser(gc)
	if !ok {
		return nil, nil
	}
	// The OpenAPI validator already rejects a missing body; a request
	// that slipped a nil body past it is treated as a bad request.
	if request.Body == nil {
		WriteProblem(gc, newProblem(http.StatusBadRequest, CodeInvalidRequest))
		return nil, nil
	}
	selected, err := tenancy.NewService(h.Dependencies.Repository).Select(gc.Request.Context(), verified, request.Body.TenantId.String())
	if err != nil {
		tenancyServiceProblem(gc, err)
		return nil, nil
	}
	selectedID, err := tenancyUUID(selected.TenantID)
	if err != nil {
		return nil, err
	}
	return api.PutTenantContext200JSONResponse{
		TenantId:   selectedID,
		SelectedAt: selected.SelectedAt,
	}, nil
}

// CreateTenant serves POST /api/v1/tenants: the explicit creation of a
// business tenant owned by the authenticated user. The first delivery of
// an idempotency key answers 201; a replay of the same key converges
// onto the already-created tenant and answers 200 with the same creation
// facts (design ruling 5). An unbound creator is an indistinguishable
// 404; input-shaped rejections are 400s.
func (h TenancyHandler) CreateTenant(ctx context.Context, request api.CreateTenantRequestObject) (api.CreateTenantResponseObject, error) {
	gc, err := tenancyGinContext(ctx)
	if err != nil {
		return nil, err
	}
	verified, ok := h.authenticateTenantUser(gc)
	if !ok {
		return nil, nil
	}
	// The OpenAPI validator already rejects a missing body, a body
	// without a display name, and a missing Idempotency-Key header; a
	// request that slipped past it is still treated as a bad request
	// (defense in depth, design ruling 5).
	if request.Body == nil || request.Params.IdempotencyKey == "" {
		WriteProblem(gc, newProblem(http.StatusBadRequest, CodeInvalidRequest))
		return nil, nil
	}
	tenant, created, err := tenancy.NewService(h.Dependencies.Repository).
		CreateBusinessTenant(gc.Request.Context(), verified, request.Body.DisplayName, request.Params.IdempotencyKey)
	if err != nil {
		tenancyServiceProblem(gc, err)
		return nil, nil
	}
	tenantID, err := tenancyUUID(tenant.TenantID)
	if err != nil {
		return nil, err
	}
	response := api.CreatedTenant{
		TenantId:    tenantID,
		Kind:        api.CreatedTenantKindBusiness,
		DisplayName: tenant.DisplayName,
		CreatedAt:   tenant.CreatedAt,
		Role:        api.CreatedTenantRoleOwner,
	}
	if created {
		return api.CreateTenant201JSONResponse(response), nil
	}
	return api.CreateTenant200JSONResponse(response), nil
}

// CreateMembershipInvitation serves POST
// /api/v1/tenants/{tenantId}/membership-invitations: the authenticated
// owner or admin invites a bound user into the tenant as a non-owner
// member (T05). The first delivery of a (tenant, invitee) pair answers
// 201 with status=invited; a repeat delivery of the still-pending
// invitation answers 200 with the identical facts (natural-key
// convergence — the Idempotency-Key header is mandatory but never
// stored). An unbound inviter or invitee, an unauthorized inviter, and
// an already-resolved membership are indistinguishable 404s;
// input-shaped rejections are 400s.
func (h TenancyHandler) CreateMembershipInvitation(ctx context.Context, request api.CreateMembershipInvitationRequestObject) (api.CreateMembershipInvitationResponseObject, error) {
	gc, err := tenancyGinContext(ctx)
	if err != nil {
		return nil, err
	}
	verified, ok := h.authenticateTenantUser(gc)
	if !ok {
		return nil, nil
	}
	// The OpenAPI validator already rejects a missing body, a body
	// without both fields, a role outside the invitable enum, and a
	// missing or malformed Idempotency-Key header; a request that
	// slipped past it is still treated as a bad request (defense in
	// depth, mirroring CreateTenant).
	if request.Body == nil || request.Params.IdempotencyKey == "" {
		WriteProblem(gc, newProblem(http.StatusBadRequest, CodeInvalidRequest))
		return nil, nil
	}
	invitation, created, err := tenancy.NewService(h.Dependencies.Repository).
		InviteMember(gc.Request.Context(), verified, request.TenantId.String(),
			request.Body.InviteeSubject, string(request.Body.Role), request.Params.IdempotencyKey)
	if err != nil {
		tenancyServiceProblem(gc, err)
		return nil, nil
	}
	tenantID, err := tenancyUUID(invitation.TenantID)
	if err != nil {
		return nil, err
	}
	response := api.MembershipInvitation{
		TenantId:  tenantID,
		Role:      api.MembershipRole(invitation.Role),
		Status:    api.MembershipInvitationStatus(invitation.Status),
		InvitedAt: invitation.InvitedAt,
	}
	if created {
		return api.CreateMembershipInvitation201JSONResponse(response), nil
	}
	return api.CreateMembershipInvitation200JSONResponse(response), nil
}

// AcceptMembershipInvitation serves POST
// /api/v1/tenants/{tenantId}/membership-invitations/acceptance: the
// authenticated invitee accepts the pending invitation, completing the
// single-row transition invited -> active. Replays converge onto the
// same activation without a second transition. No pending invitation —
// never invited, already resolved by someone else's facts, or revoked —
// is an indistinguishable 404 with zero writes.
func (h TenancyHandler) AcceptMembershipInvitation(ctx context.Context, request api.AcceptMembershipInvitationRequestObject) (api.AcceptMembershipInvitationResponseObject, error) {
	gc, err := tenancyGinContext(ctx)
	if err != nil {
		return nil, err
	}
	verified, ok := h.authenticateTenantUser(gc)
	if !ok {
		return nil, nil
	}
	activated, err := tenancy.NewService(h.Dependencies.Repository).
		AcceptInvitation(gc.Request.Context(), verified, request.TenantId.String())
	if err != nil {
		tenancyServiceProblem(gc, err)
		return nil, nil
	}
	tenantID, err := tenancyUUID(activated.TenantID)
	if err != nil {
		return nil, err
	}
	return api.AcceptMembershipInvitation200JSONResponse(api.ActivatedMembership{
		TenantId: tenantID,
		Role:     api.MembershipRole(activated.Role),
		Status:   api.ActivatedMembershipStatus(activated.Status),
	}), nil
}

// ListTenantCapabilities serves GET
// /api/v1/tenants/{tenantId}/capabilities: the authenticated user's
// active-membership Platform Role in the tenant and that role's sorted
// capability list (T06, AC1: each role gets its own visible operations).
// Identical requests produce byte-identical bodies (design ruling 6).
// Every principal without an active membership in the tenant — never a
// member, invited but not accepted, revoked, a stranger, or an unknown
// tenant — is an indistinguishable 404 (no existence oracle); a persisted
// role outside the matrix is a 400 (defensive classification, design
// ruling 5); input-shaped rejections are 400s and credential failures
// 401s.
func (h TenancyHandler) ListTenantCapabilities(ctx context.Context, request api.ListTenantCapabilitiesRequestObject) (api.ListTenantCapabilitiesResponseObject, error) {
	gc, err := tenancyGinContext(ctx)
	if err != nil {
		return nil, err
	}
	verified, ok := h.authenticateTenantUser(gc)
	if !ok {
		return nil, nil
	}
	capabilities, err := tenancy.NewService(h.Dependencies.Repository).
		Capabilities(gc.Request.Context(), verified, request.TenantId.String())
	if err != nil {
		tenancyServiceProblem(gc, err)
		return nil, nil
	}
	tenantID, err := tenancyUUID(capabilities.TenantID)
	if err != nil {
		return nil, err
	}
	return api.ListTenantCapabilities200JSONResponse(api.TenantCapabilities{
		TenantId:     tenantID,
		Role:         api.TenantCapabilitiesRole(capabilities.Role),
		Capabilities: capabilities.Capabilities,
	}), nil
}
