# Design: Engine Operational Hardening Core

## Context

Continua's engine has two active in-flight proposals that, together, ship the Phase 11 dark-launch runtime and the Phase 12 public surface:

- `add-engine-dark-launch-core` (dark launch): three worker loops, activation transaction, replay, CAS transitions, at-least-once activities, inbox consumption
- `add-engine-public-surface` (public surface): `/v1/engine/*` REST API, trace linkage columns, projection state machine, projector loop, debugger integration

Both ship cancel and lifecycle semantics that are correct for happy-path flows but under-specify how the system behaves when:

- a workflow is actively waiting on an activity, timer, or pure signal at the moment it terminates
- an operator needs to stop a run that is not cooperating with `CancellationRequested()`
- history consumers need to distinguish "workflow cancelled itself in response to cancel" from "generic failure"
- the platform debugger needs to reconcile its projected open waits/spans with a terminal engine state
- `engine.instances.status` needs to reflect the latest run outcome for retention, filtering, and operator tooling

This change does not restructure the happy path, but it does touch the existing `completed` and `failed` terminal commits in one specific way: every terminal activation transaction must also write the owning instance's status via the existing `UpdateInstanceStatus` query, so `engine.instances.status` stays authoritative going forward. Implementers should treat this as a required same-transaction write on the happy path, not an optional cleanup. Everything else in the `completed`/`failed` paths is unchanged.

## Goals / Non-Goals

### Goals
- Introduce a forceful terminate path with unambiguous semantics
- Give cancel an explicit cooperative contract that can reach a `CANCELLED` terminal outcome
- Make the engine history contract durable enough that history consumers see distinct events for cancelled vs terminated outcomes
- Ensure the debugger never shows open waits or running spans for a run that is terminal in the engine
- Make `engine.instances.status` authoritative going forward and fix pre-existing drift in one backfill
- Give operators a single read endpoint for the durable work a run is still holding

### Non-Goals
- activity retries / retry policy surface
- `ActivityWithOptions(...)` or similar per-call overrides
- retention, purge, or repair tooling
- Python or TypeScript control SDK expansion
- ContinueAsNew
- suspend/resume
- activity heartbeats

## Decisions

### 1. Cancel stays cooperative; terminate is the forceful path
**Decision:** `POST /cancel` only requests cancellation and signals the workflow. The workflow decides how to react. Terminate is a separate, preview-gated endpoint that stops the run directly and idempotently.

**Why:** Cancel has to remain replayable and integrated with activation. Forceful preemption during activation would break replay. Terminate lives outside activation, operates on the run row under lock, and has no replay implications.

**Alternatives considered:**
- Making cancel forceful: rejected — breaks Phase 11's observational cancellation guarantee and leaks into replay
- Inbox-mediated terminate: rejected — terminate must work on runs that are not being activated, including `queued` runs that have no inbox consumer scheduled soon

### 2. `workflow.ErrCancelled` is the explicit sentinel
**Decision:** Treat the already-exported `workflow.ErrCancelled` in `engine/pkg/workflow` as the explicit cancelled sentinel. When workflow code returns it, replay produces `decisionCancelled` and the run becomes `CANCELLED`, not `FAILED`. Returning `nil` after cancel remains valid and produces `COMPLETED`.

**Why:** Today, `cancellation_requested` can only surface as a generic failure or as normal completion. Distinguishing `CANCELLED` from `FAILED` is required for operator tooling, history readers, and the debugger, and users already reach for a sentinel pattern in Go. Replay must check `errors.Is(runErr, workflow.ErrCancelled)` before generic failure handling, otherwise replay-recorded `workflow.cancelled` will never match the re-run.

**Alternatives considered:**
- Encoding cancel via an error code string: rejected — fragile across Go/SDK boundaries and not discoverable
- Auto-treating any error after cancel as cancelled: rejected — masks real failures and is not explicit

### 3. Terminal sealing is rows-returned, not pre-read snapshot
**Decision:** `CancelOpenActivityTasksByRun` and `DiscardOpenInboxItemsByRun` use `UPDATE ... RETURNING *` inside the terminal transaction. The transition from open to sealed is the engine's authoritative state change. The projector later re-reads those sealed rows through explicit sqlc read queries (by status) when it processes the terminal history row and performs debugger cleanup (activity-span closure + synthetic activity/timer wait-resolution events). Those synthetic events reuse the existing `wait` event model with payload `{wait_kind, phase="resolved", wait_id, resolution}`, where `resolution` is `cancelled` or `terminated`. Signal-wait cleanup is handled by clearing the existing `engine_wait_state` projection column, not by inventing a signal-wait timeline event.

**Why:** Between a pre-read snapshot and sealing, an activity can complete or a timer can fire. Using the rows actually mutated avoids racing with those completion paths and avoids synthesizing "cancelled" events for work that actually finished. Today's projector does not emit `wait_kind='signal'` events (only `activity` and `timer`), so pure-signal-wait cleanup does not need its own synthetic event — clearing `engine_wait_state` is sufficient and scope-honest for operational hardening.

**Alternatives considered:**
- Pre-read snapshot alone: rejected — race with late completions, double-close possible
- Emit synthetic `wait_kind='signal'` `phase='resolved'` events for pure signal waits: rejected — that would introduce a new signal-wait timeline model on top of lifecycle hardening

### 4. Terminal debugger cleanup is projection-only
**Decision:** Terminal sealing does not append per-primitive cancel/terminate rows to `engine.history`. The engine history stays minimal. The projector writes synthetic wait-resolution events into `public.span_events` and closes `public.spans` rows (on processing the terminal history row — see Decision 5), all anchored to the terminal history row ID with deterministic `VariantKey`s so re-projection is upsert/do-nothing.

**Why:** Engine history is replay fuel, not a debugger log. Adding per-primitive terminal-stop events would expand the replay contract for no runtime benefit. The debugger is a projection consumer, so projection is the correct place for cleanup.

**Alternatives considered:**
- Per-primitive terminal-stop history rows: rejected — pollutes replay and history readers with events that have no replay meaning
- No cleanup at all, just mark the trace terminal: rejected — open activity spans and "waiting for signal" markers stay visible forever in the debugger

### 5. Projector owns terminal debugger cleanup (projector-as-writer)
**Decision:** Terminal debugger cleanup (closing open activity spans, emitting synthetic wait-resolution events, clearing `engine_wait_state`) lives in the projector and fires when the projector processes a `workflow.cancelled` or `workflow.terminated` history row. Neither the engine-side `decisionCancelled` activation transaction nor the root-side terminate transaction writes to projection tables. The engine transactions still perform their own state mutations (append terminal history row, transition run, update instance status, seal activity_tasks and inbox via `UPDATE ... RETURNING`), but they stop at the engine schema boundary. For activity spans, cleanup uses the existing projected failure equivalent (`status='failed'`) rather than introducing a new span status.

**Why:** The projector is already the single writer for projection tables. Adding a second writer (a shared helper called from two different transactions) would race with the projector's own forward progress. Concretely, if the terminal transaction commits cleanup for activity A, but the projector has not yet processed the earlier `activity.scheduled` event for A, the projector's later `projectActivityScheduled` would overwrite cleanup state with `status='running'`. Putting cleanup inside the projector sidesteps this race entirely: history is processed in order, the terminal row is the last event for the run, and after it there are no further projection writes for the run to race against. This also removes the need for a cross-binary shared helper, simplifying the import boundary.

**Alternatives considered:**
- Shared public helper called from both terminal transactions: rejected — creates a second writer to projection tables and races with projector forward progress
- Duplicate cleanup logic root-side and engine-side: rejected — drift risk on top of the race
- Block the terminal transaction until the projector catches up: rejected — couples operator control-plane latency to projection lag

### 6. Terminal transition queries are non-CAS on `claimed_by`
**Decision:** `TransitionRunToTerminated` guards on `status IN ('queued','running','waiting')`; `TransitionRunToCancelled` remains narrower on `status = 'running'`. Neither transition checks `claimed_by`. Zero rows after a locked read is an invariant failure, not a stale-claim condition.

**Why:** Terminate and `decisionCancelled` both take the run row lock first. Once locked, `claimed_by` is whatever it is; the caller is not a workflow worker defending its claim, it's an operator or a decision that has already won. `TransitionRunToCancelled` stays `running`-only because it is only reachable from inside activation. Applying a CAS on `claimed_by` would make terminate race with the current worker, which is exactly what we need to avoid.

**Alternatives considered:**
- Keep CAS on `claimed_by`: rejected — makes terminate unreliable and injects a race against the same worker that may still be holding the row post-commit
- Drop the status guard too: rejected — we still need to ensure we're not "terminating" a run that is already terminal

### 7. Activation-vs-terminate race is resolved by row lock ordering
**Decision:** Terminate locks the run row before transitioning. Activation also takes a row lock at the start of its transaction. Whichever commits first wins. If activation commits first, terminate reads terminal state and returns it unchanged. If terminate commits first, activation exits via its existing stale-claim path.

**Why:** This is the simplest model and reuses the existing stale-claim machinery. No new coordination primitive is needed.

**Alternatives considered:**
- Cancel inbox interrupt for terminate: rejected — terminate must succeed even when activation is not running
- Dedicated coordination channel: rejected — adds infrastructure for a race the DB can already resolve

### 8. `engine.instances.status` becomes authoritative again via backfill + runtime writes
**Decision:** A one-time backfill recomputes `engine.instances.status` from each instance's latest run. Going forward, every terminal run transition writes the matching instance status in the same transaction.

**Why:** `engine.instances.status` is already part of the schema and is surfaced in operator read paths, but it currently drifts. Fixing it end-to-end requires both a snapshot fix and a durable write path.

**Alternatives considered:**
- Compute instance status on read: rejected — pushes cost to every read, and breaks filtering by instance state
- Keep only the runtime writes: rejected — leaves pre-existing drift in place

### 9. Pending-work endpoint returns exact durable rows plus current wait
**Decision:** `GET /pending-work` returns `current_wait` from `runs.waiting_for`, plus arrays derived from `engine.activity_tasks` (statuses `queued`/`claimed`) and `engine.inbox` (split by `kind='timer'` and `kind='signal'`, statuses `pending`/`claimed`). `cancel` inbox rows are excluded. No `available_at <= NOW()` filter — future and claimed work both appear. Each item exposes only the operational identifiers and scheduling fields needed by clients: activities return `task_id`, `activity_key`, `activity_type`, `status`, `available_at`, `attempt_count`; timers return `inbox_id`, `timer_key`, `status`, `available_at`; signals return `inbox_id`, `signal_name`, `status`, `available_at`.

**Why:** Operators need to see what a run is holding regardless of whether it is immediately runnable. This endpoint is diagnostic, not a work queue view. Separating `current_wait` from the durable arrays reflects the reality that a pure signal wait is in `waiting_for` with no row to point at.

**Alternatives considered:**
- Filter by `available_at <= NOW()`: rejected — hides scheduled future timers and delivered-but-unconsumed signals
- Fold `current_wait` into one of the arrays: rejected — pure signal waits have no row and would get dropped

## Risks / Trade-offs

- **Risk:** Projector-as-writer cleanup lags behind the terminal transaction. **Mitigation:** this is by design — the projector catches up in its own loop, and the engine summary status for the run still becomes `terminated`/`cancelled` as soon as the projector processes the terminal history row. Debugger consumers already tolerate projection lag elsewhere (e.g., `projection_status='catching_up'`).
- **Risk:** Backfill recomputes thousands of instance rows in one migration. **Mitigation:** the backfill is a single `UPDATE ... FROM (SELECT latest run per instance)`; size is bounded by total instance count and is not intended to run during peak traffic.
- **Risk:** Idempotency of synthetic cleanup events regresses if `VariantKey` derivation shifts. **Mitigation:** pin `VariantKey` derivation to `terminal_reason + ":" + wait_id` and add tests that re-project the same terminal history row twice.
- **Risk:** Ordering mistakes in `decisionCancelled` cause double-resolution. **Mitigation:** explicit test covering consumed-inbox-then-seal ordering, plus a test where an activity finishes after cancel was enqueued but before workflow returned `ErrCancelled`.
- **Trade-off:** Keeping history minimal (projection-only terminal cleanup) means history readers that reconstruct wait timelines from history alone will not see the terminal resolution. **Rationale:** history is for replay; wait-timeline reconstruction is a debugger/projection concern.

## Migration Plan

1. Apply engine migration adding `terminated` to `engine.run_lifecycle_status` and `engine.instance_lifecycle_status` enums.
2. Apply engine migration defining `TransitionRunToTerminated`, `TransitionRunToCancelled` (non-CAS guard), `CancelOpenActivityTasksByRun`, `DiscardOpenInboxItemsByRun`.
3. Apply engine data backfill recomputing `engine.instances.status` from each instance's latest run.
4. Apply platform migration dropping and recreating `traces_engine_run_status_check` to accept `'terminated'`.
5. Run `make generate` to regenerate sqlc query bindings.
6. Deploy engine runtime with updated activation (ErrCancelled + decisionCancelled ordering) and projector (terminated mapping).
7. Deploy platform server with terminate handler, pending-work handler, updated cancel handler, and EngineRunStatus enum.
8. Update OpenAPI, run `make generate` for TypeScript client.

### Rollback

- Endpoints are preview-gated and additive; disabling `ENGINE_PUBLIC_API_ENABLED` turns them all off.
- The `terminated` enum values cannot be dropped if any row uses them, so rollback requires first re-transitioning `terminated` runs to `failed`. The down migration MUST guard this explicitly (mirror the `waiting` enum down migration pattern).
- CHECK-constraint rollback reinstates the original constraint, which will then reject existing `'terminated'` rows — rollback must be paired with a data fix.

## Open Questions

None at this time. Prior tentative items are resolved:

- **Where does terminal cleanup live?** Resolved (Decision 5): inside the projector as projector-as-writer. No shared cross-binary helper.
- **Does terminate need a body field for audit?** Resolved: no for this change. Audit lives in request logs and history. Can be added later without a spec break.
- **Does `pending-work` expose `history_id` per row?** Resolved: no. `pending-work` returns only engine durable-row identifiers and scheduling metadata (activity `task_id`/`activity_key`, timer or signal `inbox_id`, plus the per-item status and availability fields). Debugger cross-referencing to history is the debugger's job via projected spans/events, not this operational endpoint.
