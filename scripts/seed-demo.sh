#!/usr/bin/env bash

set -euo pipefail

if ! command -v psql >/dev/null 2>&1; then
  echo "psql is required to seed the public demo project." >&2
  exit 1
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL must be set before running the demo seed." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_URL="${CONTINUA_API_URL:-${CONTINUA_ENDPOINT:-http://localhost:8080}}"
DEMO_PROJECT_ID="${CONTINUA_DEMO_PROJECT_ID:-${PUBLIC_DEMO_PROJECT_ID:-11111111-1111-4111-8111-111111111111}}"
DEMO_PROJECT_NAME="${CONTINUA_DEMO_PROJECT_NAME:-Public Demo Project}"
DEMO_RUN_ID="${CONTINUA_DEMO_RUN_ID:-public-demo}"

if [[ -n "${CONTINUA_DEMO_API_KEY:-}" ]]; then
  DEMO_API_KEY="${CONTINUA_DEMO_API_KEY}"
elif command -v openssl >/dev/null 2>&1; then
  DEMO_API_KEY="$(openssl rand -hex 32)"
elif command -v uuidgen >/dev/null 2>&1; then
  DEMO_API_KEY="$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')"
else
  echo "Set CONTINUA_DEMO_API_KEY or install openssl/uuidgen so the seed can create a temporary project key." >&2
  exit 1
fi

DEMO_API_KEY_HASH="$(printf '%s' "${DEMO_API_KEY}" | shasum -a 256 | awk '{print $1}')"

echo "==> Resetting public demo project ${DEMO_PROJECT_ID}"
psql "${DATABASE_URL}" \
  -v ON_ERROR_STOP=1 \
  -v demo_project_id="${DEMO_PROJECT_ID}" \
  -v demo_project_name="${DEMO_PROJECT_NAME}" \
  -v demo_api_key_hash="${DEMO_API_KEY_HASH}" <<'SQL'
DELETE FROM projects
WHERE id = :'demo_project_id'::uuid;

INSERT INTO projects (id, name, api_key_hash)
VALUES (
  :'demo_project_id'::uuid,
  :'demo_project_name',
  :'demo_api_key_hash'
);
SQL

echo "==> Seeding demo traces through the ingest API at ${API_URL}"
(
  cd "${ROOT_DIR}/sdks/python"
  CONTINUA_DEMO_PROJECT_ID="${DEMO_PROJECT_ID}" \
  CONTINUA_API_URL="${API_URL}" \
  CONTINUA_API_KEY="${DEMO_API_KEY}" \
  CONTINUA_PRINT_API_KEY=0 \
  CONTINUA_DEMO_RUN_ID="${DEMO_RUN_ID}" \
  uv run python examples/e2e_demo.py
)

cat <<EOF
==> Demo seed complete
Project ID: ${DEMO_PROJECT_ID}
Project name: ${DEMO_PROJECT_NAME}
API URL: ${API_URL}

If you are running the public portfolio demo, start the server with:
  PUBLIC_DEMO_ENABLED=1
  PUBLIC_DEMO_PROJECT_ID=${DEMO_PROJECT_ID}
  PUBLIC_DEMO_LABEL="Sample data"
EOF
