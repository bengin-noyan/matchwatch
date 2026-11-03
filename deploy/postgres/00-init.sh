#!/bin/bash
set -euo pipefail

psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres \
  -v owner_pw="$MATCHWATCH_OWNER_PASSWORD" -v app_pw="$MATCHWATCH_APP_PASSWORD" <<'SQL'
CREATE ROLE matchwatch_owner LOGIN PASSWORD :'owner_pw';
CREATE ROLE matchwatch_app LOGIN PASSWORD :'app_pw' NOBYPASSRLS;
CREATE DATABASE matchwatch OWNER matchwatch_owner;
SQL

psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d matchwatch <<'SQL'
CREATE EXTENSION vector;
GRANT USAGE ON SCHEMA public TO matchwatch_app;
SQL
