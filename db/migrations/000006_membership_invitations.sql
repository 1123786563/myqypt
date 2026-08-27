-- +goose Up
-- T05 membership invitations: the membership row IS the invitation
-- record (implementation plan, design ruling 1). The status vocabulary
-- extends from ('active','revoked') to ('invited','active','revoked'):
-- an invitation is a membership row with status='invited', acceptance
-- is the single-row transition invited -> active, and discovery of
-- pending invitations is out of scope (T03's active-only JOIN keeps an
-- invited-not-accepted membership invisible at the tenant-context seam
-- by construction). Every existing constraint stays byte-identical:
-- UNIQUE(tenant_id,user_id), the partial one-active-owner index, and
-- both foreign keys. No new table: the natural key already gives replay
-- convergence on (tenant, invitee).
ALTER TABLE memberships DROP CONSTRAINT memberships_status_check;
ALTER TABLE memberships ADD CONSTRAINT memberships_status_check
  CHECK (status IN ('invited', 'active', 'revoked'));

-- +goose Down
-- Reverses the CHECK to the pre-T05 vocabulary. Fails if any invited
-- rows survive (narrowing a CHECK validates existing rows), which is
-- the intended guard: an operator must resolve pending invitations
-- before rolling this migration back.
ALTER TABLE memberships DROP CONSTRAINT memberships_status_check;
ALTER TABLE memberships ADD CONSTRAINT memberships_status_check
  CHECK (status IN ('active', 'revoked'));
