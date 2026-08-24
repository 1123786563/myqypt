#!/bin/sh
set -eu

: "${POSTGRES_DB:?POSTGRES_DB is required}"
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${KEYCLOAK_POSTGRES_DB:?KEYCLOAK_POSTGRES_DB is required}"
: "${KEYCLOAK_POSTGRES_USER:?KEYCLOAK_POSTGRES_USER is required}"
: "${KEYCLOAK_POSTGRES_PASSWORD:?KEYCLOAK_POSTGRES_PASSWORD is required}"

psql \
  --set=ON_ERROR_STOP=1 \
  --set=keycloak_db="$KEYCLOAK_POSTGRES_DB" \
  --set=keycloak_user="$KEYCLOAK_POSTGRES_USER" \
  --set=keycloak_password="$KEYCLOAK_POSTGRES_PASSWORD" \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" <<'SQL'
DO $bootstrap$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'keycloak_user') THEN
    EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L', :'keycloak_user', :'keycloak_password');
  ELSE
    EXECUTE format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'keycloak_user', :'keycloak_password');
  END IF;
END
$bootstrap$;

SELECT format('CREATE DATABASE %I OWNER %I', :'keycloak_db', :'keycloak_user')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = :'keycloak_db') \gexec

SELECT format('GRANT ALL PRIVILEGES ON DATABASE %I TO %I', :'keycloak_db', :'keycloak_user') \gexec
SQL
