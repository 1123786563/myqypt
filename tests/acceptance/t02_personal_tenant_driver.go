package acceptance

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// seamPersonalTenant names the T02 seam: the same black-box lighthouse
// position as T01, watching the first identity bind provision the new
// user's personal tenant bundle.
const seamPersonalTenant = "lighthouse-personal-tenant"

// t02StackStartupCommand is the precise recipe that brings the
// acceptance stack up for the T02 journey. It carries the same six
// required compose variables as the T01 recipe plus the audience
// override: the T02 application's fixed client id — and therefore the
// aud claim of every token it mints — is t02-acceptance, while the
// compose default audience is T01's t01-acceptance.
const t02StackStartupCommand = "cd deploy/compose && PLATFORM_IDENTITY_OIDC_AUDIENCE=t02-acceptance PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose up -d --wait"

func init() {
	platformtest.Register(seamPersonalTenant, personalTenantDriver{})
}

// t02AssertionNames is the exact set declared by
// scenarios/t02-personal-tenant.yaml, in declaration order. The harness
// reconciles by name and rejects any drift, so this list and the YAML
// must move together.
var t02AssertionNames = []string{
	"first_bind_status",
	"first_bind_body_user_id_uuid",
	"personal_tenant_count",
	"billing_customer_count",
	"owner_membership_count",
	"tenant_billing_one_to_one",
	"replay_status",
	"replay_body_identical",
	"replay_tenant_bundle_counts",
	"denial_missing_authorization_status",
	"denial_tampered_signature_status",
	"denial_row_delta",
}

// personalTenantDriver executes the T02 journey against the real compose
// stack: provision Casdoor with t02-prefixed identifiers, mint a real
// RS256 token, bind it through the callback, replay it, attack it, then
// assert the provisioned tenant bundle in the database. Everything is
// driven over the wire; nothing links into the platform internals.
type personalTenantDriver struct{}

// Execute runs the whole journey and returns one assertion result per
// declared assertion name. Details carry facts only (status codes, row
// counts, match booleans) — never tokens, subjects, or credentials.
func (personalTenantDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	j := t02JourneyFromInputs(scenario.Inputs)

	if err := j.t02PrecheckStack(ctx); err != nil {
		return t02FailedReport(err.Error()), nil
	}

	client := newJourneyClient()

	if err := j.provisionCasdoor(ctx, client); err != nil {
		return t02FailedReport(fmt.Sprintf("casdoor provisioning failed: %v", err)), nil
	}

	token, claims, err := j.mintToken(ctx, client)
	if err != nil {
		return t02FailedReport(fmt.Sprintf("token mint failed: %v", err)), nil
	}
	if claims.Issuer != j.issuer {
		return t02FailedReport("token iss does not match the configured issuer (value omitted)"), nil
	}
	if !audienceContains(claims.Audience, j.clientID) {
		return t02FailedReport("token aud does not include the provisioned client id"), nil
	}
	if claims.Subject == "" {
		return t02FailedReport("token sub is empty"), nil
	}

	// Stale-state precheck: the journey proves the FIRST bind (201) with
	// fresh tenant provisioning, so the platform database must hold zero
	// users before anything is delivered. A warm stack whose named
	// postgres volume survived a previous run already carries users and
	// would answer the idempotent 200 — fail closed here instead of
	// reporting a confusing 201-vs-200 assertion failure. The #100
	// harness redacts every driver summary and assertion detail from
	// reports and evidence, so the skip message in the test file and its
	// reset command are the only visible guidance.
	users, err := j.countPlatformUsers(ctx)
	if err != nil {
		return platformtest.Report{}, fmt.Errorf("stale-state precheck failed (platform database unreachable): %w", err)
	}
	if users > 0 {
		return platformtest.Report{}, fmt.Errorf("stale platform state: %d platform_users row(s) already exist; the journey proves a fresh first bind with tenant provisioning and requires a clean platform database — reset the stack and rerun: %s", users, stackResetCommand)
	}

	results := map[string]platformtest.AssertionResult{}
	record := func(name string, passed bool, details string) {
		results[name] = platformtest.AssertionResult{Name: name, Passed: passed, Details: details}
	}

	// Happy path: the first bind must create the user (201) with the
	// exact {"user_id":"<uuid>"} body — the T01 contract, unchanged.
	first := j.postCallback(ctx, client, "Bearer "+token)
	record("first_bind_status", first.Status == http.StatusCreated, fmt.Sprintf("status=%d", first.Status))
	bodyIsUUID := first.Status == http.StatusCreated && userIDPattern.MatchString(first.Body)
	record("first_bind_body_user_id_uuid", bodyIsUUID, fmt.Sprintf("canonical_uuid=%t", bodyIsUUID))

	// Database truth: the bound user owns exactly one personal tenant,
	// exactly one billing customer on it, and exactly one active owner
	// membership in it (T02 core invariant).
	state, err := j.tenantBundleState(ctx, claims.Subject)
	if err != nil {
		return t02FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("personal_tenant_count", state.tenants == 1, fmt.Sprintf("rows=%d", state.tenants))
	record("billing_customer_count", state.billingCustomers == 1, fmt.Sprintf("rows=%d", state.billingCustomers))
	record("owner_membership_count", state.ownerMemberships == 1, fmt.Sprintf("rows=%d", state.ownerMemberships))
	// 1:1 both ways: every personal tenant carries exactly one billing
	// customer (tenant side of the pairing) and every billing customer
	// sits on exactly one of the user's personal tenants (customer
	// side); the schema's UNIQUE tenant_id enforces the latter, the
	// journey observes both.
	oneToOne := state.tenants == 1 && state.tenantBillingPairs == 1 && state.billingCustomers == 1
	record("tenant_billing_one_to_one", oneToOne, fmt.Sprintf("tenants=%d tenant_billing_pairs=%d billing_customers=%d", state.tenants, state.tenantBillingPairs, state.billingCustomers))

	// Idempotent path: every replay of the same token must return 200
	// with the byte-identical body and never provision a second bundle.
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

	stateAfterReplay, err := j.tenantBundleState(ctx, claims.Subject)
	if err != nil {
		return t02FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	bundleUnchanged := stateAfterReplay.tenants == 1 && stateAfterReplay.billingCustomers == 1 && stateAfterReplay.ownerMemberships == 1
	record("replay_tenant_bundle_counts", bundleUnchanged, fmt.Sprintf("tenants=%d billing_customers=%d memberships=%d", stateAfterReplay.tenants, stateAfterReplay.billingCustomers, stateAfterReplay.ownerMemberships))

	// Denial path: unverifiable credentials must yield 401 without any
	// effect on any of the five business tables (row delta).
	footprintBefore, err := j.identityFootprint(ctx, claims.Subject)
	if err != nil {
		return t02FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	missing := j.postCallback(ctx, client, "")
	tamperedToken, err := tamperSignature(token)
	if err != nil {
		return t02FailedReport(fmt.Sprintf("tampered token construction failed: %v", err)), nil
	}
	tampered := j.postCallback(ctx, client, "Bearer "+tamperedToken)
	record("denial_missing_authorization_status", missing.Status == http.StatusUnauthorized, fmt.Sprintf("status=%d", missing.Status))
	record("denial_tampered_signature_status", tampered.Status == http.StatusUnauthorized, fmt.Sprintf("status=%d", tampered.Status))

	footprintAfter, err := j.identityFootprint(ctx, claims.Subject)
	if err != nil {
		return t02FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("denial_row_delta", footprintAfter-footprintBefore == 0, fmt.Sprintf("delta=%d", footprintAfter-footprintBefore))

	ordered := make([]platformtest.AssertionResult, 0, len(t02AssertionNames))
	passed := true
	for _, name := range t02AssertionNames {
		result, ok := results[name]
		if !ok {
			return t02FailedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}

	summary := fmt.Sprintf("first=%d tenants=%d billing=%d memberships=%d replay_200=%t replay_identical=%t denial_missing_auth=%d denial_tampered=%d row_delta=%d",
		first.Status, stateAfterReplay.tenants, stateAfterReplay.billingCustomers, stateAfterReplay.ownerMemberships,
		replayOK, replayIdentical, missing.Status, tampered.Status, footprintAfter-footprintBefore)
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// t02JourneyFromInputs reads the scenario inputs with t02-prefixed
// defaults so the T02 identifiers never collide with the T01 journey
// fixtures (plan ruling 12).
func t02JourneyFromInputs(inputs map[string]any) journey {
	return journey{
		issuer:       inputString(inputs, "issuer", "http://casdoor:8000"),
		apiBase:      inputString(inputs, "casdoor_api_base", "http://localhost:8000"),
		callbackBase: inputString(inputs, "callback_base", "http://localhost:8080"),
		callbackPath: inputString(inputs, "callback_path", "/internal/v1/identity/callback"),
		org:          inputString(inputs, "casdoor_organization", "t02-accept-org"),
		appName:      inputString(inputs, "casdoor_application_name", "t02-acceptance-app"),
		clientID:     inputString(inputs, "casdoor_client_id", "t02-acceptance"),
		username:     inputString(inputs, "casdoor_username", "t02-accept"),
		replays:      inputInt(inputs, "replay_deliveries", 1),
		pgHost:       inputString(inputs, "postgres_host", "localhost"),
		pgPort:       inputString(inputs, "postgres_port", "5432"),
		pgDatabase:   inputString(inputs, "postgres_database", "platform"),
		pgUser:       inputString(inputs, "postgres_user", "platform"),
	}
}

// t02PrecheckStack verifies both stack entry points answer before
// anything else runs, failing with the T02 startup command (which
// carries the audience override) in the message.
func (j journey) t02PrecheckStack(ctx context.Context) error {
	casdoorURL := strings.TrimSuffix(j.apiBase, "/") + "/api/health"
	if err := probe(ctx, casdoorURL); err != nil {
		return fmt.Errorf("casdoor unreachable at %s (%v); start the stack first: %s", casdoorURL, err, t02StackStartupCommand)
	}
	livezURL := strings.TrimSuffix(j.callbackBase, "/") + "/livez"
	if err := probe(ctx, livezURL); err != nil {
		return fmt.Errorf("platform-api unreachable at %s (%v); start the stack first: %s", livezURL, err, t02StackStartupCommand)
	}
	return nil
}

// countPlatformUsers returns the total platform_users row count, the
// zero baseline the T02 journey requires.
func (j journey) countPlatformUsers(ctx context.Context) (int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM platform_users`).Scan(&rows); err != nil {
		return 0, err
	}
	return rows, nil
}

// tenantBundleState is the observed provisioning shape for the user
// bound to the token subject: its personal tenants, the billing
// customers on those tenants, the (tenant, billing customer) pairs —
// which proves the 1:1 pairing from the tenant side — and its active
// owner memberships in those tenants.
type tenantBundleState struct {
	tenants            int
	billingCustomers   int
	tenantBillingPairs int
	ownerMemberships   int
}

// tenantBundleState reads the bundle shape from the platform database.
func (j journey) tenantBundleState(ctx context.Context, subject string) (tenantBundleState, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return tenantBundleState{}, err
	}
	defer db.Close()

	var state tenantBundleState
	if err := db.QueryRowContext(ctx, `
		WITH bound AS (
			SELECT platform_user_id AS user_id
			FROM identity_bindings
			WHERE identity_provider = $1 AND subject = $2
		), personal AS (
			SELECT t.id
			FROM tenants t
			JOIN bound b ON b.user_id = t.owner_user_id
			WHERE t.kind = 'personal'
		)
		SELECT
			(SELECT count(*) FROM personal),
			(SELECT count(*) FROM billing_customers bc WHERE bc.tenant_id IN (SELECT id FROM personal)),
			(SELECT count(*) FROM personal p LEFT JOIN billing_customers bc ON bc.tenant_id = p.id),
			(SELECT count(*) FROM memberships m
			 JOIN personal p ON p.id = m.tenant_id
			 WHERE m.user_id IN (SELECT user_id FROM bound)
			   AND m.role = 'owner' AND m.status = 'active')`,
		j.issuer, subject).Scan(
		&state.tenants,
		&state.billingCustomers,
		&state.tenantBillingPairs,
		&state.ownerMemberships); err != nil {
		return tenantBundleState{}, err
	}
	return state, nil
}

// identityFootprint returns the total row count the identity holds
// across the five business tables (its user, binding, tenants, billing
// customers, and memberships). Denied deliveries must not change it.
func (j journey) identityFootprint(ctx context.Context, subject string) (int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var rows int
	if err := db.QueryRowContext(ctx, `
		WITH bound AS (
			SELECT platform_user_id AS user_id
			FROM identity_bindings
			WHERE identity_provider = $1 AND subject = $2
		)
		SELECT
			(SELECT count(*) FROM platform_users u JOIN bound b ON b.user_id = u.id)
			+ (SELECT count(*) FROM identity_bindings ib JOIN bound b ON b.user_id = ib.platform_user_id)
			+ (SELECT count(*) FROM tenants t JOIN bound b ON b.user_id = t.owner_user_id)
			+ (SELECT count(*) FROM billing_customers bc WHERE bc.tenant_id IN (
				SELECT t.id FROM tenants t JOIN bound b ON b.user_id = t.owner_user_id))
			+ (SELECT count(*) FROM memberships m JOIN bound b ON b.user_id = m.user_id)`,
		j.issuer, subject).Scan(&rows); err != nil {
		return 0, err
	}
	return rows, nil
}

// t02FailedReport builds a failing report whose assertion set matches
// the declared T02 names (all failed), keeping the harness
// reconciliation valid.
func t02FailedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(t02AssertionNames))
	for _, name := range t02AssertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}
