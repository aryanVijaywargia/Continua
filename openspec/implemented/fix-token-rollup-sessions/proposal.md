# Change: Fix Token Rollup Alignment + Session External IDs

## Why

Three contract/data-model issues block production readiness and TypeScript SDK development:

1. **Token split hack**: The `traces` table stores a single `total_tokens` column, but the API exposes `total_tokens_in` and `total_tokens_out`. The mapper (`internal/api/mapper.go:40-46`) halves the total as a compatibility hack. Since spans already store `prompt_tokens` and `completion_tokens` separately, we have the data to compute this properly.

2. **Sessions unusable from SDKs**: Sessions exist in the DB but there's no way to create them through ingest. The SDK `session()` context manager passes a string identifier, but the ingest service currently expects UUID-shaped session IDs at the API boundary. SDK users should pass human-readable session keys that the server auto-creates and maps to internal UUIDs.

3. **Ingest token ambiguity**: Span ingest still allows `total_tokens`, but trace rollups should be computed from directional fields. Leaving both paths active creates unclear behavior and possible undercounting.

## What Changes

### Fix 1: Token Rollup Alignment
- **BREAKING** (DB schema): Replace `traces.total_tokens` with `total_tokens_in` and `total_tokens_out`
- Backfill from `SUM(prompt_tokens)` and `SUM(completion_tokens)` on spans
- Update rollup query, worker, store, domain types, and mapper
- Remove the halving hack from `mapper.go`
- **BREAKING** (ingest contract): directional token fields are the only supported rollup source; `total_tokens`-only ingest payloads are unsupported and rejected

### Fix 2: Session External IDs
- Add `external_id TEXT` column to `sessions` with a unique index on `(project_id, external_id)`
- Add `GetOrCreateSessionByExternalID` upsert query
- Update ingest service: treat all `session_id` values (including UUID-looking strings) as external keys, resolve via external ID lookup/create
- Update API spec: `IngestTraceInput.session_id` becomes a string (external key), not UUID
- Add `external_id` to Session response schema for both list and detail endpoints
- Update Python SDK to send string session keys instead of UUIDs

### Scope Rule: Pre-production cleanup
- This change set is intentionally breaking and one-shot.
- No legacy production data/backward-compatibility guarantees are required.

## Impact

- Affected specs: token-rollup (NEW), session-external-ids (NEW)
- Affected code:
  - `db/platform/migrations/postgres/` — 2 new migrations (000008, 000009)
  - `db/platform/queries/rollups.sql` — split token aggregation
  - `db/platform/queries/traces.sql` — update `UpdateTraceRollups`
  - `db/platform/queries/sessions.sql` — add `GetOrCreateSessionByExternalID`
  - `internal/store/rollups.go` — split token fields
  - `internal/store/sessions.go` — include `ExternalID` in all session mapping paths
  - `internal/domain/trace.go` — `TotalTokensIn`, `TotalTokensOut`
  - `internal/api/mapper.go` — remove halving hack, add `external_id`
  - `internal/ingest/service.go` — session resolution logic + token validation policy
  - `internal/ingest/dto.go` — ingest contract updates for session key and token policy
  - `contracts/openapi/openapi.yaml` — trace tokens, session external_id, ingest session_id type
  - `internal/api/server_gen.go`, `sdks/python/src/continua/types.py` — regenerated contract types
  - `internal/api/*_test.go`, `internal/ingest/*_test.go`, `internal/store/*_test.go` — acceptance coverage for contract behavior
  - `sdks/python/src/continua/session.py` — no UUID generation needed
