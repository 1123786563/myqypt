package httptransport_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

// tenancyCapabilitiesPath is the T06 contract path under the pinned
// business tenant of the stub fixtures.
const tenancyCapabilitiesPath = "/api/v1/tenants/" + tenancyBusinessTenant + "/capabilities"

// wantCapabilitiesBody is the exact wire body of a capabilities response
// for the admin role in the generated struct field order (capabilities,
// role, tenant_id) with the json.Encoder newline: the sorted four-domain
// admin set of CONTEXT.md.
func wantAdminCapabilitiesBody() string {
	return `{"capabilities":["configuration.manage","membership.manage","product_access.manage","purchases.manage"],` +
		`"role":"admin","tenant_id":"` + tenancyBusinessTenant + `"}` + "\n"
}

// TestTenancyCapabilitiesShape proves the 200 shape (T06 design ruling 2):
// every contract field is echoed — tenant_id, role, and the sorted
// capability list — and the repository resolved the role for the verified
// identity and the path's tenant.
func TestTenancyCapabilitiesShape(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{roleResult: "admin"}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyCapabilitiesPath)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Body.String() != wantAdminCapabilitiesBody() {
		t.Fatalf("body=%q want the exact payload %q", response.Body.String(), wantAdminCapabilitiesBody())
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content-type=%q want application/json", got)
	}
	if repository.roleCalls != 1 {
		t.Fatalf("repository role calls = %d, want 1", repository.roleCalls)
	}
	if repository.lastRoleIdent != verifiedTestIdentity || repository.lastRoleTenant != tenancyBusinessTenant {
		t.Fatalf("repository saw (%+v, %q), want (the verified identity, the path tenant)",
			repository.lastRoleIdent, repository.lastRoleTenant)
	}
}

// TestTenancyCapabilitiesBodyDeterministic proves the replay determinism
// (design ruling 6): two identical requests answer byte-identical bodies.
func TestTenancyCapabilitiesBodyDeterministic(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{roleResult: "owner"}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	first := getWithAuth(router, tenancyCapabilitiesPath)
	replay := getWithAuth(router, tenancyCapabilitiesPath)

	if first.Code != http.StatusOK || replay.Code != http.StatusOK {
		t.Fatalf("status first=%d replay=%d", first.Code, replay.Code)
	}
	if replay.Body.String() != first.Body.String() {
		t.Fatalf("replay body=%q want the byte-identical first body %q", replay.Body.String(), first.Body.String())
	}
	if repository.roleCalls != 2 {
		t.Fatalf("repository role calls = %d, want 2 (each delivery is a real port call)", repository.roleCalls)
	}
}

// TestTenancyCapabilitiesServesSortedOwnerSuperset pins the owner wire
// shape: the sorted full vocabulary (the sole accountable role's
// superset), proving the deterministic ordering on the largest list.
func TestTenancyCapabilitiesServesSortedOwnerSuperset(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{roleResult: "owner"}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyCapabilitiesPath)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	want := `{"capabilities":["billing.manage","bills.read","configuration.manage","membership.manage","ownership.manage","payments.manage","product.use","product_access.manage","purchases.manage","subscriptions.read","usage.read"],` +
		`"role":"owner","tenant_id":"` + tenancyBusinessTenant + `"}` + "\n"
	if response.Body.String() != want {
		t.Fatalf("owner body=%q want the exact sorted payload %q", response.Body.String(), want)
	}
}

// TestTenancyCapabilitiesClassifiedRejectionsAreNotFound proves the
// no-oracle mapping (design ruling 2): a principal without an active
// membership (ErrNotAnActiveMember) and a never-bound identity
// (ErrUserNotBound) are indistinguishable 404s.
func TestTenancyCapabilitiesClassifiedRejectionsAreNotFound(t *testing.T) {
	for _, err := range []error{tenancy.ErrNotAnActiveMember, tenancy.ErrUserNotBound} {
		verifier := &stubVerifier{identity: verifiedTestIdentity}
		repository := &stubTenancyRepository{roleErr: err}
		router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

		response := getWithAuth(router, tenancyCapabilitiesPath)

		assertIdentityProblem(t, response, http.StatusNotFound, "not_found")
		if repository.roleCalls != 1 {
			t.Fatalf("repository role calls = %d, want 1 (classification happens in the port)", repository.roleCalls)
		}
	}
}

// TestTenancyCapabilitiesUnknownRoleIsInvalidRequest proves the defensive
// classification (design ruling 5): a role string outside the matrix
// answers the 400 invalid_request family, never a 500.
func TestTenancyCapabilitiesUnknownRoleIsInvalidRequest(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{roleResult: "emperor"}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyCapabilitiesPath)

	assertIdentityProblem(t, response, http.StatusBadRequest, "invalid_request")
	if repository.roleCalls != 1 {
		t.Fatalf("repository role calls = %d, want 1 (the role arrives from persistence)", repository.roleCalls)
	}
}

// TestTenancyCapabilitiesRejectsMissingAuthorization proves the 401 gate:
// without credentials the capabilities path never reaches the repository.
func TestTenancyCapabilitiesRejectsMissingAuthorization(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{roleResult: "member"}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := doTenancy(router, http.MethodGet, tenancyCapabilitiesPath, "", "")

	assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
	if repository.roleCalls != 0 {
		t.Fatalf("repository role calls = %d, want 0 without credentials", repository.roleCalls)
	}
}

// TestTenancyCapabilitiesRejectsTamperedBearer proves the 401 gate for a
// present-but-malformed Authorization header.
func TestTenancyCapabilitiesRejectsTamperedBearer(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{roleResult: "member"}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := doTenancy(router, http.MethodGet, tenancyCapabilitiesPath, "", "bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
	if repository.roleCalls != 0 {
		t.Fatalf("repository role calls = %d, want 0 behind a malformed header", repository.roleCalls)
	}
}

// TestTenancyCapabilitiesVerifierUnavailable proves the 503 mapping when
// the identity provider is unreachable.
func TestTenancyCapabilitiesVerifierUnavailable(t *testing.T) {
	verifier := &stubVerifier{err: identity.ErrProviderUnavailable}
	repository := &stubTenancyRepository{roleResult: "member"}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyCapabilitiesPath)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	if repository.roleCalls != 0 {
		t.Fatalf("repository role calls = %d, want 0 behind an unreachable provider", repository.roleCalls)
	}
}

// TestTenancyCapabilitiesUnconfiguredFailsClosed proves the fail-closed
// 503 (design ruling 2): the capabilities path stays registered and
// answers with the dependency problem when no tenancy assembly is wired
// — the nil-deps 503 comes before any credential is read.
func TestTenancyCapabilitiesUnconfiguredFailsClosed(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	router := newTenancyTestRouter(t, nil)

	response := doTenancy(router, http.MethodGet, tenancyCapabilitiesPath, "", "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0 behind the nil-deps fail-closed", verifier.calls)
	}
}

// TestTenancyCapabilitiesWithoutRepositoryFailsClosed proves the
// per-dependency fail-closed: a wired verifier with a missing repository
// port still answers 503 before any verification.
func TestTenancyCapabilitiesWithoutRepositoryFailsClosed(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	router := newTenancyTestRouter(t, &httptransport.TenancyDependencies{Verifier: verifier})

	response := getWithAuth(router, tenancyCapabilitiesPath)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0 behind the missing-repository fail-closed", verifier.calls)
	}
}

// TestTenancyCapabilitiesRejectsMalformedTenantUUID proves the
// generated route gate: a non-uuid tenant path parameter is rejected
// with 400 before the handler (the generated parameter binder answers
// through the plain gin error writer, not the Problem hook, so the
// assertion pins the status and the untouched port only).
func TestTenancyCapabilitiesRejectsMalformedTenantUUID(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{roleResult: "member"}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, "/api/v1/tenants/not-a-uuid/capabilities")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q, want 400 for a malformed uuid", response.Code, response.Body.String())
	}
	if repository.roleCalls != 0 {
		t.Fatalf("repository role calls = %d, want 0 for a malformed uuid", repository.roleCalls)
	}
}

// TestTenancyCapabilitiesRepositoryErrorIsInternal proves the opaque 500:
// an unclassified repository failure never leaks its text.
func TestTenancyCapabilitiesRepositoryErrorIsInternal(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubTenancyRepository{roleErr: errTenancySaveFailed}
	router := newTenancyTestRouter(t, wiredTenancyDeps(verifier, repository))

	response := getWithAuth(router, tenancyCapabilitiesPath)

	assertIdentityProblem(t, response, http.StatusInternalServerError, "internal_error")
	if strings.Contains(response.Body.String(), errTenancySaveFailed.Error()) {
		t.Fatalf("body leaks the repository error: %s", response.Body.String())
	}
}
