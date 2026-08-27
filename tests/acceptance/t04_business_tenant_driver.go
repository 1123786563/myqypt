package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// seamBusinessTenant names the T04 seam: the same black-box lighthouse
// position as T01/T02/T03, watching the user create business tenants
// through the public contract endpoints.
const seamBusinessTenant = "lighthouse-business-tenant"

// t04StackStartupCommand is the precise recipe that brings the acceptance
// stack up for the T04 journey. It carries the same six required compose
// variables as the T01 recipe plus the audience override: the T04
// application's fixed client id — and therefore the aud claim of every
// token it mints — is t04-acceptance, while the compose default audience
// is T01's t01-acceptance.
const t04StackStartupCommand = "cd deploy/compose && PLATFORM_IDENTITY_OIDC_AUDIENCE=t04-acceptance PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose up -d --wait"

func init() {
	platformtest.Register(seamBusinessTenant, businessTenantDriver{})
}

// t04AssertionNames is the exact set declared by
// scenarios/t04-business-tenant.yaml, in declaration order. The harness
// reconciles by name and rejects any drift, so this list and the YAML
// must move together.
var t04AssertionNames = []string{
	"stale_state_zero_users",
	"bind_user_a_status",
	"create_tenant_status",
	"creation_bundle_rows",
	"replay_same_tenant_status",
	"replay_user_tenants_unchanged",
	"second_key_new_tenant_status",
	"list_tenants_count",
	"select_business_context_status",
	"denial_missing_idempotency_key_status",
	"denial_empty_display_name_status",
	"denial_missing_authorization_status",
	"denial_tampered_signature_status",
	"unbound_creator_not_found_status",
	"unbound_zero_creations",
}

// businessTenantDriver executes the T04 journey against the real compose
// stack: one Casdoor user binds through the identity callback (gaining
// its personal tenant), then creates business tenants over the public
// contract — first delivery, same-key replay, different-key second
// entity — with the T03 list and select endpoints proving the new tenants
// are immediately usable, and the rejection paths (missing key, empty
// name, missing/tampered credentials, never-bound creator) proving every
// refusal happens without an oracle or a stray write. Nothing links into
// the platform internals.
type businessTenantDriver struct{}

// t04Journey embeds the T03 journey (which itself embeds the shared
// journey helpers, including the second-user provisioning and token
// minting variants T04 reuses) and adds the creation fixtures.
type t04Journey struct {
	t03Journey
	displayName  string
	idempotencyA string
	idempotencyB string
}

// Execute runs the whole journey and returns one assertion result per
// declared assertion name. Details carry facts only (status codes, row
// counts, match booleans) — never tokens, subjects, or credentials.
func (businessTenantDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	j := t04JourneyFromInputs(scenario.Inputs)

	if err := j.t04PrecheckStack(ctx); err != nil {
		return t04FailedReport(err.Error()), nil
	}

	client := newJourneyClient()

	if err := j.provisionCasdoor(ctx, client); err != nil {
		return t04FailedReport(fmt.Sprintf("casdoor provisioning failed: %v", err)), nil
	}
	if err := j.ensureNamedUser(ctx, client, j.usernameB); err != nil {
		return t04FailedReport(fmt.Sprintf("casdoor provisioning of the second user failed: %v", err)), nil
	}

	tokenA, claimsA, err := j.mintToken(ctx, client)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("token mint for user A failed: %v", err)), nil
	}
	tokenB, claimsB, err := j.mintTokenFor(ctx, client, j.usernameB)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("token mint for user B failed: %v", err)), nil
	}
	for _, claims := range []tokenClaims{claimsA, claimsB} {
		if claims.Issuer != j.issuer {
			return t04FailedReport("token iss does not match the configured issuer (value omitted)"), nil
		}
		if !audienceContains(claims.Audience, j.clientID) {
			return t04FailedReport("token aud does not include the provisioned client id"), nil
		}
		if claims.Subject == "" {
			return t04FailedReport("token sub is empty"), nil
		}
	}

	// Stale-state precheck: the journey proves a fresh first bind and a
	// fresh first creation, so the platform database must hold zero
	// users before anything is delivered (see the T03 precedent for why
	// this fails closed).
	users, err := j.countPlatformUsers(ctx)
	if err != nil {
		return platformtest.Report{}, fmt.Errorf("stale-state precheck failed (platform database unreachable): %w", err)
	}
	if users > 0 {
		return platformtest.Report{}, fmt.Errorf("stale platform state: %d platform_users row(s) already exist; the journey requires a clean platform database — reset the stack and rerun: %s", users, stackResetCommand)
	}

	results := map[string]platformtest.AssertionResult{}
	record := func(name string, passed bool, details string) {
		results[name] = platformtest.AssertionResult{Name: name, Passed: passed, Details: details}
	}
	record("stale_state_zero_users", users == 0, fmt.Sprintf("rows=%d", users))

	// First bind: user A's callback creates the user with its personal
	// tenant bundle.
	bindA := j.postCallback(ctx, client, "Bearer "+tokenA)
	record("bind_user_a_status", bindA.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindA.Status))
	userA := t03UserID(bindA.Body)
	if userA == "" {
		return t04FailedReport("first bind did not return a canonical user id"), nil
	}

	tenantsPath := strings.TrimSuffix(j.callbackBase, "/") + "/api/v1/tenants"
	contextPath := strings.TrimSuffix(j.callbackBase, "/") + "/api/v1/tenant-context"
	createBody := `{"display_name":"` + j.displayName + `"}`

	// First creation: 201 with the creation facts echoed verbatim.
	first := t04Create(ctx, client, tenantsPath, createBody, j.idempotencyA, "Bearer "+tokenA)
	firstTenant := t04ParseCreated(first.Body)
	shapeOK := firstTenant.TenantID != "" &&
		firstTenant.Kind == "business" &&
		firstTenant.Role == "owner" &&
		firstTenant.DisplayName == j.displayName &&
		t04CreatedAtParses(first.Body)
	record("create_tenant_status",
		first.Status == http.StatusCreated && shapeOK,
		fmt.Sprintf("status=%d kind_business=%t role_owner=%t display_name_echo=%t created_at_rfc3339=%t",
			first.Status, firstTenant.Kind == "business", firstTenant.Role == "owner",
			firstTenant.DisplayName == j.displayName, t04CreatedAtParses(first.Body)))
	if first.Status != http.StatusCreated || firstTenant.TenantID == "" {
		return t04FailedReport("first business tenant creation did not answer 201 with a tenant id"), nil
	}

	// Database truth (design ruling 7): the tenant row is business with
	// the matching display name, the billing customer is exactly 1:1,
	// the single active owner membership is in place, and the creation
	// mapping holds exactly one row for the key.
	businessKind, displayNameMatch, err := j.t04TenantKindAndName(ctx, firstTenant.TenantID, j.displayName)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	billingRows, err := j.t04CountRows(ctx, `SELECT count(*) FROM billing_customers WHERE tenant_id = $1::uuid`, firstTenant.TenantID)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	ownerRows, err := j.t04CountRows(ctx,
		`SELECT count(*) FROM memberships WHERE tenant_id = $1::uuid AND role = 'owner' AND status = 'active'`,
		firstTenant.TenantID)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	creationRows, creationTenantMatches, err := j.t04CreationMapping(ctx, userA, j.idempotencyA, firstTenant.TenantID)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("creation_bundle_rows",
		businessKind && displayNameMatch && billingRows == 1 && ownerRows == 1 && creationRows == 1 && creationTenantMatches,
		fmt.Sprintf("kind_business=%t display_name_match=%t billing_rows=%d owner_membership_rows=%d creation_rows=%d mapping_match=%t",
			businessKind, displayNameMatch, billingRows, ownerRows, creationRows, creationTenantMatches))

	// Same-key replay: 200 with the identical tenant id, and user A still
	// holds exactly two tenants (1 personal + 1 business).
	replay := t04Create(ctx, client, tenantsPath, createBody, j.idempotencyA, "Bearer "+tokenA)
	replayTenant := t04ParseCreated(replay.Body)
	record("replay_same_tenant_status",
		replay.Status == http.StatusOK && replayTenant.TenantID == firstTenant.TenantID,
		fmt.Sprintf("status=%d same_tenant=%t", replay.Status, replayTenant.TenantID == firstTenant.TenantID))

	userTenants, userBusiness, err := j.t04UserTenantCounts(ctx, userA)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("replay_user_tenants_unchanged",
		userTenants == 2 && userBusiness == 1,
		fmt.Sprintf("tenants=%d business_tenants=%d", userTenants, userBusiness))

	// Different-key second creation: 201 with a new tenant (the
	// multi-entity design).
	second := t04Create(ctx, client, tenantsPath, createBody, j.idempotencyB, "Bearer "+tokenA)
	secondTenant := t04ParseCreated(second.Body)
	record("second_key_new_tenant_status",
		second.Status == http.StatusCreated && secondTenant.TenantID != "" && secondTenant.TenantID != firstTenant.TenantID,
		fmt.Sprintf("status=%d distinct_tenant=%t", second.Status,
			secondTenant.TenantID != "" && secondTenant.TenantID != firstTenant.TenantID))

	// T03 integration: the list endpoint sees all three tenants (1
	// personal + 2 business, all owner) immediately.
	list := t03Request(ctx, client, http.MethodGet, tenantsPath, "", "Bearer "+tokenA)
	entries := t03ParseTenantList(list.Body)
	allOwner, kinds := true, map[string]int{}
	for _, entry := range entries {
		kinds[entry.Kind]++
		if entry.Role != "owner" {
			allOwner = false
		}
	}
	record("list_tenants_count",
		list.Status == http.StatusOK && len(entries) == 3 && kinds["personal"] == 1 && kinds["business"] == 2 && allOwner,
		fmt.Sprintf("status=%d tenants=%d personal=%d business=%d all_owner=%t",
			list.Status, len(entries), kinds["personal"], kinds["business"], allOwner))

	// T03 integration: the newly created business tenant is immediately
	// selectable as the tenant context.
	selectBusiness := t03Request(ctx, client, http.MethodPut, contextPath,
		`{"tenant_id":"`+firstTenant.TenantID+`"}`, "Bearer "+tokenA)
	record("select_business_context_status",
		selectBusiness.Status == http.StatusOK && t03SelectedTenant(selectBusiness.Body) == firstTenant.TenantID,
		fmt.Sprintf("status=%d tenant_id_echo=%t", selectBusiness.Status,
			t03SelectedTenant(selectBusiness.Body) == firstTenant.TenantID))

	// Rejection paths: missing Idempotency-Key header and empty
	// display_name are 400s; missing or tampered credentials are 401s.
	missingKey := t04Create(ctx, client, tenantsPath, createBody, "", "Bearer "+tokenA)
	record("denial_missing_idempotency_key_status", missingKey.Status == http.StatusBadRequest,
		fmt.Sprintf("status=%d", missingKey.Status))
	emptyName := t04Create(ctx, client, tenantsPath, `{"display_name":""}`, j.idempotencyA, "Bearer "+tokenA)
	record("denial_empty_display_name_status", emptyName.Status == http.StatusBadRequest,
		fmt.Sprintf("status=%d", emptyName.Status))
	missingAuth := t04Create(ctx, client, tenantsPath, createBody, j.idempotencyA, "")
	record("denial_missing_authorization_status", missingAuth.Status == http.StatusUnauthorized,
		fmt.Sprintf("status=%d", missingAuth.Status))
	tamperedToken, err := tamperSignature(tokenA)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("tampered token construction failed: %v", err)), nil
	}
	tampered := t04Create(ctx, client, tenantsPath, createBody, j.idempotencyA, "Bearer "+tamperedToken)
	record("denial_tampered_signature_status", tampered.Status == http.StatusUnauthorized,
		fmt.Sprintf("status=%d", tampered.Status))

	// Never-bound creator: user B holds a valid token but never bound, so
	// no platform user exists to own anything — an indistinguishable 404
	// with zero rows written.
	creationsBefore, err := j.t04CountRows(ctx, `SELECT count(*) FROM business_tenant_creations`)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	unbound := t04Create(ctx, client, tenantsPath, createBody, "t04-key-unbound", "Bearer "+tokenB)
	creationsAfter, err := j.t04CountRows(ctx, `SELECT count(*) FROM business_tenant_creations`)
	if err != nil {
		return t04FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("unbound_creator_not_found_status", unbound.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", unbound.Status))
	record("unbound_zero_creations", creationsAfter-creationsBefore == 0,
		fmt.Sprintf("delta=%d", creationsAfter-creationsBefore))

	ordered := make([]platformtest.AssertionResult, 0, len(t04AssertionNames))
	passed := true
	for _, name := range t04AssertionNames {
		result, ok := results[name]
		if !ok {
			return t04FailedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}

	summary := fmt.Sprintf("bind_a=%d create=%d bundle=%t replay=%d replay_same=%t user_tenants=%d business=%d second=%d distinct=%t list=%d select=%d missing_key=%d empty_name=%d missing_auth=%d tampered=%d unbound=%d unbound_delta=%d",
		bindA.Status, first.Status,
		businessKind && displayNameMatch && billingRows == 1 && ownerRows == 1 && creationRows == 1,
		replay.Status, replayTenant.TenantID == firstTenant.TenantID, userTenants, userBusiness,
		second.Status, secondTenant.TenantID != firstTenant.TenantID, len(entries), selectBusiness.Status,
		missingKey.Status, emptyName.Status, missingAuth.Status, tampered.Status, unbound.Status,
		creationsAfter-creationsBefore)
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// t04JourneyFromInputs reads the scenario inputs with t04-prefixed
// defaults so the T04 identifiers never collide with the T01/T02/T03
// journey fixtures.
func t04JourneyFromInputs(inputs map[string]any) t04Journey {
	return t04Journey{
		t03Journey: t03Journey{
			journey: journey{
				issuer:       inputString(inputs, "issuer", "http://casdoor:8000"),
				apiBase:      inputString(inputs, "casdoor_api_base", "http://localhost:8000"),
				callbackBase: inputString(inputs, "callback_base", "http://localhost:8080"),
				callbackPath: inputString(inputs, "callback_path", "/internal/v1/identity/callback"),
				org:          inputString(inputs, "casdoor_organization", "t04-accept-org"),
				appName:      inputString(inputs, "casdoor_application_name", "t04-acceptance-app"),
				clientID:     inputString(inputs, "casdoor_client_id", "t04-acceptance"),
				username:     inputString(inputs, "casdoor_username_a", "t04-create-a"),
				replays:      inputInt(inputs, "replay_deliveries", 1),
				pgHost:       inputString(inputs, "postgres_host", "localhost"),
				pgPort:       inputString(inputs, "postgres_port", "5432"),
				pgDatabase:   inputString(inputs, "postgres_database", "platform"),
				pgUser:       inputString(inputs, "postgres_user", "platform"),
			},
			usernameB: inputString(inputs, "casdoor_username_b", "t04-create-b"),
		},
		displayName:  inputString(inputs, "display_name", "t04-accept-team"),
		idempotencyA: inputString(inputs, "idempotency_key_a", "t04-key-a"),
		idempotencyB: inputString(inputs, "idempotency_key_b", "t04-key-b"),
	}
}

// t04PrecheckStack verifies both stack entry points answer before
// anything else runs, failing with the T04 startup command (which
// carries the audience override) in the message.
func (j t04Journey) t04PrecheckStack(ctx context.Context) error {
	casdoorURL := strings.TrimSuffix(j.apiBase, "/") + "/api/health"
	if err := probe(ctx, casdoorURL); err != nil {
		return fmt.Errorf("casdoor unreachable at %s (%v); start the stack first: %s", casdoorURL, err, t04StackStartupCommand)
	}
	livezURL := strings.TrimSuffix(j.callbackBase, "/") + "/livez"
	if err := probe(ctx, livezURL); err != nil {
		return fmt.Errorf("platform-api unreachable at %s (%v); start the stack first: %s", livezURL, err, t04StackStartupCommand)
	}
	return nil
}

// t04Create issues one POST /api/v1/tenants with a JSON body, an
// optional Idempotency-Key header (empty sends none), and an optional
// Authorization header.
func t04Create(ctx context.Context, client *http.Client, path, body, idempotencyKey, authorization string) t03APIResponse {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
	if err != nil {
		return t03APIResponse{Status: -1, Body: fmt.Sprintf("build request: %v", err)}
	}
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
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

// t04CreatedTenant decodes the creation response body.
type t04CreatedTenant struct {
	TenantID    string `json:"tenant_id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
	Role        string `json:"role"`
}

// t04ParseCreated decodes a POST /api/v1/tenants body (zero value on any
// decode failure).
func t04ParseCreated(body string) t04CreatedTenant {
	var payload t04CreatedTenant
	_ = json.Unmarshal([]byte(body), &payload)
	return payload
}

// t04CreatedAtParses reports whether the created_at field of a creation
// body parses as RFC3339.
func t04CreatedAtParses(body string) bool {
	payload := t04ParseCreated(body)
	if payload.CreatedAt == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339, payload.CreatedAt)
	return err == nil
}

// t04TenantKindAndName returns whether the tenants row of one tenant id
// is kind=business and its display name matches the expected value.
func (j t04Journey) t04TenantKindAndName(ctx context.Context, tenantID, expectedName string) (bool, bool, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return false, false, err
	}
	defer db.Close()
	var kind, displayName string
	if err := db.QueryRowContext(ctx,
		`SELECT kind, COALESCE(display_name, '') FROM tenants WHERE id = $1::uuid`,
		tenantID).Scan(&kind, &displayName); err != nil {
		return false, false, err
	}
	return kind == "business", displayName == expectedName, nil
}

// t04CountRows counts rows for one single-argument scalar query.
func (j t04Journey) t04CountRows(ctx context.Context, query string, args ...any) (int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var rows int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&rows); err != nil {
		return 0, err
	}
	return rows, nil
}

// t04CreationMapping returns the business_tenant_creations row count for
// one (actor, key) and whether the mapped tenant matches.
func (j t04Journey) t04CreationMapping(ctx context.Context, userID, key, tenantID string) (int, bool, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, false, err
	}
	defer db.Close()
	var rows int
	var matches bool
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), bool_and(tenant_id::text = $3::text)
		FROM business_tenant_creations
		WHERE actor_user_id = $1::uuid AND idempotency_key = $2`,
		userID, key, tenantID).Scan(&rows, &matches); err != nil {
		return 0, false, err
	}
	return rows, matches, nil
}

// t04UserTenantCounts returns the total tenant count of one user and its
// business subset.
func (j t04Journey) t04UserTenantCounts(ctx context.Context, userID string) (int, int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()
	var total, business int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE kind = 'business')
		FROM tenants WHERE owner_user_id = $1::uuid`,
		userID).Scan(&total, &business); err != nil {
		return 0, 0, err
	}
	return total, business, nil
}

// t04FailedReport builds a failing report whose assertion set matches
// the declared T04 names (all failed), keeping the harness
// reconciliation valid.
func t04FailedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(t04AssertionNames))
	for _, name := range t04AssertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}
