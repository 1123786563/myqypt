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

// seamMembershipInvitation names the T05 seam: the same black-box
// lighthouse position as T01-T04, watching the owner invite a user and
// the user accept through the public contract endpoints.
const seamMembershipInvitation = "lighthouse-membership-invitation"

// t05StackStartupCommand is the precise recipe that brings the
// acceptance stack up for the T05 journey. It carries the same six
// required compose variables as the T01 recipe plus the audience
// override: the T05 application's fixed client id — and therefore the
// aud claim of every token it mints — is t05-acceptance, while the
// compose default audience is T01's t01-acceptance.
const t05StackStartupCommand = "cd deploy/compose && PLATFORM_IDENTITY_OIDC_AUDIENCE=t05-acceptance PLATFORM_POSTGRES_DB=platform PLATFORM_POSTGRES_USER=platform PLATFORM_POSTGRES_PASSWORD=t01-accept-pw CASDOOR_POSTGRES_DB=casdoor CASDOOR_POSTGRES_USER=casdoor CASDOOR_POSTGRES_PASSWORD=t01-accept-pw docker compose up -d --wait"

func init() {
	platformtest.Register(seamMembershipInvitation, membershipInvitationDriver{})
}

// t05AssertionNames is the exact set declared by
// scenarios/t05-membership-invitation.yaml, in declaration order. The
// harness reconciles by name and rejects any drift, so this list and
// the YAML must move together.
var t05AssertionNames = []string{
	"stale_state_zero_users",
	"bind_user_a_status",
	"create_tenant_status",
	"bind_user_b_status",
	"bind_user_c_status",
	"invitee_denied_select_before_invite",
	"invite_status",
	"replay_status_body_identical",
	"invited_denied_select",
	"accept_status",
	"accept_list_contains_tenant",
	"accept_select_status",
	"denial_missing_idempotency_key_status",
	"denial_owner_role_status",
	"denial_missing_authorization_status",
	"denial_tampered_signature_status",
	"member_inviter_not_found_status",
	"member_inviter_zero_rows",
	"unbound_subject_not_found_status",
	"unbound_subject_zero_rows",
	"stranger_accept_not_found_status",
	"stranger_accept_zero_writes",
}

// membershipInvitationDriver executes the T05 journey against the real
// compose stack: three Casdoor users bind through the identity callback
// (each gaining its personal tenant), user A creates a business tenant,
// then the journey drives the invitation endpoints over the wire — the
// owner invites B (201), a replay converges with the identical body
// (200), B stays denied at the tenant-context seam while merely invited
// (the AC1 negative), B accepts (200), B's list/select immediately see
// the tenant, and the rejection paths (missing key, owner role,
// missing/tampered credentials, member-role inviter, never-bound
// subject, stranger acceptance) prove every refusal happens without an
// oracle or a stray write. Nothing links into the platform internals.
type membershipInvitationDriver struct{}

// t05Journey embeds the T04 journey (which itself embeds the T03
// journey and the shared helpers, including the named-user
// provisioning, token minting, tenant creation, list/select requests,
// and the platform database access T05 reuses) and adds the third
// Casdoor username plus the invitation fixtures.
type t05Journey struct {
	t04Journey
	usernameC         string
	invitedRole       string
	inviteeKey        string
	neverBoundSubject string
}

// Execute runs the whole journey and returns one assertion result per
// declared assertion name. Details carry facts only (status codes, row
// counts, match booleans) — never tokens, subjects, or credentials.
func (membershipInvitationDriver) Execute(ctx context.Context, scenario platformtest.Scenario) (platformtest.Report, error) {
	j := t05JourneyFromInputs(scenario.Inputs)

	if err := j.t05PrecheckStack(ctx); err != nil {
		return t05FailedReport(err.Error()), nil
	}

	client := newJourneyClient()

	if err := j.provisionCasdoor(ctx, client); err != nil {
		return t05FailedReport(fmt.Sprintf("casdoor provisioning failed: %v", err)), nil
	}
	if err := j.ensureNamedUser(ctx, client, j.usernameB); err != nil {
		return t05FailedReport(fmt.Sprintf("casdoor provisioning of user B failed: %v", err)), nil
	}
	if err := j.ensureNamedUser(ctx, client, j.usernameC); err != nil {
		return t05FailedReport(fmt.Sprintf("casdoor provisioning of user C failed: %v", err)), nil
	}

	tokenA, claimsA, err := j.mintToken(ctx, client)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("token mint for user A failed: %v", err)), nil
	}
	tokenB, claimsB, err := j.mintTokenFor(ctx, client, j.usernameB)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("token mint for user B failed: %v", err)), nil
	}
	tokenC, claimsC, err := j.mintTokenFor(ctx, client, j.usernameC)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("token mint for user C failed: %v", err)), nil
	}
	for _, claims := range []tokenClaims{claimsA, claimsB, claimsC} {
		if claims.Issuer != j.issuer {
			return t05FailedReport("token iss does not match the configured issuer (value omitted)"), nil
		}
		if !audienceContains(claims.Audience, j.clientID) {
			return t05FailedReport("token aud does not include the provisioned client id"), nil
		}
		if claims.Subject == "" {
			return t05FailedReport("token sub is empty"), nil
		}
	}

	// Stale-state precheck (the T03/T04 precedent): the journey proves
	// fresh first binds and a fresh first invitation (201), so the
	// platform database must hold zero users before anything is
	// delivered.
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

	// User A binds (gaining its personal tenant), then creates the
	// business tenant the invitation grants membership in.
	bindA := j.postCallback(ctx, client, "Bearer "+tokenA)
	record("bind_user_a_status", bindA.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindA.Status))
	if bindA.Status != http.StatusCreated {
		return t05FailedReport("user A's first bind did not answer 201"), nil
	}

	tenantsPath := strings.TrimSuffix(j.callbackBase, "/") + "/api/v1/tenants"
	contextPath := strings.TrimSuffix(j.callbackBase, "/") + "/api/v1/tenant-context"
	created := t04Create(ctx, client, tenantsPath, `{"display_name":"`+j.displayName+`"}`, j.idempotencyA, "Bearer "+tokenA)
	createdTenant := t04ParseCreated(created.Body)
	record("create_tenant_status",
		created.Status == http.StatusCreated && createdTenant.TenantID != "",
		fmt.Sprintf("status=%d tenant_id_present=%t", created.Status, createdTenant.TenantID != ""))
	if created.Status != http.StatusCreated || createdTenant.TenantID == "" {
		return t05FailedReport("business tenant creation did not answer 201 with a tenant id"), nil
	}
	businessTenant := createdTenant.TenantID
	invitationPath := tenantsPath + "/" + businessTenant + "/membership-invitations"
	acceptancePath := invitationPath + "/acceptance"

	// Users B and C bind (gaining their personal tenants): B becomes the
	// invitee, C the stranger whose acceptance must fail.
	bindB := j.postCallback(ctx, client, "Bearer "+tokenB)
	record("bind_user_b_status", bindB.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindB.Status))
	bindC := j.postCallback(ctx, client, "Bearer "+tokenC)
	record("bind_user_c_status", bindC.Status == http.StatusCreated, fmt.Sprintf("status=%d", bindC.Status))
	if bindB.Status != http.StatusCreated || bindC.Status != http.StatusCreated {
		return t05FailedReport("users B and C did not both answer 201 on their first binds"), nil
	}
	userB := t03UserID(bindB.Body)
	userC := t03UserID(bindC.Body)
	if userB == "" || userC == "" {
		return t05FailedReport("first binds did not return canonical user ids for B and C"), nil
	}

	// AC1 negative precondition: before any invitation exists, B cannot
	// select the business tenant — an indistinguishable 404.
	deniedBefore := t03Request(ctx, client, http.MethodPut, contextPath,
		`{"tenant_id":"`+businessTenant+`"}`, "Bearer "+tokenB)
	record("invitee_denied_select_before_invite", deniedBefore.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", deniedBefore.Status))

	// First invitation: 201 with the invitation facts echoed verbatim.
	inviteBody := `{"invitee_subject":"` + claimsB.Subject + `","role":"` + j.invitedRole + `"}`
	first := t05Invite(ctx, client, invitationPath, inviteBody, j.inviteeKey, "Bearer "+tokenA)
	firstInvitation := t05ParseInvitation(first.Body)
	shapeOK := firstInvitation.TenantID == businessTenant &&
		firstInvitation.Role == j.invitedRole &&
		firstInvitation.Status == "invited" &&
		t05InvitedAtParses(first.Body)
	record("invite_status",
		first.Status == http.StatusCreated && shapeOK,
		fmt.Sprintf("status=%d tenant_id_echo=%t role_echo=%t status_invited=%t invited_at_rfc3339=%t",
			first.Status, firstInvitation.TenantID == businessTenant, firstInvitation.Role == j.invitedRole,
			firstInvitation.Status == "invited", t05InvitedAtParses(first.Body)))
	if first.Status != http.StatusCreated || !shapeOK {
		return t05FailedReport("first membership invitation did not answer 201 with the invited facts"), nil
	}

	// Replay: a repeat delivery of the still-pending invitation answers
	// 200 with the byte-identical body (natural-key convergence).
	replay := t05Invite(ctx, client, invitationPath, inviteBody, j.inviteeKey, "Bearer "+tokenA)
	record("replay_status_body_identical",
		replay.Status == http.StatusOK && replay.Body == first.Body,
		fmt.Sprintf("status=%d body_identical=%t", replay.Status, replay.Body == first.Body))

	// AC1 negative: while B is merely invited, the tenant-context seam
	// keeps denying the selection and the membership row is the invited
	// row — invited, not active.
	deniedWhileInvited := t03Request(ctx, client, http.MethodPut, contextPath,
		`{"tenant_id":"`+businessTenant+`"}`, "Bearer "+tokenB)
	rowStatus, err := j.t05MembershipStatus(ctx, businessTenant, userB)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("invited_denied_select",
		deniedWhileInvited.Status == http.StatusNotFound && rowStatus == "invited",
		fmt.Sprintf("status=%d row_status_invited=%t", deniedWhileInvited.Status, rowStatus == "invited"))

	// Acceptance: the invitee's single transition invited -> active.
	accepted := t05Accept(ctx, client, acceptancePath, "Bearer "+tokenB)
	activation := t05ParseActivation(accepted.Body)
	record("accept_status",
		accepted.Status == http.StatusOK &&
			activation.TenantID == businessTenant &&
			activation.Role == j.invitedRole &&
			activation.Status == "active",
		fmt.Sprintf("status=%d tenant_id_echo=%t role_echo=%t status_active=%t",
			accepted.Status, activation.TenantID == businessTenant, activation.Role == j.invitedRole,
			activation.Status == "active"))
	if accepted.Status != http.StatusOK {
		return t05FailedReport("membership invitation acceptance did not answer 200"), nil
	}

	// T03 integration: B's list now contains the business tenant with
	// the granted role, and the selection succeeds.
	listB := t03Request(ctx, client, http.MethodGet, tenantsPath, "", "Bearer "+tokenB)
	entries := t03ParseTenantList(listB.Body)
	roleOK := false
	for _, entry := range entries {
		if entry.TenantID == businessTenant && entry.Role == j.invitedRole {
			roleOK = true
		}
	}
	record("accept_list_contains_tenant",
		listB.Status == http.StatusOK && roleOK,
		fmt.Sprintf("status=%d tenant_with_role=%t", listB.Status, roleOK))

	selected := t03Request(ctx, client, http.MethodPut, contextPath,
		`{"tenant_id":"`+businessTenant+`"}`, "Bearer "+tokenB)
	record("accept_select_status",
		selected.Status == http.StatusOK && t03SelectedTenant(selected.Body) == businessTenant,
		fmt.Sprintf("status=%d tenant_id_echo=%t", selected.Status,
			t03SelectedTenant(selected.Body) == businessTenant))

	// Rejection paths: a missing Idempotency-Key header and an owner-role
	// invitation are 400s; missing or tampered credentials are 401s.
	missingKey := t05Invite(ctx, client, invitationPath, inviteBody, "", "Bearer "+tokenA)
	record("denial_missing_idempotency_key_status", missingKey.Status == http.StatusBadRequest,
		fmt.Sprintf("status=%d", missingKey.Status))
	ownerRole := t05Invite(ctx, client, invitationPath,
		`{"invitee_subject":"`+claimsC.Subject+`","role":"owner"}`, j.inviteeKey, "Bearer "+tokenA)
	record("denial_owner_role_status", ownerRole.Status == http.StatusBadRequest,
		fmt.Sprintf("status=%d", ownerRole.Status))
	missingAuth := t05Invite(ctx, client, invitationPath, inviteBody, j.inviteeKey, "")
	record("denial_missing_authorization_status", missingAuth.Status == http.StatusUnauthorized,
		fmt.Sprintf("status=%d", missingAuth.Status))
	tamperedToken, err := tamperSignature(tokenA)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("tampered token construction failed: %v", err)), nil
	}
	tampered := t05Invite(ctx, client, invitationPath, inviteBody, j.inviteeKey, "Bearer "+tamperedToken)
	record("denial_tampered_signature_status", tampered.Status == http.StatusUnauthorized,
		fmt.Sprintf("status=%d", tampered.Status))

	// Member-role inviter: B (now an active member, not owner/admin)
	// inviting C is an indistinguishable 404 with zero rows for C.
	memberInvites := t05Invite(ctx, client, invitationPath,
		`{"invitee_subject":"`+claimsC.Subject+`","role":"member"}`, "t05-key-member", "Bearer "+tokenB)
	cRows, err := j.t05CountTenantMemberships(ctx, businessTenant, userC)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("member_inviter_not_found_status", memberInvites.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", memberInvites.Status))
	record("member_inviter_zero_rows", cRows == 0, fmt.Sprintf("rows=%d", cRows))

	// Never-bound invitee subject: the inviter names a subject no
	// identity binding holds — 404 with zero invitation rows.
	invitedBefore, err := j.t05CountInvitedRows(ctx, businessTenant)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	unbound := t05Invite(ctx, client, invitationPath,
		`{"invitee_subject":"`+j.neverBoundSubject+`","role":"member"}`, "t05-key-unbound", "Bearer "+tokenA)
	invitedAfter, err := j.t05CountInvitedRows(ctx, businessTenant)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("unbound_subject_not_found_status", unbound.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", unbound.Status))
	record("unbound_subject_zero_rows", invitedAfter-invitedBefore == 0,
		fmt.Sprintf("delta=%d", invitedAfter-invitedBefore))

	// Stranger acceptance: C accepts an invitation that does not exist —
	// 404, zero writes (C keeps zero rows, B's row stays active).
	strangerAccept := t05Accept(ctx, client, acceptancePath, "Bearer "+tokenC)
	cRowsAfter, err := j.t05CountTenantMemberships(ctx, businessTenant, userC)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	bStatus, err := j.t05MembershipStatus(ctx, businessTenant, userB)
	if err != nil {
		return t05FailedReport(fmt.Sprintf("database assertion failed: %v", err)), nil
	}
	record("stranger_accept_not_found_status", strangerAccept.Status == http.StatusNotFound,
		fmt.Sprintf("status=%d", strangerAccept.Status))
	record("stranger_accept_zero_writes", cRowsAfter == 0 && bStatus == "active",
		fmt.Sprintf("c_rows=%d b_row_active=%t", cRowsAfter, bStatus == "active"))

	ordered := make([]platformtest.AssertionResult, 0, len(t05AssertionNames))
	passed := true
	for _, name := range t05AssertionNames {
		result, ok := results[name]
		if !ok {
			return t05FailedReport("journey produced no result for assertion " + name), nil
		}
		if !result.Passed {
			passed = false
		}
		ordered = append(ordered, result)
	}

	summary := fmt.Sprintf("bind_a=%d create=%d bind_b=%d bind_c=%d denied_before=%d invite=%d replay=%d replay_identical=%t invited_denied=%d row_invited=%t accept=%d list=%d role_ok=%t select=%d missing_key=%d owner_role=%d missing_auth=%d tampered=%d member_inviter=%d c_rows=%d unbound=%d unbound_delta=%d stranger=%d stranger_c_rows=%d b_active=%t",
		bindA.Status, created.Status, bindB.Status, bindC.Status, deniedBefore.Status,
		first.Status, replay.Status, replay.Body == first.Body, deniedWhileInvited.Status, rowStatus == "invited",
		accepted.Status, listB.Status, roleOK, selected.Status, missingKey.Status, ownerRole.Status,
		missingAuth.Status, tampered.Status, memberInvites.Status, cRows, unbound.Status,
		invitedAfter-invitedBefore, strangerAccept.Status, cRowsAfter, bStatus == "active")
	return platformtest.Report{Passed: passed, Summary: summary, Assertions: ordered}, nil
}

// t05JourneyFromInputs reads the scenario inputs with t05-prefixed
// defaults so the T05 identifiers never collide with the T01-T04
// journey fixtures.
func t05JourneyFromInputs(inputs map[string]any) t05Journey {
	return t05Journey{
		t04Journey: t04Journey{
			t03Journey: t03Journey{
				journey: journey{
					issuer:       inputString(inputs, "issuer", "http://casdoor:8000"),
					apiBase:      inputString(inputs, "casdoor_api_base", "http://localhost:8000"),
					callbackBase: inputString(inputs, "callback_base", "http://localhost:8080"),
					callbackPath: inputString(inputs, "callback_path", "/internal/v1/identity/callback"),
					org:          inputString(inputs, "casdoor_organization", "t05-accept-org"),
					appName:      inputString(inputs, "casdoor_application_name", "t05-acceptance-app"),
					clientID:     inputString(inputs, "casdoor_client_id", "t05-acceptance"),
					username:     inputString(inputs, "casdoor_username_a", "t05-invite-a"),
					replays:      inputInt(inputs, "replay_deliveries", 1),
					pgHost:       inputString(inputs, "postgres_host", "localhost"),
					pgPort:       inputString(inputs, "postgres_port", "5432"),
					pgDatabase:   inputString(inputs, "postgres_database", "platform"),
					pgUser:       inputString(inputs, "postgres_user", "platform"),
				},
				usernameB: inputString(inputs, "casdoor_username_b", "t05-invite-b"),
			},
			displayName:  inputString(inputs, "display_name", "t05-accept-team"),
			idempotencyA: inputString(inputs, "idempotency_key_tenant", "t05-key-tenant"),
			idempotencyB: inputString(inputs, "idempotency_key_invite", "t05-invite-key"),
		},
		usernameC:         inputString(inputs, "casdoor_username_c", "t05-invite-c"),
		invitedRole:       inputString(inputs, "invited_role", "member"),
		inviteeKey:        inputString(inputs, "idempotency_key_invite", "t05-invite-key"),
		neverBoundSubject: inputString(inputs, "never_bound_subject", "t05-never-bound-subject"),
	}
}

// t05PrecheckStack verifies both stack entry points answer before
// anything else runs, failing with the T05 startup command (which
// carries the audience override) in the message.
func (j t05Journey) t05PrecheckStack(ctx context.Context) error {
	casdoorURL := strings.TrimSuffix(j.apiBase, "/") + "/api/health"
	if err := probe(ctx, casdoorURL); err != nil {
		return fmt.Errorf("casdoor unreachable at %s (%v); start the stack first: %s", casdoorURL, err, t05StackStartupCommand)
	}
	livezURL := strings.TrimSuffix(j.callbackBase, "/") + "/livez"
	if err := probe(ctx, livezURL); err != nil {
		return fmt.Errorf("platform-api unreachable at %s (%v); start the stack first: %s", livezURL, err, t05StackStartupCommand)
	}
	return nil
}

// t05Invite issues one membership-invitation POST with a JSON body, an
// optional Idempotency-Key header (empty sends none), and an optional
// Authorization header.
func t05Invite(ctx context.Context, client *http.Client, path, body, idempotencyKey, authorization string) t03APIResponse {
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

// t05Accept issues one invitation-acceptance POST with an optional
// Authorization header and an empty body.
func t05Accept(ctx context.Context, client *http.Client, path, authorization string) t03APIResponse {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, path, nil)
	if err != nil {
		return t03APIResponse{Status: -1, Body: fmt.Sprintf("build request: %v", err)}
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

// t05InvitationBody decodes the invitation response body.
type t05InvitationBody struct {
	TenantID  string `json:"tenant_id"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	InvitedAt string `json:"invited_at"`
}

// t05ActivationBody decodes the acceptance response body.
type t05ActivationBody struct {
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

// t05ParseInvitation decodes an invitation body (zero value on any
// decode failure).
func t05ParseInvitation(body string) t05InvitationBody {
	var payload t05InvitationBody
	_ = json.Unmarshal([]byte(body), &payload)
	return payload
}

// t05ParseActivation decodes an acceptance body (zero value on any
// decode failure).
func t05ParseActivation(body string) t05ActivationBody {
	var payload t05ActivationBody
	_ = json.Unmarshal([]byte(body), &payload)
	return payload
}

// t05InvitedAtParses reports whether the invited_at field of an
// invitation body parses as RFC3339.
func t05InvitedAtParses(body string) bool {
	payload := t05ParseInvitation(body)
	if payload.InvitedAt == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339, payload.InvitedAt)
	return err == nil
}

// t05MembershipStatus reads the status of one tenant membership
// straight from the platform database.
func (j t05Journey) t05MembershipStatus(ctx context.Context, tenantID, userID string) (string, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var status string
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM memberships
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		tenantID, userID).Scan(&status); err != nil {
		return "", err
	}
	return status, nil
}

// t05CountTenantMemberships counts the membership rows of one tenant
// restricted to one user.
func (j t05Journey) t05CountTenantMemberships(ctx context.Context, tenantID, userID string) (int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var rows int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM memberships
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		tenantID, userID).Scan(&rows); err != nil {
		return 0, err
	}
	return rows, nil
}

// t05CountInvitedRows counts the pending invitation rows of one tenant.
func (j t05Journey) t05CountInvitedRows(ctx context.Context, tenantID string) (int, error) {
	db, err := j.openPlatformDB(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var rows int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM memberships
		WHERE tenant_id = $1::uuid AND status = 'invited'`,
		tenantID).Scan(&rows); err != nil {
		return 0, err
	}
	return rows, nil
}

// t05FailedReport builds a failing report whose assertion set matches
// the declared T05 names (all failed), keeping the harness
// reconciliation valid.
func t05FailedReport(reason string) platformtest.Report {
	results := make([]platformtest.AssertionResult, 0, len(t05AssertionNames))
	for _, name := range t05AssertionNames {
		results = append(results, platformtest.AssertionResult{Name: name, Passed: false})
	}
	return platformtest.Report{Passed: false, Summary: reason, Assertions: results}
}
