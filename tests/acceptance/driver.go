package acceptance

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/1123786563/myqypt/tests/platformtest"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the database/sql "pgx" driver
)

// seamLighthouse names the black-box seam: the journey drives the stack
// purely from the outside, like a lighthouse watching the harbor.
const seamLighthouse = "lighthouse-black-box"

// stackStartupCommand is the precise recipe that brings the acceptance
// stack up; it is embedded in precheck failures and the skip message.
const stackStartupCommand = "cd deploy/compose && PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose up -d --wait"

// stackResetCommand tears the stack down with its volumes (compose
// interpolation needs the six required variables even for down). The
// journey proves the FIRST bind (201): the platform-postgres-data named
// volume survives a plain down/up cycle, so rerunning against a warm
// stack would hit the idempotent 200 instead. Every rerun must start
// from this reset — the skip message says so and the stale-state
// precheck enforces it.
const stackResetCommand = "cd deploy/compose && PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose down -v"

func init() {
	platformtest.Register(seamLighthouse, lighthouseDriver{})
}

// Fixture credentials for the ephemeral acceptance stack. None of these is
// a real secret: they exist only inside the throwaway compose containers
// brought up by the documented integration command (whose postgres
// password is fixed on the command line anyway). Every value can be
// overridden through the environment so a differently-provisioned stack
// still runs. They never flow into reports or evidence.
var (
	fixtureAdminPassword    = envOrDefault("T01_CASDOOR_ADMIN_PASSWORD", "123")
	fixtureClientSecret     = envOrDefault("T01_CASDOOR_CLIENT_SECRET", "t01-acceptance-secret")
	fixtureUserPassword     = envOrDefault("T01_CASDOOR_USER_PASSWORD", "t01-accept-password")
	fixturePostgresPassword = envOrDefault("T01_PLATFORM_POSTGRES_PASSWORD", "t01-accept-pw")
)

// casdoorSigningCert is the global certificate Casdoor signs JWTs with;
// the JWKS at /.well-known/jwks publishes its key as kid "cert-built-in".
// An application must reference it explicitly to issue verifiable tokens.
const casdoorSigningCert = "cert-built-in"

// userIDPattern pins the exact success body: one user_id whose value is a
// canonical lowercase UUID, and nothing else.
var userIDPattern = regexp.MustCompile(`^\{"user_id":"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"\}$`)

// journeyTimeout bounds each single HTTP exchange of the journey.
const journeyTimeout = 10 * time.Second

// lighthouseDriver executes the T01 identity binding journey against the
// real compose stack: provision Casdoor, mint a real RS256 token, bind it
// through the callback, replay it, attack it, then assert the database
// state. Everything is driven over the wire; nothing links into the
// platform internals.
type lighthouseDriver struct{}

// assertionNames is the exact set declared by
// scenarios/t01-identity-binding.yaml, in declaration order. The harness
// reconciles by name and rejects any drift, so this list and the YAML
// must move together.
var assertionNames = []string{
	"first_bind_status",
	"first_bind_body_user_id_uuid",
	"replay_status",
	"replay_body_identical",
	"binding_count",
	"binding_subject_matches_token",
	"denial_missing_authorization_status",
	"denial_tampered_signature_status",
	"denial_binding_count_delta",
}

// journey holds the scenario inputs the driver reads, with defaults.
type journey struct {
	issuer       string
	apiBase      string
	callbackBase string
	callbackPath string
	org          string
	appName      string
	clientID     string
	username     string
	replays      int
	pgHost       string
	pgPort       string
	pgDatabase   string
	pgUser       string
}

// Execute runs the whole journey and returns one assertion result per
// declared assertion name. Details carry facts only (status codes, row
// counts, match booleans) — never tokens, subjects, or credentials.
func (lighthouseDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	j := journeyFromInputs(scenario.Inputs)

	if err := j.precheckStack(ctx); err != nil {
		return failedReport(err.Error()), nil
	}

	client := newJourneyClient()

	if err := j.provisionCasdoor(ctx, client); err != nil {
		return failedReport(fmt.Sprintf("casdoor provisioning failed: %v", err)), nil
	}

	token, claims, err := j.mintToken(ctx, client)
	if err != nil {
		return failedReport(fmt.Sprintf("token mint failed: %v", err)), nil
	}
	if claims.Issuer != j.issuer {
		return failedReport(fmt.Sprintf("token iss %q does not match the configured issuer (value omitted)", claims.Issuer)), nil
	}
	if !audienceContains(claims.Audience, j.clientID) {
		return failedReport("token aud does not include the provisioned client id"), nil
	}
	if claims.Subject == "" {
		return failedReport("token sub is empty"), nil
	}

	// Stale-state precheck (quality review ruling 1b): the journey proves
	// the FIRST bind (201), so the platform database must hold zero rows
	// for this identity before anything is delivered. A warm stack whose
	// named postgres volume survived a previous run already carries the
	// binding, and the first callback would correctly answer the
	// idempotent 200 — fail closed here instead of reporting a confusing
	// 201-vs-200 assertion failure. This query doubles as the proven
	// zero baseline for the binding_count assertion. Note: the #100
	// harness redacts every driver summary and assertion detail from the
	// returned report and evidence, so neither this message nor any other
	// driver-side explanation reaches the operator through test output —
	// the skip message and its reset command are the only visible
	// guidance, which is why they must stay in sync with this check.
	existing, err := j.countBindings(ctx, claims.Subject)
	if err != nil {
		return platformtest.Report{}, fmt.Errorf("stale-state precheck failed (platform database unreachable): %w", err)
	}
	if existing > 0 {
		return platformtest.Report{}, fmt.Errorf("stale platform state: %d binding row(s) already exist for this identity; the journey proves the first bind and requires a clean platform database — reset the stack and rerun: %s", existing, stackResetCommand)
	}

	results := map[string]platformtest.AssertionResult{}
	record := func(name string, passed bool, details string) {
		results[name] = platformtest.AssertionResult{Name: name, Passed: passed, Details: details}
	}

	// Happy path: first bind must create the user (201) with the exact
	// {"user_id":"<uuid>"} body.
	first := j.postCallback(ctx, client, "Bearer "+token)
	record("first_bind_status", first.Status == http.StatusCreated, fmt.Sprintf("status=%d", first.Status))
	bodyIsUUID := first.Status == http.StatusCreated && userIDPattern.MatchString(first.Body)
	record("first_bind_body_user_id_uuid", bodyIsUUID, fmt.Sprintf("canonical_uuid=%t", bodyIsUUID))
	userID := ""
	if bodyIsUUID {
		var payload struct {
			UserID string `json:"user_id"`
		}
		_ = json.Unmarshal([]byte(first.Body), &payload)
		userID = payload.UserID
	}

	// Idempotent path: every replay of the same token must return 200
	// with the byte-identical body.
	replayOK, replayIdentical := true, true
	for i := 0; i < j.replays; i++ {
		replay := j.postCallback(ctx, client, "Bearer "+token)
		if replay.Status != http.StatusOK {
			replayOK = false
		}
		if replay.Body != first.Body {
			replayIdentical = false
		}
	}
	record("replay_status", replayOK, fmt.Sprintf("deliveries=%d all_200=%t", j.replays, replayOK))
	record("replay_body_identical", replayIdentical, fmt.Sprintf("identical=%t", replayIdentical))

	// Denial path: unverifiable credentials must yield 401 without any
	// binding effect. The tampered token flips one deterministic byte of
	// the signature segment of the valid token.
	rowsBefore, err := j.countBindings(ctx, claims.Subject)
	if err != nil {
		return failedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	missing := j.postCallback(ctx, client, "")
	tamperedToken, err := tamperSignature(token)
	if err != nil {
		return failedReport(fmt.Sprintf("tampered token construction failed: %v", err)), nil
	}
	tampered := j.postCallback(ctx, client, "Bearer "+tamperedToken)
	record("denial_missing_authorization_status", missing.Status == http.StatusUnauthorized, fmt.Sprintf("status=%d", missing.Status))
	record("denial_tampered_signature_status", tampered.Status == http.StatusUnauthorized, fmt.Sprintf("status=%d", tampered.Status))

	rowsAfter, boundUserID, distinctUsers, err := j.bindingState(ctx, claims.Subject)
	if err != nil {
		return failedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("denial_binding_count_delta", rowsAfter-rowsBefore == 0, fmt.Sprintf("delta=%d", rowsAfter-rowsBefore))

	// Database truth: exactly one binding for the verified subject,
	// bound to exactly the user the callback returned. zero_baseline is
	// proven by the stale-state precheck before the first delivery.
	record("binding_count", rowsBefore == 1, fmt.Sprintf("zero_baseline=true rows=%d", rowsBefore))
	subjectMatches := rowsBefore == 1 && distinctUsers == 1 && boundUserID == userID && userID != ""
	record("binding_subject_matches_token", subjectMatches, fmt.Sprintf("rows=%d distinct_users=%d user_id_matches=%t", rowsBefore, distinctUsers, boundUserID == userID))

	ordered := make([]platformtest.AssertionResult, 0, len(assertionNames))
	passed := true
	for _, name := range assertionNames {
		result, ok := results[name]
		if !ok {
			return failedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}

	summary := fmt.Sprintf("first=%d replay_200=%t replay_identical=%t denial_missing_auth=%d denial_tampered=%d bindings=%d delta=%d",
		first.Status, replayOK, replayIdentical, missing.Status, tampered.Status, rowsBefore, rowsAfter-rowsBefore)
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// journeyFromInputs reads the scenario inputs with the same defaults the
// compose stack is wired with.
func journeyFromInputs(inputs map[string]any) journey {
	return journey{
		issuer:       inputString(inputs, "issuer", "http://casdoor:8000"),
		apiBase:      inputString(inputs, "casdoor_api_base", "http://localhost:8000"),
		callbackBase: inputString(inputs, "callback_base", "http://localhost:8080"),
		callbackPath: inputString(inputs, "callback_path", "/internal/v1/identity/callback"),
		org:          inputString(inputs, "casdoor_organization", "t01-accept-org"),
		appName:      inputString(inputs, "casdoor_application_name", "t01-acceptance-app"),
		clientID:     inputString(inputs, "casdoor_client_id", "t01-acceptance"),
		username:     inputString(inputs, "casdoor_username", "t01-accept"),
		replays:      inputInt(inputs, "replay_deliveries", 1),
		pgHost:       inputString(inputs, "postgres_host", "localhost"),
		pgPort:       inputString(inputs, "postgres_port", "5432"),
		pgDatabase:   inputString(inputs, "postgres_database", "platform"),
		pgUser:       inputString(inputs, "postgres_user", "platform"),
	}
}

// precheckStack verifies both stack entry points answer before anything
// else runs, failing with the exact startup command in the message.
func (j journey) precheckStack(ctx context.Context) error {
	casdoorURL := strings.TrimSuffix(j.apiBase, "/") + "/api/health"
	if err := probe(ctx, casdoorURL); err != nil {
		return fmt.Errorf("casdoor unreachable at %s (%v); start the stack first: %s", casdoorURL, err, stackStartupCommand)
	}
	livezURL := strings.TrimSuffix(j.callbackBase, "/") + "/livez"
	if err := probe(ctx, livezURL); err != nil {
		return fmt.Errorf("platform-api unreachable at %s (%v); start the stack first: %s", livezURL, err, stackStartupCommand)
	}
	return nil
}

// probe issues a GET and accepts any 2xx status.
func probe(ctx context.Context, target string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: journeyTimeout}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("http status %d", response.StatusCode)
	}
	return nil
}

// newJourneyClient builds the HTTP client with a cookie jar so the admin
// login session sticks for the provisioning calls.
func newJourneyClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Timeout: journeyTimeout, Jar: jar}
}

// casdoorResponse is the envelope every Casdoor API call returns.
type casdoorResponse struct {
	Status string          `json:"status"`
	Msg    string          `json:"msg"`
	Data   json.RawMessage `json:"data"`
}

// callCasdoor performs one JSON API call against Casdoor.
func callCasdoor(ctx context.Context, client *http.Client, method, target string, body any) (casdoorResponse, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return casdoorResponse{}, err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return casdoorResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return casdoorResponse{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return casdoorResponse{}, err
	}
	var envelope casdoorResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return casdoorResponse{}, fmt.Errorf("%s %s: non-json response (http %d)", method, target, response.StatusCode)
	}
	return envelope, nil
}

// provisionCasdoor makes the stack ready to mint tokens for the journey:
// an admin session, a dedicated organization, an application pinned to
// the fixed client id and the global signing cert with the password
// grant enabled, and the test user. Every step is idempotent.
func (j journey) provisionCasdoor(ctx context.Context, client *http.Client) error {
	loginBody := map[string]any{
		"type":         "login",
		"username":     "admin",
		"password":     fixtureAdminPassword,
		"application":  "app-built-in",
		"organization": "built-in",
		"autoSignin":   true,
	}
	envelope, err := callCasdoor(ctx, client, http.MethodPost, j.apiBase+"/api/login", loginBody)
	if err != nil {
		return fmt.Errorf("admin login: %w", err)
	}
	if envelope.Status != "ok" {
		return fmt.Errorf("admin login rejected (password value omitted)")
	}

	if err := j.ensureOrganization(ctx, client); err != nil {
		return err
	}
	if err := j.ensureApplication(ctx, client); err != nil {
		return err
	}
	return j.ensureUser(ctx, client)
}

// ensureOrganization creates the journey organization when missing.
func (j journey) ensureOrganization(ctx context.Context, client *http.Client) error {
	envelope, err := callCasdoor(ctx, client, http.MethodGet, j.apiBase+"/api/get-organization?id="+url.PathEscape("admin/"+j.org), nil)
	if err != nil {
		return fmt.Errorf("get organization: %w", err)
	}
	if envelope.Status == "ok" && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		return nil
	}
	body := map[string]any{
		"owner":           "admin",
		"name":            j.org,
		"displayName":     "T01 Acceptance Org",
		"websiteUrl":      "https://t01-acceptance.invalid",
		"passwordType":    "plain",
		"passwordOptions": []string{"AtLeast6"},
		"countryCodes":    []string{"US"},
		"tags":            []string{},
		"languages":       []string{"en"},
	}
	envelope, err = callCasdoor(ctx, client, http.MethodPost, j.apiBase+"/api/add-organization", body)
	if err != nil {
		return fmt.Errorf("add organization: %w", err)
	}
	if envelope.Status != "ok" {
		return fmt.Errorf("add organization rejected: %s", envelope.Msg)
	}
	return nil
}

// applicationBody is the canonical application definition: fixed client
// id (the token audience), the global signing cert, JWT tokens, and the
// password grant enabled. The application's owner must equal the
// organization's owner or Casdoor refuses user creation in that
// organization (spike finding).
func (j journey) applicationBody() map[string]any {
	return map[string]any{
		"owner":                "admin",
		"name":                 j.appName,
		"organization":         j.org,
		"clientId":             j.clientID,
		"clientSecret":         fixtureClientSecret,
		"cert":                 casdoorSigningCert,
		"expireInHours":        24,
		"refreshExpireInHours": 0,
		"grantTypes":           []string{"authorization_code", "implicit", "password", "client_credentials", "refresh_token"},
		"tokenFormat":          "JWT",
		"redirectUris":         []string{"http://localhost:8080/callback"},
		"signinMethods":        []map[string]any{{"name": "Password", "displayName": "Password", "rule": "None"}},
		"signupItems": []map[string]any{
			{"name": "ID", "visible": false, "required": true, "prompted": false, "rule": "Random"},
			{"name": "Username", "visible": true, "required": true, "prompted": false, "rule": "None"},
			{"name": "Display name", "visible": true, "required": true, "prompted": false, "rule": "None"},
			{"name": "Password", "visible": true, "required": true, "prompted": false, "rule": "None"},
			{"name": "Confirm password", "visible": true, "required": true, "prompted": false, "rule": "None"},
		},
	}
}

// ensureApplication creates the application when missing and repairs any
// drift in the fields the journey depends on (client id, cert, grants).
func (j journey) ensureApplication(ctx context.Context, client *http.Client) error {
	id := url.PathEscape("admin/" + j.appName)
	envelope, err := callCasdoor(ctx, client, http.MethodGet, j.apiBase+"/api/get-application?id="+id, nil)
	if err != nil {
		return fmt.Errorf("get application: %w", err)
	}
	exists := envelope.Status == "ok" && len(envelope.Data) > 0 && string(envelope.Data) != "null"

	method, target := http.MethodPost, j.apiBase+"/api/add-application"
	if exists {
		method, target = http.MethodPost, j.apiBase+"/api/update-application?id="+id
	}
	envelope, err = callCasdoor(ctx, client, method, target, j.applicationBody())
	if err != nil {
		return fmt.Errorf("write application: %w", err)
	}
	if envelope.Status != "ok" {
		return fmt.Errorf("write application rejected: %s", envelope.Msg)
	}

	// Read back: the client id is the token audience wired into
	// platform-api, so it must be exactly the fixed value.
	envelope, err = callCasdoor(ctx, client, http.MethodGet, j.apiBase+"/api/get-application?id="+id, nil)
	if err != nil {
		return fmt.Errorf("read back application: %w", err)
	}
	var stored struct {
		ClientID string `json:"clientId"`
		Cert     string `json:"cert"`
	}
	if err := json.Unmarshal(envelope.Data, &stored); err != nil {
		return fmt.Errorf("decode application read back: %w", err)
	}
	if stored.ClientID != j.clientID {
		return fmt.Errorf("application client id drift (expected the fixed journey id)")
	}
	if stored.Cert != casdoorSigningCert {
		return fmt.Errorf("application cert drift (expected the global signing cert)")
	}
	return nil
}

// ensureUser creates the journey user inside the organization.
func (j journey) ensureUser(ctx context.Context, client *http.Client) error {
	id := url.PathEscape(j.org + "/" + j.username)
	envelope, err := callCasdoor(ctx, client, http.MethodGet, j.apiBase+"/api/get-user?id="+id, nil)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if envelope.Status == "ok" && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		return nil
	}
	body := map[string]any{
		"owner":       j.org,
		"name":        j.username,
		"displayName": "T01 Acceptance",
		"type":        "normal-user",
		"password":    fixtureUserPassword,
		"email":       "t01-accept@example.com",
		"phone":       "",
		"isAdmin":     false,
		"isForbidden": false,
		"isDeleted":   false,
	}
	envelope, err = callCasdoor(ctx, client, http.MethodPost, j.apiBase+"/api/add-user", body)
	if err != nil {
		return fmt.Errorf("add user: %w", err)
	}
	if envelope.Status != "ok" {
		return fmt.Errorf("add user rejected: %s", envelope.Msg)
	}
	return nil
}

// tokenClaims is the subset of the minted token the journey inspects.
type tokenClaims struct {
	Issuer   string   `json:"iss"`
	Subject  string   `json:"sub"`
	Audience []string `json:"aud"`
}

// mintToken obtains a real RS256 access token through the password grant
// (spike finding: the plain username form, client credentials in the form
// body) and decodes it. The token value never leaves this function's
// caller boundary into reports.
func (j journey) mintToken(ctx context.Context, client *http.Client) (string, tokenClaims, error) {
	form := url.Values{
		"grant_type":    {"password"},
		"client_id":     {j.clientID},
		"client_secret": {fixtureClientSecret},
		"username":      {j.username},
		"password":      {fixtureUserPassword},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, j.apiBase+"/api/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", tokenClaims{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return "", tokenClaims{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", tokenClaims{}, err
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", tokenClaims{}, fmt.Errorf("token endpoint returned non-json (http %d)", response.StatusCode)
	}
	if payload.AccessToken == "" {
		return "", tokenClaims{}, fmt.Errorf("token endpoint denied the grant: %s", payload.Description)
	}

	parts := strings.Split(payload.AccessToken, ".")
	if len(parts) != 3 {
		return "", tokenClaims{}, errors.New("token is not a compact JWT")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	headerJSON, err := decodeSegment(parts[0])
	if err != nil {
		return "", tokenClaims{}, fmt.Errorf("decode token header: %w", err)
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return "", tokenClaims{}, fmt.Errorf("parse token header: %w", err)
	}
	if header.Alg != "RS256" {
		return "", tokenClaims{}, fmt.Errorf("token alg %q is not RS256", header.Alg)
	}
	if header.Kid == "" {
		return "", tokenClaims{}, errors.New("token header carries no kid")
	}
	claimsJSON, err := decodeSegment(parts[1])
	if err != nil {
		return "", tokenClaims{}, fmt.Errorf("decode token claims: %w", err)
	}
	var claims tokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return "", tokenClaims{}, fmt.Errorf("parse token claims: %w", err)
	}
	return payload.AccessToken, claims, nil
}

// callbackResponse is one observed callback exchange.
type callbackResponse struct {
	Status int
	Body   string
}

// postCallback delivers the identity callback with the given
// Authorization header value (empty sends none).
func (j journey) postCallback(ctx context.Context, client *http.Client, authorization string) callbackResponse {
	target := strings.TrimSuffix(j.callbackBase, "/") + j.callbackPath
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
	if err != nil {
		return callbackResponse{Status: -1, Body: fmt.Sprintf("build request: %v", err)}
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := client.Do(request)
	if err != nil {
		return callbackResponse{Status: -1, Body: fmt.Sprintf("post callback: %v", err)}
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return callbackResponse{Status: response.StatusCode, Body: string(raw)}
}

// openPlatformDB connects to the platform postgres through the existing
// pgx driver (registered by the postgres adapter package).
func (j journey) openPlatformDB(ctx context.Context) (*sql.DB, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		url.PathEscape(j.pgUser), url.PathEscape(fixturePostgresPassword), j.pgHost, j.pgPort, j.pgDatabase)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// countBindings returns the binding rows for the token subject.
func (j journey) countBindings(ctx context.Context, subject string) (int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var rows int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM identity_bindings WHERE identity_provider = $1 AND subject = $2`,
		j.issuer, subject).Scan(&rows); err != nil {
		return 0, err
	}
	return rows, nil
}

// bindingState returns the binding rows, the bound user id, and the
// distinct bound user count for the token subject.
func (j journey) bindingState(ctx context.Context, subject string) (int, string, int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, "", 0, err
	}
	defer db.Close()
	var rows, distinct int
	var boundUserID string
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), count(DISTINCT platform_user_id), COALESCE(min(platform_user_id::text), '')
		FROM identity_bindings
		WHERE identity_provider = $1 AND subject = $2`,
		j.issuer, subject).Scan(&rows, &distinct, &boundUserID); err != nil {
		return 0, "", 0, err
	}
	return rows, boundUserID, distinct, nil
}

// tamperSignature flips one deterministic byte of the token's signature
// segment, keeping the JWS shape intact so the failure is a signature
// verification failure, not a parse failure. A malformed signature
// segment is a construction bug, never a reason to send the valid token:
// that would make the denial assertion observe the server accepting what
// was meant to be tampered credentials.
func tamperSignature(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("token has %d dot-separated segments, want 3", len(parts))
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode signature segment: %w", err)
	}
	if len(signature) == 0 {
		return "", errors.New("signature segment is empty")
	}
	signature[0] ^= 0xff
	parts[2] = base64.RawURLEncoding.EncodeToString(signature)
	return strings.Join(parts, "."), nil
}

// failedReport builds a failing report whose assertion set matches the
// declared names (all failed), keeping the harness reconciliation valid.
func failedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(assertionNames))
	for _, name := range assertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}

// audienceContains reports whether the aud claim admits the client id.
func audienceContains(audience []string, clientID string) bool {
	for _, item := range audience {
		if item == clientID {
			return true
		}
	}
	return false
}

// decodeSegment decodes an unpadded base64url JWT segment.
func decodeSegment(seg string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(seg)
}

// inputString reads a string scenario input with a fallback.
func inputString(inputs map[string]any, key, fallback string) string {
	if inputs == nil {
		return fallback
	}
	if value, ok := inputs[key].(string); ok && value != "" {
		return value
	}
	return fallback
}

// inputInt reads an integer scenario input with a fallback.
func inputInt(inputs map[string]any, key string, fallback int) int {
	if inputs == nil {
		return fallback
	}
	switch value := inputs[key].(type) {
	case int:
		if value > 0 {
			return value
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return fallback
}

// envOrDefault reads an environment variable with a fallback.
func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
