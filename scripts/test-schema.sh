#!/usr/bin/env bash
# Applies the real migrations to a fresh database, then runs contracts/schema_test.sql
# (RLS and constraint tests). Needs psql and a Postgres where TEST_DATABASE_URL can
# create databases (default: postgres/postgres on localhost). The URL must end in
# /<database> with an optional ?query.
set -euo pipefail

admin_url="${TEST_DATABASE_URL:-postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable}"
name="gp_schema_test_$$"
owner_url="$(printf '%s' "$admin_url" | sed -E "s#/[^/?]+(\?|$)#/${name}\1#")"
root="$(cd "$(dirname "$0")/.." && pwd)"

cleanup() { psql "$admin_url" -qc "DROP DATABASE IF EXISTS ${name} WITH (FORCE)" >/dev/null 2>&1 || true; }
trap cleanup EXIT

psql "$admin_url" -qv ON_ERROR_STOP=1 -c "CREATE DATABASE ${name}"
(cd "$root/backend" && go run ./cmd/migrate -url "$owner_url" up)

# The last line of the test file is "passed | failed".
result="$(psql "$owner_url" -qtA -F' ' -v ON_ERROR_STOP=1 -f "$root/contracts/schema_test.sql" | tail -n 1)"
echo "schema_test.sql: passed/failed = ${result}"
[ "$result" = "30 0" ] || { echo "expected 30 passed and 0 failed" >&2; exit 1; }
