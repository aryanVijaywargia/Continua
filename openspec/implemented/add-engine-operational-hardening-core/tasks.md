## 1. Engine Schema Migrations

- [x] 1.1 Create `engine/db/migrations/postgres/NNNN_terminated_lifecycle.up.sql` with: `ALTER TYPE engine.run_lifecycle_status ADD VALUE 'terminated'`, `ALTER TYPE engine.instance_lifecycle_status ADD VALUE 'terminated'`
- [x] 1.2 Create matching down migration that fails explicitly if any row uses `terminated`, otherwise renames the old enums, creates replacements without `terminated`, alters the columns with explicit casts, and drops the old enums (mirror the `waiting` down migration pattern)
- [x] 1.3 Create `engine/db/migrations/postgres/NNNN_backfill_instance_status.up.sql` with a single `UPDATE engine.instances SET status = ... FROM (SELECT latest run per instance)` statement that sets each instance's status to its latest run's terminal status or `active` for non-terminal latest runs
- [x] 1.4 Create matching down migration that is a safe no-op (the backfill is idempotent and advisory)

**Validation:** Migrations up/down round-trip on a database with and without `terminated` rows

## 2. Engine Query Additions

- [x] 2.1 Update `TransitionRunToCancelled` in `engine/db/queries/runs.sql` to drop the `claimed_by` CAS while keeping status scoped to `running` only: `WHERE id = $1 AND status = 'running'`, clearing `claimed_by`, `claimed_at`, `lease_expires_at`, `waiting_for`, `result`, setting `completed_at = NOW()`, `last_error_code='cancelled'`, `last_error_message='workflow cancelled'`, with `RETURNING *` (this query is called only from `decisionCancelled` inside activation, which already holds the row lock)
- [x] 2.2 Add `TransitionRunToTerminated` in `engine/db/queries/runs.sql` with the same non-CAS status guard, setting `status='terminated'`, `last_error_code='terminated'`, `last_error_message='run terminated by operator'`, with `RETURNING *`
- [x] 2.3 Verify the existing `UpdateInstanceStatus` query in `engine/db/queries/instances.sql` accepts the new `terminated` enum value (no query changes needed beyond the enum migration in Task 1.1); callers will reuse this query with `(instance_id, status)` from the already-held instance ID
- [x] 2.4 Add `CancelOpenActivityTasksByRun :many` in `engine/db/queries/activity_tasks.sql`: `UPDATE engine.activity_tasks SET status='cancelled', completed_at=NOW() WHERE run_id=$1 AND status IN ('queued','claimed') RETURNING *`
- [x] 2.5 Add `DiscardOpenInboxItemsByRun :many` in `engine/db/queries/inbox.sql`: `UPDATE engine.inbox SET status='discarded', resolved_at=NOW() WHERE run_id=$1 AND status IN ('pending','claimed') RETURNING *`
- [x] 2.6 Update `CountOpenInboxByRun` in `engine/db/queries/inbox.sql` to exclude cancel inbox rows: add `AND kind <> 'cancel'` to the WHERE clause so the count reflects only operator-visible pending work (timers + signals) and matches the semantic used by `pending-work` and by projected `engine_pending_inbox_items`
- [x] 2.7 Add `ListOpenActivityTasksByRun :many` in `engine/db/queries/activity_tasks.sql`: `SELECT * FROM engine.activity_tasks WHERE run_id=$1 AND status IN ('queued','claimed') ORDER BY available_at ASC, id ASC`
- [x] 2.8 Add `ListOpenInboxItemsByRunAndKind :many` in `engine/db/queries/inbox.sql`: `SELECT * FROM engine.inbox WHERE run_id=$1 AND kind=$2 AND status IN ('pending','claimed') ORDER BY available_at ASC, id ASC`
- [x] 2.9 Add `ListCancelledActivityTasksByRun :many` in `engine/db/queries/activity_tasks.sql`: `SELECT * FROM engine.activity_tasks WHERE run_id=$1 AND status='cancelled' ORDER BY available_at ASC, id ASC`
- [x] 2.10 Add `ListDiscardedTimerInboxItemsByRun :many` in `engine/db/queries/inbox.sql`: `SELECT * FROM engine.inbox WHERE run_id=$1 AND kind='timer' AND status='discarded' ORDER BY available_at ASC, id ASC`
- [x] 2.11 Run `make generate` and verify new queries produce correct Go signatures in `engine/db/gen/go`

**Validation:** `make generate` succeeds; generated `:many` queries return ordered row slices with all columns needed by handlers/projector

## 3. Store Layer Updates

- [x] 3.1 Update the store wrapper for `TransitionRunToCancelled` to treat zero rows under an active-status lock as an invariant failure (not `ErrStaleClaim`), returning an explicit internal error
- [x] 3.2 Add store wrapper for `TransitionRunToTerminated` with the same invariant-failure semantics
- [x] 3.3 Confirm the existing `UpdateInstanceStatus` store wrapper handles the new `terminated` value (no new wrapper needed)
- [x] 3.4 Add store wrappers for `CancelOpenActivityTasksByRun` and `DiscardOpenInboxItemsByRun` that return the slices of returned rows directly
- [x] 3.5 Add store wrappers for `ListOpenActivityTasksByRun`, `ListOpenInboxItemsByRunAndKind`, `ListCancelledActivityTasksByRun`, and `ListDiscardedTimerInboxItemsByRun`, preserving DB order for handler/projector consumers
- [x] 3.6 Review all callers of the old `TransitionRunToCancelled` CAS behaviour and update call sites to the new signature
- [x] 3.7 Grep callers of `CountOpenInboxByRun` (at minimum: `GetRunResult` in `internal/api/engine_control.go`) and confirm each caller's expected semantic is "operator-visible pending work" — document in a code comment on any caller that was previously relying on cancel rows being included

**Validation:** Store package compiles; wrappers match new query signatures; callers/projector compile with the new behavior; `CountOpenInboxByRun` callers are audited

## 4. History Event Package

- [x] 4.1 Add `EventWorkflowCancelled = "workflow.cancelled"` and `EventWorkflowTerminated = "workflow.terminated"` constants to `engine/pkg/history/history.go`
- [x] 4.2 Add `WorkflowCancelledPayload struct{}` and `WorkflowTerminatedPayload struct { ErrorCode string; ErrorMessage string }` with canonical JSON tags (`error_code`, `error_message`)
- [x] 4.3 Register both event types in `payloadTarget` (decode switch) so `DecodePayload` returns the typed structs
- [x] 4.4 Add a round-trip test covering marshal/unmarshal of both payload types

**Validation:** Package compiles; decode returns typed payload for both event types; terminated payload preserves error fields

## 5. Public Workflow Sentinel Documentation

- [x] 5.1 Add a Go doc comment on the already-exported `workflow.ErrCancelled` in `engine/pkg/workflow/context.go` stating: returning it produces terminal `CANCELLED` with a `workflow.cancelled` history row; returning `nil` after observing cancellation produces `COMPLETED`; it participates in replay via `errors.Is`

**Validation:** `go doc engine/pkg/workflow ErrCancelled` shows the new comment

## 6. Terminal Cleanup Lives in the Projector

This change deliberately does NOT introduce a shared cross-binary helper. Terminal debugger cleanup is owned by the projector (projector-as-writer) and fires when the projector processes a `workflow.cancelled` or `workflow.terminated` history row. Implementation tasks live in Section 8 (Engine Projector Updates). This placeholder section exists only to make that architectural choice visible in the task list.

- [x] 6.1 (Doc-only) Record in `engine/internal/projector/` package doc that terminal debugger cleanup (closing activity spans, emitting synthetic wait-resolution events, clearing `engine_wait_state`) fires when the projector processes `workflow.cancelled` / `workflow.terminated` history rows, and that no other code path writes projection tables for terminal cleanup

**Validation:** Package doc is added; grep confirms no cross-binary helper is created

## 7. Engine-Side Runtime Updates

- [x] 7.1 In `engine/internal/workflow/replay.go`, reorder error handling so `errors.Is(runErr, publicworkflow.ErrCancelled)` is checked BEFORE generic failure and produces `decisionCancelled`
- [x] 7.2 Make `decisionCancelled` append `workflow.cancelled` to history (with empty payload) instead of any `workflow.failed` encoding
- [x] 7.3 In `engine/internal/workflow/activation.go`, restructure the `decisionCancelled` commit ordering: mark consumed inbox rows `processed` first, append `workflow.cancelled`, call `TransitionRunToCancelled`, then call `CancelOpenActivityTasksByRun` and `DiscardOpenInboxItemsByRun` to seal remaining open work
- [x] 7.4 Ensure the activation transaction writes ONLY to `engine.*` schema (history, runs, instances, activity_tasks, inbox) — no writes to `public.*` projection tables occur from activation, even on terminal
- [x] 7.5 Update the instance status in the same activation transaction to `cancelled` via the existing `UpdateInstanceStatus(instance_id, 'cancelled')` query (and, as part of the same restructuring, also ensure `completed` and `failed` terminal activation paths call `UpdateInstanceStatus` with the matching status so the instance authority invariant holds for every terminal)
- [x] 7.6 Add engine-side tests: workflow returning `ErrCancelled` ends as `CANCELLED` with `workflow.cancelled` history; workflow returning `nil` after observing cancellation ends as `COMPLETED`

**Validation:** Engine tests pass for the cancelled path; replay detects trailing events after `workflow.cancelled` as a mismatch

## 8. Engine Projector Updates (includes terminal cleanup)

- [x] 8.1 Extend the projector's terminal mapping in `engine/internal/projector/projector.go` so `terminated` runs project to raw trace status `failed`, root span status `failed`, engine summary status `TERMINATED`, with failure details `error_code='terminated'`, `error_message='run terminated by operator'`
- [x] 8.2 Ensure the projector consumes `workflow.cancelled` and `workflow.terminated` via the existing `DecodePayload` path without falling through to an unknown-event-type error
- [x] 8.3 When the projector processes a `workflow.cancelled` or `workflow.terminated` history row, run terminal cleanup INSIDE the projector's own transaction using the new `engine/internal/store` wrappers around the sqlc read queries: load `ListCancelledActivityTasksByRun` and `ListDiscardedTimerInboxItemsByRun` to drive activity/timer cleanup directly, and use the existing projected `engine_wait_state` only to determine whether a pure signal wait must be cleared
- [x] 8.4 Close still-open projected activity spans for each cancelled activity using the existing failure equivalent: set `status='failed'`, set `status_message` to `workflow cancelled` or `run terminated by operator`, and set `end_time` from the terminal history row timestamp
- [x] 8.5 Emit synthetic terminal cleanup events with the existing wait-event model only: `EventType="wait"` and payload `{wait_kind, phase:"resolved", wait_id, resolution}`, where `wait_id` is `activity:<activity_key>` or `timer:<timer_key>` and `resolution` is `cancelled` or `terminated` from the terminal reason
- [x] 8.6 Assign synthetic cleanup event sequences from the terminal row base: keep the terminal row's normal projected sequence, then assign cleanup waits starting at `sequence_no*10 + 1` with activities first in query order and timers second in query order
- [x] 8.7 Derive idempotent `VariantKey` for synthetic events as `terminal_reason + ":" + wait_id` so re-projecting the same terminal history row is a no-op
- [x] 8.8 Exclude `kind='cancel'` inbox rows from synthetic wait-resolution emission
- [x] 8.9 Anchor every synthetic event and span close to the terminal history row ID for traceability
- [x] 8.10 Do NOT emit `wait_kind='signal'` synthetic events — signal waits are cleared via `engine_wait_state` only

**Validation:** Projector tests cover the terminated mapping, terminal cleanup on `workflow.cancelled`/`workflow.terminated`, exact wait payloads/resolutions, deterministic sequence offsets, idempotency on re-projection, and cleared wait state on every terminal

## 9. Platform Schema Migration

- [x] 9.1 Create `db/platform/migrations/postgres/NNNN_engine_run_status_terminated.up.sql` that drops `traces_engine_run_status_check` and recreates it with values `('queued','running','waiting','completed','failed','cancelled','terminated')`
- [x] 9.2 Create matching down migration that fails if any row has `engine_run_status='terminated'`, otherwise restores the original constraint (without `terminated`)

**Validation:** Migration applies on a database containing `completed`/`failed`/`cancelled` rows without errors; down fails safely when `terminated` rows exist

## 10. OpenAPI + TypeScript Client

- [x] 10.1 Extend `EngineRunStatus` in `contracts/openapi/openapi.yaml` to include `TERMINATED`
- [x] 10.2 Add `POST /v1/engine/runs/{run_id}/terminate` route definition with preview-header requirement; response schema reuses the existing `EngineRunResultResponse` (so `200 OK` returns `{run_id, status, result=null, failure}`)
- [x] 10.3 Add `GET /v1/engine/runs/{run_id}/pending-work` route definition with a new `EnginePendingWorkResponse` schema containing: `run_id` (uuid), `current_wait` (nullable), `activities` (array of `EnginePendingActivityItem` with `task_id`, `activity_key`, `activity_type`, `status`, `available_at`, `attempt_count`), `timers` (array of `EnginePendingTimerItem` with `inbox_id`, `timer_key`, `status`, `available_at`), `signals` (array of `EnginePendingSignalItem` with `inbox_id`, `signal_name`, `status`, `available_at`), `pending_activity_tasks` (int), `pending_inbox_items` (int)
- [x] 10.4 Update `GET /v1/engine/runs/{run_id}/result` description to explicitly document `CANCELLED` and `TERMINATED` terminal response shapes
- [x] 10.5 Update the description on `POST /v1/engine/runs/{run_id}/cancel` to reflect its cooperative contract: success does NOT guarantee terminal `CANCELLED`; the workflow decides between `COMPLETED` and `CANCELLED` via `workflow.ErrCancelled`
- [x] 10.6 Run `make generate` and verify Go server bindings + TypeScript client regenerate without drift

**Validation:** `make generate` succeeds; generated bindings include `TERMINATED`, terminate route reusing `EngineRunResultResponse`, and the new `EnginePendingWorkResponse`

## 11. Platform Mapper + Status Classification

- [x] 11.1 Update `engineRunStatusToAPI` in `internal/api/engine_mapper.go` to map `terminated` engine status to `EngineRunStatusTerminated`
- [x] 11.2 Update `isTerminalEngineRun` (or equivalent) to classify `TERMINATED` as terminal so `cancel`, `signal`, and `terminate` routes all reject it uniformly
- [x] 11.3 Update `GetRunResult` to return the documented `result=null` + populated `failure` shape for `CANCELLED` and `TERMINATED`

**Validation:** Mapper and classification tests cover the new terminal value

## 12. Platform Terminate Handler

- [x] 12.1 In `internal/api/engine_control.go`, add a `TerminateRun` service method that opens a transaction, locks the run row (`SELECT ... FOR UPDATE`), returns the current terminal state if already terminal (idempotent), otherwise appends `workflow.terminated` to history, calls `TransitionRunToTerminated`, calls `CancelOpenActivityTasksByRun` and `DiscardOpenInboxItemsByRun`, calls the existing `UpdateInstanceStatus(instance_id, 'terminated')`, commits the transaction, and returns the new terminal state mapped through `EngineRunResultResponse`. The handler writes ONLY to `engine.*` schema; projection cleanup is handled by the projector when it later processes the `workflow.terminated` history row
- [x] 12.2 Treat zero rows from `TransitionRunToTerminated` under the active-status lock as an invariant failure (rollback and return internal error)
- [x] 12.3 Verify that only `engine/db/gen/go` and public engine packages (like `engine/pkg/history`) are imported — NO `engine/internal/*` imports from the handler file, and NO writes to `public.*` projection tables
- [x] 12.4 Wire the HTTP route to enforce `ENGINE_PUBLIC_API_ENABLED=true` gating, `X-Continua-Engine-Preview: 1` preview header, API key authentication, and project scoping
- [x] 12.5 Add handler unit tests covering: active-run terminate, already-terminal idempotency, missing-run 404, cross-project 404, missing preview header 400, disabled-gate 404

**Validation:** Handler compiles with import boundary preserved; terminate route tests pass

## 13. Platform Pending-Work Handler

- [x] 13.1 In `internal/api/engine_control.go`, add a `GetRunPendingWork` service method that loads the run's `waiting_for`, lists open activity tasks (`status IN ('queued','claimed')`) ordered by `available_at ASC, id ASC`, lists open timer inbox rows (`kind='timer'`, `status IN ('pending','claimed')`) ordered by `available_at ASC, id ASC`, lists open signal inbox rows (`kind='signal'`, `status IN ('pending','claimed')`) ordered the same way, and excludes `kind='cancel'` rows
- [x] 13.2 Implement `GetRunPendingWork` with the new generated `*enginedb.Queries` methods (`ListOpenActivityTasksByRun`, `ListOpenInboxItemsByRunAndKind`) in `internal/api/engine_control.go` rather than handwritten SQL or the existing due-now/all-status queries
- [x] 13.3 Synthesize `current_wait` from `runs.waiting_for` (nullable) and return it alongside the three typed arrays plus `pending_activity_tasks` and `pending_inbox_items` counts in the `EnginePendingWorkResponse` body; populate activity items from durable task columns, and decode `timer_key` / `signal_name` from the existing inbox payload contracts without exposing `history_id` or raw payload blobs
- [x] 13.4 Wire the HTTP route with `ENGINE_PUBLIC_API_ENABLED=true` gating and API key authentication; the preview header is NOT required for this GET route
- [x] 13.5 Add handler unit tests covering: run with activities, run with timers, run waiting on pure signal (empty arrays + populated current_wait), run with cancel inbox rows (not shown), cross-project 404
**Validation:** Handler tests pass; response shape matches spec; cancel rows are excluded

## 14. Cancel Handler Documentation

- [x] 14.1 Add a top-of-function comment on `CancelRun` in `internal/api/engine_control.go` documenting its explicit cooperative contract: success does NOT guarantee terminal `CANCELLED`; workflow code decides via `workflow.ErrCancelled`
- [x] 14.2 Confirm the OpenAPI description of `POST /v1/engine/runs/{run_id}/cancel` (updated in Task 10.5) is reflected in the regenerated client docs

**Validation:** Comment renders; OpenAPI description includes the cooperative contract

## 15. Integration Tests

- [x] 15.1 Integration test: `ErrCancelled` path — start run, send cancel, workflow returns `ErrCancelled`, assert terminal `CANCELLED`, `workflow.cancelled` history, instance status `cancelled`, projected summary `CANCELLED`
- [x] 15.2 Integration test: `nil`-after-cancel path — start run, send cancel, workflow returns `nil`, assert terminal `COMPLETED` and NOT `CANCELLED`
- [x] 15.3 Integration test: operator terminate path — start run, terminate while `waiting`, assert terminal `TERMINATED`, `workflow.terminated` history, instance status `terminated`, projected summary `TERMINATED`, `engine_wait_state` cleared to NULL
- [x] 15.4 Integration test: terminate idempotency — terminate an already-terminal run twice, assert second call returns the existing terminal state unchanged with no new history rows
- [x] 15.5 Integration test: terminate-vs-activation race — when activation commits first, terminate observes terminal status under lock and returns it; when terminate commits first, subsequent activation exits stale-claim without reviving the run
- [x] 15.6 Integration test: decisionCancelled ordering — consumed inbox rows are marked `processed` before `CancelOpenActivityTasksByRun`/`DiscardOpenInboxItemsByRun` run, and already-processed rows do not appear in the discarded set
- [x] 15.7 Integration test: late activity completion vs terminate — activity completes just before terminate's row lock, assert no synthetic cancellation is emitted for that activity; when terminate wins, activity completion path returns no-op without reviving the run
- [x] 15.8 Integration test: pending-work endpoint — run with activities, timers, signals (delivered but unconsumed), and pure signal wait; assert response shape and ordering
- [x] 15.9 Integration test: pending-work excludes cancel inbox rows
- [x] 15.10 Integration test: debugger terminal cleanup — after terminate, the projector eventually closes open activity spans, emits synthetic activity/timer wait-resolution events, and clears `engine_wait_state` to NULL (including the pure-signal-wait case); re-projecting the same terminal history row produces no duplicates
- [x] 15.11 Integration test: pending-work/run-read/projected-count consistency — create a run with 1 timer inbox row, 1 signal inbox row, and 1 cancel inbox row; wait for projector catch-up, then assert `GET /v1/engine/runs/{run_id}` (via `CountOpenInboxByRun`), `GET /pending-work`, and the projected `engine_pending_inbox_items` column all report 2 (not 3)

**Validation:** All integration tests pass against a real Postgres

## 16. Projector Tests

- [x] 16.1 Projector test: `terminated` run projects to raw trace `failed`, root span `failed`, engine summary `TERMINATED`, with failure details
- [x] 16.2 Projector test: existing `engine_wait_state` column is cleared to NULL on every terminal transition (completed, failed, cancelled, terminated)
- [x] 16.3 Projector test: `engine_pending_activity_tasks` and `engine_pending_inbox_items` counts are zero after terminal sealing
- [x] 16.4 Projector test: synthetic terminal cleanup emits `wait` events with `resolution=cancelled|terminated`, `wait_id` values `activity:<activity_key>` / `timer:<timer_key>`, and deterministic terminal-row-based sequence offsets

**Validation:** Projector tests pass

## 17. Instance Status Authority Tests

- [x] 17.1 Test: run `completed` writes instance `completed` in the same transaction
- [x] 17.2 Test: run `failed` writes instance `failed` in the same transaction
- [x] 17.3 Test: run `cancelled` writes instance `cancelled` in the same transaction
- [x] 17.4 Test: run `terminated` writes instance `terminated` in the same transaction
- [x] 17.5 Test: backfill migration correctly recomputes instance statuses from latest runs (mix of terminal and non-terminal)

**Validation:** Instance status tests pass; backfill is idempotent

## 18. Regression Guard

- [x] 18.1 Run `make generate` and verify no drift
- [x] 18.2 Run `cd engine && go test ./...` — all engine tests pass (including new replay/activation/projector tests)
- [x] 18.3 Run `go test ./internal/api/... ./internal/ingest/... ./internal/store/... ./internal/jobs/...`
- [x] 18.4 Run `pnpm --filter web test` to confirm the web app compiles against the regenerated TypeScript client
- [x] 18.5 Confirm existing Phase 11/12 happy-path gate tests still pass without modification

**Validation:** Engine, root Go, and web regressions pass; no generated-code drift

---

### Parallelization Notes

- Tasks 1, 4, 5, 6, 9 can run in parallel (engine migration, history events, public sentinel doc, projector-placement doc, platform migration are independent)
- Task 2 depends on Task 1 (queries reference the `terminated` enum value)
- Task 3 depends on Task 2 (store wraps new queries)
- Task 7 depends on Tasks 3+4 (activation uses store wrappers and history constants)
- Task 8 depends on Tasks 3+4 (projector uses `engine/internal/store` wrappers around the new read queries and decodes new event types for terminal cleanup)
- **Task 10 (OpenAPI + regenerate) must precede Tasks 11–13**; the generated server interfaces and client types define the handler signatures and request/response shapes
- Task 11 depends on Task 10 (mapper references the regenerated `EngineRunStatus` values)
- Task 12 depends on Tasks 2+4+9+10+11 (terminate handler uses generated engine sqlc queries directly in `internal/api/engine_control.go`, plus history events, the updated CHECK constraint, the generated server interface, and the mapper — but does NOT depend on Task 8 because projection cleanup is decoupled and happens asynchronously)
- Task 13 depends on Tasks 2+10+11 (pending-work handler uses generated engine sqlc queries directly in `internal/api/engine_control.go`, the generated `EnginePendingWorkResponse` schema, and the mapper)
- Task 14 depends on Task 10 (cancel comment references the regenerated OpenAPI text)
- Tasks 15–17 depend on everything that came before them
- Task 18 is the final regression guard
