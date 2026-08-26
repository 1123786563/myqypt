-- +goose Up
CREATE TABLE platform_users (
  id uuid PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE identity_bindings (
  identity_provider text NOT NULL,
  subject text NOT NULL,
  platform_user_id uuid NOT NULL REFERENCES platform_users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (identity_provider, subject),
  UNIQUE (platform_user_id, identity_provider)
);

-- +goose Down
DROP TABLE identity_bindings;
DROP TABLE platform_users;
