## 1. Token Rollup Alignment

- [x] 1.1 Create migration `000008_split_trace_tokens.up.sql`: ADD `total_tokens_in BIGINT DEFAULT 0` and `total_tokens_out BIGINT DEFAULT 0` to traces; backfill from spans `SUM(prompt_tokens)` and `SUM(completion_tokens)`; DROP `total_tokens`
- [x] 1.2 Create migration `000008_split_trace_tokens.down.sql`: ADD `total_tokens`; backfill as `total_tokens_in + total_tokens_out`; DROP the two new columns
- [x] 1.3 Update `db/platform/queries/rollups.sql`: `ComputeTraceRollups` returns `total_tokens_in` (from `SUM(prompt_tokens)`) and `total_tokens_out` (from `SUM(completion_tokens)`) separately
- [x] 1.4 Update `db/platform/queries/traces.sql`: `UpdateTraceRollups` sets `total_tokens_in` and `total_tokens_out` instead of `total_tokens`
- [x] 1.5 Run `make generate` to regenerate sqlc types
- [x] 1.6 Update `internal/store/rollups.go`: `TraceRollups` struct and `ComputeAndUpdateTraceRollups` to use split token fields
- [x] 1.7 Update `internal/domain/trace.go`: Replace `TotalTokens *int64` with `TotalTokensIn *int64` and `TotalTokensOut *int64`
- [x] 1.8 Update `internal/api/mapper.go`: Remove halving hack (lines 40-46), map `TotalTokensIn` → `total_tokens_in` and `TotalTokensOut` → `total_tokens_out` directly from DB fields
- [x] 1.9 Enforce ingest token contract: reject `total_tokens`-only span payloads (directional fields required for supported rollups)
- [x] 1.10 Update OpenAPI ingest span schema to reflect token policy (deprecate/remove `total_tokens`; document rejection behavior)
- [x] 1.11 Run `make generate` to regenerate API and SDK types from OpenAPI
- [x] 1.12 Fix compile errors in Go/Python code caused by token contract updates
- [x] 1.13 Add tests: split-token rollup correctness, zero-span trace behavior, and explicit rejection/unsupported handling for `total_tokens`-only spans

## 2. Session External IDs

- [x] 2.1 Create migration `000009_session_external_id.up.sql`: ADD `external_id TEXT NOT NULL` to sessions; ADD unique index `idx_sessions_project_external` on `(project_id, external_id)`; ensure existing rows are backfilled safely
- [x] 2.2 Create migration `000009_session_external_id.down.sql`: DROP index; DROP column
- [x] 2.3 Add `GetOrCreateSessionByExternalID` query to `db/platform/queries/sessions.sql` using `INSERT ... ON CONFLICT (project_id, external_id) DO UPDATE SET updated_at = now() RETURNING *`
- [x] 2.4 Run `make generate` to regenerate sqlc types
- [x] 2.5 Update `internal/store/sessions.go`: Add `GetOrCreateSessionByExternalID` and `GetOrCreateSessionByExternalIDTx` methods
- [x] 2.6 Update `internal/ingest/service.go`: In `upsertTrace`, if `SessionID` is present, always treat it as an external session key and resolve via `GetOrCreateSessionByExternalID` (including UUID-looking strings)
- [x] 2.7 Update OpenAPI (`contracts/openapi/openapi.yaml`): `IngestTraceInput.session_id` becomes plain `string` (external key), and Session schema includes `external_id`
- [x] 2.8 Run `make generate` to regenerate oapi/server and SDK types
- [x] 2.9 Update API mapping paths to include `external_id` consistently (`sessionToAPI` and list/detail session pathways)
- [x] 2.10 Update Python SDK/session helpers to keep `session_id` as external string and ensure no UUID assumption in client-side validation
- [x] 2.11 Add tests: ingest accepts non-UUID session keys, session detail includes `external_id`, session list includes `external_id` and correct `trace_count`
- [x] 2.12 Run `go test ./...` and `cd sdks/python && uv run pytest -q`; fix failures

## 3. Final Validation and Cleanup

- [x] 3.1 Run `openspec validate fix-token-rollup-sessions --strict`
- [x] 3.2 Ensure OpenSpec docs and code behavior match exactly (no stale wording like backward-compatible/partial unique index)
- [x] 3.3 Exclude unrelated workspace changes from this change set before implementation PR (e.g., local tool config edits)
