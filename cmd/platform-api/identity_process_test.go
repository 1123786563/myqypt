package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1123786563/myqypt/db/migrations"
	"github.com/1123786563/myqypt/internal/adapter/postgres"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
)

// identityTestAudience is the configured audience every minted token
// targets (or deliberately misses, in the wrong-audience case).
const identityTestAudience = "test-audience"

// identityCallbackTimeout bounds one callback request. It is generous
// compared to processRequestTimeout because the database-outage leg must
// also absorb a failed pool acquire, not just a local rejection.
const identityCallbackTimeout = 10 * time.Second

// identityUserIDPattern pins the exact success body: one user_id whose
// value is a canonical lowercase UUID, and nothing else.
var identityUserIDPattern = regexp.MustCompile(`^\{"user_id":"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"\}$`)

// TestPlatformAPIIdentityProcess is the black-box identity acceptance
// test (design ruling 8): a real platform-api process, an in-test OIDC
// issuer (discovery + JWKS over httptest), and a real postgres database
// isolated as identity_blackbox_<pid> on the TEST_DATABASE_URL server.
// The matrix covers AC1 (first bind 201, idempotent rebind 200 with the
// same user id, exactly one user + one binding row), the AC2 failure
// determinism (401 causes, 503 causes, 500 during a database outage with
// recovery to the same user), the unconfigured 404 control, and evidence
// hygiene (tokens and claims never appear in any response body).
func TestPlatformAPIIdentityProcess(t *testing.T) {
	serverURL := os.Getenv("TEST_DATABASE_URL")
	if serverURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres integration test")
	}

	binary := buildPlatformAPI(t)
	dsn := createIdentityBlackboxDB(t, serverURL)
	db := openIdentityBlackboxDB(t, dsn)

	idp := startIdentityIdP(t)
	happyToken := idp.signToken(t, "subject-ac1", idp.validClaims("subject-ac1"))
	outageToken := idp.signToken(t, "subject-db-outage", idp.validClaims("subject-db-outage"))

	// bodies records every response body for the final evidence-hygiene
	// sweep. Subtests are sequential, so no locking is needed.
	var bodies []string
	record := func(t *testing.T, resp identityResponse) identityResponse {
		t.Helper()
		bodies = append(bodies, resp.body)
		return resp
	}

	t.Run("ac1 first bind 201 then idempotent rebind 200 same user", func(t *testing.T) {
		base := identityProcessBaseURL(t, binary, map[string]string{
			"DATABASE_URL":                    dsn,
			"PLATFORM_IDENTITY_OIDC_ISSUER":   idp.server.URL,
			"PLATFORM_IDENTITY_OIDC_AUDIENCE": identityTestAudience,
		})

		first := record(t, postIdentityCallback(t, base, "Bearer "+happyToken))
		if first.status != http.StatusCreated {
			t.Fatalf("first callback status = %d, want %d; body: %s", first.status, http.StatusCreated, first.body)
		}
		if !identityUserIDPattern.MatchString(first.body) {
			t.Fatalf("first callback body = %q, want exactly {\"user_id\": \"<uuid>\"}", first.body)
		}

		second := record(t, postIdentityCallback(t, base, "Bearer "+happyToken))
		if second.status != http.StatusOK {
			t.Fatalf("rebind callback status = %d, want %d; body: %s", second.status, http.StatusOK, second.body)
		}
		if second.body != first.body {
			t.Fatalf("rebind body = %q, want the identical first-bind body %q", second.body, first.body)
		}

		assertIdentityRows(t, db, idp.server.URL, "subject-ac1", 1, identityUserID(t, first.body))
	})

	t.Run("401 unauthorized causes", func(t *testing.T) {
		base := identityProcessBaseURL(t, binary, map[string]string{
			"DATABASE_URL":                    dsn,
			"PLATFORM_IDENTITY_OIDC_ISSUER":   idp.server.URL,
			"PLATFORM_IDENTITY_OIDC_AUDIENCE": identityTestAudience,
		})

		wrongKey := idp.signedByForeignToken(t, "subject-401")
		claims := func(mutate func(map[string]any)) string {
			c := idp.validClaims("subject-401")
			mutate(c)
			return idp.signToken(t, "subject-401", c)
		}
		cases := []struct {
			name          string
			authorization string
		}{
			{"no authorization header", ""},
			{"non bearer scheme", "Basic xyz"},
			{"token signed with wrong rsa key", "Bearer " + wrongKey},
			{"alg none token", "Bearer " + idp.noneToken(t)},
			{"wrong issuer claim", "Bearer " + claims(func(c map[string]any) { c["iss"] = "https://attacker.example/realms/main" })},
			{"wrong audience claim", "Bearer " + claims(func(c map[string]any) { c["aud"] = "other-api" })},
			{"expired token", "Bearer " + claims(func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() })},
			{"not yet valid nbf", "Bearer " + claims(func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() })},
			{"tampered signature", "Bearer " + idp.tamperedToken(t, "subject-401")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				resp := record(t, postIdentityCallback(t, base, tc.authorization))
				assertProblem(t, resp, http.StatusUnauthorized, httptransport.CodeUnauthorized)
			})
		}
	})

	t.Run("503 dependency unavailable when idp unreachable", func(t *testing.T) {
		deadIssuer := deadListenerURL(t)
		base := identityProcessBaseURL(t, binary, map[string]string{
			"DATABASE_URL":                    dsn,
			"PLATFORM_IDENTITY_OIDC_ISSUER":   deadIssuer,
			"PLATFORM_IDENTITY_OIDC_AUDIENCE": identityTestAudience,
		})
		// The bearer must be a well-formed RS256 JWT: a malformed one
		// fails parsing (401) before any network I/O, while a parseable
		// token drives the verifier into discovery against the dead
		// issuer, which reports the provider as unavailable.
		resp := record(t, postIdentityCallback(t, base, "Bearer "+idp.signToken(t, "subject-503", idp.validClaims("subject-503"))))
		assertProblem(t, resp, http.StatusServiceUnavailable, httptransport.CodeDependencyUnavailable)
	})

	t.Run("503 dependency unavailable when database unconfigured", func(t *testing.T) {
		// Identity configured, DATABASE_URL deliberately absent: the
		// repository port stays unwired and every callback fails closed
		// with 503 (design ruling 6).
		base := identityProcessBaseURL(t, binary, map[string]string{
			"PLATFORM_IDENTITY_OIDC_ISSUER":   idp.server.URL,
			"PLATFORM_IDENTITY_OIDC_AUDIENCE": identityTestAudience,
		})
		resp := record(t, postIdentityCallback(t, base, "Bearer "+happyToken))
		assertProblem(t, resp, http.StatusServiceUnavailable, httptransport.CodeDependencyUnavailable)
	})

	t.Run("500 during database outage then recovery to same user", func(t *testing.T) {
		proxy := startIdentityDBProxy(t, identityHostPort(t, dsn))
		base := identityProcessBaseURL(t, binary, map[string]string{
			"DATABASE_URL":                    identityProxyDSN(t, dsn, proxy.address()),
			"PLATFORM_IDENTITY_OIDC_ISSUER":   idp.server.URL,
			"PLATFORM_IDENTITY_OIDC_AUDIENCE": identityTestAudience,
		})

		first := record(t, postIdentityCallback(t, base, "Bearer "+outageToken))
		if first.status != http.StatusCreated {
			t.Fatalf("pre-outage callback status = %d, want %d; body: %s", first.status, http.StatusCreated, first.body)
		}
		if !identityUserIDPattern.MatchString(first.body) {
			t.Fatalf("pre-outage callback body = %q, want exactly {\"user_id\": \"<uuid>\"}", first.body)
		}

		proxy.breakDatabase()
		outage := record(t, postIdentityCallback(t, base, "Bearer "+outageToken))
		assertProblem(t, outage, http.StatusInternalServerError, httptransport.CodeInternalError)

		proxy.restoreDatabase()
		recovered := record(t, postIdentityCallback(t, base, "Bearer "+outageToken))
		if recovered.status != http.StatusOK {
			t.Fatalf("post-recovery callback status = %d, want %d; body: %s", recovered.status, http.StatusOK, recovered.body)
		}
		if recovered.body != first.body {
			t.Fatalf("post-recovery body = %q, want the identical pre-outage body %q", recovered.body, first.body)
		}

		assertIdentityRows(t, db, idp.server.URL, "subject-db-outage", 1, identityUserID(t, first.body))
	})

	t.Run("404 when identity env unconfigured", func(t *testing.T) {
		base := identityProcessBaseURL(t, binary, nil)
		resp := record(t, postIdentityCallback(t, base, "Bearer "+happyToken))
		assertProblem(t, resp, http.StatusNotFound, httptransport.CodeNotFound)
	})

	// Evidence hygiene: none of the minted tokens, claim values, or the
	// issuer URL may appear in any response body observed by the matrix.
	for i, body := range bodies {
		for _, secret := range []string{happyToken, outageToken, idp.server.URL, identityTestAudience, "subject-ac1", "subject-db-outage", "subject-401"} {
			if strings.Contains(body, secret) {
				t.Errorf("response body %d leaks secret material (matched a token, claim value, or issuer URL)", i)
				break
			}
		}
	}

	// Only the two valid identities ever reached the repository; every
	// rejected delivery must have left the tables untouched.
	assertTableTotals(t, db, 2, 2)
}

// identityResponse is one observed callback exchange.
type identityResponse struct {
	status int
	body   string
}

// postIdentityCallback delivers the identity callback with the given
// Authorization header value (empty sends none) and returns the status
// and body.
func postIdentityCallback(t *testing.T, baseURL, authorization string) identityResponse {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/internal/v1/identity/callback", nil)
	if err != nil {
		t.Fatalf("build callback request: %v", err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	client := &http.Client{Timeout: identityCallbackTimeout}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("post identity callback: %v", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read identity callback body: %v", err)
	}
	return identityResponse{status: response.StatusCode, body: string(raw)}
}

// assertProblem asserts an RFC 9457 problem payload with the expected
// status and stable code.
func assertProblem(t *testing.T, resp identityResponse, status int, code string) {
	t.Helper()
	if resp.status != status {
		t.Fatalf("callback status = %d, want %d; body: %s", resp.status, status, resp.body)
	}
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(resp.body), &problem); err != nil {
		t.Fatalf("decode problem body %q: %v", resp.body, err)
	}
	if problem.Code != code {
		t.Fatalf("problem code = %q, want %q; body: %s", problem.Code, code, resp.body)
	}
}

// createIdentityBlackboxDB provisions the isolated database
// identity_blackbox_<pid> on the TEST_DATABASE_URL server and applies
// every up migration (design ruling 8). A cleanup drops the database
// again; a database left behind by an earlier crashed run is removed
// first so the test is self-healing.
func createIdentityBlackboxDB(t *testing.T, serverURL string) string {
	t.Helper()
	name := fmt.Sprintf("identity_blackbox_%d", os.Getpid())

	admin, err := sql.Open("pgx", serverURL)
	if err != nil {
		t.Fatalf("open server admin handle: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatalf("ping TEST_DATABASE_URL server: %v", err)
	}

	if _, err := admin.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, name)); err != nil {
		t.Fatalf("drop stale %s: %v", name, err)
	}
	if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE %s`, name)); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, name)); err != nil {
			t.Logf("drop %s: %v", name, err)
		}
	})

	return identityDatabaseDSN(t, serverURL, name)
}

// openIdentityBlackboxDB opens a database/sql handle to the isolated
// database for row assertions. The "pgx" driver is registered through
// the postgres package's side-effect stdlib import (see migrate.go).
func openIdentityBlackboxDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open isolated database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := postgres.Migrate(ctx, db, migrations.FS); err != nil {
		_ = db.Close()
		t.Fatalf("migrate isolated database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// assertIdentityRows asserts the binding row count, the distinct bound
// user row count, and the binding's user id for one identity.
func assertIdentityRows(t *testing.T, db *sql.DB, provider, subject string, want int, userID string) {
	t.Helper()
	var bindings, users int
	var boundUserID string
	if err := db.QueryRow(`
		SELECT count(*),
		       count(DISTINCT platform_user_id),
		       min(platform_user_id::text)
		FROM identity_bindings
		WHERE identity_provider = $1 AND subject = $2`,
		provider, subject).Scan(&bindings, &users, &boundUserID); err != nil {
		t.Fatalf("count rows for (%s, %s): %v", provider, subject, err)
	}
	if bindings != want || users != want {
		t.Fatalf("rows for (%s, %s) = %d binding(s)/%d user(s), want %d/%d", provider, subject, bindings, users, want, want)
	}
	if boundUserID != userID {
		t.Fatalf("bound user id = %q, want the id returned by the callback %q", boundUserID, userID)
	}
}

// assertTableTotals asserts the whole-table row counts, proving no
// rejected or duplicated delivery left business effects behind.
func assertTableTotals(t *testing.T, db *sql.DB, wantUsers, wantBindings int) {
	t.Helper()
	var users, bindings int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM platform_users),
		(SELECT count(*) FROM identity_bindings)`).Scan(&users, &bindings); err != nil {
		t.Fatalf("count table totals: %v", err)
	}
	if users != wantUsers || bindings != wantBindings {
		t.Fatalf("table totals = %d user(s)/%d binding(s), want %d/%d", users, bindings, wantUsers, wantBindings)
	}
}

// identityProcessEnv builds the child environment: every variable the
// identity assembly reads is dropped from the inherited environment and
// set only from values, so the parent test environment cannot leak into
// the process under test.
func identityProcessEnv(values map[string]string) []string {
	controlled := map[string]bool{
		"PLATFORM_API_ADDR":               true,
		"PLATFORM_API_ADDR_FILE":          true,
		"DATABASE_URL":                    true,
		"PLATFORM_IDENTITY_OIDC_ISSUER":   true,
		"PLATFORM_IDENTITY_OIDC_AUDIENCE": true,
	}
	env := make([]string, 0, len(os.Environ())+len(values)+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if controlled[name] {
			continue
		}
		env = append(env, entry)
	}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		env = append(env, name+"="+values[name])
	}
	return env
}

// identityProcessBaseURL starts a real platform-api serve process with
// the given controlled environment, waits for /livez, and returns its
// base URL.
func identityProcessBaseURL(t *testing.T, binary string, env map[string]string) string {
	t.Helper()
	command := exec.Command(binary, "serve")
	command.Env = identityProcessEnv(env)
	addressFile := filepath.Join(t.TempDir(), "platform-api-address")
	command.Env = append(command.Env, "PLATFORM_API_ADDR=127.0.0.1:0", "PLATFORM_API_ADDR_FILE="+addressFile)
	output := &outputBuffer{}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		t.Fatalf("start platform-api: %v", err)
	}
	process := &platformProcess{
		command:       command,
		output:        output,
		done:          make(chan error, 1),
		addressFile:   addressFile,
		reportAddress: true,
	}
	go func() {
		process.done <- command.Wait()
	}()
	t.Cleanup(func() {
		// Never read command.ProcessState here: the waiter goroutine
		// above writes it inside command.Wait, and an unsynchronized
		// read from this goroutine is a data race. Instead drain a
		// completed wait (the receive establishes the happens-before
		// edge with the waiter), kill unconditionally — killing an
		// already-exited process returns a harmless error — and let
		// waitForProcess select on the done channel.
		select {
		case <-process.done:
		default:
		}
		_ = command.Process.Kill()
		if err := waitForProcess(process, processStartupTimeout); err != nil {
			t.Logf("cleanup wait for platform-api: %v\n%s", err, output.String())
		}
	})
	address := waitForReportedAddress(t, process)
	waitForLivez(t, process, address)
	return "http://" + address
}

// identityUserID extracts the user_id from a callback success body.
func identityUserID(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode callback body %q: %v", body, err)
	}
	return payload.UserID
}

// identityIdP is an in-test OIDC identity provider: an RSA signing key
// served as a JWKS document plus the discovery document binding the
// issuer to the test server URL (the same technique as the
// internal/adapter/oidc verifier tests).
type identityIdP struct {
	server *httptest.Server
	key    *rsa.PrivateKey
}

// startIdentityIdP generates a fresh RSA keypair and serves discovery
// and JWKS for it.
func startIdentityIdP(t *testing.T) *identityIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	const kid = "identity-blackbox-key-1"
	idp := &identityIdP{key: key}
	idp.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			fmt.Fprintf(w, `{"issuer":%q,"jwks_uri":%q}`, "http://"+r.Host, "http://"+r.Host+"/jwks")
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
				"kty": "RSA",
				"kid": kid,
				"use": "sig",
				"alg": "RS256",
				"n":   identityB64(key.PublicKey.N.Bytes()),
				"e":   identityB64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(idp.server.Close)
	return idp
}

// validClaims returns claims the configured verifier accepts for
// subject.
func (idp *identityIdP) validClaims(subject string) map[string]any {
	return map[string]any{
		"iss": idp.server.URL,
		"sub": subject,
		"aud": identityTestAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

// signToken hand-assembles and signs a compact RS256 JWT for subject.
func (idp *identityIdP) signToken(t *testing.T, subject string, claims map[string]any) string {
	t.Helper()
	return identitySignJWT(t, idp.key,
		identityB64JSON(t, map[string]any{"alg": "RS256", "typ": "JWT", "kid": "identity-blackbox-key-1"}),
		identityB64JSON(t, claims))
}

// signedByForeignToken signs valid-looking claims with an unrelated key.
func (idp *identityIdP) signedByForeignToken(t *testing.T, subject string) string {
	t.Helper()
	foreign, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate foreign rsa key: %v", err)
	}
	return identitySignJWT(t, foreign,
		identityB64JSON(t, map[string]any{"alg": "RS256", "typ": "JWT", "kid": "identity-blackbox-key-1"}),
		identityB64JSON(t, idp.validClaims(subject)))
}

// noneToken assembles an alg=none token with an empty signature.
func (idp *identityIdP) noneToken(t *testing.T) string {
	t.Helper()
	header := identityB64JSON(t, map[string]any{"alg": "none", "typ": "JWT"})
	return header + "." + identityB64JSON(t, idp.validClaims("subject-401")) + "."
}

// tamperedToken flips a byte of a valid token's signature.
func (idp *identityIdP) tamperedToken(t *testing.T, subject string) string {
	t.Helper()
	parts := strings.Split(idp.signToken(t, subject, idp.validClaims(subject)), ".")
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	signature[0] ^= 0xff
	parts[2] = identityB64(signature)
	return strings.Join(parts, ".")
}

func identityB64(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func identityB64JSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return identityB64(raw)
}

// identitySignJWT returns a compact-serialized RS256 JWT over the given
// base64url header and claims segments.
func identitySignJWT(t *testing.T, key *rsa.PrivateKey, header, claims string) string {
	t.Helper()
	signingInput := header + "." + claims
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signingInput + "." + identityB64(signature)
}

// deadListenerURL reserves an ephemeral port, releases it, and returns
// its URL: connections to it are refused, standing in for an IdP that
// is down.
func deadListenerURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve dead issuer port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release dead issuer port: %v", err)
	}
	return "http://" + address
}

// identityDBProxy is a controllable TCP proxy in front of the real
// postgres server: the process under test dials the proxy and the proxy
// splices the bytes through. breakDatabase kills every established
// connection and makes every new dial land on a socket that closes
// immediately — the deterministic, self-contained stand-in for a
// mid-flight database outage at the process seam. restoreDatabase
// reopens the forwarding path so the same process recovers against the
// same database.
type identityDBProxy struct {
	listener net.Listener
	target   string

	mu    sync.Mutex
	up    bool
	conns map[net.Conn]net.Conn
}

// startIdentityDBProxy starts the proxy in front of target (host:port).
func startIdentityDBProxy(t *testing.T, target string) *identityDBProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen db proxy: %v", err)
	}
	proxy := &identityDBProxy{
		listener: listener,
		target:   target,
		up:       true,
		conns:    make(map[net.Conn]net.Conn),
	}
	go proxy.acceptLoop()
	t.Cleanup(func() {
		_ = listener.Close()
		proxy.killConns()
	})
	return proxy
}

func (p *identityDBProxy) address() string {
	return p.listener.Addr().String()
}

func (p *identityDBProxy) acceptLoop() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.handle(client)
	}
}

func (p *identityDBProxy) handle(client net.Conn) {
	p.mu.Lock()
	up := p.up
	p.mu.Unlock()
	if !up {
		_ = client.Close()
		return
	}
	server, err := net.Dial("tcp", p.target)
	if err != nil {
		_ = client.Close()
		return
	}
	p.mu.Lock()
	if !p.up {
		p.mu.Unlock()
		_ = client.Close()
		_ = server.Close()
		return
	}
	p.conns[client] = server
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			delete(p.conns, client)
			p.mu.Unlock()
			_ = client.Close()
			_ = server.Close()
		}()
		done := make(chan struct{}, 2)
		go func() {
			_, _ = io.Copy(server, client)
			done <- struct{}{}
		}()
		go func() {
			_, _ = io.Copy(client, server)
			done <- struct{}{}
		}()
		<-done
	}()
}

// breakDatabase enters the outage state: established pairs are killed
// and new connections are accepted then closed immediately.
func (p *identityDBProxy) breakDatabase() {
	p.mu.Lock()
	p.up = false
	p.mu.Unlock()
	p.killConns()
}

// restoreDatabase leaves the outage state; subsequent dials are spliced
// to the real server again.
func (p *identityDBProxy) restoreDatabase() {
	p.mu.Lock()
	p.up = true
	p.mu.Unlock()
}

func (p *identityDBProxy) killConns() {
	p.mu.Lock()
	pairs := make([]net.Conn, 0, 2*len(p.conns))
	for client, server := range p.conns {
		pairs = append(pairs, client, server)
	}
	p.mu.Unlock()
	for _, conn := range pairs {
		_ = conn.Close()
	}
}

// identityDatabaseDSN rewrites a postgres URL DSN to target database
// name.
func identityDatabaseDSN(t *testing.T, serverURL, name string) string {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		t.Fatalf("TEST_DATABASE_URL must be a postgres:// URL DSN")
	}
	parsed.Path = "/" + name
	return parsed.String()
}

// identityProxyDSN rewrites the DSN's host:port to the proxy address.
func identityProxyDSN(t *testing.T, dsn, hostPort string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	parsed.Host = hostPort
	return parsed.String()
}

// identityHostPort extracts host:port from a URL DSN.
func identityHostPort(t *testing.T, dsn string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Host == "" {
		t.Fatalf("parse dsn for host:port: %v", err)
	}
	return parsed.Host
}
