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
