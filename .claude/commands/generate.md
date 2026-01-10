<!--
  Source: codex_claude_setup
  Original Path: commands/generate/COMMAND.md
  Adaptations: Minor formatting for consistency
-->
---
description: Regenerate all code from contracts and SQLC
argument-hint: (no arguments)
---

# Code Generation

Runs the single supported generation entrypoint for Continua.

## Context
- **Source of truth:**
  - `contracts/openapi/openapi.yaml` - API contract
  - `contracts/websocket/events.ts` - WebSocket events
  - `db/platform/migrations/postgres/` - SQLC schema
  - `db/platform/queries/` - SQLC queries

- **Generated outputs:**
  - `contracts/generated/go/server_gen.go`
  - `internal/api/server_gen.go` (copied from contracts)
  - `contracts/generated/typescript/api.ts`
  - `db/gen/go/platform/*`
  - `sdks/python/src/continua/types.py`

- CI expects `make generate` to be the only generation entrypoint.

## Process

1. Verify repo root (presence of `Makefile`)
2. Run generation:
   ```bash
   make generate
   ```
3. Optionally verify no drift:
   ```bash
   ./scripts/check-generated.sh
   ```
4. Report generated files that changed (if any)

## Examples
- `/project:generate`

## Anti-patterns
- Editing generated files by hand (`*_gen.go`, `contracts/generated/*`, `db/gen/*`)
- Running component generators directly instead of `make generate`
- Forgetting to commit generated file changes
