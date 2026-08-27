-- +goose Up
-- T03 tenant context selections: one row per platform user, the
-- explicitly selected tenant and when the selection was persisted or
-- last switched. The client-submitted tenant id is only a selection
-- request: the server validates an active membership before persisting
-- (in the repository's write transaction) and re-validates on every
-- read (a JOIN against active memberships), so a revocation invalidates
-- a persisted selection without deleting the row.
CREATE TABLE tenant_context_selections (
  platform_user_id uuid PRIMARY KEY REFERENCES platform_users(id) ON DELETE CASCADE,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  selected_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE tenant_context_selections;
