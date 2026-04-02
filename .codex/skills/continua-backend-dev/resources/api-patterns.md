# API patterns

## Current endpoint families
- ingest: `POST /v1/ingest`, `GET /v1/ingest/batches/{id}`
- traces: list, detail, spans, events
- sessions: list, detail, compare
- health: `GET /api/health` is routed directly in Chi and stays outside OpenAPI auth handling

## Contract-first flow
1. Edit `contracts/openapi/openapi.yaml`
2. Run `make generate`
3. Implement the generated interface in `internal/api`
4. Update mappers and tests

Do not add handler-only fields without first updating OpenAPI.

## Current `internal/api` patterns

### Router
- public health route
- protected OpenAPI-generated routes under API-key auth
- embedded SPA fallback at `/`

### Handler shape
- get `project_id` from middleware context with `projectIDOrUnauthorized`
- normalize query params with helpers in `server_helpers.go`
- call store or ingest service
- map store models to API types
- return spec-compliant errors with `writeError`

### Mapper rules
- `traceToAPI`, `traceDetailToAPI`, `spanToAPI`, `sessionToAPI`
- map lower-case DB statuses/types to public API enums
- parse JSON bytes into API payloads in the mapper layer
- never return sqlc structs directly

## Trace and timeline specifics
- trace list uses `ListTracesFiltered` in `internal/store/search.go` when filters are present
- trace detail and spans are project-scoped
- timeline API merges:
  - explicit `span_events`
  - synthetic span lifecycle events derived from spans
- timeline pagination is cursor-based and implemented in `internal/api/timeline.go`

## Session compare specifics
- compare is exposed at `GET /api/sessions/{id}/compare`
- requests are project-scoped and validate that both traces belong to the session
- both traces must be terminal before compare runs
- large comparisons fail with a structured `422` detail rather than timing out or streaming partial results

## Error and auth conventions
- missing/invalid API key -> `401`
- project-scoped misses -> `404`
- validation errors for ingest -> `400`
- unsupported async-version header -> `400 unsupported_async_version`
- invalid session compare request -> `400 invalid_compare_request`

## Good boundaries
- auth logic in middleware
- request normalization in `server_helpers.go`
- feature logic in feature handler files
- state translation in mappers
- business persistence in store/service layers
