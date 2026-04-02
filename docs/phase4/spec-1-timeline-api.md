> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Phase 4 Spec 1: Timeline API

## Implemented Surface

- Added `GET /api/traces/{id}/events` to `contracts/openapi/openapi.yaml`
- Generated:
  - `contracts/openapi/openapi.bundle.yaml`
  - `contracts/generated/go/server_gen.go`
  - `internal/api/server_gen.go`
  - `contracts/generated/typescript/api.ts`
  - `sdks/python/src/continua/types.py`

## Response Model

Implemented response shape:

- `events`
- `trace_status`
- `has_more`
- `next_cursor`
- `poll_cursor`

Timeline event shape includes:

- `id`
- `trace_id`
- `event_type`
- `timestamp`
- `source`
- optional `span_id`
- optional `span_name`
- optional `level`
- optional `sequence`
- optional `message`
- optional `payload`

## Backend Implementation

Files:

- `internal/api/server.go`
- `internal/api/mapper.go`
- `internal/api/timeline.go`

Implemented behavior:

- trace ownership is validated with the same project-scoping path used by other trace endpoints
- explicit events are sourced from `span_events`
- synthetic lifecycle events are generated from `spans`
- orphan explicit events are preserved with `span_name` unset
- merged results are display-sorted by event timestamp
- opaque cursor pagination is based on monotonic `created_at` ordering
- `poll_cursor` tracks the last event included in a response, even when `has_more` is false
- invalid cursors return `400` with error code `invalid_cursor`

## Backend Tests

Added:

- `internal/api/trace_events_test.go`

Coverage added for:

- merged timeline output
- ordering rules
- cursor pagination without duplicates
- invalid cursor handling
- orphan event inclusion
- project scoping
- trace status propagation

## Verification

Commands run:

- `go test ./internal/api/...`
- `GOCACHE=/tmp/continua-go-build make test`

Observed result:

- unit and non-DB Go tests passed
- DB-backed API/store tests were skipped in this environment because localhost Postgres access was blocked by sandbox permissions
