package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// seamTenantContext names the T03 seam: the same black-box lighthouse
// position as T01/T02, watching the user select, switch, and lose a
// tenant context through the public contract endpoints.
const seamTenantContext = "lighthouse-tenant-context"

// t03StackStartupCommand is the precise recipe that brings the
// acceptance stack up for the T03 journey. It carries the same six
// required compose variables as the T01 recipe plus the audience
// override: the T03 application's fixed client id — and therefore the
// aud claim of every token it mints — is t03-acceptance, while the
// compose default audience is T01's t01-acceptance.
const t03StackStartupCommand = "cd deploy/compose && PLATFORM_IDENTITY_OIDC_AUDIENCE=t03-acceptance PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose up -d --wait"

func init() {
	platformtest.Register(seamTenantContext, tenantContextDriver{})
}

// t03AssertionNames is the exact set declared by
// scenarios/t03-tenant-context.yaml, in declaration order. The harness
// reconciles by name and rejects any drift, so this list and the YAML
// must move together.
var t03AssertionNames = []string{
	"stale_state_zero_users",
	"bind_user_a_status",
	"bind_user_b_status",
	"initial_list_count",
	"select_own_tenant_status",
	"select_replay_single_row",
	"second_membership_list_count",
	"switch_to_b_status",
	"current_reflects_switch",
	"switch_back_status",
	"revoked_current_status",
	"revoked_list_count",
	"reselect_own_status",
	"cross_tenant_denied_status",
	"denial_row_delta",
	"denial_missing_authorization_status",
	"denial_tampered_signature_status",
}

// tenantContextDriver executes the T03 journey against the real compose
// stack: two Casdoor users bind through the identity callback (each
// gaining its personal tenant), then the journey drives the tenant
// context endpoints over the wire — list, select, replay, switch, revoke
// (a driver-side database fixture), reselect, cross-tenant denials, and
// credential denials. Nothing links into the platform internals.
type tenantContextDriver struct{}

// t03Journey embeds the shared journey helpers and adds the second
// Casdoor username the tenant-context journey needs (the shared struct
// was designed around a single user; the variants below keep T01/T02
// semantics untouched).
type t03Journey struct {
	journey
	usernameB string
}

// Execute runs the whole journey and returns one assertion result per
// declared assertion name. Details carry facts only (status codes, row
// counts, match booleans) — never tokens, subjects, or credentials.
func (tenantContextDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	j := t03JourneyFromInputs(scenario.Inputs)

	if err := j.t03PrecheckStack(ctx); err != nil {
		return t03FailedReport(err.Error()), nil
	}

	client := newJourneyClient()

	if err := j.provisionCasdoor(ctx, client); err != nil {
		return t03FailedReport(fmt.Sprintf("casdoor provisioning failed: %v", err)), nil
	}
	if err := j.ensureNamedUser(ctx, client, j.usernameB); err != nil {
		return t03FailedReport(fmt.Sprintf("casdoor provisioning of the second user failed: %v", err)), nil
	}

	tokenA, claimsA, err := j.mintToken(ctx, client)
	if err != nil {
		return t03FailedReport(fmt.Sprintf("token mint for user A failed: %v", err)), nil
	}
	tokenB, claimsB, err := j.mintTokenFor(ctx, client, j.usernameB)
	if err != nil {
		return t03FailedReport(fmt.Sprintf("token mint for user B failed: %v", err)), nil
	}
	for _, claims := range []tokenClaims{claimsA, claimsB} {
		if claims.Issuer != j.issuer {
			return t03FailedReport("token iss does not match the configured issuer (value omitted)"), nil
		}
		if !audienceContains(claims.Audience, j.clientID) {
			return t03FailedReport("token aud does not include the provisioned client id"), nil
		}
		if claims.Subject == "" {
			return t03FailedReport("token sub is empty"), nil
		}
	}

	// Stale-state precheck (design ruling 8): the journey proves fresh
	// first binds with fresh tenant provisioning, so the platform
	// database must hold zero users before anything is delivered. A warm
	// stack whose named postgres volume survived a previous run already
	// carries users and would answer the idempotent 200 — fail closed
	// here instead of reporting confusing assertion failures. The #100
	// harness redacts every driver summary and assertion detail, so the
	// skip message in the test file and its reset command are the only
	// visible guidance.
	users, err := j.countPlatformUsers(ctx)
	if err != nil {
		return platformtest.Report{}, fmt.Errorf("stale-state precheck failed (platform database unreachable): %w", err)
	}
	if users > 0 {
		return platformtest.Report{}, fmt.Errorf("stale platform state: %d platform_users row(s) already exist; the journey proves fresh first binds and requires a clean platform database — reset the stack and rerun: %s", users, stackResetCommand)
	}

	results := map[string]platformtest.AssertionResult{}
	record := func(name string, passed bool, details string) {
		results[name] = platformtest.AssertionResult{Name: name, Passed: passed, Details: details}
	}
	record("stale_state_zero_users", users == 0, fmt.Sprintf("rows=%d", users))

	// Double first bind (design ruling 8): each user delivers its
	// callback once and must be created with its personal tenant bundle.
	bindA := j.postCallback(ctx, client, "Bearer "+tokenA)
	record("bind_user_a_status", bindA.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindA.Status))
	bindB := j.postCallback(ctx, client, "Bearer "+tokenB)
	record("bind_user_b_status", bindB.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindB.Status))

	userA := t03UserID(bindA.Body)
	userB := t03UserID(bindB.Body)
	if userA == "" || userB == "" || userA == userB {
		return t03FailedReport("first binds did not return two distinct canonical user ids"), nil
	}

	tenantsPath := strings.TrimSuffix(j.callbackBase, "/") + "/api/v1/tenants"
	contextPath := strings.TrimSuffix(j.callbackBase, "/") + "/api/v1/tenant-context"

	// Initial list: user A sees exactly its own personal tenant with the
	// owner role; user B's list supplies B's tenant id.
	listA1 := t03Request(ctx, client, http.MethodGet, tenantsPath, "", "Bearer "+tokenA)
	initialList := t03ParseTenantList(listA1.Body)
	record("initial_list_count",
		listA1.Status == http.StatusOK && len(initialList) == 1 &&
			initialList[0].Kind == "personal" && initialList[0].Role == "owner",
		fmt.Sprintf("status=%d tenants=%d", listA1.Status, len(initialList)))
	if len(initialList) != 1 {
		return t03FailedReport("initial tenant list does not carry exactly one personal/owner tenant"), nil
	}
	tenantA := initialList[0].TenantID

	listB := t03Request(ctx, client, http.MethodGet, tenantsPath, "", "Bearer "+tokenB)
	initialListB := t03ParseTenantList(listB.Body)
	if len(initialListB) != 1 {
		return t03FailedReport("user B's tenant list does not carry exactly one tenant"), nil
	}
	tenantB := initialListB[0].TenantID

	// First selection: user A selects its own tenant; the response echoes
	// the tenant id with an RFC3339 selected_at.
	selectOwn := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantA+`"}`, "Bearer "+tokenA)
	record("select_own_tenant_status",
		selectOwn.Status == http.StatusOK && t03SelectedTenant(selectOwn.Body) == tenantA && t03SelectedAtParses(selectOwn.Body),
		fmt.Sprintf("status=%d tenant_id_echo=%t selected_at_rfc3339=%t",
			selectOwn.Status, t03SelectedTenant(selectOwn.Body) == tenantA, t03SelectedAtParses(selectOwn.Body)))

	// Idempotent replay: the same PUT answers 200 with the same tenant
	// echoed and the database holds exactly one selection row for the
	// user. selected_at is refreshed by design (the upsert is
	// last-write-wins), so the replay is not byte-identical — the T01
	// callback's identical-body semantics do not carry over.
	replay := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantA+`"}`, "Bearer "+tokenA)
	selectionRows, err := j.t03CountSelections(ctx, userA)
	if err != nil {
		return t03FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("select_replay_single_row",
		replay.Status == http.StatusOK && t03SelectedTenant(replay.Body) == tenantA && selectionRows == 1,
		fmt.Sprintf("replay_status=%d tenant_id_echo=%t rows=%d",
			replay.Status, t03SelectedTenant(replay.Body) == tenantA, selectionRows))

	// Fixture (design ruling 8): user A gains a second, member/active
	// membership in user B's tenant — injected straight into the
	// database because multi-member tenants are T04/T05 territory with
	// no Stage-1 API. This is a precondition write, never an assertion
	// read.
	if err := j.t03InsertActiveMembership(ctx, tenantB, userA, "member"); err != nil {
		return t03FailedReport(fmt.Sprintf("membership fixture failed: %v", err)), nil
	}
	listA2 := t03Request(ctx, client, http.MethodGet, tenantsPath, "", "Bearer "+tokenA)
	switchable := t03ParseTenantList(listA2.Body)
	record("second_membership_list_count",
		listA2.Status == http.StatusOK && len(switchable) == 2,
		fmt.Sprintf("status=%d tenants=%d", listA2.Status, len(switchable)))

	// Switch to B and back: two real switches, each 200, and the read
	// reflects the switched tenant in between.
	switchToB := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantB+`"}`, "Bearer "+tokenA)
	record("switch_to_b_status",
		switchToB.Status == http.StatusOK && t03SelectedTenant(switchToB.Body) == tenantB,
		fmt.Sprintf("status=%d tenant_id_echo=%t", switchToB.Status, t03SelectedTenant(switchToB.Body) == tenantB))

	currentDuringSwitch := t03Request(ctx, client, http.MethodGet, contextPath, "", "Bearer "+tokenA)
	record("current_reflects_switch",
		currentDuringSwitch.Status == http.StatusOK && t03SelectedTenant(currentDuringSwitch.Body) == tenantB,
		fmt.Sprintf("status=%d reflects_b=%t", currentDuringSwitch.Status, t03SelectedTenant(currentDuringSwitch.Body) == tenantB))

	switchBack := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantA+`"}`, "Bearer "+tokenA)
	record("switch_back_status",
		switchBack.Status == http.StatusOK && t03SelectedTenant(switchBack.Body) == tenantA,
		fmt.Sprintf("status=%d tenant_id_echo=%t", switchBack.Status, t03SelectedTenant(switchBack.Body) == tenantA))

	// Revocation (design ruling 8): the driver first re-points the
	// selection at B, then revokes A's membership in B. The server-side
	// re-validation must veto the persisted selection: the read answers
	// 404 while the row itself survives.
	repoint := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantB+`"}`, "Bearer "+tokenA)
	if repoint.Status != http.StatusOK {
		return t03FailedReport(fmt.Sprintf("pre-revocation switch status=%d, want 200", repoint.Status)), nil
	}
	if err := j.t03RevokeMembership(ctx, tenantB, userA); err != nil {
		return t03FailedReport(fmt.Sprintf("revocation fixture failed: %v", err)), nil
	}
	revokedCurrent := t03Request(ctx, client, http.MethodGet, contextPath, "", "Bearer "+tokenA)
	record("revoked_current_status", revokedCurrent.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", revokedCurrent.Status))

	listA3 := t03Request(ctx, client, http.MethodGet, tenantsPath, "", "Bearer "+tokenA)
	afterRevocation := t03ParseTenantList(listA3.Body)
	record("revoked_list_count",
		listA3.Status == http.StatusOK && len(afterRevocation) == 1 && afterRevocation[0].TenantID == tenantA,
		fmt.Sprintf("status=%d tenants=%d", listA3.Status, len(afterRevocation)))

	reselect := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantA+`"}`, "Bearer "+tokenA)
	record("reselect_own_status",
		reselect.Status == http.StatusOK && t03SelectedTenant(reselect.Body) == tenantA,
		fmt.Sprintf("status=%d tenant_id_echo=%t", reselect.Status, t03SelectedTenant(reselect.Body) == tenantA))

	// Cross-tenant attack (design ruling 8): user A may not select the
	// revoked tenant B and user B may not select A's tenant — both are
	// indistinguishable 404s and neither leaves a row behind.
	rowsBefore, err := j.t03CountSelections(ctx, "")
	if err != nil {
		return t03FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	deniedA := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantB+`"}`, "Bearer "+tokenA)
	deniedB := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantA+`"}`, "Bearer "+tokenB)
	rowsAfter, err := j.t03CountSelections(ctx, "")
	if err != nil {
		return t03FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("cross_tenant_denied_status",
		deniedA.Status == http.StatusNotFound && deniedB.Status == http.StatusNotFound,
		fmt.Sprintf("denied_a=%d denied_b=%d", deniedA.Status, deniedB.Status))
	record("denial_row_delta", rowsAfter-rowsBefore == 0,
		fmt.Sprintf("delta=%d", rowsAfter-rowsBefore))

	// Credential denials: each endpoint answers 401 for missing or
	// tampered credentials (design ruling 8).
	missingPut := t03Request(ctx, client, http.MethodPut, contextPath, `{"tenant_id":"`+tenantA+`"}`, "")
	missingGet := t03Request(ctx, client, http.MethodGet, contextPath, "", "")
	tamperedToken, err := tamperSignature(tokenA)
	if err != nil {
		return t03FailedReport(fmt.Sprintf("tampered token construction failed: %v", err)), nil
	}
	tamperedList := t03Request(ctx, client, http.MethodGet, tenantsPath, "", "Bearer "+tamperedToken)
	record("denial_missing_authorization_status",
		missingPut.Status == http.StatusUnauthorized && missingGet.Status == http.StatusUnauthorized,
		fmt.Sprintf("put=%d get=%d", missingPut.Status, missingGet.Status))
	record("denial_tampered_signature_status", tamperedList.Status == http.StatusUnauthorized,
		fmt.Sprintf("status=%d", tamperedList.Status))

	ordered := make([]platformtest.AssertionResult, 0, len(t03AssertionNames))
	passed := true
	for _, name := range t03AssertionNames {
		result, ok := results[name]
		if !ok {
			return t03FailedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}

	summary := fmt.Sprintf("bind_a=%d bind_b=%d initial=%d select=%d replay=%d rows=%d after_fixture=%d switch_b=%d current_b=%t switch_a=%d revoked_get=%d revoked_list=%d reselect=%d denied_a=%d denied_b=%d delta=%d missing_put=%d missing_get=%d tampered=%d",
		bindA.Status, bindB.Status, len(initialList), selectOwn.Status, replay.Status, selectionRows,
		len(switchable), switchToB.Status, t03SelectedTenant(currentDuringSwitch.Body) == tenantB, switchBack.Status,
		revokedCurrent.Status, len(afterRevocation), reselect.Status, deniedA.Status, deniedB.Status,
		rowsAfter-rowsBefore, missingPut.Status, missingGet.Status, tamperedList.Status)
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// t03JourneyFromInputs reads the scenario inputs with t03-prefixed
// defaults so the T03 identifiers never collide with the T01/T02 journey
// fixtures.
func t03JourneyFromInputs(inputs map[string]any) t03Journey {
	return t03Journey{
		journey: journey{
			issuer:       inputString(inputs, "issuer", "http://casdoor:8000"),
			apiBase:      inputString(inputs, "casdoor_api_base", "http://localhost:8000"),
			callbackBase: inputString(inputs, "callback_base", "http://localhost:8080"),
			callbackPath: inputString(inputs, "callback_path", "/internal/v1/identity/callback"),
			org:          inputString(inputs, "casdoor_organization", "t03-accept-org"),
			appName:      inputString(inputs, "casdoor_application_name", "t03-acceptance-app"),
			clientID:     inputString(inputs, "casdoor_client_id", "t03-acceptance"),
			username:     inputString(inputs, "casdoor_username_a", "t03-select-a"),
			replays:      inputInt(inputs, "replay_deliveries", 1),
			pgHost:       inputString(inputs, "postgres_host", "localhost"),
			pgPort:       inputString(inputs, "postgres_port", "5432"),
			pgDatabase:   inputString(inputs, "postgres_database", "platform"),
			pgUser:       inputString(inputs, "postgres_user", "platform"),
		},
		usernameB: inputString(inputs, "casdoor_username_b", "t03-select-b"),
	}
}

// t03PrecheckStack verifies both stack entry points answer before
// anything else runs, failing with the T03 startup command (which
// carries the audience override) in the message.
func (j t03Journey) t03PrecheckStack(ctx context.Context) error {
	casdoorURL := strings.TrimSuffix(j.apiBase, "/") + "/api/health"
	if err := probe(ctx, casdoorURL); err != nil {
		return fmt.Errorf("casdoor unreachable at %s (%v); start the stack first: %s", casdoorURL, err, t03StackStartupCommand)
	}
	livezURL := strings.TrimSuffix(j.callbackBase, "/") + "/livez"
	if err := probe(ctx, livezURL); err != nil {
		return fmt.Errorf("platform-api unreachable at %s (%v); start the stack first: %s", livezURL, err, t03StackStartupCommand)
	}
	return nil
}

// ensureNamedUser creates (idempotently) a user with an explicit
// username inside the journey organization — the variant the
// single-user provisionCasdoor flow needs for the second journey user
// (the T01/T02 helper and its semantics stay untouched).
func (j t03Journey) ensureNamedUser(ctx context.Context, client *http.Client, username string) error {
	id := url.PathEscape(j.org + "/" + username)
	envelope, err := callCasdoor(ctx, client, http.MethodGet, j.apiBase+"/api/get-user?id="+id, nil)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if envelope.Status == "ok" && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		return nil
	}
	body := map[string]any{
		"owner":       j.org,
		"name":        username,
		"displayName": "T03 Tenant Context",
		"type":        "normal-user",
		"password":    fixtureUserPassword,
		"email":       username + "@example.com",
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

// mintTokenFor obtains a real RS256 access token through the password
// grant for an explicit username — the variant of mintToken the second
// journey user needs (mintToken and its T01/T02 semantics stay
// untouched). The token value never leaves this function's caller
// boundary into reports.
func (j t03Journey) mintTokenFor(ctx context.Context, client *http.Client, username string) (string, tokenClaims, error) {
	form := url.Values{
		"grant_type":    {"password"},
		"client_id":     {j.clientID},
		"client_secret": {fixtureClientSecret},
		"username":      {username},
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

// t03APIResponse is one observed tenant-context HTTP exchange.
type t03APIResponse struct {
	Status int
	Body   string
}

// t03Request issues one tenant-context endpoint request with an optional
// JSON body and an optional Authorization header value.
func t03Request(ctx context.Context, client *http.Client, method, path, body, authorization string) t03APIResponse {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, path, reader)
	if err != nil {
		return t03APIResponse{Status: -1, Body: fmt.Sprintf("build request: %v", err)}
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := client.Do(request)
	if err != nil {
		return t03APIResponse{Status: -1, Body: fmt.Sprintf("do request: %v", err)}
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return t03APIResponse{Status: response.StatusCode, Body: string(raw)}
}

// t03TenantEntry is one tenant of the list response body.
type t03TenantEntry struct {
	TenantID string `json:"tenant_id"`
	Kind     string `json:"kind"`
	Role     string `json:"role"`
}

// t03ParseTenantList decodes a GET /api/v1/tenants body (nil on any
// decode failure).
func t03ParseTenantList(body string) []t03TenantEntry {
	var payload struct {
		Tenants []t03TenantEntry `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil
	}
	return payload.Tenants
}

// t03SelectedTenant decodes the tenant_id of a selection body (empty on
// any decode failure).
func t03SelectedTenant(body string) string {
	var payload struct {
		TenantID string `json:"tenant_id"`
	}
	_ = json.Unmarshal([]byte(body), &payload)
	return payload.TenantID
}

// t03SelectedAtParses reports whether the selected_at field of a
// selection body parses as RFC3339.
func t03SelectedAtParses(body string) bool {
	var payload struct {
		SelectedAt string `json:"selected_at"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return false
	}
	_, err := time.Parse(time.RFC3339, payload.SelectedAt)
	return err == nil
}

// t03UserID decodes the user_id of a callback body, accepting only the
// canonical UUID shape (empty otherwise).
func t03UserID(body string) string {
	if !userIDPattern.MatchString(body) {
		return ""
	}
	var payload struct {
		UserID string `json:"user_id"`
	}
	_ = json.Unmarshal([]byte(body), &payload)
	return payload.UserID
}

// t03CountSelections returns the tenant_context_selections row count for
// one platform user (empty userID counts every row).
func (j journey) t03CountSelections(ctx context.Context, userID string) (int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	query := `SELECT count(*) FROM tenant_context_selections`
	var args []any
	if userID != "" {
		query += ` WHERE platform_user_id = $1::uuid`
		args = append(args, userID)
	}
	var rows int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&rows); err != nil {
		return 0, err
	}
	return rows, nil
}

// t03InsertActiveMembership is the journey's fixture write: an active
// membership no Stage-1 API can produce (design ruling 8).
func (j journey) t03InsertActiveMembership(ctx context.Context, tenantID, userID, role string) error {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3)`,
		tenantID, userID, role)
	return err
}

// t03RevokeMembership is the journey's revocation fixture: it flips the
// membership to revoked and proves exactly one row changed.
func (j journey) t03RevokeMembership(ctx context.Context, tenantID, userID string) error {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `
		UPDATE memberships SET status = 'revoked'
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		tenantID, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("revocation affected %d membership rows, want 1", rows)
	}
	return nil
}

// t03FailedReport builds a failing report whose assertion set matches
// the declared T03 names (all failed), keeping the harness
// reconciliation valid.
func t03FailedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(t03AssertionNames))
	for _, name := range t03AssertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}
