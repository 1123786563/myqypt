#!/bin/sh
set -eu

: "${POSTGRES_DB:?POSTGRES_DB is required}"
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${CASDOOR_POSTGRES_DB:?CASDOOR_POSTGRES_DB is required}"
: "${CASDOOR_POSTGRES_USER:?CASDOOR_POSTGRES_USER is required}"
: "${CASDOOR_POSTGRES_PASSWORD:?CASDOOR_POSTGRES_PASSWORD is required}"

# The credential stays in the environment and reaches the server through the
# libpq options string; the SQL below reads it via current_setting() and never
# puts it on the command line.
export PGOPTIONS="-c casdoor.role_password=$CASDOOR_POSTGRES_PASSWORD"

psql \
  --set=ON_ERROR_STOP=1 \
  --set=casdoor_db="$CASDOOR_POSTGRES_DB" \
  --set=casdoor_user="$CASDOOR_POSTGRES_USER" \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'casdoor_user', current_setting('casdoor.role_password'))
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'casdoor_user') \gexec

SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'casdoor_user', current_setting('casdoor.role_password'))
WHERE EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'casdoor_user') \gexec

SELECT format('CREATE DATABASE %I OWNER %I', :'casdoor_db', :'casdoor_user')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = :'casdoor_db') \gexec

SELECT format('GRANT ALL PRIVILEGES ON DATABASE %I TO %I', :'casdoor_db', :'casdoor_user') \gexec
SQL
