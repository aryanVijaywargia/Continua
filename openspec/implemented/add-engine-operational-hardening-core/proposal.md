# Change: Add Engine Operational Hardening Core

## Why

The engine dark-launch runtime (Phase 11) and public engine surface (Phase 12) ship cancel, signal, and terminal-state handling that work for happy-path flows but leave operators and the debugger exposed in several narrow scenarios:

- cancel is purely cooperative and has no way to distinguish "workflow chose to end cleanly" from "operator intent to cancel"
- there is no forceful terminate path, so a stuck or misbehaving run cannot be stopped from the outside
- when a run ends while it had open activity tasks, timer inbox rows, or a pure signal wait, the debugger can still show those waits and spans as live
- `engine.instances.status` can stay `active` after the latest run has reached a terminal state, because the runtime does not currently rewrite it on terminal transitions
- operators have no read endpoint to see, at a glance, the durable work a run is still holding onto
- the public history contract has no events that explicitly represent "this run was cancelled" or "this run was terminated by an operator", so history consumers cannot tell those outcomes apart from a generic failure

This change hardens the existing lifecycle and control surface for these cases without changing the Phase 11 worker-loop topology or happy-path ingest, start, and signal flows. It does tighten terminal commit behavior inside the workflow worker path so cancelled/completed/failed instance-status writes stay authoritative.

## What Changes

### API surface (extends `engine-public-api`)
- Extend `EngineRunStatus` with `TERMINATED`
- Add `POST /v1/engine/runs/{run_id}/terminate` (preview-header gated, forceful)
- Add `GET /v1/engine/runs/{run_id}/pending-work` with exact row/status semantics and explicit per-item schemas
- Make `POST /v1/engine/runs/{run_id}/cancel` contract explicit as cooperative
- Define `GET /v1/engine/runs/{run_id}/result` terminal response explicitly for `CANCELLED` and `TERMINATED`

### History contract (extends `engine-history-events`)
- Add `workflow.cancelled` event type (empty payload)
- Add `workflow.terminated` event type with payload `error_code`, `error_message`
- Register both in decode/output paths for history readers

### Runtime (extends `engine-runtime-execution`)
- Adopt the already-exported public sentinel `workflow.ErrCancelled` as the explicit cancelled replay signal
- Replay consults `workflow.ErrCancelled` before generic failure, producing `decisionCancelled`
- `decisionCancelled` processes already-consumed inbox rows first, then seals the rest
- Add forceful terminate path that is direct (not inbox-mediated), idempotent, and locks the run row first
- Terminal transactions (engine-side `decisionCancelled` and root-side terminate) write only to engine schema — projection-table writes are owned exclusively by the projector
- Sealing is driven by rows actually returned from `UPDATE ... RETURNING`, not only a pre-read snapshot
- After any terminal transition, `engine.instances.status` is written to match the run's terminal state

### Schema (extends `engine-schema-runtime-delta`)
- Add engine migration: `terminated` added to `engine.run_lifecycle_status`
- Add engine migration: `terminated` added to `engine.instance_lifecycle_status`
- Update `TransitionRunToCancelled` to drop the `claimed_by` CAS while keeping scope narrowly guarded on `status='running'` (only `decisionCancelled` calls it, only from inside activation)
- Add `TransitionRunToTerminated` non-CAS guarded transition (used by the operator terminate handler for `queued`/`running`/`waiting`)
- Add `CancelOpenActivityTasksByRun :many` using `UPDATE ... RETURNING`
- Add `DiscardOpenInboxItemsByRun :many` using `UPDATE ... RETURNING`
- Update `CountOpenInboxByRun` to exclude `kind='cancel'` so all pending-inbox counts (run summary, pending-work, projected `engine_pending_inbox_items`) agree on the operator-visible definition
- Add one-time data backfill that recomputes `engine.instances.status` from the latest run

### Platform projection (extends `engine-trace-projection`)
- Add platform migration that drops and recreates `traces_engine_run_status_check` so `public.traces.engine_run_status` accepts `'terminated'`
- Define `terminated` projection mapping: raw trace status `failed`, root span status `failed`, engine summary `TERMINATED`
- Projector performs terminal debugger cleanup when processing `workflow.cancelled` / `workflow.terminated` history rows: closes still-running activity spans, emits synthetic wait-resolution events for activity/timer rows, writes terminal trace/root-span state, and clears the existing `engine_wait_state` projection column (signal-wait visibility is cleared via `engine_wait_state` only — no new signal-wait timeline event is introduced)
- Projector remains the single writer for projection tables — terminal transactions do not touch `public.*`
- Synthetic terminal cleanup events are projection-only, idempotent, and anchored to the terminal history row

## Impact

- Affected specs:
  - `engine-public-api` (ADDED: terminate endpoint, pending-work endpoint, terminated status, cancel contract, terminal result response)
  - `engine-runtime-execution` (ADDED: documented ErrCancelled semantics, decisionCancelled ordering, terminate handler, shared sealing, instance status authority, terminal transactions stay inside engine schema)
  - `engine-history-events` (ADDED: workflow.cancelled, workflow.terminated)
  - `engine-schema-runtime-delta` (ADDED: terminated enums, narrowed `TransitionRunToCancelled`, new `TransitionRunToTerminated`, RETURNING sealing queries, `CountOpenInboxByRun` cancel-excluding fix, instance status backfill, instance status updates reuse existing query)
  - `engine-trace-projection` (ADDED: CHECK-constraint migration, terminated mapping, projector-owned terminal debugger cleanup, wait-state clearing on terminal)
- Affected code:
  - `engine/db/migrations/postgres/` — new migration for `terminated` enums, backfill
  - `engine/db/queries/runs.sql`, `activity_tasks.sql`, `inbox.sql` — new transitions and sealing queries
  - `engine/pkg/workflow/` — existing `ErrCancelled` sentinel with documented replay contract
  - `engine/pkg/history/` — new `workflow.cancelled` and `workflow.terminated` constants and payload structs
  - `engine/internal/workflow/` — `decisionCancelled` ordering, `workflow.ErrCancelled` replay handling, sealing via `UPDATE ... RETURNING`
  - `engine/internal/projector/` — terminated mapping, terminal cleanup on `workflow.cancelled`/`workflow.terminated` history rows
  - `internal/api/engine_control.go` — terminate handler, pending-work handler, cancel handler doc, instance status write on terminal (engine-schema only, no projection writes)
  - `contracts/openapi/openapi.yaml` — terminate route, pending-work route, status and response schema updates
  - `db/platform/migrations/postgres/` — new migration updating `traces_engine_run_status_check`
- No changes to happy-path ingest, non-engine traces, sessions, or the overall Phase 11 worker-loop topology
- API surface is additive for existing engine API consumers (`TERMINATED` and new routes are additive), but pending-work/run-summary inbox counts are intentionally corrected to exclude `kind='cancel'`

## Assumptions

- `add-engine-dark-launch-core` and `add-engine-public-surface` represent the baseline this change extends; their requirements are not rewritten here
- Terminal debugger cleanup is projection-only; the engine history does not gain per-primitive terminal-stop history rows
- The Phase 11 single-primitive-wait assumption still holds
- Activity retries, `ActivityWithOptions(...)`, retention/purge, repair, Python control SDK, ContinueAsNew, suspend/resume, and heartbeats remain out of scope for this change
