package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/1123786563/myqypt/tests/platformtest"
)

// seamPlatformRoles names the T06 seam: the same black-box lighthouse
// position as T01-T05, watching the four Platform Roles read their own
// capability sets and stay denied beyond them through the public
// contract endpoints.
const seamPlatformRoles = "lighthouse-platform-roles"

// t06StackStartupCommand is the precise recipe that brings the
// acceptance stack up for the T06 journey. It carries the same six
// required compose variables as the T01 recipe plus the audience
// override (the T06 application's fixed client id — and therefore the
// aud claim of every token it mints — is t06-acceptance, while the
// compose default audience is T01's) and the port-override file that
// remaps the published host ports (platform-api 18080, postgres 25432,
// casdoor 18000) so the stack never collides with the ports other
// projects hold on this machine.
const t06StackStartupCommand = "cd deploy/compose && PLATFORM_IDENTITY_OIDC_AUDIENCE=t06-acceptance PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose -f compose.yaml -f /tmp/t06-port-override.yaml up -d --wait"

// t06StackResetCommand tears the stack down with its volumes (compose
// interpolation needs the six required variables even for down). The
// journey proves fresh first binds and fresh first invitations (201),
// so every rerun must start from this reset — the named postgres volume
// survives a plain down/up cycle.
const t06StackResetCommand = "cd deploy/compose && PLATFORM_IDENTITY_OIDC_AUDIENCE=t06-acceptance PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose -f compose.yaml -f /tmp/t06-port-override.yaml down -v"

func init() {
	platformtest.Register(seamPlatformRoles, platformRolesDriver{})
}

// t06AssertionNames is the exact set declared by
// scenarios/t06-platform-roles.yaml, in declaration order. The harness
// reconciles by name and rejects any drift, so this list and the YAML
// must move together.
var t06AssertionNames = []string{
	"stale_state_zero_users",
	"bind_user_a_status",
	"create_tenant_status",
	"bind_user_b_status",
	"bind_user_c_status",
	"bind_user_d_status",
	"bind_user_e_status",
	"bind_user_f_status",
	"invite_admin_status",
	"accept_admin_status",
	"invite_billing_member_status",
	"accept_billing_member_status",
	"invite_member_status",
	"accept_member_status",
	"capabilities_owner_status",
	"capabilities_admin_status",
	"capabilities_billing_member_status",
	"capabilities_member_status",
	"capability_sets_pairwise_distinct",
	"member_invites_denied",
	"member_invite_zero_rows",
	"billing_member_invites_denied",
	"billing_member_invite_zero_rows",
	"admin_invites_status",
	"invited_denied_capabilities",
	"never_invited_denied_capabilities",
	"unknown_tenant_denied_capabilities",
	"denial_missing_authorization_status",
	"denial_tampered_signature_status",
	"accept_final_member_status",
	"final_member_capabilities_status",
	"capabilities_replay_body_identical",
}

// t06WantCapabilities pins the exact sorted wire sets derived verbatim
// from the CONTEXT.md Platform access definitions (design ruling 1):
// Owner holds the full superset, Admin the four administration domains,
// Billing Member the four billing domains, Member the single product.use
// domain. The journey compares the served bodies against these lists
// element by element.
var t06WantCapabilities = map[string][]string{
	"owner": {
		"billing.manage",
		"bills.read",
		"configuration.manage",
		"membership.manage",
		"ownership.manage",
		"payments.manage",
		"product.use",
		"product_access.manage",
		"purchases.manage",
		"subscriptions.read",
		"usage.read",
	},
	"admin": {
		"configuration.manage",
		"membership.manage",
		"product_access.manage",
		"purchases.manage",
	},
	"billing_member": {
		"bills.read",
		"payments.manage",
		"subscriptions.read",
		"usage.read",
	},
	"member": {
		"product.use",
	},
}

// platformRolesDriver executes the T06 journey against the real compose
// stack: six Casdoor users bind through the identity callback (each
// gaining its personal tenant), user A creates the business tenant, A
// invites B/C/D as admin/billing_member/member and each accepts, then
// the journey drives the capabilities and invitation endpoints over the
// wire — the four roles read their own exact, pairwise-distinct
// capability sets (the AC1 positive), a member and a billing_member
// inviting E are refused without an oracle or a stray row, E while
// merely invited and F while bound-but-never-invited read capabilities
// as 404, an unknown tenant id is an indistinguishable 404, missing and
// tampered credentials are 401s, admin B invites E (the membership
// .manage path end to end), E accepts, reads the member set, and a
// replay answers the byte-identical body. Nothing links into the
// platform internals.
type platformRolesDriver struct{}

// t06Journey embeds the T05 journey (which itself embeds the T03/T04
// journeys and the shared helpers, including the named-user
// provisioning, token minting, tenant creation, invitation requests,
// and the platform database access T06 reuses) and adds the fourth
// through sixth Casdoor usernames plus the role fixtures and expected
// capability sets.
type t06Journey struct {
	t05Journey
	usernameD        string
	usernameE        string
	usernameF        string
	roleAdmin        string
	roleBilling      string
	roleMember       string
	keyAdmin         string
	keyBilling       string
	keyMember        string
	keyDenialMember  string
	keyDenialBilling string
	keyFinal         string
	unknownTenant    string
}

// Execute runs the whole journey and returns one assertion result per
// declared assertion name. Details carry facts only (status codes, row
// counts, match booleans) — never tokens, subjects, or credentials.
func (platformRolesDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	j := t06JourneyFromInputs(scenario.Inputs)

	if err := j.t06PrecheckStack(ctx); err != nil {
		return t06FailedReport(err.Error()), nil
	}

	client := newJourneyClient()

	if err := j.provisionCasdoor(ctx, client); err != nil {
		return t06FailedReport(fmt.Sprintf("casdoor provisioning failed: %v", err)), nil
	}
	for _, username := range []string{j.usernameB, j.usernameC, j.usernameD, j.usernameE, j.usernameF} {
		if err := j.ensureNamedUser(ctx, client, username); err != nil {
			return t06FailedReport(fmt.Sprintf("casdoor provisioning of %s failed: %v", username, err)), nil
		}
	}

	tokenA, claimsA, err := j.mintToken(ctx, client)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("token mint for user A failed: %v", err)), nil
	}
	tokenB, claimsB, err := j.mintTokenFor(ctx, client, j.usernameB)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("token mint for user B failed: %v", err)), nil
	}
	tokenC, claimsC, err := j.mintTokenFor(ctx, client, j.usernameC)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("token mint for user C failed: %v", err)), nil
	}
	tokenD, claimsD, err := j.mintTokenFor(ctx, client, j.usernameD)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("token mint for user D failed: %v", err)), nil
	}
	tokenE, claimsE, err := j.mintTokenFor(ctx, client, j.usernameE)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("token mint for user E failed: %v", err)), nil
	}
	tokenF, claimsF, err := j.mintTokenFor(ctx, client, j.usernameF)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("token mint for user F failed: %v", err)), nil
	}
	for _, claims := range []tokenClaims{claimsA, claimsB, claimsC, claimsD, claimsE, claimsF} {
		if claims.Issuer != j.issuer {
			return t06FailedReport("token iss does not match the configured issuer (value omitted)"), nil
		}
		if !audienceContains(claims.Audience, j.clientID) {
			return t06FailedReport("token aud does not include the provisioned client id"), nil
		}
		if claims.Subject == "" {
			return t06FailedReport("token sub is empty"), nil
		}
	}

	// Stale-state precheck (the T03/T04/T05 precedent): the journey
	// proves fresh first binds and fresh first invitations (201), so the
	// platform database must hold zero users before anything is
	// delivered.
	users, err := j.countPlatformUsers(ctx)
	if err != nil {
		return platformtest.Report{}, fmt.Errorf("stale-state precheck failed (platform database unreachable): %w", err)
	}
	if users > 0 {
		return platformtest.Report{}, fmt.Errorf("stale platform state: %d platform_users row(s) already exist; the journey requires a clean platform database — reset the stack and rerun: %s", users, t06StackResetCommand)
	}

	results := map[string]platformtest.AssertionResult{}
	record := func(name string, passed bool, details string) {
		results[name] = platformtest.AssertionResult{Name: name, Passed: passed, Details: details}
	}
	record("stale_state_zero_users", users == 0, fmt.Sprintf("rows=%d", users))

	// User A binds (gaining its personal tenant), then creates the
	// business tenant whose membership roles the journey exercises.
	bindA := j.postCallback(ctx, client, "Bearer "+tokenA)
	record("bind_user_a_status", bindA.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindA.Status))
	if bindA.Status != http.StatusCreated {
		return t06FailedReport("user A's first bind did not answer 201"), nil
	}

	tenantsPath := strings.TrimSuffix(j.callbackBase, "/") + "/api/v1/tenants"
	created := t04Create(ctx, client, tenantsPath, `{"display_name":"`+j.displayName+`"}`, j.idempotencyA, "Bearer "+tokenA)
	createdTenant := t04ParseCreated(created.Body)
	record("create_tenant_status",
		created.Status == http.StatusCreated && createdTenant.TenantID != "",
		fmt.Sprintf("status=%d tenant_id_present=%t", created.Status, createdTenant.TenantID != ""))
	if created.Status != http.StatusCreated || createdTenant.TenantID == "" {
		return t06FailedReport("business tenant creation did not answer 201 with a tenant id"), nil
	}
	businessTenant := createdTenant.TenantID
	invitationPath := tenantsPath + "/" + businessTenant + "/membership-invitations"
	acceptancePath := invitationPath + "/acceptance"
	capabilitiesPath := tenantsPath + "/" + businessTenant + "/capabilities"

	// Users B through F bind (gaining their personal tenants): B becomes
	// the admin, C the billing member, D the member, E the late invitee,
	// F the never-invited reader.
	bindB := j.postCallback(ctx, client, "Bearer "+tokenB)
	record("bind_user_b_status", bindB.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindB.Status))
	bindC := j.postCallback(ctx, client, "Bearer "+tokenC)
	record("bind_user_c_status", bindC.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindC.Status))
	bindD := j.postCallback(ctx, client, "Bearer "+tokenD)
	record("bind_user_d_status", bindD.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindD.Status))
	bindE := j.postCallback(ctx, client, "Bearer "+tokenE)
	record("bind_user_e_status", bindE.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindE.Status))
	bindF := j.postCallback(ctx, client, "Bearer "+tokenF)
	record("bind_user_f_status", bindF.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindF.Status))
	if bindB.Status != http.StatusCreated || bindC.Status != http.StatusCreated ||
		bindD.Status != http.StatusCreated || bindE.Status != http.StatusCreated ||
		bindF.Status != http.StatusCreated {
		return t06FailedReport("a journey user's first bind did not answer 201"), nil
	}
	userE := t03UserID(bindE.Body)
	if userE == "" {
		return t06FailedReport("user E's first bind did not return a canonical user id"), nil
	}

	// The owner grants the three non-owner roles: invite (201) and the
	// invitee's acceptance (200) for each of admin, billing_member,
	// member.
	inviteStatuses := map[string]int{}
	acceptStatuses := map[string]int{}
	grantRole := func(inviteeToken string, inviteeClaims tokenClaims, role, idempotencyKey, inviteName, acceptName string) bool {
		body := `{"invitee_subject":"` + inviteeClaims.Subject + `","role":"` + role + `"}`
		invited := t05Invite(ctx, client, invitationPath, body, idempotencyKey, "Bearer "+tokenA)
		inviteStatuses[inviteName] = invited.Status
		record(inviteName, invited.Status == http.StatusCreated, fmt.Sprintf("status=%d", invited.Status))
		if invited.Status != http.StatusCreated {
			return false
		}
		accepted := t05Accept(ctx, client, acceptancePath, "Bearer "+inviteeToken)
		acceptStatuses[acceptName] = accepted.Status
		record(acceptName, accepted.Status == http.StatusOK, fmt.Sprintf("status=%d", accepted.Status))
		return accepted.Status == http.StatusOK
	}
	if !grantRole(tokenB, claimsB, j.roleAdmin, j.keyAdmin, "invite_admin_status", "accept_admin_status") {
		return t06FailedReport("the admin invitation or acceptance did not answer 201/200"), nil
	}
	if !grantRole(tokenC, claimsC, j.roleBilling, j.keyBilling, "invite_billing_member_status", "accept_billing_member_status") {
		return t06FailedReport("the billing_member invitation or acceptance did not answer 201/200"), nil
	}
	if !grantRole(tokenD, claimsD, j.roleMember, j.keyMember, "invite_member_status", "accept_member_status") {
		return t06FailedReport("the member invitation or acceptance did not answer 201/200"), nil
	}

	// AC1 positive: each of the four roles reads its own capability set
	// — the exact CONTEXT.md list, echoed tenant and role, sorted.
	readCapabilities := func(token, role, name string) (t06CapabilitiesBody, bool) {
		response := t03Request(ctx, client, http.MethodGet, capabilitiesPath, "", "Bearer "+token)
		served := t06ParseCapabilities(response.Body)
		setExact := t06EqualSets(served.Capabilities, t06WantCapabilities[role])
		exact := response.Status == http.StatusOK &&
			served.Role == role &&
			served.TenantID == businessTenant &&
			setExact
		record(name, exact, fmt.Sprintf("status=%d role_echo=%t set_exact=%t", response.Status, served.Role == role, setExact))
		return served, exact
	}
	ownerServed, ownerOK := readCapabilities(tokenA, "owner", "capabilities_owner_status")
	adminServed, adminOK := readCapabilities(tokenB, j.roleAdmin, "capabilities_admin_status")
	billingServed, billingOK := readCapabilities(tokenC, j.roleBilling, "capabilities_billing_member_status")
	memberServed, memberOK := readCapabilities(tokenD, j.roleMember, "capabilities_member_status")
	if !ownerOK || !adminOK || !billingOK || !memberOK {
		return t06FailedReport("one of the four capability reads did not answer 200 with the exact set"), nil
	}

	// The four visible sets are pairwise distinct (AC1: each role gets
	// its own operations).
	servedSets := [][]string{ownerServed.Capabilities, adminServed.Capabilities, billingServed.Capabilities, memberServed.Capabilities}
	pairwiseDistinct := true
	for i := range servedSets {
		for k := i + 1; k < len(servedSets); k++ {
			if t06EqualSets(servedSets[i], servedSets[k]) {
				pairwiseDistinct = false
			}
		}
	}
	record("capability_sets_pairwise_distinct", pairwiseDistinct, fmt.Sprintf("distinct=%t", pairwiseDistinct))

	// Non-escalation (AC2): the member D and the billing member C invite
	// E — both are indistinguishable 404s and E holds zero rows.
	memberInvites := t05Invite(ctx, client, invitationPath,
		`{"invitee_subject":"`+claimsE.Subject+`","role":"`+j.roleMember+`"}`, j.keyDenialMember, "Bearer "+tokenD)
	eRows, err := j.t05CountTenantMemberships(ctx, businessTenant, userE)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("member_invites_denied", memberInvites.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", memberInvites.Status))
	record("member_invite_zero_rows", eRows == 0, fmt.Sprintf("rows=%d", eRows))

	billingInvites := t05Invite(ctx, client, invitationPath,
		`{"invitee_subject":"`+claimsE.Subject+`","role":"`+j.roleMember+`"}`, j.keyDenialBilling, "Bearer "+tokenC)
	eRowsAfterBilling, err := j.t05CountTenantMemberships(ctx, businessTenant, userE)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("billing_member_invites_denied", billingInvites.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", billingInvites.Status))
	record("billing_member_invite_zero_rows", eRowsAfterBilling == 0, fmt.Sprintf("rows=%d", eRowsAfterBilling))

	// membership.manage end to end: the admin B invites E — 201, and E
	// now holds the invited row.
	adminInvites := t05Invite(ctx, client, invitationPath,
		`{"invitee_subject":"`+claimsE.Subject+`","role":"`+j.roleMember+`"}`, j.keyFinal, "Bearer "+tokenB)
	record("admin_invites_status", adminInvites.Status == http.StatusCreated,
		fmt.Sprintf("status=%d", adminInvites.Status))
	if adminInvites.Status != http.StatusCreated {
		return t06FailedReport("the admin invitation of E did not answer 201"), nil
	}

	// E while merely invited (not accepted) reads capabilities: 404.
	invitedRead := t03Request(ctx, client, http.MethodGet, capabilitiesPath, "", "Bearer "+tokenE)
	record("invited_denied_capabilities", invitedRead.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", invitedRead.Status))

	// F while bound but never invited reads capabilities: 404.
	neverInvitedRead := t03Request(ctx, client, http.MethodGet, capabilitiesPath, "", "Bearer "+tokenF)
	record("never_invited_denied_capabilities", neverInvitedRead.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", neverInvitedRead.Status))

	// An unknown tenant id is an indistinguishable 404.
	unknownRead := t03Request(ctx, client, http.MethodGet,
		tenantsPath+"/"+j.unknownTenant+"/capabilities", "", "Bearer "+tokenA)
	record("unknown_tenant_denied_capabilities", unknownRead.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", unknownRead.Status))

	// Credential denials: missing and tampered credentials are 401s.
	missingAuth := t03Request(ctx, client, http.MethodGet, capabilitiesPath, "", "")
	record("denial_missing_authorization_status", missingAuth.Status == http.StatusUnauthorized,
		fmt.Sprintf("status=%d", missingAuth.Status))
	tamperedToken, err := tamperSignature(tokenA)
	if err != nil {
		return t06FailedReport(fmt.Sprintf("tampered token construction failed: %v", err)), nil
	}
	tampered := t03Request(ctx, client, http.MethodGet, capabilitiesPath, "", "Bearer "+tamperedToken)
	record("denial_tampered_signature_status", tampered.Status == http.StatusUnauthorized,
		fmt.Sprintf("status=%d", tampered.Status))

	// E accepts and immediately reads the member set.
	finalAccept := t05Accept(ctx, client, acceptancePath, "Bearer "+tokenE)
	record("accept_final_member_status", finalAccept.Status == http.StatusOK,
		fmt.Sprintf("status=%d", finalAccept.Status))
	if finalAccept.Status != http.StatusOK {
		return t06FailedReport("E's acceptance did not answer 200"), nil
	}
	finalRead := t03Request(ctx, client, http.MethodGet, capabilitiesPath, "", "Bearer "+tokenE)
	finalServed := t06ParseCapabilities(finalRead.Body)
	finalSetExact := t06EqualSets(finalServed.Capabilities, t06WantCapabilities[j.roleMember])
	finalExact := finalRead.Status == http.StatusOK &&
		finalServed.Role == j.roleMember &&
		finalSetExact
	record("final_member_capabilities_status", finalExact,
		fmt.Sprintf("status=%d role_echo=%t set_exact=%t", finalRead.Status, finalServed.Role == j.roleMember, finalSetExact))

	// Replay determinism (design ruling 6): two more identical requests
	// answer byte-identical bodies.
	replay1 := t03Request(ctx, client, http.MethodGet, capabilitiesPath, "", "Bearer "+tokenE)
	replay2 := t03Request(ctx, client, http.MethodGet, capabilitiesPath, "", "Bearer "+tokenE)
	replayIdentical := replay1.Body == finalRead.Body && replay2.Body == finalRead.Body
	record("capabilities_replay_body_identical", replayIdentical,
		fmt.Sprintf("identical=%t", replayIdentical))

	ordered := make([]platformtest.AssertionResult, 0, len(t06AssertionNames))
	passed := true
	for _, name := range t06AssertionNames {
		result, ok := results[name]
		if !ok {
			return t06FailedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}

	summary := fmt.Sprintf("bind_a=%d create=%d bind_b=%d bind_c=%d bind_d=%d bind_e=%d bind_f=%d invite_admin=%d accept_admin=%d invite_billing=%d accept_billing=%d invite_member=%d accept_member=%d owner=%t admin=%t billing=%t member=%t distinct=%t member_invite=%d member_rows=%d billing_invite=%d billing_rows=%d admin_invite=%d invited_read=%d never_invited=%d unknown=%d missing=%d tampered=%d accept_e=%d e_set=%t replay_identical=%t",
		bindA.Status, created.Status, bindB.Status, bindC.Status, bindD.Status, bindE.Status, bindF.Status,
		inviteStatuses["invite_admin_status"], acceptStatuses["accept_admin_status"],
		inviteStatuses["invite_billing_member_status"], acceptStatuses["accept_billing_member_status"],
		inviteStatuses["invite_member_status"], acceptStatuses["accept_member_status"],
		ownerOK, adminOK, billingOK, memberOK, pairwiseDistinct,
		memberInvites.Status, eRows, billingInvites.Status, eRowsAfterBilling,
		adminInvites.Status, invitedRead.Status, neverInvitedRead.Status, unknownRead.Status,
		missingAuth.Status, tampered.Status, finalAccept.Status, finalExact, replayIdentical)
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// t06JourneyFromInputs reads the scenario inputs with t06-prefixed
// defaults so the T06 identifiers never collide with the T01-T05 journey
// fixtures.
func t06JourneyFromInputs(inputs map[string]any) t06Journey {
	return t06Journey{
		t05Journey: t05Journey{
			t04Journey: t04Journey{
				t03Journey: t03Journey{
					journey: journey{
						issuer:       inputString(inputs, "issuer", "http://casdoor:8000"),
						apiBase:      inputString(inputs, "casdoor_api_base", "http://localhost:18000"),
						callbackBase: inputString(inputs, "callback_base", "http://localhost:18080"),
						callbackPath: inputString(inputs, "callback_path", "/internal/v1/identity/callback"),
						org:          inputString(inputs, "casdoor_organization", "t06-accept-org"),
						appName:      inputString(inputs, "casdoor_application_name", "t06-acceptance-app"),
						clientID:     inputString(inputs, "casdoor_client_id", "t06-acceptance"),
						username:     inputString(inputs, "casdoor_username_a", "t06-roles-a"),
						replays:      inputInt(inputs, "replay_deliveries", 1),
						pgHost:       inputString(inputs, "postgres_host", "localhost"),
						pgPort:       inputString(inputs, "postgres_port", "25432"),
						pgDatabase:   inputString(inputs, "postgres_database", "platform"),
						pgUser:       inputString(inputs, "postgres_user", "platform"),
					},
					usernameB: inputString(inputs, "casdoor_username_b", "t06-roles-b"),
				},
				displayName:  inputString(inputs, "display_name", "t06-accept-team"),
				idempotencyA: inputString(inputs, "idempotency_key_tenant", "t06-key-tenant"),
				idempotencyB: inputString(inputs, "idempotency_key_invite", "t06-key-admin"),
			},
			usernameC:         inputString(inputs, "casdoor_username_c", "t06-roles-c"),
			invitedRole:       inputString(inputs, "role_member", "member"),
			inviteeKey:        inputString(inputs, "idempotency_key_member", "t06-key-member"),
			neverBoundSubject: inputString(inputs, "never_bound_subject", "t06-never-bound-subject"),
		},
		usernameD:        inputString(inputs, "casdoor_username_d", "t06-roles-d"),
		usernameE:        inputString(inputs, "casdoor_username_e", "t06-roles-e"),
		usernameF:        inputString(inputs, "casdoor_username_f", "t06-roles-f"),
		roleAdmin:        inputString(inputs, "role_admin", "admin"),
		roleBilling:      inputString(inputs, "role_billing_member", "billing_member"),
		roleMember:       inputString(inputs, "role_member", "member"),
		keyAdmin:         inputString(inputs, "idempotency_key_admin", "t06-key-admin"),
		keyBilling:       inputString(inputs, "idempotency_key_billing", "t06-key-billing"),
		keyMember:        inputString(inputs, "idempotency_key_member", "t06-key-member"),
		keyDenialMember:  inputString(inputs, "idempotency_key_denial_member", "t06-key-denial-member"),
		keyDenialBilling: inputString(inputs, "idempotency_key_denial_billing", "t06-key-denial-billing"),
		keyFinal:         inputString(inputs, "idempotency_key_final", "t06-key-final"),
		unknownTenant:    inputString(inputs, "unknown_tenant", "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e906"),
	}
}

// t06PrecheckStack verifies both stack entry points answer before
// anything else runs, failing with the T06 startup command (which
// carries the audience override and the port-override file) in the
// message.
func (j t06Journey) t06PrecheckStack(ctx context.Context) error {
	casdoorURL := strings.TrimSuffix(j.apiBase, "/") + "/api/health"
	if err := probe(ctx, casdoorURL); err != nil {
		return fmt.Errorf("casdoor unreachable at %s (%v); start the stack first: %s", casdoorURL, err, t06StackStartupCommand)
	}
	livezURL := strings.TrimSuffix(j.callbackBase, "/") + "/livez"
	if err := probe(ctx, livezURL); err != nil {
		return fmt.Errorf("platform-api unreachable at %s (%v); start the stack first: %s", livezURL, err, t06StackStartupCommand)
	}
	return nil
}

// t06CapabilitiesBody decodes the capabilities response body.
type t06CapabilitiesBody struct {
	TenantID     string   `json:"tenant_id"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
}

// t06ParseCapabilities decodes a capabilities body (zero value on any
// decode failure).
func t06ParseCapabilities(body string) t06CapabilitiesBody {
	var payload t06CapabilitiesBody
	_ = json.Unmarshal([]byte(body), &payload)
	return payload
}

// t06EqualSets reports whether got equals want element by element (both
// are the sorted wire lists, so an exact ordered comparison is the pin).
func t06EqualSets(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// t06FailedReport builds a failing report whose assertion set matches
// the declared T06 names (all failed), keeping the harness
// reconciliation valid.
func t06FailedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(t06AssertionNames))
	for _, name := range t06AssertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}
