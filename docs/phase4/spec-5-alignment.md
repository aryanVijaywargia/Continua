# Phase 4 Spec 5: Contract Alignment

## Generated Artifacts

Regenerated from the updated OpenAPI contract:

- `contracts/openapi/openapi.bundle.yaml`
- `contracts/generated/go/server_gen.go`
- `internal/api/server_gen.go`
- `contracts/generated/typescript/api.ts`
- `sdks/python/src/continua/types.py`

## Cross-Layer Alignment Work

Backend:

- implemented `GetTraceEvents`
- mapped backend timeline responses to generated Go contract types
- kept ingest event enums explicit-only while timeline event enums include synthetic lifecycle types

Web:

- added manual timeline client types matching the contract
- aligned `Session` manual type with `external_id`

Python:

- regenerated OpenAPI-derived `types.py`

## Verification

Commands run:

- `make generate`
- `make type-check`
- `GOCACHE=/tmp/continua-go-build make test`
- `uv run pytest -q`

Observed result:

- generation completed successfully
- TypeScript type-check passed
- Go and TypeScript tests passed
- DB-backed Go tests skipped where local Postgres was inaccessible inside the sandbox
- Python SDK tests passed
