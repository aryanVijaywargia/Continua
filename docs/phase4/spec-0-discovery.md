> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Phase 4 Discovery

## Scope Reviewed

This discovery pass documents the existing implementation state that Phase 4 builds on.

Reviewed areas:

- `span_events` schema and queries
- `spans` lifecycle fields and ordering
- API handler and mapper patterns
- project scoping and auth context
- Python SDK span helper surface
- trace detail web UI structure

## Data Model Baseline

### `span_events`

Source: `db/platform/migrations/postgres/000001_initial_schema.up.sql`

Current columns relevant to the timeline:

- `id`
- `project_id`
- `trace_id`
- `span_id`
- `event_type`
- `level`
- `event_ts`
- `server_ingested_at`
- `sequence`
- `message`
- `payload`
- `idempotency_key`
- `created_at`

Important characteristics:

- append-only table
- `span_id` is required
- there is no foreign key to `spans`
- orphan events are allowed by design for out-of-order ingestion
- `created_at` and `server_ingested_at` are both available

Indexes already present:

- `idx_span_events_trace`
- `idx_span_events_trace_span`
- `idx_span_events_project`
- `idx_span_events_event_ts`

### `spans`

Source: `db/gen/go/platform/models.go`

Current lifecycle and ordering fields relevant to the timeline:

- `span_id`
- `name`
- `status`
- `start_time`
- `end_time`
- `server_received_at`
- `sequence`
- `created_at`

Important behavior:

- `start_time` is a concrete `time.Time`
- `end_time` is nullable
- upsert logic preserves earliest `start_time` and latest `end_time`
- status protection prevents downgrading a failed span

## Query Baseline

### Event queries

Source: `db/platform/queries/events.sql`

Current trace timeline inputs:

- `ListSpanEventsByTrace`
  - `SELECT * FROM span_events WHERE trace_id = $1`
  - ordered by `COALESCE(event_ts, server_ingested_at) ASC, sequence NULLS LAST`
- `ListOrphanEvents`
  - explicit support already exists for discovering orphan events

### Span queries

Source: `db/platform/queries/spans.sql`

Current trace span input:

- `ListSpansByTrace`
  - `SELECT * FROM spans WHERE trace_id = $1`
  - ordered by `COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST`

## Store Layer Baseline

Sources:

- `internal/store/span_events.go`
- `internal/store/spans.go`
- `internal/store/traces.go`

Relevant store methods already exist and are thin wrappers over SQLC:

- `GetTrace`
- `ListSpanEventsByTrace`
- `ListSpansByTrace`
- `ListOrphanEvents`

There is no existing timeline-specific store abstraction.

## API Handler Baseline

Source: `internal/api/server.go`

Current handler pattern:

1. read `project_id` from auth context via `middleware.GetProjectID`
2. fetch the trace by internal UUID
3. return `404` if missing
4. verify the trace belongs to the authenticated project
5. return `404` again for cross-project access
6. fetch related resources
7. map DB rows to API types in `mapper.go`
8. encode JSON directly with `writeJSON`

Current trace detail endpoints:

- `GET /api/traces/{id}`
- `GET /api/traces/{id}/spans`

There is no `GET /api/traces/{id}/events` endpoint yet.

## Mapper Baseline

Source: `internal/api/mapper.go`

Current mapper behavior:

- `traceToAPI` maps DB trace status to API status using `mapTraceStatus`
- `spanToAPI` maps DB span type/status to API enums using `mapSpanKind` and `mapSpanStatus`
- JSON metadata and payload fields are unmarshaled in mapper helpers
- numeric values are converted through `numericToFloat32`

There are no timeline mapping helpers yet.

## Project Scoping Baseline

Sources:

- `internal/api/middleware/auth.go`
- `internal/api/server.go`

Current multi-tenant pattern:

- auth middleware injects `project_id` into request context
- handlers fetch by internal UUID, then verify `trace.ProjectID == projectID`
- on mismatch, handlers intentionally return `404` instead of `403`

This is the pattern the timeline endpoint should follow.

## Web UI Baseline

Sources:

- `web/src/pages/TraceDetailPage.tsx`
- `web/src/components/SpanTree.tsx`
- `web/src/components/SpanDetail.tsx`
- `web/src/api/client.ts`

Current trace detail UI:

- two-panel layout
- left: span tree
- right: selected span detail
- no tabs or timeline section
- no polling on the trace detail page

Current client support:

- manual types for `Trace`, `Span`, `Session`
- fetch helpers for traces, spans, and sessions
- no timeline types or fetch function

## Python SDK Baseline

Sources:

- `sdks/python/src/continua/span.py`
- `sdks/python/tests/test_span.py`
- `sdks/python/tests/test_errors.py`

Current span helper surface:

- `log(message, level="info", payload=None)`
- `set_error(message)`
- `set_llm_response(...)`
- `set_tool_call(...)`

Current event behavior:

- `log()` queues explicit span events through `Continua.get_instance().add_event(...)`
- event payload includes `trace_id`, `span_id`, `event_type="log"`, `level`, `message`, and optional `payload`
- there are no `error()`, `exception()`, or `metric()` helper methods yet

## Testing Baseline

Relevant existing patterns:

- Go integration-style tests use `internal/testutil.TestDB`
- API utility tests already exist in `internal/api/server_pagination_test.go`
- multi-tenant behavior is covered in `internal/api/middleware/auth_test.go`
- Python SDK behavior is covered in `sdks/python/tests/test_span.py` and `sdks/python/tests/test_errors.py`
- the web package has `type-check` but no existing vitest setup

## Implementation Constraints Confirmed

- OpenAPI is the REST source of truth
- generated Go and TypeScript API types must be refreshed via `make generate`
- Python SDK types are generated from `contracts/openapi/openapi.bundle.yaml`
- manual web client types still need to stay aligned with the contract
- no database migration is required for Phase 4
