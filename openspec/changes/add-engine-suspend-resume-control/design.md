# Design: Engine Suspend/Resume Control

## Context

This is the first change in the Phase 4 runtime lifecycle plan. It introduces operator-initiated suspend/resume as an external control on engine runs, alongside a prep step that stabilizes the test baseline and records maintenance ownership rules that govern all three Phase 4 changes.

The engine currently supports seven run statuses: `queued`, `running`, `waiting`, `completed`, `failed`, `cancelled`, `terminated`. All non-terminal transitions are either workflow-owned (activation decisions) or operator-initiated (terminate). Suspend/resume adds a new non-terminal external control that pauses execution without terminating the workflow.

## Goals / Non-Goals

### Goals
- Allow operators to pause a workflow run without losing accumulated state
- Ensure accumulated timers, signals, activity completions, and cancel requests are reconciled on resume
- Ensure cancel-during-suspension is observed immediately on the first resumed activation
- Record maintenance ownership rules as a repo-level decision before new runtime features land
- Restore the web test baseline broken by engine trace column additions

### Non-Goals
- Workflow-authored suspend (the workflow choosing to suspend itself)
- Suspend from `running` state (mid-activation)
- Timeout-based auto-resume
- Bulk suspend/resume operations
- Instance-level suspend (suspending all runs of an instance)

## Decisions

### Decision: SUSPENDED is a run-level status only, not an instance-level status
- `SUSPENDED` is added to `engine.run_lifecycle_status` only.
- The instance remains `active` while a run is suspended — there is still a non-terminal run.
- This matches the existing pattern where `queued`, `running`, and `waiting` are run-level states that all map to instance `active`.
- **Alternatives considered:** adding `suspended` to `engine.instance_lifecycle_status`. Rejected because instance status tracks terminal outcomes; suspension is transient.

### Decision: Suspend is allowed from `queued` and `waiting` only; `running` returns 409
- `queued → suspended`: the run hasn't been claimed yet. CAS on `status = 'queued'` ensures atomicity with the claim operation.
- `waiting → suspended`: the run is blocked and not claimed. CAS on `status = 'waiting'` is straightforward.
- `running → 409`: the run is mid-activation with an active lease. Suspending mid-activation would require either aborting the activation (breaking the single-transaction invariant) or introducing a deferred-suspend flag. Both add complexity for marginal benefit.
- **Operator workflow:** if a run is `running`, the operator waits for the current activation to finish (it returns to `waiting` or terminal), then suspends.
- **Alternatives considered:** a `suspend_requested` flag on the run that the activation commit checks. Rejected for this phase — it adds a new column and an extra code path in every activation commit. If operators need this, it can be added as a future refinement.

### Decision: Suspend/resume append history events outside activation transactions
- `workflow.suspended` and `workflow.resumed` are recorded by the suspend/resume handler, not by an activation.
- The handler opens a transaction, CAS-transitions the run, appends the history event, and commits.
- This matches the `workflow.terminated` pattern from the operational hardening change.
- The history events serve as an audit trail and as projector input for the debugger timeline.
- Sequence allocation follows the shared non-activation history rule used by control-path and activity-worker appenders: lock the owning run row `FOR UPDATE`, read the latest history row under that lock, allocate `next_sequence`, then call `AppendHistory` in the same transaction. This is what prevents collisions with `activity.retry_scheduled` events while a run is `waiting` or `suspended`.
- **Alternatives considered:** delivering suspend/resume as inbox items consumed by the next activation. Rejected because suspend should take effect immediately, not deferred to the next activation.

### Decision: Accumulated state reconciliation uses existing frontier logic
- While suspended, inbox items (timers, signals, cancel requests) accumulate because:
  - Timer fires: maintenance's `ListDueTimerRunIDs()` queries for `waiting` runs, which skips `suspended` runs. Timer inbox items stay `pending` with `available_at` in the past.
  - Signals: `POST /v1/engine/runs/{run_id}/signal` creates an inbox item regardless of run status.
  - Cancel requests: `POST /v1/engine/runs/{run_id}/cancel` creates a `cancel` inbox item regardless of run status.
  - Activity completions: the activity worker completes/fails the task and calls `WakeWaitingRun()`, which requires `status = 'waiting'`. For suspended runs this is a no-op (CAS fails), but the task's terminal status is recorded.
- On resume: `suspended → queued`. The workflow worker claims the run, loads pending inbox items, and the existing `consumeFrontierCancels` + frontier processing logic in `replay.go` handles all accumulated items.
- **No change to maintenance, activity worker, signal handler, or cancel handler is required.**
- **Alternatives considered:** a dedicated "resume and wake" operation that immediately processes accumulated state. Rejected — the existing claim→activate→replay flow already handles this correctly.

### Decision: Cancel-during-suspension is naturally ordered by frontier processing
- The existing `consumeFrontierCancels` in `replay.go` processes cancel inbox items BEFORE timer/signal/activity items in the frontier. This is the existing behavior for all activations.
- When a cancel arrives during suspension, it becomes a `cancel` inbox item. On resume, the first activation's frontier contains all accumulated items. `consumeFrontierCancels` runs first, setting `cancelRequested = true` before any `SleepUntil`, `ReceiveSignal`, or `Activity` call resumes.
- **No special cancel-during-suspension logic is needed.** The existing frontier ordering delivers the correct semantics.

### Decision: Suspend and resume return EngineRunResponse, not EngineControlResponse
- Both endpoints return `EngineRunResponse` (the full run detail including status, custom_status, wait_state, pending counts).
- This gives callers the authoritative post-transition state immediately, and distinguishes "actually suspended" from "already was suspended" or "was already queued" without needing a separate flag.
- `EngineControlResponse` only carries `accepted` and `wake_applied`, which is insufficient: callers cannot tell "resumed from suspended" from "was already queued/waiting/running."
- **Alternatives considered:** `EngineControlResponse` with a new `previous_status` field. Rejected — `EngineRunResponse` already exists and gives callers everything they need.

### Decision: Suspend idempotency returns success for already-suspended runs
- `suspend()` on a `suspended` run returns 200 with the current `EngineRunResponse` (status remains `SUSPENDED`).
- `resume()` on a non-suspended active run (`queued`, `running`, `waiting`) returns 200 with the current `EngineRunResponse`.
- Both return 409 only for terminal runs (where suspend/resume are meaningless), and they reuse the existing engine control error code `run_terminal` rather than introducing a new terminal-run variant.
- This matches the idempotent pattern used by cancel and signal.

### Decision: Resume clears `waiting_for` and sets `ready_at = NOW()`
- Resume transitions `suspended → queued` with `waiting_for = NULL` and `ready_at = NOW()`.
- The workflow worker claims the run and replays from the full history, reconstructing the wait state from the accumulated inbox items.
- Clearing `waiting_for` is correct because the run may no longer be waiting for the same thing — accumulated items may satisfy or change the wait.

### Decision: Terminate is widened to include suspended runs
- The current `TransitionRunToTerminated` CAS checks `status IN ('queued', 'running', 'waiting')`. This change widens it to `status IN ('queued', 'running', 'waiting', 'suspended')`.
- Operators should be able to terminate a suspended run without first resuming it. Requiring resume→terminate is an unnecessary extra step.
- The terminate handler, history event, and projected terminal mapping are unchanged; only the CAS predicate is widened.
- **Alternatives considered:** defer to a future change. Rejected — this is a one-line SQL change and leaving suspended runs un-terminatable is a correctness gap.

### Decision: Suspend/resume handlers call syncProjectedTraceSummary
- The existing `syncProjectedTraceSummary` function writes `engine_run_status`, `engine_custom_status`, `engine_wait_state`, `engine_pending_activity_tasks`, and `engine_pending_inbox_items` to `public.traces` in the same transaction.
- Suspend/resume handlers MUST call this function after the status CAS so that `engine_run_status=suspended` (or the resumed status) is immediately visible in the debugger and trace-search filters.
- Without this call, the projected trace would show stale `engine_run_status` until the projector processes the history event — which could be delayed, and the projector's generic event path does not refresh summary fields.
- **Alternatives considered:** teaching the projector to refresh summary fields on `workflow.suspended`/`workflow.resumed` events. Rejected — the projector is the async path; summary sync is the existing pattern used by signal, cancel, and terminate handlers for immediate consistency.

### Decision: Projected trace status for suspended runs is `running`
- From the observability perspective, a suspended run is still in-flight — it hasn't completed, failed, or been cancelled.
- The raw trace status `running` is the correct mapping because it indicates "work is still happening" to the debugger.
- `engine_run_status` on `public.traces` carries `suspended` as a string for engine-specific filtering.
- The projector maps `suspended` → trace `running`, root-span `running`.

### Decision: Maintenance ownership is recorded as a repo-level decision
- This is captured in `.codex/references/decisions.md` during the prep step.
- The rules:
  - Engine maintenance (`engine/internal/worker/maintenance.go`) owns due-timer wakeups for non-suspended runs and request-dedupe expiry.
  - Activity retries use durable `available_at` on `engine.activity_tasks`, not a new maintenance loop (relevant to the next change).
  - Root-side maintenance (`internal/jobs/`) owns retention and bulk backfill triggering.
- These rules exist implicitly today; recording them prevents future changes from accidentally introducing duplicate maintenance loops.

## Risks / Trade-offs

- **Suspend from `running` rejected:** operators must wait for the current activation to finish before suspending. This is a UX trade-off for simplicity. Mitigation: activations are typically short (single replay cycle); a deferred-suspend flag can be added later if needed.

- **Projected summary lag during suspension:** the activity worker can complete/fail tasks while the run remains suspended, and those changes may not be reflected immediately in the projected trace summary on `public.traces` until the projector or an explicit summary sync updates it. The live pending-work endpoint reads engine tables directly and remains authoritative. Mitigation: document that debugger/search summary fields can lag, while `GET /v1/engine/runs/{run_id}/pending-work` stays current.

- **Timer wakeup gap:** timers that fire during suspension are not waked. Their inbox items stay `pending` with `available_at` in the past. After resume, the next activation processes them immediately. Mitigation: this is the intended behavior — suspended runs should not be waked.

- **History event ordering:** `workflow.suspended` and `workflow.resumed` can now interleave in commit order with other non-activation appenders such as `activity.retry_scheduled` while a run is `waiting` or `suspended`. Mitigation: all non-activation appenders use the shared run-lock sequence-allocation rule, so ordering is serialized per run and `UNIQUE (run_id, sequence_no)` collisions are prevented.

## Migration Plan

- One enum migration adding `suspended` to `engine.run_lifecycle_status`.
- No table schema changes to `public.traces` or `engine.runs` columns.
- Suspend/resume endpoints are additive; no existing client contract is broken.
- The `suspended` value is additive to the `engine_run_status` filter in trace search.

## Resolved Questions

- **Suspend from `running`:** Rejected for this phase. Operators wait for activation to finish.
- **Instance-level suspend:** Rejected. Suspension is per-run.
- **Auto-resume timer:** Rejected for this phase. Can be added as a future refinement via a scheduled inbox item.
