-- +goose Up
-- T04 business tenants: an authenticated user explicitly creates a
-- business tenant and becomes its owner. The display name is required
-- for business tenants (the service rejects empty names; this CHECK is
-- the database-level backstop) while personal tenants keep it NULL.
ALTER TABLE tenants ADD COLUMN display_name text;
ALTER TABLE tenants ADD CONSTRAINT tenants_business_display_name_required
  CHECK (kind = 'personal' OR display_name IS NOT NULL);

-- Replay mapping for idempotent creation: one row per (actor, retry
-- key). A retried delivery with the same key converges onto the same
-- tenant instead of provisioning a second one.
CREATE TABLE business_tenant_creations (
  actor_user_id uuid NOT NULL REFERENCES platform_users(id),
  idempotency_key text NOT NULL,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (actor_user_id, idempotency_key)
);

-- +goose Down
DROP TABLE business_tenant_creations;
ALTER TABLE tenants DROP CONSTRAINT tenants_business_display_name_required;
ALTER TABLE tenants DROP COLUMN display_name;
