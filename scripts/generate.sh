#!/bin/bash
set -e

echo "==> Generating all code..."

pnpm --filter @continua/contracts generate

cd db/platform && sqlc generate && cd ../..

if [ -d "engine/db/queries" ] && [ "$(ls -A engine/db/queries 2>/dev/null | grep -v .gitkeep)" ]; then
    cd engine/db && sqlc generate && cd ../..
fi

mkdir -p internal/api
if [ -f contracts/generated/go/server_gen.go ]; then
    cp contracts/generated/go/server_gen.go internal/api/server_gen.go
fi

if [ -f contracts/openapi/openapi.bundle.yaml ]; then
    cd sdks/python && uv run python scripts/generate_types.py && cd ../..
fi

echo "✅ All code generated"
