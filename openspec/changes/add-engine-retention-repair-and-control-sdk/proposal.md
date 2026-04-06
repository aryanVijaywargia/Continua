# Change: Add Engine Retention, Repair, And Control SDK

## Why

Once `add-engine-operational-hardening-core` (Proposal 1) freezes the engine statuses, history events, and control/read endpoints, operators still have no built-in way to:

- purge detail projections or engine history for old runs without hand-deleting rows
- re-run the projector against retained engine history to rebuild detail after an incident or a projection purge
- bound projection/history growth automatically via retention rules
- filter the trace list by engine-specific metadata (instance key, definition name, run status, projection state)
- drive the engine's full control surface (start/signal/cancel/terminate/purge/repair/read) from Python without rebuilding boilerplate on top of the ingest batching client

This change adds those operator tools as a second layer on top of Proposal 1: a manual purge endpoint, a per-run repair-request endpoint (scoped to catching_up recovery), an opt-in env-driven retention maintenance job, additive engine filters on the existing trace search, and a separate Python control client.

The retention and purge paths are designed to preserve a minimal operator-useful shell (trace row, root span, terminal summary, engine run id, instance key, definition name/version, projection state) even when detail spans and/or engine history are deleted, so the debugger never loses the trace's identity or terminal outcome.

## What Changes

### API surface (extends `engine-public-api`)
- Add `POST /v1/engine/runs/{run_id}/purge` with request body `{"mode": "projection_only" | "full"}`
- Add `POST /v1/engine/runs/{run_id}/repair` (per-run only)
- Purge is terminal-only; non-terminal runs return `409 Conflict` with the existing typed engine error `run_not_terminal`
- `GET /v1/engine/runs/{run_id}/history` adds an explicit `expired: true` marker for `journal_expired` runs to distinguish purged-empty from never-had-events (the endpoint already returns HTTP 200 with empty events today; this change adds the typed marker, not a new status code)
- `GET /v1/engine/runs/{run_id}/result` continues to return the retained terminal summary shell for `summary_only` and `journal_expired` runs; it does not become `404` after purge
- No new engine trace-shell columns are added in this phase
- `definition_version_mismatch` surfacing is already baseline from Proposal 1 (the run detail response already carries `definition_name`, `definition_version`, and the `failure` object); this change adds only a documentation note and a regression test — no new endpoint or field

### Purge + repair semantics (extends `engine-trace-projection` and `engine-runtime-execution`)
- `projection_only` purge deletes all `public.span_events` for the trace and all non-root `public.spans` for the trace, keeps the `public.traces` row and the root span that carries the terminal summary/result/failure payload, and sets `engine_projection_state = summary_only`
- `full` purge performs the same projection purge, then purges `engine.history` / journal rows for the run, and sets `engine_projection_state = journal_expired`
- Purge never deletes the last operator-useful shell (trace row, root span, run id, instance key, definition name/version, terminal status, projection state, terminal summary)
- Purge is allowed even when the trace is still `catching_up`; in a race, purge wins by flipping `engine_projection_state` to `summary_only` or `journal_expired` under row lock or equivalent CAS guard
- The projector treats `summary_only` and `journal_expired` as hard write barriers and does not recreate detailed projection rows after purge
- Repair is an async request that reopens projection by flipping `summary_only -> catching_up` when retained engine history exists beyond `engine_last_projected_history_id`; the separate `continua-engine` projector loop performs the actual rebuild afterward
- Repair on `summary_only` where the checkpoint already equals `engine_latest_history_id` (i.e. the trace was fully projected before purge) returns `{accepted: false, reason: "no_events_to_project"}` and preserves `summary_only` — it never "undoes" an operator's purge decision
- Repair on `journal_expired` cannot restore detail and returns `{accepted: false, reason: "history_expired"}`
- Repair on `up_to_date` is a no-op and returns `{accepted: false, reason: "already_up_to_date"}`
- Repair on `catching_up` returns `{accepted: true, reason: "already_catching_up"}` and does not enqueue duplicate work
- Single-run repair API is the only public repair surface in this phase; no bulk/internal repair driver, RPC bridge, or synchronous in-process projector entry point ships here

### Automated retention (NEW capability `engine-retention-maintenance`)
- Retention is env-only: `ENGINE_PROJECTION_RETENTION_AFTER`, `ENGINE_HISTORY_RETENTION_AFTER`
- Empty or `0` disables that retention stage
- `ENGINE_HISTORY_RETENTION_AFTER` requires `ENGINE_PROJECTION_RETENTION_AFTER` and must be greater than it
- Invalid retention config is a startup error (fail-fast, not silent disable)
- Maintenance adds one platform-side retention job only, running from the root server's existing River maintenance surface
- Retention runs on a fixed daily cadence in this phase, without `RunOnStart`, using active-state uniqueness plus an advisory lock so duplicate enqueues across platform instances collapse into harmless no-ops
- Stage 1 applies `projection_only` purge to terminal runs older than `ENGINE_PROJECTION_RETENTION_AFTER`
- Stage 2 applies `full` purge to terminal runs older than `ENGINE_HISTORY_RETENTION_AFTER`
- Retention transitions are idempotent across crash/restart

### Trace search filter wiring (NEW capability `engine-trace-search`)
- Add additive filters to the existing trace list endpoint (`GET /api/traces`) in the handwritten dynamic builder: `engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`
- Queries without engine filters behave exactly as they do today
- Engine-filtered queries naturally exclude non-engine traces because those columns are NULL
- `engine_definition_version`, `engine_wait_kind`, and error-code filters remain deferred

### Schema + store (extends `engine-schema-runtime-delta`)
- Reuse existing `public.traces` engine columns from Proposal 1's baseline (`engine_run_id`, `engine_instance_key`, `engine_definition_name`, `engine_definition_version`, `engine_projection_state`, `engine_projection_updated_at`, `engine_run_status`, `engine_wait_state`); no new trace-shell columns are added
- Add an engine store query for deleting engine history rows for a run
- Add root-side retention candidate selection in `internal/store/` as a handwritten cross-schema join between `engine.runs` and `public.traces`, plus platform store queries for span/span-event deletion by trace (projection purge), barrier flip on `public.traces.engine_projection_state` under CAS, and the four additive engine filters in `internal/store/search.go`

### Python control client (NEW capability `engine-python-control-client`)
- Ship a separate control module (`sdks/python/src/continua/engine_control.py` or equivalent), not additions to the ingest batching client
- Wrap the full frozen control surface from Proposal 1 and this proposal: `start`, `signal`, `cancel`, `terminate`, `get_instance`, `get_run`, `get_result`, `get_history`, `get_pending_work`, `purge`, `repair`, `wait_for_terminal`
- `wait_for_terminal()` is a polling helper with default poll interval `1s`, optional timeout, return value = final run summary/detail object, and timeout error on deadline exceeded

## Impact

- Affected specs (delta per capability):
  - `engine-public-api` (ADDED: purge endpoint, repair endpoint, history-expired condition response, result-on-summary-only response; mismatch field surfacing is docs + regression test only — the fields already exist in Proposal 1 baseline)
  - `engine-trace-projection` (ADDED: projection-only purge mapping, full purge mapping, barrier enforcement, retained operator shell, purge/catching_up race, repair projection behavior, projection state transitions on retention)
  - `engine-runtime-execution` (ADDED: purge service semantics, repair service semantics, terminal-only gate, barrier CAS)
  - `engine-schema-runtime-delta` (ADDED: retention env config validation in root config, engine history delete query, root-side retention candidate selection, platform span/span-event delete queries, projection state CAS writer)
  - `engine-retention-maintenance` (NEW: env-driven platform-side retention job with two stages, idempotency, fail-fast config validation)
  - `engine-trace-search` (NEW: additive engine filters on the existing trace list endpoint)
  - `engine-python-control-client` (NEW: separate control module, frozen control surface wrapper, `wait_for_terminal` polling helper)
- Affected code:
  - `contracts/openapi/openapi.yaml` — purge, repair, history-expired condition, mismatch surfacing; `make generate`
  - `engine/db/queries/` — history delete by run
  - `engine/db/migrations/postgres/` — one index-only migration for retention candidate queries (index on `engine.runs.completed_at` filtered by terminal statuses); no table schema changes
  - `engine/internal/projector/` — barrier enforcement inside the projector write path; async repair resumes via the existing projector loop after the projection state is flipped back to `catching_up`
  - `db/platform/queries/` + `internal/store/` — root-side retention candidate selection across `engine.runs` + `public.traces`, span/span-event delete by trace, `engine_projection_state` CAS writer, additive engine filters in `search.go`
  - `internal/enginecontrol/` (or equivalent shared Fx-provided service) — purge and repair orchestration used by API handlers and retention
  - `internal/api/engine_control.go` — purge and repair handlers wired on `/v1/engine/runs/{run_id}/purge` and `/v1/engine/runs/{run_id}/repair`, mapped onto the shared control service
  - `internal/config/config.go` — retention env parsing and validation in the platform server
  - `internal/jobargs/` + `internal/jobs/` — periodic retention job registration and worker implementation
  - `sdks/python/src/continua/` — new control module, tests
  - No non-engine trace, session, or ingest behavior changes
- No table schema migrations on `public.traces` or `engine.*` tables are required in this phase; one index-only migration is needed on `engine.runs` for retention candidate queries
- API is strictly additive for existing engine clients; the `engine_history_expired` condition is a new case of the existing history endpoint and does not change today's 200/404 shapes for non-expired runs

## Assumptions

- `add-engine-operational-hardening-core` (Proposal 1) is fully implemented and archived; its `TERMINATED` status, `workflow.cancelled`/`workflow.terminated` history events, terminate handler, pending-work endpoint, cooperative cancel contract, instance-status authority, and projector terminal cleanup are frozen and referenced as baseline
- All engine trace-shell columns needed for retention/filters (`engine_run_id`, `engine_instance_key`, `engine_definition_name`, `engine_definition_version`, `engine_projection_state`, `engine_projection_updated_at`, `engine_run_status`, `engine_wait_state`) already exist in `public.traces`
- The projector is the single writer for `public.spans`, `public.span_events`, and projection-state fields on `public.traces`; the purge handler becomes the ONLY other allowed writer for projection-detail deletion and operates via a coordinated barrier CAS
- The platform server (`cmd/continua`) and the engine runtime (`continua-engine`) remain separate binaries in this repo, so retention must run root-side with direct access to the purge service and platform tables rather than by injecting callbacks into the engine process
- The same binary split means repair cannot synchronously drive projector catch-up from the API process in this phase; repair is therefore specified as an async state-transition request that the engine projector later consumes
- `engine_wait_kind` and `engine_last_error_code` columns remain deferred and out of scope
- `projection_purged_at` column is not added; purge timing is represented by `engine_projection_state` plus `engine_projection_updated_at`
- ContinueAsNew, suspend/resume, activity heartbeats, TypeScript control SDK, WebSocket runtime, public bulk backfill APIs, and definition-catalog lookups all remain out of scope
