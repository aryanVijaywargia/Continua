#!/bin/bash
set -e

echo "==> Generating all code..."

pnpm --filter @continua/contracts generate

cd db/platform && sqlc generate && cd ../..

cd engine/db && sqlc generate && cd ../..

mkdir -p internal/api
if [ -f contracts/generated/go/server_gen.go ]; then
    cp contracts/generated/go/server_gen.go internal/api/server_gen.go
fi

if [ -f contracts/openapi/openapi.bundle.yaml ]; then
    cd sdks/python && uv run --with datamodel-code-generator python scripts/generate_types.py && cd ../..
fi

echo "✅ All code generated"
