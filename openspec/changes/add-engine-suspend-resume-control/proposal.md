# Change: Add Engine Suspend/Resume Control

## Why

Operators currently have no way to pause a running workflow instance without terminating it. In maintenance windows, incident response, or controlled rollouts, operators need to suspend a workflow — freezing execution while allowing timers, signals, activity completions, and cancel requests to accumulate — and then resume it when ready, with all accumulated state reconciled into the first post-resume activation.

This is also the prep landing zone for Phase 4 runtime lifecycle work: before any new statuses or transitions land, the web test baseline must be restored and maintenance ownership rules must be recorded as a repo-level decision.

## What Changes

### Prep
- Restore `pnpm --filter web test` baseline (fix any currently broken tests or type errors introduced by recent engine trace-shell column additions in the regenerated TypeScript client)
- Record maintenance ownership as a repo-level decision in `.codex/references/decisions.md`:
  - engine maintenance owns due-timer wakeups for non-suspended runs and request-dedupe expiry
  - activity retries (next change) use durable `available_at` on `engine.activity_tasks`, not a new maintenance loop
  - root-side maintenance owns retention and bulk backfill triggering

### Engine runtime
- Add `SUSPENDED` to `engine.run_lifecycle_status` (not to `engine.instance_lifecycle_status` — the instance stays `active` while a run is suspended)
- Add `TransitionRunToSuspended` CAS query: from `queued` or `waiting` only; reject `running` with a typed error
- Add `TransitionRunToQueuedFromSuspended` (resume): from `suspended` only, clears `waiting_for`, sets `ready_at = NOW()`
- Both suspend and resume append their own history event (`workflow.suspended`, `workflow.resumed`) outside of an activation transaction, in the same transaction as the status CAS, using the shared non-activation history sequencing rule (lock run row, allocate next `sequence_no`, append)
- While suspended: inbox items (timer fires, signals, activity completions, cancel requests) accumulate normally because the run is neither `waiting` (maintenance skips it) nor `queued`/`running` (claim skips it)
- On resume: the run transitions to `queued`, the workflow worker claims it, and the next activation's inbox frontier contains all accumulated items
- Cancel-during-suspension: a cancel request delivered while suspended adds a `cancel` inbox item. On resume, the first activation's frontier processes cancel items first (existing `consumeFrontierCancels` logic applies), so the workflow sees `CancellationRequested() == true` before any wait resumes

### API surface
- Add `POST /v1/engine/runs/{run_id}/suspend` — returns `EngineRunResponse` (full run detail after transition); idempotent (suspending a suspended run is a no-op); returns 409 `run_not_suspendable` if the run is `running` (mid-activation) and 409 `run_terminal` for terminal runs
- Add `POST /v1/engine/runs/{run_id}/resume` — returns `EngineRunResponse`; idempotent (resuming a non-suspended active run is a no-op); returns 409 `run_terminal` if the run is terminal
- `SUSPENDED` is added to the `EngineRunStatus` API enum as uppercase `SUSPENDED` (matching the existing `QUEUED`, `RUNNING`, `WAITING`, etc. convention); the trace-search filter value remains lowercase `suspended` (matching the existing lowercase filter convention)

### Trace projection
- Projected raw trace status for `suspended` engine runs is `running` (still in-flight from the observability perspective)
- Projected root-span status for `suspended` engine runs is `running`
- `engine_run_status` on `public.traces` carries `suspended` as a string value for debugger filtering

### Python SDK
- Add `suspend(run_id)` and `resume(run_id)` methods to `EngineControlClient`

## Impact

- Affected specs (delta per capability):
  - `engine-runtime-execution` (ADDED: SUSPENDED status, suspend/resume transitions, accumulation semantics, cancel-during-suspension)
  - `engine-public-api` (ADDED: suspend endpoint, resume endpoint)
  - `engine-trace-projection` (ADDED: suspended→running projection mapping)
  - `engine-python-control-client` (ADDED: suspend/resume SDK methods)
- Affected code:
  - `engine/db/migrations/postgres/` — enum migration adding `suspended` to `run_lifecycle_status`
  - `engine/db/queries/runs.sql` — `TransitionRunToSuspended`, `TransitionRunToQueuedFromSuspended`
  - `engine/pkg/history/history.go` — `workflow.suspended`, `workflow.resumed` event types and payloads
  - `contracts/openapi/openapi.yaml` — suspend/resume endpoints; `make generate`
  - `internal/api/engine_handlers.go` + `internal/api/engine_control.go` (or `internal/enginecontrol/`) — suspend/resume handlers
  - `internal/store/search.go` — `suspended` added to allowed `engine_run_status` filter values
  - `engine/internal/projector/` — suspended→running mapping
  - `sdks/python/src/continua/engine_control.py` — suspend/resume methods
  - `sdks/python/src/continua/types.py` — SUSPENDED enum value
  - `.codex/references/decisions.md` — maintenance ownership decision
  - `engine/db/queries/runs.sql` — widen `TransitionRunToTerminated` to include `suspended`
  - `web/` — restore test baseline (no new UI changes)
- Terminate is widened to accept `suspended` (in addition to `queued`, `running`, `waiting`); operators should be able to terminate a suspended run without resuming it first
- No instance-level status change; instance stays `active` while a run is suspended
- No new maintenance loop required; existing maintenance naturally skips suspended runs
- The suspend/resume handlers MUST call `syncProjectedTraceSummary` after the status transition so that `engine_run_status`, `engine_wait_state`, and pending-work counts are immediately reflected on `public.traces` — not deferred to the projector

## Assumptions

- `add-engine-retention-repair-and-control-sdk` is fully implemented and its purge, repair, retention, trace-search, and Python control client contracts are frozen
- The shared `engineControlService` (or extracted `internal/enginecontrol/`) from the retention change is available for suspend/resume handler injection
- The existing `consumeFrontierCancels` logic in `replay.go` correctly processes cancel inbox items before timer/signal/activity items in the frontier, which is what delivers the cancel-during-suspension semantic
- Suspend from `running` is intentionally rejected rather than deferred; operators should wait for the current activation to finish (it returns to `waiting` or terminal) before suspending
