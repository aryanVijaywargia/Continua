## 1. Schema Delta & Generation

- [x] 1.1 Create `engine/db/migrations/postgres/000002_runtime_columns.up.sql` with: `ALTER TYPE engine.run_lifecycle_status ADD VALUE 'waiting'`, `ALTER TABLE engine.runs ADD COLUMN result JSONB`, `ADD COLUMN custom_status JSONB`, `ADD COLUMN waiting_for JSONB`, `ADD COLUMN completed_at TIMESTAMPTZ`
- [x] 1.2 Create `engine/db/migrations/postgres/000002_runtime_columns.down.sql` with: guard that fails if any run has `status = 'waiting'`, rename old enum, create replacement without `waiting`, alter `engine.runs.status` with explicit cast, drop old enum, drop the four added columns
- [x] 1.3 Run `make generate` and verify new columns appear in generated `EngineRun` model and `EngineRunLifecycleStatus` includes `waiting`

**Validation:** Migration up/down round-trips cleanly; `make generate` produces no drift

## 2. New & Modified Queries

- [x] 2.1 Add guarded workflow-owned transition queries to `engine/db/queries/runs.sql`: `TransitionRunToWaiting` (`running -> waiting` with `waiting_for`, CAS on `status + claimed_by`), `TransitionRunToCompleted` (`running -> completed` with `result + completed_at`, CAS on `status + claimed_by`), `TransitionRunToFailed` (`running -> failed` with error + `completed_at`, CAS on `status + claimed_by`)
- [x] 2.2 Add guarded external wakeup query to `engine/db/queries/runs.sql`: `WakeWaitingRun` (`waiting -> queued`, CAS on `status` only, clears `waiting_for + claimed_by + claimed_at + lease_expires_at`)
- [x] 2.3 Modify `CompleteActivityTask` and `FailActivityTask` in `engine/db/queries/activity_tasks.sql` to add `AND status = 'claimed' AND claimed_by = sqlc.arg(claimed_by)` to the WHERE clause (both status and claimant CAS required)
- [x] 2.4 Add `ListActivityTasksByRun` to `engine/db/queries/activity_tasks.sql` (all tasks for a run, ordered by `created_at ASC, id ASC`)
- [x] 2.5 Add `ListPendingInboxByRun` to `engine/db/queries/inbox.sql` (pending rows for a run where `available_at <= NOW()`, ordered by `available_at ASC, id ASC`)
- [x] 2.5a Tighten `MarkInboxProcessed` in `engine/db/queries/inbox.sql` to add `AND status = 'pending'` to the WHERE clause (guard against double-processing)
- [x] 2.6 Add request-dedupe helpers to `engine/db/queries/request_dedupe.sql` for atomic `start` claims (helper query/query pair needed to lock an existing row and renew either an expired `in_progress` claim or an already `expired` row under the unique key without delete-then-insert races)
- [x] 2.7 Add `ExpireRequestDedupe` to `engine/db/queries/request_dedupe.sql` (transition `in_progress` rows with `expires_at < NOW()` to `expired`, return count)
- [x] 2.8 Run `make generate` and verify all new queries produce correct Go signatures

**Validation:** `make generate` succeeds; generated code matches query intent; existing query signatures preserved where not modified

## 3. Store Layer Updates

- [x] 3.1 Add `ErrStaleClaim` sentinel error to `engine/internal/store/store.go`
- [x] 3.2 Add store wrappers for guarded workflow-owned run transitions: `TransitionRunToWaiting`, `TransitionRunToCompleted`, `TransitionRunToFailed` — each must distinguish `ErrNotFound` vs `ErrStaleClaim` on zero rows affected
- [x] 3.2a Add `WakeWaitingRun` store wrapper that returns applied-vs-no-op semantics for status-only wakeups; it returns `ErrNotFound` only when the run row is missing and never uses `ErrStaleClaim`
- [x] 3.3 Update `CompleteActivityTask` and `FailActivityTask` store wrappers to accept `claimedBy` parameter and return `ErrStaleClaim` on CAS miss
- [x] 3.4 Add `ClaimStartRequestDedupe` store primitive that, inside the caller transaction, returns one of: newly claimed row, expired-claim reclaim (`status = 'expired'` or stale `in_progress`), finalized existing row, or live `in_progress` duplicate
- [x] 3.5 Add store wrappers: `ListActivityTasksByRun`, `ListPendingInboxByRun`, `ExpireRequestDedupe`

**Validation:** Store compiles; wrappers match new query signatures; `ErrStaleClaim` is testable

## 4. Runtime Config

- [x] 4.1 Extend `engine/internal/config/config.go` with `RuntimeConfig` struct: `WorkflowPollInterval time.Duration`, `ActivityPollInterval time.Duration`, `MaintenancePollInterval time.Duration`, `RunLeaseTTL time.Duration`, `ActivityLeaseTTL time.Duration`, `RequestDedupeTTL time.Duration`
- [x] 4.2 Load from env vars: `ENGINE_WORKFLOW_POLL_INTERVAL`, `ENGINE_ACTIVITY_POLL_INTERVAL`, `ENGINE_MAINTENANCE_POLL_INTERVAL`, `ENGINE_RUN_LEASE_TTL`, `ENGINE_ACTIVITY_LEASE_TTL`, `ENGINE_REQUEST_DEDUPE_TTL` with sensible defaults (no `ENGINE_RUNTIME_ENABLED` — invoking `serve` is the gate)

**Validation:** Config loads; defaults are applied; missing optional vars don't error

## 5. History Event Package

- [x] 5.1 Create `engine/internal/history/events.go` with canonical event type constants: `workflow.started`, `workflow.completed`, `workflow.failed`, `workflow.replay_mismatch`, `activity.scheduled`, `activity.completed`, `activity.failed`, `timer.scheduled`, `timer.fired`, `signal.received`, `cancel.requested`, `custom_status.updated`
- [x] 5.2 Create typed payload structs for each event type with canonical JSON tags
- [x] 5.3 Use standard `encoding/json` for payload serialization; replay comparison deserializes into typed structs rather than comparing raw bytes (no custom canonicalizer in Phase 11)
- [x] 5.4 Add `WaitingFor` tagged union types: `ActivityWait`, `TimerWait`, `SignalWait` with `kind` discriminator

**Validation:** Package compiles; payload round-trips through JSON deterministically

## 6. Workflow Authoring API

- [x] 6.1 Create `engine/pkg/workflow/definition.go` with `Definition{Name, Version, Run}` struct
- [x] 6.2 Create `engine/pkg/workflow/context.go` with `Context` type and primitives: `Input(out)`, `Activity(key, activityType, input, out)`, `SleepUntil(key, at)`, `ReceiveSignal(name, out)`, `CancellationRequested()`, `SetCustomStatus(value)`
- [x] 6.3 Enforce stable key requirement: `Activity()` and `SleepUntil()` return error on empty key

**Validation:** Package compiles; public API surface matches spec; no runtime dependencies leak into the public package

## 7. Replay Engine

- [x] 7.1 Create `engine/internal/workflow/replay.go` with replay state machine that walks recorded history, initializes workflow input from `workflow.started`, and compares against workflow execution
- [x] 7.2 Implement replay comparison: match primitive kind, stable key, and canonical payload against recorded events, including `workflow.started` input bootstrap, `activity.scheduled` before `activity.completed` / `activity.failed`, `timer.scheduled` before `timer.fired`, `signal.received`, `cancel.requested`, `custom_status.updated`, and terminal `workflow.completed` / `workflow.failed`
- [x] 7.3 Implement mismatch handling: append `workflow.replay_mismatch` event and fail the run terminally
- [x] 7.4 Implement version check: exact `definition_name + definition_version` match required; fail with compatibility error on mismatch

**Validation:** Unit tests for replay match, mismatch, and version check scenarios

## 8. Workflow Activation

- [x] 8.1 Create `engine/internal/workflow/activation.go` with activation transaction: load claimed run + instance + ordered history (including the initial `workflow.started` event) + run-scoped activity tasks + due pending inbox rows
- [x] 8.2 Implement replay phase: re-execute workflow definition against history, seed `Context.Input(...)` from `workflow.started`, and fold in durable activity outcomes (completed and failed) and inbox items
- [x] 8.3 Implement commit phase: append new history rows at explicit write points (`activity.scheduled`, `activity.completed`, `activity.failed`, `timer.scheduled`, `timer.fired`, `signal.received`, `cancel.requested`, `custom_status.updated`, `workflow.completed`, `workflow.failed`), write materialized caches (`result`, `custom_status`, `waiting_for`), create wake sources before transitioning (activity task row for `Activity()`, timer inbox row with `kind = 'timer'` and `available_at = due_at` for `SleepUntil()`, signal waits durably registered via `waiting_for` only), mark consumed inbox rows processed, transition run via guarded CAS
- [x] 8.4 Implement error handling: rollback entire transaction on any failure

**Validation:** Activation compiles; unit tests for load/replay/commit phases

## 9. Worker Loops

- [x] 9.1 Create `engine/internal/worker/loop.go` with generic poll-based worker loop infrastructure (poll interval, identity generation, graceful shutdown)
- [x] 9.2 Create `engine/internal/workflow/worker.go` with workflow worker: claim run via `ClaimNextRun`, execute activation, handle `ErrStaleClaim`
- [x] 9.3 Create `engine/internal/activity/worker.go` with activity worker: claim task, execute handler outside transaction, complete/fail with CAS, wake owning run on both success and failure (so activation can observe the outcome)
- [x] 9.4 Create maintenance worker (in `engine/internal/worker/` or `engine/cmd/continua-engine/`): wake waiting runs with due timers, expire stale request dedupe rows

**Validation:** Worker loops compile; can start and stop gracefully

## 10. CLI Commands

- [x] 10.1 Add `serve` command to `engine/cmd/continua-engine/main.go`: start all three worker loops, register demo definitions, handle graceful shutdown
- [x] 10.2 Add `start` command: accept `--instance-key`, `--definition`, `--version`, `--request-key`, `--input`; validate `(definition, version)` against the compiled registry before writing; implement in a single transaction using `ClaimStartRequestDedupe`; append initial `workflow.started` with optional input; handle stranded `in_progress` rows via atomic expiry reclaim; output JSON
- [x] 10.3 Add `signal` command: accept `--instance-key`, `--signal-name`, `--payload`, optional `--dedupe-key`; reject terminal active runs before writing; otherwise insert inbox row, attempt guarded wakeup, output JSON
- [x] 10.4 Add `cancel` command: accept `--instance-key`; reject terminal active runs before writing; otherwise insert inbox row with fixed dedupe key, attempt guarded wakeup, output JSON
- [x] 10.5 Add `inspect` command: accept `--instance-key`; load instance + latest run + history; output JSON
- [x] 10.6 Implement JSON output conventions: success at exit 0, error at exit 1 with `{"error":{"code":"...","message":"..."}}`

**Validation:** All commands compile and produce correct JSON output format

## 11. Dark-Launch Demo

- [x] 11.1 Create `engine/cmd/continua-engine/internal/darklaunch/workflow.go` with a demo workflow: schedules one activity, sleeps on a timer, receives a signal, completes with a result
- [x] 11.2 Create `engine/cmd/continua-engine/internal/darklaunch/activities.go` with demo activity handlers
- [x] 11.3 Register demo definitions in the `serve` command

**Validation:** Demo workflow can be started, executed, and inspected via CLI commands

## 12. Gate Tests

- [x] 12.1 Happy path: start -> activity completes -> timer fires -> signal received -> workflow completes; verify via `inspect`, including history order `workflow.started`, `activity.scheduled`, `activity.completed`, `timer.scheduled`, `timer.fired`, `signal.received`, `workflow.completed`
- [x] 12.2 Duplicate start: same `instance_key` with different request returns `instance_conflict`; same `request_key` returns recorded response
- [x] 12.2a Unknown definition/version: `start` returns `definition_not_registered` and writes no engine rows
- [x] 12.2b Request-key retry after maintenance expiry: expire a stale `request_dedupe` row to `status = 'expired'`, retry the same `request_key`, and verify the claim is reusable
- [x] 12.2c Workflow input round-trip: `start --input` writes `workflow.started.input`, `Context.Input(...)` reads it on first activation, and replay reads the same value deterministically
- [x] 12.3 Mid-activity restart recovery: start workflow, let activity schedule, stop `serve`, restart `serve`, verify activity completes and workflow progresses (document at-least-once duplicate execution)
- [x] 12.4 Timer-persistence restart recovery: start workflow, let timer schedule, stop `serve`, restart `serve`, verify timer fires and workflow resumes
- [x] 12.5 Signal-persistence restart recovery: send signal while `serve` is stopped, restart `serve`, verify signal is consumed
- [x] 12.5a Terminal signal/cancel rejection: after a run reaches terminal state, both commands return `run_terminal` and do not insert new inbox rows
- [x] 12.6 Stale-claim rejection: activity completion with wrong `claimed_by` returns `ErrStaleClaim`
- [x] 12.7 Replay mismatch: modify workflow definition between activations, verify `workflow.replay_mismatch` event and failed run
- [x] 12.8 Version mismatch: register definition with different version, verify compatibility failure

**Validation:** All gate tests pass with `cd engine && go test ./...`

## 13. Secondary Hardening Tests

- [x] 13.1 Restart after run claim: stop `serve` after `ClaimNextRun` but before activation completes; verify run is reclaimable after lease expiry
- [x] 13.2 Restart after activity schedule but before worker pickup: verify activity task is claimed by new worker
- [x] 13.3 Restart just before final run completion commit: verify run is reclaimable and completes on retry

**Validation:** All hardening tests pass

## 14. Regression Guard

- [ ] 14.1 Run `make generate` and verify no drift
- [x] 14.2 Run `cd engine && go test ./...` — all engine tests pass
- [x] 14.3 Run root Go regression: `go test ./internal/api/... ./internal/ingest/... ./internal/store/... ./internal/jobs/...`
- [ ] 14.4 Run existing web tests: `pnpm --filter web test`

**Validation:** Engine and root Go regressions pass; `make generate` still requires a plain `pnpm` binary in PATH and `pnpm --filter web test` still hangs after suite completion in this environment

---

### Parallelization Notes

- Tasks 1, 4, 5, 6 can run in parallel (schema delta, config, history events, and authoring API are independent)
- Task 2 depends on task 1 (queries reference new columns/enum)
- Task 3 depends on task 2 (store wraps new queries)
- Task 6 is independent (authoring API defines primitives; runtime maps them to history events, not the reverse)
- Task 7 depends on tasks 5+6 (replay uses history events and authoring primitives)
- Task 8 depends on tasks 3+7 (activation uses store wrappers and replay)
- Task 9 depends on task 8 (workers use activation)
- Task 10 depends on task 9 (CLI commands wire workers)
- Task 11 depends on tasks 6+10 (demo uses authoring API and CLI)
- Task 12 depends on tasks 10+11 (gate tests exercise full stack)
- Task 13 depends on task 12 (hardening after gate tests pass)
- Task 14 runs after task 12 (regression guard)
