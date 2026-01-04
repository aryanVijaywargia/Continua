#!/bin/bash
set -e

docker compose -f deploy/docker-compose/docker-compose.dev.yml up -d postgres

echo "✅ Database started"
echo ""
echo "Now run:"
echo "  make dev-server"
echo "  make dev-web"
