# Continua Shared Decisions

## Contracts: Source of Truth
- REST: `Continua/contracts/openapi/openapi.yaml`
- WebSocket: `Continua/contracts/websocket/events.ts`

## Codegen Entry Points
- Preferred: `make generate`
- Contract-only: `pnpm --filter @continua/contracts generate`
- Drift check: `Continua/scripts/check-generated.sh`

## Generated Files (Do Not Edit)
- `Continua/contracts/openapi/openapi.bundle.yaml`
- `Continua/contracts/generated/typescript/api.ts`
- `Continua/contracts/generated/go/server_gen.go`
- `Continua/internal/api/server_gen.go`
- `Continua/db/gen/go/platform/*`
- `Continua/engine/db/gen/go/*`

## Database Requirements
- Platform DB maintains both Postgres and SQLite migrations.
- Engine DB changes stay within `Continua/engine`.

## Data Model Reference
- Canonical semantics: `Continua/docs/architecture/data-model.md`.

## Architectural Rules
- Review: `Continua/docs/architecture/RULES.md` and `Continua/docs/architecture/decisions/`.
