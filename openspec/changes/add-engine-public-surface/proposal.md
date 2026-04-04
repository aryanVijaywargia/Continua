# Change: Add Public Engine Surface With Shared-DB Projection

## Why

The engine runtime (Phase 11) runs end-to-end internally but has no external surface. Users need a REST API to start, inspect, signal, and cancel engine workflows, and the existing debugger needs to display engine-backed traces alongside traditional ingest traces without requiring a separate UI.

## What Changes

- Platform `traces` table gains engine linkage columns and a projection state machine
- `continua-engine` publishes a DB-backed definition catalog and a small public history DTO surface so `cmd/continua` can validate and append start history without crossing `engine/internal/*` boundaries
- `cmd/continua` hosts new `/v1/engine/*` REST routes behind feature-flag and preview-header gating
- `continua-engine serve` gains a fourth `projector` loop that writes engine execution state into `public.*` tables
- Existing debugger read schemas extend with a nested `engine` object where it materially helps
- Trace list, trace detail, session detail, and compare flows surface engine metadata and projection state
- Root-side engine control code imports the public `engine/db/gen/go` query package (never `engine/internal/*`)

Ships in three slices:
- **12a**: Hardening, root-side import boundaries, trace linkage columns, projection state contract
- **12b**: Public engine REST API under `/v1/engine/*`
- **12c**: Async projector loop, writer ownership, debugger read-model integration

## Impact

- Affected specs: engine-trace-projection (new), engine-public-api (new), engine-projector-runtime (new), engine-debugger-integration (new)
- Affected code:
  - `db/platform/migrations/postgres/` — new migration for trace linkage columns
  - `db/platform/queries/` — updated trace queries for engine fields
  - `engine/db/migrations/postgres/` — definition catalog migration
  - `engine/db/queries/` — extended engine queries for root-side control
  - `engine/pkg/history/` (or equivalent public engine package) — shared event constants and payload DTOs for the cross-binary start/read boundary
  - `engine/internal/` — new projector package
  - `engine/cmd/continua-engine/` — fourth worker loop
  - `internal/api/` — engine route handlers, engine read helpers
  - `contracts/openapi/openapi.yaml` — engine API routes and extended trace schemas
  - `web/src/` — engine badges, projection-state banners, wait-state summaries
- Non-engine traces, sessions, and existing ingest flows remain unchanged
- No breaking changes to existing API contracts
