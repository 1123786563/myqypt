package httptransport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/1123786563/myqypt/internal/transport/http/api"
	"github.com/gin-gonic/gin"
)

// stubTenancyRepository stubs the tenancy.Repository port with no
// database I/O.
type stubTenancyRepository struct {
	tenants    []tenancy.TenantSummary
	current    tenancy.TenantContext
	hasCurrent bool
	listErr    error
	currentErr error
	saveErr    error

	createTenant    tenancy.BusinessTenant
	createCreated   bool
	createErr       error
	createCalls     int
	lastCreateName  string
	lastCreateKey   string
	lastCreateIdent identity.VerifiedIdentity

	listCalls    int
	currentCalls int
	saveCalls    int

	lastListVerified identity.VerifiedIdentity
	lastSaveVerified identity.VerifiedIdentity
	lastSavedTenant  string
}

func (r *stubTenancyRepository) ListMembershipTenants(_ context.Context, verified identity.VerifiedIdentity) ([]tenancy.TenantSummary, error) {
	r.listCalls++
	r.lastListVerified = verified
	return r.tenants, r.listErr
}

func (r *stubTenancyRepository) SelectedTenant(_ context.Context, verified identity.VerifiedIdentity) (tenancy.TenantContext, error) {
	r.currentCalls++
	if !r.hasCurrent {
		return tenancy.TenantContext{}, tenancy.ErrNoTenantContext
	}
	if r.currentErr != nil {
		return tenancy.TenantContext{}, r.currentErr
	}
	return r.current, nil
}

func (r *stubTenancyRepository) SaveSelection(_ context.Context, verified identity.VerifiedIdentity, tenantID string) (tenancy.TenantContext, error) {
	r.saveCalls++
	r.lastSaveVerified = verified
	r.lastSavedTenant = tenantID
	if r.saveErr != nil {
		return tenancy.TenantContext{}, r.saveErr
	}
	return tenancy.TenantContext{TenantID: tenantID, SelectedAt: tenancyFixedSelectedAt}, nil
}

func (r *stubTenancyRepository) CreateBusinessTenant(_ context.Context, verified identity.VerifiedIdentity, displayName, idempotencyKey string) (tenancy.BusinessTenant, bool, error) {
	r.createCalls++
	r.lastCreateIdent = verified
	r.lastCreateName = displayName
	r.lastCreateKey = idempotencyKey
	if r.createErr != nil {
		return tenancy.BusinessTenant{}, false, r.createErr
	}
	return r.createTenant, r.createCreated, nil
}

// tenancyFixedSelectedAt is the deterministic selection timestamp the
// stub serves; its JSON form pins the exact success bodies below.
var tenancyFixedSelectedAt = time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)

const (
	// tenancyTenantA is the personal tenant of the stubbed membership set.
	tenancyTenantA = "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88a"
	// tenancyTenantB is a second, business-shaped tenant.
	tenancyTenantB = "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88b"

	tenancyTenantsPath    = "/api/v1/tenants"
	tenancyContextPath    = "/api/v1/tenant-context"
	tenancySelectedAtJSON = `"2026-08-27T01:02:03Z"`
)

// errTenancySaveFailed is the sentinel error the stub repository returns
// in the generic-error case.
var errTenancySaveFailed = errors.New("repository selection failed")

// wiredTenancyDeps returns a fully wired tenancy assembly; individual
// tests override stub fields.
func wiredTenancyDeps(verifier *stubVerifier, repository *stubTenancyRepository) *httptransport.TenancyDependencies {
	return &httptransport.TenancyDependencies{Verifier: verifier, Repository: repository}
}

// newTenancyTestRouter builds the production router with the given tenancy
// assembly; nil leaves the endpoints fail-closed (503), not unregistered.
func newTenancyTestRouter(t *testing.T, deps *httptransport.TenancyDependencies) http.Handler {
	t.Helper()
	useTestGinMode(t)
	return httptransport.NewRouter(httptransport.Dependencies{Version: "test", Tenancy: deps})
}

// doTenancy issues one tenant-context request with an optional
// Authorization header and an optional JSON body.
func doTenancy(handler http.Handler, method, path, body, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// getWithAuth issues an authenticated GET.
func getWithAuth(handler http.Handler, path string) *httptest.ResponseRecorder {
	return doTenancy(handler, http.MethodGet, path, "", "Bearer "+bearerTestToken)
}

// putSelection issues an authenticated PUT selecting tenantID.
func putSelection(handler http.Handler, tenantID string) *httptest.ResponseRecorder {
	return doTenancy(handler, http.MethodPut, tenancyContextPath, `{"tenant_id":"`+tenantID+`"}`, "Bearer "+bearerTestToken)
}

func TestTenancyListShape(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{
		tenants: []tenancy.TenantSummary{
			{TenantID: tenancyTenantA, Kind: "personal", Role: "owner"},
			{TenantID: tenancyTenantB, Kind: "business", Role: "member"},
		},
	}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyTenantsPath)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	// The wire bytes pin the exact generated marshaling: the struct field
	// order (kind, role, tenant_id) and the json.Encoder newline. JSON
	// object order is not contractual; the field set and values are.
	want := `{"tenants":[{"kind":"personal","role":"owner","tenant_id":"` + tenancyTenantA + `"},` +
		`{"kind":"business","role":"member","tenant_id":"` + tenancyTenantB + `"}]}` + "\n"
	if response.Body.String() != want {
		t.Fatalf("body=%q want the exact payload %q", response.Body.String(), want)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content-type=%q want application/json", got)
	}
	if repository.listCalls != 1 || repository.lastListVerified != verifiedTestIdentity {
		t.Fatalf("repository saw (%d calls, %+v), want (1, the verified identity)", repository.listCalls, repository.lastListVerified)
	}
}

func TestTenancyCurrentShape(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{
		hasCurrent: true,
		current:    tenancy.TenantContext{TenantID: tenancyTenantA, SelectedAt: tenancyFixedSelectedAt},
	}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyContextPath)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	// Wire bytes in the generated struct field order (selected_at,
	// tenant_id) with the json.Encoder newline.
	want := `{"selected_at":` + tenancySelectedAtJSON + `,"tenant_id":"` + tenancyTenantA + `"}` + "\n"
	if response.Body.String() != want {
		t.Fatalf("body=%q want the exact payload %q", response.Body.String(), want)
	}
	if repository.currentCalls != 1 {
		t.Fatalf("repository current calls = %d, want 1", repository.currentCalls)
	}
}

func TestTenancyPutSelectsAndReplaysIdempotently(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	first := putSelection(router, tenancyTenantA)
	if first.Code != http.StatusOK {
		t.Fatalf("first PUT status=%d body=%q", first.Code, first.Body.String())
	}
	// Wire bytes in the generated struct field order (selected_at,
	// tenant_id) with the json.Encoder newline.
	want := `{"selected_at":` + tenancySelectedAtJSON + `,"tenant_id":"` + tenancyTenantA + `"}` + "\n"
	if first.Body.String() != want {
		t.Fatalf("first PUT body=%q want the exact payload %q", first.Body.String(), want)
	}
	if repository.saveCalls != 1 {
		t.Fatalf("repository save calls = %d, want 1", repository.saveCalls)
	}
	if repository.lastSavedTenant != tenancyTenantA {
		t.Fatalf("repository selected tenant %q, want %q", repository.lastSavedTenant, tenancyTenantA)
	}
	if repository.lastSaveVerified != verifiedTestIdentity {
		t.Fatalf("repository got %+v, want the verified identity", repository.lastSaveVerified)
	}

	replay := putSelection(router, tenancyTenantA)
	if replay.Code != http.StatusOK {
		t.Fatalf("replayed PUT status=%d body=%q", replay.Code, replay.Body.String())
	}
	if replay.Body.String() != first.Body.String() {
		t.Fatalf("replayed PUT body=%q want the identical first body %q", replay.Body.String(), first.Body.String())
	}
	if strings.Contains(replay.Body.String(), bearerTestToken) {
		t.Fatalf("body leaks the bearer token: %s", replay.Body.String())
	}
}

func TestTenancyPutRejectsNonMemberSelection(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{saveErr: tenancy.ErrNotAnActiveMember}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := putSelection(router, tenancyTenantB)

	assertIdentityProblem(t, response, http.StatusNotFound, "not_found")
	if repository.saveCalls != 1 {
		t.Fatalf("repository save calls = %d, want 1", repository.saveCalls)
	}
}

func TestTenancyCurrentAbsentSelectionIsNotFound(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyContextPath)

	assertIdentityProblem(t, response, http.StatusNotFound, "not_found")
	if repository.currentCalls != 1 {
		t.Fatalf("repository current calls = %d, want 1", repository.currentCalls)
	}
}

// tenancyEndpointCases enumerates the three contract endpoints for the
// matrix tests below.
func tenancyEndpointCases() []struct {
	name   string
	method string
	path   string
	body   string
} {
	return []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"list tenants", http.MethodGet, tenancyTenantsPath, ""},
		{"get context", http.MethodGet, tenancyContextPath, ""},
		{"put context", http.MethodPut, tenancyContextPath, `{"tenant_id":"` + tenancyTenantA + `"}`},
	}
}

func TestTenancyEndpointsRejectMissingAuthorization(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	for _, tc := range tenancyEndpointCases() {
		t.Run(tc.name, func(t *testing.T) {
			response := doTenancy(router, tc.method, tc.path, tc.body, "")
			assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
		})
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0 without credentials", verifier.calls)
	}
	if repository.listCalls != 0 || repository.currentCalls != 0 || repository.saveCalls != 0 {
		t.Fatalf("repository touched without credentials: list=%d current=%d save=%d",
			repository.listCalls, repository.currentCalls, repository.saveCalls)
	}
}

func TestTenancyEndpointsRejectMalformedAuthorization(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	for _, authorization := range []string{"Basic dXNlcjpwYXNz", "bearer " + bearerTestToken, "Bearer "} {
		response := doTenancy(router, http.MethodGet, tenancyTenantsPath, "", authorization)
		assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
	}
	if verifier.calls != 0 || repository.listCalls != 0 {
		t.Fatalf("stubs touched on malformed headers: verifier=%d list=%d", verifier.calls, repository.listCalls)
	}
}

func TestTenancyEndpointsRejectInvalidToken(t *testing.T) {
	verifier := &stubVerifier{err: identity.ErrInvalidToken}
	repository := &stubTenancyRepository{}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	for _, tc := range tenancyEndpointCases() {
		t.Run(tc.name, func(t *testing.T) {
			response := doTenancy(router, tc.method, tc.path, tc.body, "Bearer "+bearerTestToken)
			assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
		})
	}
	if repository.listCalls != 0 || repository.currentCalls != 0 || repository.saveCalls != 0 {
		t.Fatalf("repository touched behind a failed verification: list=%d current=%d save=%d",
			repository.listCalls, repository.currentCalls, repository.saveCalls)
	}
}

func TestTenancyVerifierUnavailable(t *testing.T) {
	verifier := &stubVerifier{err: identity.ErrProviderUnavailable}
	repository := &stubTenancyRepository{}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyTenantsPath)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	if repository.listCalls != 0 {
		t.Fatalf("repository list calls = %d, want 0 behind an unreachable provider", repository.listCalls)
	}
}

func TestTenancyUnconfiguredFailsClosed(t *testing.T) {
	router := newTenancyTestRouter(t, nil)

	for _, tc := range tenancyEndpointCases() {
		t.Run(tc.name, func(t *testing.T) {
			// The contract paths stay registered: the failure is the
			// fail-closed 503, never an unregistered-route 404.
			response := doTenancy(router, tc.method, tc.path, tc.body, "Bearer "+bearerTestToken)
			assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
		})
	}
}

func TestTenancyWithoutRepositoryFailsClosed(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	router := newTenancyTestRouter(t, &httptransport.TenancyDependencies{Verifier: verifier})

	response := getWithAuth(router, tenancyTenantsPath)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	// The nil-deps 503 comes first ("nil deps → 503 先行"): the assembly
	// is checked before any credential is read or verified.
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0 behind the nil-deps fail-closed", verifier.calls)
	}
}

func TestTenancyPutRejectsUnknownField(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := doTenancy(router, http.MethodPut, tenancyContextPath,
		`{"tenant_id":"`+tenancyTenantA+`","request_id":"abc"}`, "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusBadRequest, "invalid_request")
	if repository.saveCalls != 0 {
		t.Fatalf("repository save calls = %d, want 0 for an unknown field", repository.saveCalls)
	}
}

func TestTenancyPutRejectsMalformedUUID(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := doTenancy(router, http.MethodPut, tenancyContextPath,
		`{"tenant_id":"not-a-uuid"}`, "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusBadRequest, "invalid_request")
	if repository.saveCalls != 0 {
		t.Fatalf("repository save calls = %d, want 0 for a malformed uuid", repository.saveCalls)
	}
}

func TestTenancyRepositoryErrorIsInternal(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{saveErr: errTenancySaveFailed}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := putSelection(router, tenancyTenantA)

	assertIdentityProblem(t, response, http.StatusInternalServerError, "internal_error")
	if strings.Contains(response.Body.String(), errTenancySaveFailed.Error()) {
		t.Fatalf("body leaks the repository error: %s", response.Body.String())
	}
}

// tenancyBusinessTenant is the business tenant the stub serves; the fixed
// timestamp reuses tenancyFixedSelectedAt whose JSON form pins the exact
// creation bodies below.
const tenancyBusinessTenant = "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88c"

// stubbedBusinessTenant is the canned port result the create tests serve.
func stubbedBusinessTenant() tenancy.BusinessTenant {
	return tenancy.BusinessTenant{
		TenantID:    tenancyBusinessTenant,
		DisplayName: "Corner Cafe",
		CreatedAt:   tenancyFixedSelectedAt,
	}
}

// postCreateTenant issues one POST /api/v1/tenants with the given JSON
// body, Idempotency-Key header, and optional Authorization header.
func postCreateTenant(handler http.Handler, body, idempotencyKey, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, tenancyTenantsPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// wantCreatedTenantBody is the exact wire body of a create response in
// the generated struct field order (created_at, display_name, kind, role,
// tenant_id) with the json.Encoder newline.
func wantCreatedTenantBody() string {
	return `{"created_at":` + tenancySelectedAtJSON +
		`,"display_name":"Corner Cafe","kind":"business","role":"owner","tenant_id":"` +
		tenancyBusinessTenant + `"}` + "\n"
}

// TestTenancyCreateTenantCreatedShape proves the 201 creation shape
// (design ruling 5): every contract field is echoed verbatim — tenant_id,
// kind=business, display_name, RFC3339 created_at, role=owner.
func TestTenancyCreateTenantCreatedShape(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{createTenant: stubbedBusinessTenant(), createCreated: true}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := postCreateTenant(router, `{"display_name":"Corner Cafe"}`, "key-create-1", "Bearer "+bearerTestToken)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Body.String() != wantCreatedTenantBody() {
		t.Fatalf("body=%q want the exact payload %q", response.Body.String(), wantCreatedTenantBody())
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content-type=%q want application/json", got)
	}
	if repository.createCalls != 1 {
		t.Fatalf("repository create calls = %d, want 1", repository.createCalls)
	}
	if repository.lastCreateIdent != verifiedTestIdentity ||
		repository.lastCreateName != "Corner Cafe" || repository.lastCreateKey != "key-create-1" {
		t.Fatalf("repository saw (%+v, %q, %q), want (the verified identity, Corner Cafe, key-create-1)",
			repository.lastCreateIdent, repository.lastCreateName, repository.lastCreateKey)
	}
}

// TestTenancyCreateTenantReplayIs200WithSameTenant proves the replay
// shape: a converged delivery answers 200 with the identical creation
// facts (same tenant_id).
func TestTenancyCreateTenantReplayIs200WithSameTenant(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{createTenant: stubbedBusinessTenant(), createCreated: false}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := postCreateTenant(router, `{"display_name":"Corner Cafe"}`, "key-create-1", "Bearer "+bearerTestToken)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Body.String() != wantCreatedTenantBody() {
		t.Fatalf("body=%q want the exact payload %q", response.Body.String(), wantCreatedTenantBody())
	}
	if repository.createCalls != 1 {
		t.Fatalf("repository create calls = %d, want 1", repository.createCalls)
	}
}

// TestTenancyCreateTenantRejectsWhitespaceDisplayName proves the
// write-path validation: a display name of only whitespace passes the
// contract's minLength but is rejected by the service (trim) with a 400
// before a single repository call.
func TestTenancyCreateTenantRejectsWhitespaceDisplayName(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{createTenant: stubbedBusinessTenant(), createCreated: true}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := postCreateTenant(router, `{"display_name":"   "}`, "key-create-1", "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusBadRequest, "invalid_request")
	if repository.createCalls != 0 {
		t.Fatalf("repository create calls = %d, want 0 for a whitespace display name", repository.createCalls)
	}
}

// TestTenancyCreateTenantRejectsMissingKeyHeader proves the transport
// contract: the Idempotency-Key header is required; a delivery without
// it is rejected by the validator with a 400 before the handler runs.
func TestTenancyCreateTenantRejectsMissingKeyHeader(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{createTenant: stubbedBusinessTenant(), createCreated: true}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	request := httptest.NewRequest(http.MethodPost, tenancyTenantsPath,
		strings.NewReader(`{"display_name":"Corner Cafe"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+bearerTestToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertIdentityProblem(t, response, http.StatusBadRequest, "invalid_request")
	if repository.createCalls != 0 {
		t.Fatalf("repository create calls = %d, want 0 without the key header", repository.createCalls)
	}
}

// TestTenancyCreateTenantDefensiveNilBodyAndKey proves the handler's
// defense in depth (design ruling 5): a nil body or an empty
// Idempotency-Key that somehow slipped past the validator is still a
// 400, never a panic or a 500. The strict method is invoked directly
// because the wire path cannot express these shapes.
func TestTenancyCreateTenantDefensiveNilBodyAndKey(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{createTenant: stubbedBusinessTenant(), createCreated: true}
	handler := httptransport.TenancyHandler{Dependencies: wiredTenancyDeps(verifier, repository)}

	useTestGinMode(t)
	recorder := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(recorder)
	gc.Request = httptest.NewRequest(http.MethodPost, tenancyTenantsPath, nil)
	gc.Request.Header.Set("Authorization", "Bearer "+bearerTestToken)

	if _, err := handler.CreateTenant(gc, api.CreateTenantRequestObject{
		Params: api.CreateTenantParams{IdempotencyKey: "key-create-1"},
		Body:   nil,
	}); err != nil {
		t.Fatalf("direct nil-body call returned error: %v", err)
	}
	assertIdentityProblem(t, recorder, http.StatusBadRequest, "invalid_request")

	recorder = httptest.NewRecorder()
	gc, _ = gin.CreateTestContext(recorder)
	gc.Request = httptest.NewRequest(http.MethodPost, tenancyTenantsPath, nil)
	gc.Request.Header.Set("Authorization", "Bearer "+bearerTestToken)
	name := "Corner Cafe"
	if _, err := handler.CreateTenant(gc, api.CreateTenantRequestObject{
		Params: api.CreateTenantParams{IdempotencyKey: ""},
		Body:   &api.CreateTenantRequest{DisplayName: name},
	}); err != nil {
		t.Fatalf("direct empty-key call returned error: %v", err)
	}
	assertIdentityProblem(t, recorder, http.StatusBadRequest, "invalid_request")
	if repository.createCalls != 0 {
		t.Fatalf("repository create calls = %d, want 0 behind the defensive rejections", repository.createCalls)
	}
}

// TestTenancyCreateTenantUnboundCreatorIsNotFound proves the no-oracle
// mapping (design ruling 5): an unbound creator's ErrUserNotBound is an
// indistinguishable 404.
func TestTenancyCreateTenantUnboundCreatorIsNotFound(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{createErr: tenancy.ErrUserNotBound}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := postCreateTenant(router, `{"display_name":"Corner Cafe"}`, "key-create-1", "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusNotFound, "not_found")
	if repository.createCalls != 1 {
		t.Fatalf("repository create calls = %d, want 1", repository.createCalls)
	}
}

// TestTenancyCreateTenantRejectsMissingAuthorization proves the 401 gate:
// without credentials the create path never reaches the repository.
func TestTenancyCreateTenantRejectsMissingAuthorization(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{createTenant: stubbedBusinessTenant(), createCreated: true}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := postCreateTenant(router, `{"display_name":"Corner Cafe"}`, "key-create-1", "")

	assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
	if repository.createCalls != 0 {
		t.Fatalf("repository create calls = %d, want 0 without credentials", repository.createCalls)
	}
}

// TestTenancyCreateTenantRejectsTamperedBearer proves the 401 gate for a
// present-but-malformed Authorization header.
func TestTenancyCreateTenantRejectsTamperedBearer(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{createTenant: stubbedBusinessTenant(), createCreated: true}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := postCreateTenant(router, `{"display_name":"Corner Cafe"}`, "key-create-1", "bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
	if repository.createCalls != 0 {
		t.Fatalf("repository create calls = %d, want 0 behind a malformed header", repository.createCalls)
	}
}

// TestTenancyCreateTenantVerifierUnavailable proves the 503 mapping when
// the identity provider is unreachable.
func TestTenancyCreateTenantVerifierUnavailable(t *testing.T) {
	verifier := &stubVerifier{err: identity.ErrProviderUnavailable}
	repository := &stubTenancyRepository{createTenant: stubbedBusinessTenant(), createCreated: true}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := postCreateTenant(router, `{"display_name":"Corner Cafe"}`, "key-create-1", "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	if repository.createCalls != 0 {
		t.Fatalf("repository create calls = %d, want 0 behind an unreachable provider", repository.createCalls)
	}
}

// TestTenancyCreateTenantUnconfiguredFailsClosed proves the fail-closed
// 503: the create path stays registered and answers with the dependency
// problem when no tenancy assembly is wired.
func TestTenancyCreateTenantUnconfiguredFailsClosed(t *testing.T) {
	router := newTenancyTestRouter(t, nil)

	response := postCreateTenant(router, `{"display_name":"Corner Cafe"}`, "key-create-1", "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
}
