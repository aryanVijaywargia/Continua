## Context

Phase 10.1 delivered the engine foundation: 7 tables, 5 enums, sqlc queries, store wrappers, and a CLI with `version` and `migrate` commands. Phase 11 adds the runtime execution core that proves durable workflow execution end-to-end, entirely inside the `continua-engine` binary, before any public surface is exposed.

### Current State

- `engine.instances`, `engine.runs`, `engine.history`, `engine.inbox`, `engine.activity_tasks`, `engine.request_dedupe`, `engine.projection_checkpoints` exist with full schema, queries, and store wrappers
- `engine.run_lifecycle_status` has values: `queued`, `running`, `completed`, `failed`, `cancelled`
- `ClaimNextRun`, `ClaimNextActivityTask`, `ClaimNextInboxItem` implement lease-based claiming
- `CompleteActivityTask` and `FailActivityTask` do not CAS on `claimed_by`
- Placeholder packages exist at `engine/internal/workflow`, `engine/internal/activity`, `engine/internal/worker`, `engine/internal/history`
- No runtime execution, replay, or workflow authoring API exists

### Constraints

- Zero changes to the live product: `cmd/continua`, `internal/*`, `contracts/*`, `db/platform/*`, `web/*`, `sdks/*`
- Engine module isolation: cannot import root `internal/*` or root `db/gen/go/*`
- Postgres is the only runtime DB target
- Existing migrations are immutable
- No new tables in Phase 11; only column additions and enum extension
- No `lease_token` columns; lease identity uses existing `claimed_by`

## Goals / Non-Goals

**Goals**
- Prove one complete internal workflow: start -> activity -> timer -> signal -> replay-correct completion
- Prove restart recovery: mid-activity, timer-persistence, signal-persistence
- Establish the activation transaction model as the single write path for workflow state
- Establish CAS-based guarded transitions for all runtime state changes
- Establish history as the authoritative truth with materialized caches on `runs`
- Ship `serve`, `start`, `signal`, `cancel`, and `inspect` commands
- Ship a minimal public Go workflow authoring API

**Non-Goals**
- Public OpenAPI routes for engine operations
- Debugger projection from engine history
- `cmd/continua` integration or Fx wiring
- SDK control surface (Python or TypeScript)
- Multi-primitive waits (blocking on activity AND timer simultaneously)
- ContinueAsNew, subworkflows, or child workflows
- Sticky cache or worker affinity
- Remote activity workers
- Orphan `waiting` run recovery (see Decision 6)

## Decisions

### Decision 1: Activation Transaction Model

**Choice:** Each workflow activation is a single DB transaction that loads state, replays history, folds in completions/inbox, appends new events, writes caches, and transitions the run.

**Alternatives considered:**
- Multi-step approach with separate load, process, and commit phases using optimistic locking
- Event-sourcing with a separate projection step

**Why this choice:**
- Single transaction guarantees atomicity: either the entire activation succeeds (new events + state transition) or nothing changes
- Eliminates race conditions between loading state and writing results
- History and side-table consumption happen in the same isolation boundary, so no external observer can see partial state
- Simpler to reason about for Phase 11's single-primitive blocking model
- Performance is acceptable because Phase 11 workflows are small (one activity, one timer, one signal)

**Event append discipline:**
- `workflow.started` is appended by the `start` transaction for the new run.
- The activation transaction appends `activity.scheduled` and `timer.scheduled` when those primitives first block.
- The activation transaction appends `activity.completed` / `activity.failed` when it observes durable activity outcomes after replay reaches the history frontier.
- The activation transaction appends `timer.fired`, `signal.received`, and `cancel.requested` when it consumes those inbox rows.
- The activation transaction appends `custom_status.updated` for each new `SetCustomStatus()` call and appends `workflow.completed` / `workflow.failed` on the terminal transition.

**Risk:** Large workflows with many history events could make transactions long-running. Mitigation: Phase 11 workflows are bounded; transaction optimization is a future concern when workflow complexity grows.

### Decision 2: CAS-Based Guarded Transitions

**Choice:** All runtime state transitions use compare-and-swap (CAS) queries. Workflow-owned transitions CAS on `(status, claimed_by)`. External wakeups CAS on `status` only.

**Why two CAS patterns:**
- Workflow-owned transitions (`running -> waiting`, `running -> completed`, `running -> failed`) must verify the current worker still owns the run. A stale worker that lost its lease must not overwrite a new owner's state.
- External wakeups (`waiting -> queued`) are status-only because they don't need ownership — any caller (signal handler, activity completer, maintenance worker) can wake a waiting run, and the wake is idempotent.

**Why not use `UpdateRunStatus` for runtime transitions:**
- `UpdateRunStatus` is a generic status setter with no CAS guard. Using it in runtime paths would create TOCTOU races between checking ownership and applying the transition.
- `UpdateRunStatus` remains available for legacy/admin/test-only use cases.

**New error: `ErrStaleClaim`:**
- Ownership-based CAS wrappers must distinguish three outcomes: (1) transition succeeded, (2) row not found (`ErrNotFound`), (3) row exists but CAS failed (`ErrStaleClaim`)
- Status-only wakeups do not use `ErrStaleClaim`. They distinguish: (1) wake applied, (2) row exists but was already past `waiting` (idempotent no-op), (3) row missing (`ErrNotFound`)
- This is implemented by checking `rows affected = 0` after the update, then doing a follow-up existence check when the wrapper needs to distinguish missing from stale/no-op

### Decision 3: Single-Primitive Blocking

**Choice:** Phase 11 blocks on exactly one primitive at a time. When a workflow calls `Activity()`, `SleepUntil()`, or `ReceiveSignal()`, the run transitions to `waiting` with a `waiting_for` JSON tag identifying the single blocked primitive.

**Alternatives considered:**
- Multi-primitive select/race (block on activity OR timer, whichever completes first)
- Callback-based continuation model

**Why single-primitive:**
- Dramatically simpler activation logic: after replay, the activation either (a) encounters a blocking call and transitions to `waiting`, or (b) reaches the end and completes/fails
- Wakeup logic can check `status = 'waiting'` without validating `waiting_for` contents, because exactly one wake source exists
- Multi-primitive waits require validating `waiting_for` on every wakeup to determine if the specific primitive that completed is the one the workflow is waiting for. This validation logic is non-trivial and deferred.

**Migration path:** When multi-primitive waits are introduced, wakeup queries must add a `waiting_for` content check. The `waiting_for` JSON structure already supports this via the `kind` discriminator.

### Decision 4: History as Authoritative Truth

**Choice:** The `engine.history` table is the sole source of truth for workflow execution state. `runs.result`, `runs.custom_status`, and `runs.waiting_for` are materialized caches rewritten by each activation.

**Why caches on `runs`:**
- `inspect` needs to return current status without replaying history
- `waiting_for` enables maintenance worker timer checks without history replay
- `result` enables completion queries without scanning history for `workflow.completed`

**Why rewrite on every activation:**
- Cache invalidation is hard. Rewriting is simple and correct.
- Activation already has the full state in memory after replay. Writing it back is cheap.
- No stale cache risk: if a cache value is wrong, the next activation fixes it.

**Initial workflow input:**
- Phase 11 stores `start --input` only in the initial `workflow.started` history event; no new input column or side table is added.
- The `start` transaction appends `workflow.started` as the first history row for the new run, and replay seeds `Context.Input(out)` from that payload on both first execution and later replays.

### Decision 5: Worker Identity Model

**Choice:** Each loop iteration generates a unique `claimed_by` identity: `workflow:<uuid>`, `activity:<uuid>`, or `maintenance:<uuid>`.

**Alternatives considered:**
- Per-process identity (one UUID for the entire `serve` process)
- Per-loop identity (one UUID per worker loop, reused across iterations)

**Why per-iteration:**
- Per-process identity would mean a crashed-and-restarted process with the same identity could satisfy its own stale CAS checks, bypassing lease expiry
- Per-loop identity has the same problem for a loop that panics and restarts
- Per-iteration identity is the simplest model: every claim is unique, and stale claims are always detectable
- The UUID is cheap to generate and appears in logs for tracing

### Decision 6: Waiting Run Orphan Recovery

**Choice:** Phase 11 does not implement orphan recovery for `waiting` runs. A `waiting` run with no satisfiable wake path is considered an orphan but is not automatically recovered.

**Why this is safe in Phase 11:**
- For activity and timer waits, the activation transaction creates the durable wake source (activity task row or timer inbox row) before committing `running -> waiting`. These happen in the same transaction.
- For signal waits, Phase 11 does not create a separate side-table registration row. The durable registration is `runs.waiting_for = {"kind":"signal",...}` on the run itself, and future `signal` commands enqueue the wakeup inbox row against that waiting run.
- Therefore, activity and timer waits always have a corresponding durable wake source at the moment they become `waiting`, while signal waits are durably registered on the run and rely on later signal delivery rather than pre-created inbox rows.
- The main orphan risk is therefore a bug that writes `waiting_for` incorrectly, or an external deletion of an activity/timer wake source (which nothing in Phase 11 does). A maintenance-worker crash after a timer becomes due is not an orphan: the same timer inbox row remains and will be retried on the next iteration.

**Risk:** A bug that creates a malformed `waiting_for` registration or omits an activity/timer wake source would leave the run stuck. Mitigation: gate tests verify the complete lifecycle, and `inspect` makes stuck runs visible.

### Decision 7: At-Least-Once Activity Execution

**Choice:** Activity execution is explicitly at-least-once. The activity worker claims a task, executes the handler outside a transaction, then completes/fails with a CAS on `claimed_by`.

**Why outside a transaction:**
- Activity handlers may be long-running (network calls, external APIs). Holding a DB transaction open for the duration would exhaust connection pool resources.
- Executing outside the transaction means the claim could expire while the handler runs. Another worker could reclaim and re-execute the same activity.

**Why CAS on complete/fail:**
- If the original worker finishes after its lease expired and another worker reclaimed the task, the CAS on `claimed_by` rejects the stale completion.
- The stale worker receives `ErrStaleClaim` and drops its result — no double-completion, but the activity handler may have executed twice.

**Implication for workflow authors:** Activity handlers should be idempotent or tolerate duplicate execution.

### Decision 8: Replay Semantics

**Choice:** Replay compares primitive kind, stable key, and canonical payload against recorded history. For activities, replay must honor both `activity.completed` and `activity.failed` outcomes. On mismatch, append `workflow.replay_mismatch` and fail the run terminally.

**Why terminal failure on mismatch:**
- A replay mismatch means the workflow definition has changed in a way that is incompatible with the recorded history. Continuing execution would produce incorrect state.
- Recording `workflow.replay_mismatch` in history provides a diagnostic trail.
- Phase 11 requires exact `definition_name` + `definition_version` match, so mismatches indicate a code bug rather than a version migration.

**Stable key requirement:**
- Activities and timers must have caller-supplied stable keys (e.g., `"fetch-user"`, `"wait-for-approval"`), not auto-generated sequence numbers.
- Stable keys survive code refactoring that changes execution order, as long as the logical operation is the same.
- This is enforced at the authoring API level: `Context.Activity(key, ...)` and `Context.SleepUntil(key, ...)` require a non-empty key.

### Decision 9: Observational Cancellation

**Choice:** Cancellation is observational in Phase 11. `cancel` inserts an inbox row for the active non-terminal run; the workflow checks `Context.CancellationRequested()` during execution.

**Why not forced preemption:**
- Forced preemption (killing a running workflow mid-execution) creates partial state that is difficult to reason about and clean up.
- Observational cancellation lets the workflow decide how to handle cancellation: clean up resources, return a partial result, or ignore it.
- The inbox dedupe key ensures repeated `cancel` calls coalesce into a single inbox row.

**Terminal-run boundary:**
- Phase 11 rejects `signal` and `cancel` for terminal runs instead of persisting unconsumable inbox rows.
- This keeps the dark-launch storage surface clean and avoids implying that terminal runs can still observe new control-plane input.

### Decision 10: `waiting_for` JSON Union Structure

**Choice:** `waiting_for` is a tagged JSON union with a `kind` discriminator:
- Activity wait: `{"kind":"activity","activity_key":"...","activity_type":"..."}`
- Timer wait: `{"kind":"timer","timer_key":"...","due_at":"RFC3339"}`
- Signal wait: `{"kind":"signal","signal_name":"..."}`

**Why tagged union:**
- Enables future multi-primitive waits by extending to an array of tagged objects
- Self-documenting: `inspect` output shows exactly what the workflow is waiting for
- Maintenance worker can check `kind = "timer"` and `due_at` without history replay

### Decision 11: Start Command Transaction Ordering

**Choice:** The `start` command executes its entire flow — atomic dedupe claim/takeover, instance creation, run creation, initial `workflow.started` append, dedupe finalization — in a single database transaction.

**Why single transaction:**
- If the process crashes after creating the instance/run but before finalizing the dedupe row, the dedupe row would be stranded as `in_progress` forever, blocking retries until it expires.
- A single transaction makes this impossible: either everything commits (instance, run, finalized dedupe) or nothing does.
- The `request_dedupe.expires_at` column still serves as a safety net for any truly orphaned `in_progress` rows (e.g., if a transaction hangs beyond the TTL and the connection is killed by Postgres). The maintenance worker's `ExpireRequestDedupe` query cleans these up.

**Atomic claim primitive requirement:**
- Phase 11 cannot safely implement `start` with an unlocked get-then-insert or delete-then-insert pattern because `request_dedupe` is unique on `(project_id, request_scope, request_key)`.
- The store layer therefore needs a dedicated atomic start-claim primitive that, under row lock inside the transaction, does exactly one of: create a new `in_progress` row, return a finalized row, return a live `in_progress` row, or reclaim an expired claim.
- A claim is reclaimable either when it is still `in_progress` with `expires_at < NOW()` or when maintenance has already transitioned it to `status = 'expired'`.
- Reclaiming an expired claim renews the existing dedupe record in place rather than deleting and reinserting under the unique key.

**Initial history write:**
- The same `start` transaction appends `workflow.started` as `sequence_no = 1` for the new run, with `definition_name`, `definition_version`, `instance_key`, and optional input payload.
- This makes the CLI request input executable by the workflow runtime without introducing a separate storage path.

**Retry behavior for stranded rows:**
- A `start` retry that finds an `in_progress` row with `expires_at < NOW()` treats it as expired and atomically takes over that same claim with a refreshed `expires_at`.
- A `start` retry that finds an already `expired` row atomically reclaims that same row with `status = 'in_progress'` and a refreshed `expires_at`.
- A `start` retry that finds an `in_progress` row with `expires_at >= NOW()` returns `request_in_progress` (another `start` is actively executing).

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| Long-running activation transactions | DB contention under load | Phase 11 workflows are bounded; optimize in future phases |
| At-least-once activity duplicates | Activity side effects may execute twice | Document idempotency requirement; CAS prevents double-completion |
| No orphan recovery | Stuck `waiting` runs if bug creates malformed signal registration or omits an activity/timer wake source | Activity/timer waits create durable wake sources in-transaction, signal waits are registered via `waiting_for`, and `inspect` makes stuck runs visible |
| Exact version match is restrictive | Cannot evolve definitions without creating new instances | Phase 11 simplification; versioned compatibility is a future concern |
| Single-primitive blocking limits expressiveness | Cannot model select/race patterns | Explicit deferral; `waiting_for` structure supports future extension |

## Open Questions

None. The user specification is comprehensive and all design decisions are resolved.
