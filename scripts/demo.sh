#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/deploy/docker-compose/docker-compose.yml"

echo "==> Building and starting Continua demo services"
docker compose -f "${COMPOSE_FILE}" up -d --build postgres continua

echo "==> Seeding deterministic demo traces"
docker compose -f "${COMPOSE_FILE}" --profile seed run --rm demo-seed

cat <<'MSG'

Continua demo is running.

Open:
  http://localhost:8080

Useful checks:
  curl http://localhost:8080/api/health
  make docker-logs

Stop:
  make docker-down
MSG
