-- +goose Up
CREATE TABLE schema_health (
    id boolean PRIMARY KEY,
    applied_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE schema_health;
