## 1. OpenAPI Contract Updates

- [x] 1.1 Add `POST /v1/engine/runs/{run_id}/purge` to `contracts/openapi/openapi.yaml` with request body `EnginePurgeRequest { mode: "projection_only" | "full" }` and response `EnginePurgeResponse { run_id, mode, projection_state, deleted }`
- [x] 1.2 Add `POST /v1/engine/runs/{run_id}/repair` with no request body and response `EngineRepairResponse { run_id, accepted, reason, projection_state }`; document allowed `reason` values (`already_up_to_date`, `history_expired`, `no_events_to_project`, `repair_requested`, `already_catching_up`)
- [x] 1.3 Update `GET /v1/engine/runs/{run_id}/history` response to include an optional `expired: boolean` marker for `journal_expired` runs; document that the endpoint never returns 404 for purged runs
- [x] 1.4 Document in OpenAPI descriptions that `GET /v1/engine/runs/{run_id}/result` continues to return `EngineRunResultResponse` for `summary_only` and `journal_expired` runs (no 404)
- [x] 1.5 Document the existing `run_not_terminal` typed 409 error for purge on non-terminal runs
- [x] 1.6 Extend `GET /api/traces` query parameters with optional filters `engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`
- [x] 1.7 Run `make generate` and verify Go server bindings, TypeScript client, and Python types regenerate without drift

**Validation:** `make generate` succeeds; generated bindings include purge/repair request/response schemas, `expired` history marker, and trace-list engine filter parameters

## 2. Engine Queries + Store (History Delete)

- [x] 2.1 Add `DeleteHistoryByRun :exec` in `engine/db/queries/history.sql`: `DELETE FROM engine.history WHERE run_id = $1`
- [x] 2.2 Run `make generate` and verify the generated query; add the engine store wrapper `DeleteHistoryByRun`

**Validation:** engine generate succeeds; the history-delete wrapper compiles

## 3. Platform / Root Store (Retention Candidates + Projection Delete + Barrier CAS)

- [x] 3.1 Add a root-side retention candidate helper in `internal/store/` using handwritten SQL that joins `engine.runs` with `public.traces` via `engine_run_id` for stage 1, filters `runs.status IN ('completed','failed','cancelled','terminated')`, `runs.completed_at < $1`, `traces.engine_projection_state IN ('up_to_date','catching_up')`, orders by `runs.completed_at ASC, runs.id ASC`, and applies a bounded `LIMIT`
- [x] 3.2 Add the matching root-side handwritten helper for stage 2, filtering `traces.engine_projection_state IN ('summary_only','up_to_date','catching_up')`
- [x] 3.3 Add `DeleteSpanEventsByTrace :exec` in `db/platform/queries/spans.sql` (or the closest existing file): `DELETE FROM span_events WHERE trace_id = $1`
- [x] 3.4 Add `DeleteNonRootSpansByTrace :exec` that deletes `public.spans` rows for a trace excluding the root span (use a subquery or `WHERE trace_id = $1 AND parent_span_id IS NOT NULL` depending on how the existing schema identifies the root)
- [x] 3.5 Add `FlipProjectionStateToSummaryOnly :exec` (CAS on current state ∈ `{'up_to_date','catching_up'}`) updating `engine_projection_state='summary_only'` and `engine_projection_updated_at=NOW()`, keyed by `engine_run_id`
- [x] 3.6 Add `FlipProjectionStateToJournalExpired :exec` (CAS on current state ∈ `{'up_to_date','catching_up','summary_only'}`)
- [x] 3.7 Add `FlipProjectionStateToCatchingUp :exec` (CAS on current state = `'summary_only'`) for repair entry
- [x] 3.8 Run `make generate` for the platform queries and add/extend root-side store helpers so the retention candidate helpers preserve ordering and limits, and the CAS writers return the number of rows affected (or a boolean `mutated` flag) so callers can detect no-op CAS
- [x] 3.9 Grep callers of `public.traces.engine_projection_state` writes and confirm the projector is the only other writer; document the purge-as-co-writer exception in `internal/store/` package doc

**Validation:** platform generate succeeds; root-side candidate helpers return ordered terminal runs; wrappers compile; CAS writers return zero rows when the predicate does not match

## 4. Purge Handler + Service

- [x] 4.0 Extract purge/repair orchestration out of the API server's private `engineControlService` construction into a separately Fx-provided shared service package (for example `internal/enginecontrol/`), then inject that shared service into both the API layer and jobs
- [x] 4.1 Add `PurgeRun` on that shared service so it: opens a transaction, locks `public.traces` by `engine_run_id` via `SELECT ... FOR UPDATE`, verifies run terminal status via the engine runs store, dispatches on mode, calls the platform delete wrappers + CAS writer, and (for `full` mode) calls `DeleteHistoryByRun` against the engine store
- [x] 4.2 Map `run_not_terminal` into a typed HTTP 409 response
- [x] 4.3 Handle idempotent no-op cases: `projection_only` on `summary_only`/`journal_expired`, `full` on `journal_expired` — return 200 with `deleted=false` and the current `projection_state`
- [x] 4.4 Wire the HTTP route with `ENGINE_PUBLIC_API_ENABLED=true` gating, `X-Continua-Engine-Preview: 1` header requirement, API key authentication, and project scoping
- [x] 4.5 Add handler unit tests covering: terminal+projection_only, terminal+full, already-purged idempotency, non-terminal 409, missing-run 404, cross-project 404

**Validation:** handler tests pass; no-op cases return correct structured responses; cross-project scoping is enforced

## 5. Repair Handler + Service

- [x] 5.1 Add `RepairRun` on the shared control service that dispatches on current `engine_projection_state`: `up_to_date` → `{accepted:false, reason:"already_up_to_date"}`, `summary_only` → check if checkpoint equals `engine_latest_history_id`; if equal return `{accepted:false, reason:"no_events_to_project"}` (trace was fully projected before purge, never undo the operator's purge decision); otherwise flip to `catching_up` via CAS writer and return `{accepted:true, reason:"repair_requested", projection_state:"catching_up"}`, `journal_expired` → `{accepted:false, reason:"history_expired"}`, `catching_up` → `{accepted:true, reason:"already_catching_up", projection_state:"catching_up"}`
- [x] 5.2 Ensure repair remains async against the current runtime split: the API/shared service MUST NOT try to call `engine/internal/projector` synchronously, and must rely on the separate `continua-engine` projector loop to catch up after the state flip
- [x] 5.3 Ensure repair on a fully-projected-then-purged trace (checkpoint == latest) is a no-op that preserves `summary_only`; full checkpoint rewind is explicitly out of scope
- [x] 5.4 Wire the HTTP route with `ENGINE_PUBLIC_API_ENABLED=true` gating, `X-Continua-Engine-Preview: 1` header requirement, API key authentication, and project scoping
- [x] 5.5 Add handler unit tests covering: up_to_date no-op, summary_only accepted repair request (`catching_up`), summary_only `no_events_to_project`, journal_expired rejection, catching_up `already_catching_up`, missing-run 404, cross-project 404

**Validation:** handler tests pass; `reason` strings are stable; repair responses reflect current state without attempting synchronous projector work

## 6. Projector Barrier Enforcement

- [x] 6.1 Introduce a single centralized write wrapper that ALL projector detail-write paths (`projectHistoryRows`, terminal projection writes, etc.) call before writing to `public.spans` / `public.span_events`. The wrapper MUST read/lock the `public.traces` row within the same transaction and abort if `engine_projection_state IN ('summary_only','journal_expired')`
- [x] 6.2 Refactor existing projector fan-out paths to call through the centralized wrapper instead of writing directly; ensure no code path can bypass the barrier check
- [x] 6.3 Prevent the projector from advancing `engine_last_projected_history_id` past events whose detail writes were skipped
- [x] 6.4 Add projector tests: catching_up + concurrent purge flips to summary_only, and the next projector iteration does NOT recreate detail rows; re-projecting after `journal_expired` is also a no-op
- [x] 6.5 Add a package-doc note in `engine/internal/projector/` (or the projector's platform-side equivalent) calling out that purge is the only other writer allowed to mutate projection state, via the CAS writer

**Validation:** projector tests pass; no detail row reappears after purge; checkpoint does not advance past barrier

## 7. Retention Worker

- [x] 7.0 Add index-only migration on `engine.runs(completed_at) WHERE status IN ('completed','failed','cancelled','terminated')` to support retention candidate queries efficiently
- [x] 7.1 Add retention env parsing in `internal/config/config.go`: `EngineProjectionRetentionAfter` and `EngineHistoryRetentionAfter` (as `time.Duration`), with fail-fast validation (history > projection, history without projection is an error, unparseable is an error, `0` disables the stage, empty disables the stage)
- [x] 7.2 Add `RetentionArgs` (or equivalent) in `internal/jobargs/` with active-state uniqueness, plus a fixed `RetentionInterval = 24 * time.Hour` for this phase
- [x] 7.3 Register a single platform-side periodic retention job in `internal/jobs/module.go` that runs only when at least one stage is enabled, routes onto the maintenance queue, uses the fixed daily cadence with `RunOnStart: false`, and does NOT add an engine-side maintenance subroutine
- [x] 7.4 Add a retention worker in `internal/jobs/` that uses the root-side retention candidate helpers and calls the shared root-side purge service directly in-process; it must NOT self-call the public HTTP endpoint
- [x] 7.5 Ensure the worker depends on the separately Fx-provided shared control service, not on a service constructed privately inside `internal/api`
- [x] 7.6 Keep advisory-lock serialization around actual execution even though periodic enqueue also uses active-state uniqueness
- [x] 7.7 Implement stage 1 (projection_only) inside the worker: compute `threshold = NOW() - ENGINE_PROJECTION_RETENTION_AFTER`, call the stage 1 candidate helper, iterate in order, and call the shared purge service with `projection_only`
- [x] 7.8 Implement stage 2 (full): compute `threshold = NOW() - ENGINE_HISTORY_RETENTION_AFTER`, call the stage 2 candidate helper, iterate in order, and call the shared purge service with `full`
- [x] 7.9 Enforce bounded batch size per iteration (`limit` argument to candidate helpers)
- [x] 7.10 Add retention tests: no env → no periodic job/worker; invalid env → startup fails; daily cadence is registered with `RunOnStart: false`; active-state uniqueness is configured; stage1-only → only projection_only purges occur; both stages → stage1 then stage2; idempotency after simulated crash; stage2 skips `journal_expired` traces; the shared purge service is called with the correct mode for each candidate

**Validation:** retention tests pass; startup fails fast on invalid config; idempotent transitions hold

## 8. Trace Search Filter Wiring

- [x] 8.1 Extend `internal/store/search.go` dynamic SQL builder with the four additive predicates (`engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`), using positional placeholders and the existing optional-predicate pattern
- [x] 8.2 Validate enum values before binding (`engine_run_status` against the seven allowed values, `engine_projection_state` against the four allowed values)
- [x] 8.3 Map query parameters from the trace list handler into the builder's optional filter struct without altering existing parameters, pagination, or ordering
- [x] 8.4 Add search tests: no engine filter → current behavior unchanged; each of the four filters individually; AND combinations; invalid enum value → 400; mixed engine + non-engine traces in the underlying table → engine-filtered result set excludes non-engine traces
- [x] 8.5 Add EXPLAIN-based tests verifying that each of the four engine filter queries uses index scans (not sequential scans) against the current index set (`engine_run_id`, projection-pending indexes from `000013_engine_trace_linkage.up.sql`). If EXPLAIN reveals sequential scans for `engine_instance_key`, `engine_definition_name`, `engine_run_status`, or `engine_projection_state`, add a targeted index in this phase

**Validation:** search tests pass; EXPLAIN tests confirm no sequential scans; non-engine traces are excluded when filters are present; existing traces list regression tests remain green

## 9. Engine Run Detail Mismatch Surfacing (docs + regression test only)

Note: `definition_version_mismatch` surfacing is already baseline from Proposal 1 — the run detail response already carries `definition_name`, `definition_version`, and the `failure` object. This is NOT new feature work.

- [x] 9.1 Confirm `definition_name`, `definition_version`, and `failure` are already returned by the existing engine run detail response (Proposal 1 baseline)
- [x] 9.2 Add a documentation-only OpenAPI note (in the endpoint description or schema description) describing how clients identify `definition_version_mismatch` from the existing fields
- [x] 9.3 Add a single regression test asserting that a run failed with `error_code=definition_version_mismatch` has the requested definition fields and the failure code in the same response body

**Validation:** existing fields are populated as documented; regression test confirms the mismatch code is surfaced alongside definition fields

## 10. Python Control Client Module

- [x] 10.1 Add `sdks/python/src/continua/engine_control.py` exporting a standalone `EngineControlClient` class with `endpoint`, `api_key`, and optional `timeout` / HTTP session arguments (using `endpoint` for consistency with the existing ingest `Client`)
- [x] 10.2 Implement all control methods: `start`, `signal`, `cancel`, `terminate`, `get_instance` (keyed by `instance_key` matching the OpenAPI path `/v1/engine/instances/{instance_key}`), `get_run`, `get_result`, `get_history`, `get_pending_work`, `purge`, `repair`
- [x] 10.3 Define typed exceptions (`EngineRunNotTerminalError`, `EngineRunNotFoundError`, `EngineRunWaitTimeoutError`) and map HTTP status codes / typed engine error codes to them. Note: `get_history()` returns a typed response with an `expired: bool` field for purged history (HTTP 200, not an error), so there is no `EngineHistoryExpiredError` — callers check `response.expired` instead
- [x] 10.4 Use types derived from the OpenAPI schema for request/response bodies (regenerate via `make generate`, or hand-author mirrors kept in sync) and ensure responses decode into typed values
- [x] 10.5 Export the control client and exceptions from `sdks/python/src/continua/__init__.py`
- [x] 10.6 Confirm the control client is standalone and does NOT modify the ingest `Client` class or its batching state

**Validation:** module imports cleanly; typed exceptions are raised for expected error conditions; control client works without an ingest client instance

## 11. wait_for_terminal Helper

- [x] 11.1 Implement `EngineControlClient.wait_for_terminal(run_id, *, timeout=None, poll_interval=1.0)`
- [x] 11.2 Poll `get_result(run_id)` at `poll_interval` seconds until the run is terminal OR the timeout is exceeded; treat 409 `run_not_terminal` as "not yet terminal" and continue polling (do NOT raise it as an exception during the loop)
- [x] 11.3 Return the terminal `EngineRunResultResponse` object when terminal is observed
- [x] 11.4 Raise `EngineRunWaitTimeoutError` when the timeout elapses without observing terminal
- [x] 11.5 Ensure the helper uses the same `api_key`, `endpoint`, and HTTP session as the control client instance
- [x] 11.6 Add helper tests covering: fast terminal (no wait), completed/failed/cancelled/terminated runs, timeout path, zero-timeout edge case, custom poll_interval

**Validation:** helper tests pass; timeout raises the typed error; terminal observation returns the typed result

## 12. Python Control Client Tests

- [x] 12.1 Add unit tests under `sdks/python/tests/` mocking the HTTP layer for each control method's request shape, response decoding, and error mapping
- [x] 12.2 Add integration tests under `sdks/python/tests/test_engine_control_integration.py` (or equivalent) that exercise the live platform control surface (including purge + repair + wait_for_terminal round trips) against the test harness used by other Python SDK integration tests
- [x] 12.3 Cover purge terminal-only rejection (non-terminal run → typed error), purge idempotency, and repair `already_up_to_date` / `history_expired` / `repair_requested` / `already_catching_up` / `no_events_to_project` reasons
- [x] 12.4 Run `cd sdks/python && uv run pytest` locally and verify all tests pass

**Validation:** `uv run pytest` is green; integration tests round-trip all control methods

## 13. Integration Tests (Platform)

- [x] 13.1 Integration test: `projection_only` purge deletes `public.span_events` and non-root `public.spans` for the trace, preserves trace row + root span, flips state to `summary_only`
- [x] 13.2 Integration test: `full` purge performs the projection_only path then deletes `engine.history` rows for the run and flips state to `journal_expired`
- [x] 13.3 Integration test: purge on a non-terminal run returns 409 with the typed error and performs no deletion
- [x] 13.4 Integration test: purge during `catching_up` is correct — after commit, the projector's next iteration does not recreate detail rows
- [x] 13.5 Integration test: automated retention with `ENGINE_PROJECTION_RETENTION_AFTER` only does exactly one round of projection_only purges on eligible terminal runs
- [x] 13.6 Integration test: automated retention with both vars runs stage 1 then stage 2 and skips `journal_expired` traces
- [x] 13.7 Integration test: invalid retention config fails startup (history without projection, history ≤ projection, unparseable duration)
- [x] 13.8 Integration test: retention is idempotent across crash/restart (simulate crash, restart, verify same end state)
- [x] 13.9 Integration test: repair on a `summary_only` trace with retained history returns `{accepted:true, reason:"repair_requested", projection_state:"catching_up"}` without trying to synchronously run the projector in the API process
- [x] 13.10 Integration test: after accepted repair on a previously purged `catching_up` trace, the separate projector loop rebuilds detail from checkpoint forward (not from zero) and eventually restores `up_to_date`
- [x] 13.11 Integration test: repair on `up_to_date` run is a no-op with `reason=already_up_to_date`
- [x] 13.12 Integration test: repair on `journal_expired` returns `reason=history_expired` and does not rebuild detail
- [x] 13.13 Integration test: repair on `catching_up` returns `reason=already_catching_up` and does not duplicate work
- [x] 13.14 Integration test: `GET /v1/engine/runs/{run_id}/history` returns `expired: true` for `journal_expired` runs and is never 404
- [x] 13.15 Integration test: `GET /v1/engine/runs/{run_id}/result` continues to return the terminal shell for `summary_only` and `journal_expired` runs
- [x] 13.16 Integration test: mixed engine + non-engine trace list query returns both when no engine filter is provided
- [x] 13.17 Integration test: each of the four engine filters individually returns the expected rows; AND combinations narrow correctly; non-engine traces are excluded when any engine filter is applied

**Validation:** all integration tests pass against a real Postgres

## 14. Regression Guard

- [x] 14.1 Run `make generate` and verify no drift
- [x] 14.2 Run `cd engine && go test ./...` — all engine tests pass (including history-delete and barrier-related coverage)
- [x] 14.3 Run `go test ./internal/api/... ./internal/ingest/... ./internal/store/... ./internal/jobs/...`
- [x] 14.4 Run `pnpm --filter web test` to confirm the web app compiles against the regenerated TypeScript client (trace filters are additive query params; no UI change required in this phase)
- [x] 14.5 Run `cd sdks/python && uv run pytest`
- [x] 14.6 Confirm Proposal 1 happy-path tests still pass without modification (terminate, pending-work, workflow.cancelled/terminated history, projector terminal cleanup)

**Validation:** engine, root Go, Python SDK, and web Vitest suites pass

---

### Parallelization Notes

- Tasks 1, 2, 3 can run partially in parallel: Task 1 (OpenAPI) must complete before Tasks 4, 5, 8, 9, 10; Tasks 2 and 3 are independent store/query additions and can run in parallel
- Task 4 (purge handler) depends on Tasks 1, 2, 3 (OpenAPI + engine history delete + platform delete/CAS)
- Task 5 (repair handler) depends on Tasks 1, 3, 4 (OpenAPI + platform CAS + shared control-service extraction)
- Task 6 (projector barrier) depends on Task 3 (CAS writer semantics) but can be developed in parallel with Tasks 4 and 5
- Task 7 (retention) depends on Tasks 2, 3, 4 (candidates, CAS writer, purge service)
- Task 8 (search filters) is independent and can run in parallel with everything else
- Task 9 (mismatch surfacing) is doc + test only and can run in parallel
- Tasks 10, 11, 12 (Python client) depend on Task 1 (OpenAPI frozen)
- Tasks 13, 14 (integration tests + regression guard) are final
