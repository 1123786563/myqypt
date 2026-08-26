package httptransport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1123786563/myqypt/internal/application/identity"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

// stubVerifier stubs the identity.Verifier port with no network I/O.
type stubVerifier struct {
	identity  identity.VerifiedIdentity
	err       error
	calls     int
	lastToken string
}

func (v *stubVerifier) Verify(_ context.Context, rawToken string) (identity.VerifiedIdentity, error) {
	v.calls++
	v.lastToken = rawToken
	return v.identity, v.err
}

// stubRepository stubs the identity.Repository port with no database I/O.
type stubRepository struct {
	user         identity.User
	created      bool
	err          error
	calls        int
	lastProvider string
	lastSubject  string
}

func (r *stubRepository) BindOrLoad(_ context.Context, identityProvider, subject string) (identity.User, bool, error) {
	r.calls++
	r.lastProvider = identityProvider
	r.lastSubject = subject
	return r.user, r.created, r.err
}

// verifiedTestIdentity is the identity the stub verifier attests on success.
var verifiedTestIdentity = identity.VerifiedIdentity{
	Issuer:  "https://issuer.example.test",
	Subject: "subject-1",
}

const (
	// identityCallbackPath is the internal callback route under test.
	identityCallbackPath = "/internal/v1/identity/callback"
	// bearerTestToken is an opaque stub token: the transport never parses
	// it, it only forwards it to the Verifier port.
	bearerTestToken = "stub-bearer-token"
	// testUserID is the platform user the stub repository binds.
	testUserID = "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88a"
)

// newIdentityTestRouter builds the production router with the given identity
// assembly; nil leaves the endpoint unregistered.
func newIdentityTestRouter(t *testing.T, deps *httptransport.IdentityDependencies) http.Handler {
	t.Helper()
	useTestGinMode(t)
	return httptransport.NewRouter(httptransport.Dependencies{Version: "test", Identity: deps})
}

// postIdentityCallback issues a POST against the callback route with an
// optional Authorization header value.
func postIdentityCallback(handler http.Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, identityCallbackPath, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// identityWireProblem mirrors the problem fields asserted on. Local struct
// tags are an independent source of truth, not the production tags.
type identityWireProblem struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

// assertIdentityProblem asserts a Problem Details response for a stable code.
func assertIdentityProblem(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status=%d want=%d body=%q", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content-type=%q want application/problem+json", got)
	}
	var p identityWireProblem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode problem body %q: %v", rec.Body.String(), err)
	}
	if p.Code != wantCode {
		t.Fatalf("problem.code=%q want=%q body=%q", p.Code, wantCode, rec.Body.String())
	}
	if wantType := "https://api.myqypt.dev/problems/" + wantCode; p.Type != wantType {
		t.Fatalf("problem.type=%q want=%q", p.Type, wantType)
	}
}

// assertUserIdentityBody asserts the exact AC3-hygienic success body: only
// the platform user id, never the token or any claim.
func assertUserIdentityBody(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status=%d want=%d body=%q", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content-type=%q want application/json", got)
	}
	if want := `{"user_id":"` + testUserID + `"}`; rec.Body.String() != want {
		t.Fatalf("body=%q want the exact payload %q", rec.Body.String(), want)
	}
	if strings.Contains(rec.Body.String(), bearerTestToken) {
		t.Fatalf("body leaks the bearer token: %s", rec.Body.String())
	}
}

// wiredIdentityDeps returns a fully wired assembly whose stubs return no
// error; individual tests override stub fields.
func wiredIdentityDeps(verifier *stubVerifier, repository *stubRepository) *httptransport.IdentityDependencies {
	return &httptransport.IdentityDependencies{Verifier: verifier, Repository: repository}
}

func TestIdentityCallbackRejectsMissingAuthorization(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubRepository{user: identity.User{ID: testUserID}, created: true}
	router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

	response := postIdentityCallback(router, "")

	assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0 without credentials", verifier.calls)
	}
	if repository.calls != 0 {
		t.Fatalf("repository calls = %d, want 0 without credentials", repository.calls)
	}
}

func TestIdentityCallbackRejectsMalformedAuthorization(t *testing.T) {
	cases := []struct {
		name          string
		authorization string
	}{
		{"non-bearer scheme", "Basic dXNlcjpwYXNz"},
		{"lowercase scheme", "bearer " + bearerTestToken},
		{"scheme without token", "Bearer "},
		{"scheme without space", "Bearer" + bearerTestToken},
		{"token with interior whitespace", "Bearer abc def"},
		{"tab after scheme", "Bearer\t" + bearerTestToken},
		{"empty header value", " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifier := &stubVerifier{identity: verifiedTestIdentity}
			repository := &stubRepository{user: identity.User{ID: testUserID}, created: true}
			router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

			response := postIdentityCallback(router, tc.authorization)

			assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
			if verifier.calls != 0 {
				t.Fatalf("verifier calls = %d, want 0 for a malformed header", verifier.calls)
			}
			if repository.calls != 0 {
				t.Fatalf("repository calls = %d, want 0 for a malformed header", repository.calls)
			}
		})
	}
}

func TestIdentityCallbackRejectsInvalidToken(t *testing.T) {
	verifier := &stubVerifier{err: identity.ErrInvalidToken}
	repository := &stubRepository{user: identity.User{ID: testUserID}, created: true}
	router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

	response := postIdentityCallback(router, "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusUnauthorized, "unauthorized")
	if verifier.calls != 1 {
		t.Fatalf("verifier calls = %d, want 1", verifier.calls)
	}
	if repository.calls != 0 {
		t.Fatalf("repository calls = %d, want 0 behind a failed verification", repository.calls)
	}
}

func TestIdentityCallbackFirstBindCreatesUser(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubRepository{user: identity.User{ID: testUserID}, created: true}
	router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

	response := postIdentityCallback(router, "Bearer "+bearerTestToken)

	assertUserIdentityBody(t, response, http.StatusCreated)
	if verifier.calls != 1 || verifier.lastToken != bearerTestToken {
		t.Fatalf("verifier saw (%d calls, token %q), want (1, %q)", verifier.calls, verifier.lastToken, bearerTestToken)
	}
	if repository.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repository.calls)
	}
	if repository.lastProvider != verifiedTestIdentity.Issuer || repository.lastSubject != verifiedTestIdentity.Subject {
		t.Fatalf(
			"repository got (%q, %q), want the verified identity (%q, %q)",
			repository.lastProvider, repository.lastSubject,
			verifiedTestIdentity.Issuer, verifiedTestIdentity.Subject,
		)
	}
}

func TestIdentityCallbackRebindReturnsSameBody(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubRepository{user: identity.User{ID: testUserID}}
	router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

	response := postIdentityCallback(router, "Bearer "+bearerTestToken)

	assertUserIdentityBody(t, response, http.StatusOK)
	if repository.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repository.calls)
	}
}

func TestIdentityCallbackVerifierUnavailable(t *testing.T) {
	verifier := &stubVerifier{err: identity.ErrProviderUnavailable}
	repository := &stubRepository{user: identity.User{ID: testUserID}, created: true}
	router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

	response := postIdentityCallback(router, "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	if repository.calls != 0 {
		t.Fatalf("repository calls = %d, want 0 behind an unreachable provider", repository.calls)
	}
}

func TestIdentityCallbackWithoutRepositoryFailsClosed(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	router := newIdentityTestRouter(t, &httptransport.IdentityDependencies{Verifier: verifier})

	response := postIdentityCallback(router, "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	if verifier.calls != 1 {
		t.Fatalf("verifier calls = %d, want 1", verifier.calls)
	}
}

func TestIdentityCallbackWithoutVerifierFailsClosed(t *testing.T) {
	repository := &stubRepository{user: identity.User{ID: testUserID}, created: true}
	router := newIdentityTestRouter(t, &httptransport.IdentityDependencies{Repository: repository})

	response := postIdentityCallback(router, "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusServiceUnavailable, "dependency_unavailable")
	if repository.calls != 0 {
		t.Fatalf("repository calls = %d, want 0 without a wired verifier", repository.calls)
	}
}

// errBindFailed is the sentinel error the stub repository returns in the
// generic-error case.
var errBindFailed = errors.New("repository bind failed")

func TestIdentityCallbackRepositoryErrorIsInternal(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubRepository{err: errBindFailed}
	router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

	response := postIdentityCallback(router, "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusInternalServerError, "internal_error")
	if repository.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repository.calls)
	}
	if strings.Contains(response.Body.String(), errBindFailed.Error()) {
		t.Fatalf("body leaks the repository error: %s", response.Body.String())
	}
}

func TestIdentityCallbackUnconfiguredIsNotRegistered(t *testing.T) {
	router := newIdentityTestRouter(t, nil)

	response := postIdentityCallback(router, "Bearer "+bearerTestToken)

	assertIdentityProblem(t, response, http.StatusNotFound, "not_found")
}

func TestIdentityCallbackIgnoresForgedIdentity(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubRepository{user: identity.User{ID: testUserID}, created: true}
	router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

	// The request forges issuer and subject in the body and in invented
	// headers; only the token the stub verifier attests may count.
	request := httptest.NewRequest(
		http.MethodPost,
		identityCallbackPath,
		strings.NewReader(`{"issuer":"https://forged.example.test","subject":"forged-subject"}`),
	)
	request.Header.Set("Authorization", "Bearer "+bearerTestToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Issuer", "https://forged.example.test")
	request.Header.Set("X-Subject", "forged-subject")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertUserIdentityBody(t, response, http.StatusCreated)
	if repository.lastProvider != verifiedTestIdentity.Issuer || repository.lastSubject != verifiedTestIdentity.Subject {
		t.Fatalf(
			"repository got the forged identity (%q, %q), want the verified one (%q, %q)",
			repository.lastProvider, repository.lastSubject,
			verifiedTestIdentity.Issuer, verifiedTestIdentity.Subject,
		)
	}
	if strings.Contains(response.Body.String(), "forged") {
		t.Fatalf("body leaks the forged identity: %s", response.Body.String())
	}
}

func TestIdentityCallbackRejectsWrongMethod(t *testing.T) {
	verifier := &stubVerifier{identity: verifiedTestIdentity}
	repository := &stubRepository{user: identity.User{ID: testUserID}, created: true}
	router := newIdentityTestRouter(t, wiredIdentityDeps(verifier, repository))

	// The callback registers POST only; the router's global
	// HandleMethodNotAllowed maps other methods to the 405 problem.
	request := httptest.NewRequest(http.MethodGet, identityCallbackPath, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertIdentityProblem(t, response, http.StatusMethodNotAllowed, "method_not_allowed")
	if verifier.calls != 0 || repository.calls != 0 {
		t.Fatalf("stubs touched on a rejected method: verifier=%d repository=%d", verifier.calls, repository.calls)
	}
}
