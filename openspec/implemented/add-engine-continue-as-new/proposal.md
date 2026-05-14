# Change: Add Engine ContinueAsNew

## Why

Long-running workflows that process unbounded work (event loops, polling loops, batch processors) accumulate history indefinitely: every timer, signal, and activity appends events that must be replayed on every activation. Without a mechanism to start fresh, replay latency grows linearly with workflow lifetime, and history retention becomes a scaling bottleneck.

`ContinueAsNew` allows a workflow to atomically terminate the current run and start a new run of the same instance with fresh history, preserving the logical workflow identity (instance key, session, trace presentation) while resetting the replay burden. The old run is terminal (`CONTINUED_AS_NEW` in the public API, stored internally as `continued_as_new`) and linked to the new run via chain fields, giving operators and the debugger a navigable continuation trail.

## What Changes

### Workflow authoring API
- Add `workflow.ContinueAsNew(input any) error` — returns a sentinel error recognized by the replay engine
- Returning the sentinel from the workflow `Run` function, or returning an error that wraps it, triggers continuation
- Swallowing the sentinel, or otherwise failing to return it after producing it, is a programming error that produces a replay mismatch on subsequent activations
- Replay matches continuation input using the existing JSON-equivalence rule: trimmed byte equality first, otherwise decoded semantic JSON equality
- The sentinel is replay-aware: on replay, `workflow.continued_as_new` is consumed as a terminal event

### Engine runtime
- Add lowercase `continued_as_new` to `engine.run_lifecycle_status` (the public API continues to expose uppercase `CONTINUED_AS_NEW`; the instance stays `active`)
- Add `decisionContinuedAsNew` to the activation decision kinds
- When the replay engine observes the `ContinueAsNew` sentinel:
  1. Append `workflow.continued_as_new` history event (payload: `input`)
  2. Transition the current run to stored status `continued_as_new` with `completed_at = NOW()`
  3. Create run N+1 on the same instance with `run_number = previous + 1`, same `definition_version` (or allow override — deferred), fresh history starting with `workflow.started`
  4. Cancel open activity tasks and discard open inbox items on the old run (same cleanup as cancellation)
  5. Set `continued_to_run_id` on the old run, `continued_from_run_id` on the new run
  6. Update terminal-status helpers, retention candidate selection, purge eligibility, and terminal projection logic to treat `continued_as_new` as terminal
  7. All within the same activation transaction
- Instance status remains `active` during continuation (there is still an active run)
- Per-instance "latest run" lookups switch to `run_number DESC, id DESC` so the continuation target is unambiguously current once an instance has multiple runs

### Trace projection
- `StartRun` keeps its existing root-module trace bootstrap path
- `ContinueAsNew` creates the new trace shell/root span inside the engine module activation path, using inherited session and trace presentation fields (name, user_id, tags, environment, release, metadata) from the previous run's projected trace shell
- The new trace shell is created immediately within the continuation transaction, with the same live-shell shape as `StartRun`: trace/root raw status `running`, `output = null`, input set to the continuation input, `engine_run_status = queued`, nil wait/custom status, zero pending counts, `engine_instance_key = instance.instance_key`, and `engine_definition_name` / `engine_definition_version` set for the new run, plus `engine_projection_state = up_to_date` with the new `workflow.started` event as the initial checkpoint
- `continued_as_new` maps to projected raw trace status `completed` and root-span status `completed` (the old run is terminal)
- The terminal projection for `continued_as_new` keeps trace/root `output = null`; it MUST NOT synthesize the failure-shaped payload used for failed terminal states
- `continued_as_new` uses its own terminal cleanup reason so discarded activity/timer waits on the old trace emit resolved wait events with continuation-specific resolution, consistent with the existing cancellation/termination cleanup model
- When the old run was waiting on a signal, terminal projection clears the projected signal wait state so the debugger does not retain a stale pending wait; a pure signal wait does not require a synthetic resolved signal event
- If the previous projected trace shell is unexpectedly missing, continuation fails the activation as an invariant violation rather than silently creating a degraded shell; nullable presentation fields may still remain null when the source trace row exists

### API surface
- `EngineRunSummary`, run detail, and run result responses include `continued_from_run_id`, `continued_to_run_id`, `continued_from_trace_id`, and `continued_to_trace_id` as nullable fields
- `continued_from_trace_id` / `continued_to_trace_id` are derived from the corresponding run chain fields using the existing `engine:<run_id>` trace ID format; no extra trace-link columns are added
- `GET /v1/engine/runs/{run_id}/result` returns status `CONTINUED_AS_NEW` for a continued run, while keeping `result` reserved for the stored workflow result payload, so continued runs return `null` there
- Instance responses return the highest `run_number` as the current run, which is the continuation target after a continue-as-new hop
- Trace detail surfaces that expose `TraceDetail.engine` inherit the same continuation fields through `EngineRunSummary`; they do not require a second run-detail fetch just to discover continuation links
- Public OpenAPI and SDK enums use uppercase `CONTINUED_AS_NEW`; stored `engine_run_status` values and trace-search filters use lowercase `continued_as_new`

### Schema
- Add `continued_from_run_id UUID REFERENCES engine.runs(id)` and `continued_to_run_id UUID REFERENCES engine.runs(id)` to `engine.runs`
- Add `workflow.continued_as_new` history event type
- Update the `public.traces.engine_run_status` constraint to allow `continued_as_new`

### Python SDK
- Extend `wait_for_terminal(run_id, ..., follow_continuations=False, max_continuations=32)`:
  - When `follow_continuations=True` and the result status is `CONTINUED_AS_NEW`, follow `continued_to_run_id` and poll the next run
  - Repeat up to `max_continuations` hops
  - Raise `EngineRunContinuationDepthError` if the chain exceeds the limit
  - Default `follow_continuations=False` preserves backward compatibility
- Add `CONTINUED_AS_NEW` to the `EngineRunStatus` enum

## Impact

- Affected specs (delta per capability):
  - `engine-runtime-execution` (ADDED: CONTINUED_AS_NEW status, continuation decision, run chain linkage, latest-run ordering, old-run cleanup, terminal-status plumbing)
  - `engine-public-api` (ADDED: continuation chain fields on summary/run/result responses, uppercase status contract, current-run semantics, trace-search filter enum)
  - `engine-workflow-api` (ADDED: ContinueAsNew API)
  - `engine-trace-projection` (ADDED: continued_as_new→completed mapping, immediate continuation trace shell creation, explicit live-shell/output semantics, signal-wait cleanup, presentation inheritance)
  - `engine-python-control-client` (ADDED: follow_continuations on wait_for_terminal, CONTINUED_AS_NEW status, continuation depth error)
- Affected code:
  - `engine/pkg/workflow/context.go` — `ContinueAsNew(input any) error`, `ErrContinueAsNew` sentinel
  - `engine/pkg/history/history.go` — `workflow.continued_as_new` event type and payload
  - `engine/db/migrations/postgres/` — `continued_as_new` enum value, chain linkage columns on `engine.runs`
  - `engine/db/queries/runs.sql` — `TransitionRunToContinuedAsNew`, chain linkage updates, and `run_number`-based latest-run ordering
  - `engine/internal/workflow/replay.go` — `decisionContinuedAsNew`, replay handling of continuation sentinel
  - `engine/internal/workflow/activation.go` — continuation decision commit: create run N+1, update chain links, read inherited trace presentation fields, and create the new trace shell/root span
  - `internal/api/engine_control.go` — terminal-status handling and current-run semantics continue through the updated latest-run query
  - `internal/api/engine_mapper.go` — add continuation fields to run/result response mapping and synthesize trace IDs from run links
  - `contracts/openapi/openapi.yaml` — continuation chain fields on engine response schemas, uppercase `EngineRunStatus`, and lowercase `engine_run_status` trace-filter enum; `make generate`
  - `db/platform/migrations/postgres/` — extend the `traces_engine_run_status_check` constraint for `continued_as_new`
  - `internal/store/search.go` — accept `continued_as_new` in trace filtering
  - `internal/store/engine_projection.go`, `internal/api/engine_control.go`, `internal/enginecontrol/service.go`, `engine/pkg/projection/terminal.go`, `engine/internal/projector/` — continued-as-new terminal handling for retention, purge, terminal shells, projected status mapping, and continuation-specific cleanup semantics
  - `sdks/python/src/continua/engine_control.py` — `follow_continuations` parameter on `wait_for_terminal`
  - `sdks/python/src/continua/types.py` — `CONTINUED_AS_NEW` enum value
  - `sdks/python/src/continua/exceptions.py` — `EngineRunContinuationDepthError`

## Assumptions

- `add-engine-activity-retries` is implemented; the `RetryPolicy` and `ActivityOptions` contracts are frozen
- The continuation creates a new run of the SAME `definition_version` — version override on continuation is deferred
- ContinueAsNew is a workflow-authored decision, not an operator control; there is no external `POST /v1/engine/runs/{run_id}/continue` endpoint
- The old run's open activity tasks and inbox items are cancelled/discarded (same cleanup as cancellation), not carried over to the new run
- The new run starts fresh with `run_number = N+1` and a single `workflow.started` event; no history is carried over
- Instance key is preserved; the continuation is logically the same workflow instance continuing with fresh state
- The continuation input remains available in the old run's terminal history event and in the new run's `workflow.started` input; it is not overloaded into the old run's `result` field
- `CONTINUED_AS_NEW` is terminal for purge and retention purposes (eligible for projection_only and full purge after retention window)
