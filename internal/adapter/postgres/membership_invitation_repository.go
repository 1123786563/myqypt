package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/1123786563/myqypt/internal/application/tenancy"
	"github.com/jackc/pgx/v5"
)

// InviteMember delivers one membership invitation (Issue #6, T05).
//
// The whole operation runs in one transaction: the inviter's verified
// identity is resolved to its platform user first — a never-bound
// inviter is rejected with ErrUserNotBound before any further read —
// then the invitee is addressed by its external subject and resolved
// through identity_bindings by (issuer, subject) (design ruling 3); a
// never-bound subject is likewise ErrUserNotBound (404, no existence
// oracle, zero writes). A transaction-scoped advisory lock on
// hashtextextended(tenant || ':' || invitee, 0) serializes concurrent
// deliveries of the same (tenant, invitee) pair (design ruling 2),
// then the authorization check and the write run under it: the inviter
// must hold an active membership with role owner or admin in the
// tenant (design ruling 5) — anything else (member, billing_member,
// non-member, revoked, or a nonexistent tenant) is
// ErrInviterNotAuthorized, the same 404, with zero rows written.
//
// The membership row IS the invitation record (design ruling 1): an
// existing (tenant, invitee) row with status='invited' is the replay
// path (created=false, the same facts); an existing row in any other
// state (active or revoked) classifies as ErrInvitationNotFound —
// indistinguishable from no row; a miss inserts the single row with
// status='invited' and answers created=true. The idempotency key never
// reaches this port: the transport enforces its presence (400 before
// any write) and convergence rides the natural key UNIQUE(tenant_id,
// user_id).
func (r *TenancyRepository) InviteMember(ctx context.Context, verified identity.VerifiedIdentity, tenantID, inviteeSubject, role string) (tenancy.MembershipInvitation, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tenancy.MembershipInvitation{}, false, fmt.Errorf("postgres: begin membership invitation: %w", err)
	}
	defer tx.Rollback(ctx)

	inviterID, err := r.boundUserID(ctx, tx, verified)
	if err != nil {
		return tenancy.MembershipInvitation{}, false, err
	}
	if inviterID == "" {
		return tenancy.MembershipInvitation{}, false, tenancy.ErrUserNotBound
	}

	inviteeID, err := r.boundUserID(ctx, tx, identity.VerifiedIdentity{
		Issuer:  verified.Issuer,
		Subject: inviteeSubject,
	})
	if err != nil {
		return tenancy.MembershipInvitation{}, false, err
	}
	if inviteeID == "" {
		return tenancy.MembershipInvitation{}, false, tenancy.ErrUserNotBound
	}

	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenantID+":"+inviteeID); err != nil {
		return tenancy.MembershipInvitation{}, false, fmt.Errorf("postgres: lock membership invitation: %w", err)
	}

	var inviterAuthorized bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM memberships
			WHERE tenant_id = $1::uuid AND user_id = $2::uuid
			  AND role IN ('owner', 'admin') AND status = 'active'
		)`,
		tenantID, inviterID).Scan(&inviterAuthorized); err != nil {
		return tenancy.MembershipInvitation{}, false, fmt.Errorf("postgres: validate inviter membership: %w", err)
	}
	if !inviterAuthorized {
		return tenancy.MembershipInvitation{}, false, tenancy.ErrInviterNotAuthorized
	}

	var invitation tenancy.MembershipInvitation
	err = tx.QueryRow(ctx, `
		SELECT role, status, created_at
		FROM memberships
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		tenantID, inviteeID).Scan(&invitation.Role, &invitation.Status, &invitation.InvitedAt)
	switch {
	case err == nil:
		if invitation.Status != "invited" {
			return tenancy.MembershipInvitation{}, false, tenancy.ErrInvitationNotFound
		}
		invitation.TenantID = tenantID
		return invitation, false, tx.Commit(ctx)
	case !errors.Is(err, pgx.ErrNoRows):
		return tenancy.MembershipInvitation{}, false, fmt.Errorf("postgres: load membership invitation: %w", err)
	}

	if err := tx.QueryRow(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role, status)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3, 'invited')
		RETURNING role, status, created_at`,
		tenantID, inviteeID, role).Scan(&invitation.Role, &invitation.Status, &invitation.InvitedAt); err != nil {
		return tenancy.MembershipInvitation{}, false, fmt.Errorf("postgres: insert membership invitation: %w", err)
	}
	invitation.TenantID = tenantID

	if err := tx.Commit(ctx); err != nil {
		return tenancy.MembershipInvitation{}, false, fmt.Errorf("postgres: commit membership invitation: %w", err)
	}
	return invitation, true, nil
}

// AcceptInvitation delivers the invitee-only acceptance of the pending
// invitation of tenantID (design ruling 6). One transaction: the
// verified identity is resolved to its platform user — a never-bound
// invitee is ErrUserNotBound (404, no oracle) — a transaction-scoped
// advisory lock on hashtextextended(tenant || ':' || invitee, 0)
// serializes concurrent acceptances of the same row, then the row is
// matched: status='invited' is the single-row transition to active;
// status='active' is the converged replay (the same activation facts,
// no second transition); anything else — no row, or a revoked row — is
// ErrInvitationNotFound with zero writes. Someone else's row cannot be
// reached: the lookup is keyed by the invitee's own resolved user.
func (r *TenancyRepository) AcceptInvitation(ctx context.Context, verified identity.VerifiedIdentity, tenantID string) (tenancy.ActivatedMembership, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tenancy.ActivatedMembership{}, fmt.Errorf("postgres: begin membership invitation acceptance: %w", err)
	}
	defer tx.Rollback(ctx)

	inviteeID, err := r.boundUserID(ctx, tx, verified)
	if err != nil {
		return tenancy.ActivatedMembership{}, err
	}
	if inviteeID == "" {
		return tenancy.ActivatedMembership{}, tenancy.ErrUserNotBound
	}

	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenantID+":"+inviteeID); err != nil {
		return tenancy.ActivatedMembership{}, fmt.Errorf("postgres: lock membership invitation acceptance: %w", err)
	}

	var membership tenancy.ActivatedMembership
	err = tx.QueryRow(ctx, `
		SELECT role, status
		FROM memberships
		WHERE tenant_id = $1::uuid AND user_id = $2::uuid`,
		tenantID, inviteeID).Scan(&membership.Role, &membership.Status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return tenancy.ActivatedMembership{}, tenancy.ErrInvitationNotFound
	case err != nil:
		return tenancy.ActivatedMembership{}, fmt.Errorf("postgres: load membership invitation for acceptance: %w", err)
	}
	membership.TenantID = tenantID

	switch membership.Status {
	case "active":
		// Converged replay (design ruling 6): the invitation was already
		// accepted by this same invitee; answer the same activation facts
		// without a second transition.
		return membership, tx.Commit(ctx)
	case "invited":
		if _, err := tx.Exec(ctx, `
			UPDATE memberships SET status = 'active'
			WHERE tenant_id = $1::uuid AND user_id = $2::uuid AND status = 'invited'`,
			tenantID, inviteeID); err != nil {
			return tenancy.ActivatedMembership{}, fmt.Errorf("postgres: activate membership: %w", err)
		}
		membership.Status = "active"
		if err := tx.Commit(ctx); err != nil {
			return tenancy.ActivatedMembership{}, fmt.Errorf("postgres: commit membership invitation acceptance: %w", err)
		}
		return membership, nil
	default:
		// Revoked or any other state: indistinguishable from no invitation.
		return tenancy.ActivatedMembership{}, tenancy.ErrInvitationNotFound
	}
}
