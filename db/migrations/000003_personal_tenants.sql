-- +goose Up
CREATE TABLE tenants (
  id uuid PRIMARY KEY,
  owner_user_id uuid NOT NULL REFERENCES platform_users(id),
  kind text NOT NULL CHECK (kind IN ('personal', 'business')),
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Every user owns at most one personal tenant (T02 core invariant).
CREATE UNIQUE INDEX tenants_one_personal_per_owner
  ON tenants (owner_user_id) WHERE kind = 'personal';

CREATE TABLE billing_customers (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL UNIQUE REFERENCES tenants(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  user_id uuid NOT NULL REFERENCES platform_users(id),
  role text NOT NULL CHECK (role IN ('owner', 'admin', 'billing_member', 'member')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, user_id)
);

-- Every tenant has at most one owner in place (ADR 0013: membership
-- lifecycle vocabulary is active/revoked only).
CREATE UNIQUE INDEX memberships_one_active_owner_per_tenant
  ON memberships (tenant_id) WHERE role = 'owner' AND status = 'active';

-- +goose Down
DROP TABLE memberships;
DROP TABLE billing_customers;
DROP TABLE tenants;
