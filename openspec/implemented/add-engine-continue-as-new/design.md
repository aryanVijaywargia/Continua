# Design: Engine ContinueAsNew

## Context

This is the third and final change in the Phase 4 runtime lifecycle plan. It adds `ContinueAsNew`, the workflow-authored mechanism for atomically terminating the current run and starting a new run of the same instance with fresh history.

The engine currently supports single-run instances: `StartRun` creates an instance and run 1, and the run proceeds to a terminal status. There is no mechanism to continue an instance with a new run, and no run-chain linkage exists. History grows monotonically with every activation, and long-running workflows suffer increasing replay latency.

The `ContinueAsNew` mechanism is standard in durable workflow engines (Temporal, Azure Durable Functions) and is the canonical solution for unbounded workflows. This change implements it within the existing activation transaction model while preserving the existing trace-shell shape for the continuation run's projected trace.

## Goals / Non-Goals

### Goals
- Allow workflow authors to continue execution with fresh history via `ContinueAsNew(input)`
- Create run N+1 atomically within the activation transaction that terminates run N
- Preserve instance identity (instance key, session, definition name) across continuations
- Inherit trace presentation fields (name, user_id, tags, environment, release, metadata) from the previous trace shell
- Expose navigable run-chain and trace-chain linkage in API responses
- Support continuation-following in the Python SDK's `wait_for_terminal`

### Non-Goals
- Definition version override on continuation (always uses the current run's version)
- Carrying over open activity tasks or inbox items to the new run
- Automatic ContinueAsNew (engine-triggered based on history size)
- Cross-instance continuation (continuing as a different instance)
- External ContinueAsNew endpoint (this is workflow-authored only)
- Session reassignment during continuation

## Decisions

### Decision: ContinueAsNew is a sentinel error, following the ErrCancelled pattern
- `workflow.ContinueAsNew(input any) error` returns an `ErrContinueAsNew` sentinel wrapping the continuation input.
- The replay engine detects the sentinel via `errors.Is` (same pattern as `ErrCancelled`).
- Returning the sentinel from the workflow `Run` function, or returning an error that wraps it, triggers continuation.
- Swallowing the sentinel, or otherwise failing to return it after producing it, is a programming error and will produce a replay mismatch on subsequent activations (the history records `workflow.continued_as_new` but the replayed workflow does not return the sentinel).
- **Alternatives considered:** a `Context.ContinueAsNew(input)` method that panics (like `blockOnWait`). Rejected — the error-return pattern is idiomatic Go and consistent with `ErrCancelled`.

### Decision: CONTINUED_AS_NEW is a terminal run status
- The current run is terminal: it will not be claimed, activated, or further modified.
- Lowercase `continued_as_new` is added to `engine.run_lifecycle_status`, and the public API / SDK continue to expose uppercase `CONTINUED_AS_NEW`; the instance stays `active`.
- For purge and retention purposes, `CONTINUED_AS_NEW` is terminal (eligible after the retention window).
- All helpers that currently enumerate terminal run states must include `continued_as_new`, including read-path terminal checks, purge eligibility, retention candidate selection, and projector terminal mapping.
- **Alternatives considered:** reusing `completed` with a special result payload. Rejected — `CONTINUED_AS_NEW` is semantically distinct (the workflow did not "complete" its work; it chose to continue) and needs its own status for filtering and debugger display.

### Decision: Latest/current run semantics use run_number, not created_at
- Once an instance can have multiple runs, "latest run" is defined as the highest `run_number` for that instance.
- `GetLatestRunByInstance` and `ListRunsByInstance` must therefore order by `run_number DESC, id DESC`.
- This ensures `GET /v1/engine/instances/{instance_key}` and other per-instance read paths reliably return the continuation target.
- **Alternatives considered:** continuing to order by `created_at DESC, id DESC`. Rejected — timestamp ordering is no longer a safe proxy once an instance intentionally creates multiple runs.

### Decision: Continuation creates run N+1 within the same activation transaction
- The activation transaction that terminates run N also creates run N+1, its `workflow.started` history event, and the projected trace shell.
- This ensures atomicity: either the continuation fully succeeds (old run terminal, new run created) or nothing changes.
- The new run is created with `run_number = previous + 1` on the same instance.
- `continued_to_run_id` is set on run N, `continued_from_run_id` on run N+1.
- **Alternatives considered:** a two-phase approach (terminate run N, then externally start run N+1). Rejected — this creates a window where the instance has no active run, and requires the operator or a background job to create the continuation.

### Decision: Old run cleanup mirrors cancellation
- Open activity tasks on run N are cancelled (same `CancelOpenActivityTasksByRun` query used by cancellation).
- Open inbox items on run N are discarded (same `DiscardOpenInboxItemsByRun` query).
- This is correct because run N is terminal; its pending work is no longer relevant.
- The new run starts fresh — no state is carried over except the continuation input and the instance identity.
- On the projection side, `continued_as_new` gets its own cleanup reason so the old trace emits resolved wait events for cancelled activity waits and discarded timer waits, consistent with the existing terminal cleanup model.
- Continuation also follows the existing terminal signal-wait behavior: terminal projection clears `engine_wait_state` for a signal wait so the debugger does not retain a stale pending signal state, and a pure signal wait does not require a synthetic resolved-signal timeline event.

### Decision: Continuation trace bootstrap stays in the engine module
- `StartRun` currently creates the initial trace shell/root span in the root module at `internal/api/engine_control.go`.
- The continuation commit path lives in `engine/internal/workflow/activation.go`, in a separate Go module (`./engine`).
- This change keeps continuation-side trace-shell/root-span creation in the engine module, where the activation transaction already runs and where engine code already writes `public.traces` / `public.spans` directly for projection work.
- `StartRun` does not need a structural refactor for this change; the continuation path creates the same shell shape with inherited fields inside the engine module.
- That shell shape is explicit: trace raw status `running`, root-span status `running`, `output = null`, trace/root input set to the continuation input, `engine_run_status = queued`, nil `engine_custom_status`, nil `engine_wait_state`, zero pending counts, `engine_instance_key = instance.instance_key`, and `engine_definition_name` / `engine_definition_version` populated for the new run.
- **Alternatives considered:** extracting a helper in `internal/api` or `internal/enginecontrol`. Rejected — those packages are not importable from the separate engine module. Extracting a neutral shared package is possible later, but is unnecessary scope for this change.

### Decision: ContinueAsNew inherits trace presentation fields from the previous trace shell
- `StartRun` uses request-supplied session, trace name, user_id, tags, environment, release, metadata.
- `ContinueAsNew` reads these fields from the previous run's `public.traces` row and uses them for the new trace shell.
- The continuation does NOT re-read the request or accept overrides — it inherits what was set.
- Session is preserved: the new trace is linked to the same session as the previous trace.
- The previous projected trace shell is required to exist. If the trace row is unexpectedly missing, continuation fails the activation as an invariant violation instead of silently creating a degraded shell with guessed observability fields.
- Nullable presentation fields on an existing trace row may still be absent and propagate as null; only the missing-row case is fatal.
- **Alternatives considered:** allowing field overrides on continuation. Deferred — adds complexity for a rare use case. The workflow can set custom status if it needs to communicate state to the next run.

### Decision: Continuation trace IDs are derived, not stored
- The external engine trace ID is already deterministic: `engine:<run_id>`.
- Run detail and result responses can therefore synthesize `continued_from_trace_id` / `continued_to_trace_id` directly from `continued_from_run_id` / `continued_to_run_id`.
- No `public.traces` columns are required for continuation trace linkage.
- **Alternatives considered:** storing `continued_from_trace_id` / `continued_to_trace_id` on `public.traces`. Rejected — those values are redundant derived data and add unnecessary platform-schema scope.

### Decision: Run chain linkage uses UUID foreign keys
- `continued_from_run_id UUID REFERENCES engine.runs(id)` and `continued_to_run_id UUID REFERENCES engine.runs(id)` on `engine.runs`.
- UUID FKs are appropriate here because both the old and new runs are in the same `engine.runs` table and the linkage is a hard constraint.
- These are set within the continuation transaction and are immutable after creation.

### Decision: `workflow.continued_as_new` history event carries only the continuation input
- Payload: `{ input: <raw JSON> }`
- No other fields (definition version, instance key) are needed because the continuation uses the same values from the instance.
- The event is the last event in run N's history and serves as both an audit trail and a projector input.

### Decision: `get_result` does not overload the result payload
- `GET /v1/engine/runs/{run_id}/result` for a `CONTINUED_AS_NEW` run returns status `CONTINUED_AS_NEW` plus the continuation chain fields.
- The `result` field keeps its existing meaning: the stored workflow result payload from `run.Result`.
- A continued run does not produce a terminal workflow result, so `result` remains `null` for this status.
- The continuation input remains available in the terminal `workflow.continued_as_new` history event and in the new run's `workflow.started` input.
- **Alternatives considered:** stuffing the continuation input into `result`. Rejected — it changes the meaning of the result endpoint and is not supported by the current read-path storage model.

### Decision: Continuation chain fields belong on EngineRunSummary
- `EngineRunSummary` is the shared engine shape used by `EngineInstanceResponse.current_run` and `TraceDetail.engine`, not just by run-detail handlers.
- Continuation chain fields therefore belong on `EngineRunSummary` itself, with run detail and run result reusing the same fields rather than introducing a detail-only navigation surface.
- This keeps debugger and instance surfaces continuation-aware without requiring an extra `GET /v1/engine/runs/{id}` fetch.

### Decision: continued_as_new projects as outputless completion
- `continued_as_new` projects as a completed terminal trace/root span for status purposes, but it does not represent successful workflow output for the finished run.
- The projected trace/root span therefore keep `output = null` for this status and do not reuse the failure-shaped payload emitted for failed terminal states.
- **Alternatives considered:** reusing the non-completed failure payload shape. Rejected — it produces a visually completed trace with synthetic failure output, which is semantically inconsistent.

### Decision: Replay input matching uses equalJSON semantics
- Replay matching for `workflow.continued_as_new` input uses the same rule already used for completion result replay: compare trimmed raw bytes first; if those differ, decode both payloads as JSON and require semantic equality of the decoded values.
- This allows equivalent JSON objects with different key ordering or insignificant whitespace to replay successfully.
- **Alternatives considered:** byte-identical matching only. Rejected — it is stricter than the engine's existing replay payload comparison semantics.

### Decision: Internal and public status casing remain split
- The engine runtime, DB enum value, projector state, and trace search filter use lowercase `continued_as_new`, matching the existing internal status style.
- The OpenAPI `EngineRunStatus` enum and generated SDK enums use uppercase `CONTINUED_AS_NEW`, matching the existing public API contract.
- **Alternatives considered:** flattening everything to one casing convention. Rejected — the current product already distinguishes lowercase internal/search values from uppercase public enums, and this change should extend that contract rather than silently rewrite it.

### Decision: Python SDK follow_continuations defaults to False
- `wait_for_terminal(run_id, ..., follow_continuations=False, max_continuations=32)`
- Default `False` preserves backward compatibility: callers get the result of the specific run they asked about.
- When `True`: if the result status is `CONTINUED_AS_NEW`, the SDK extracts `continued_to_run_id` and polls the next run, repeating up to `max_continuations` hops.
- `EngineRunContinuationDepthError` is raised if the chain exceeds `max_continuations`.
- **Alternatives considered:** always following continuations. Rejected — this changes the semantics of `wait_for_terminal` for existing callers and can create unexpected long waits.

## Risks / Trade-offs

- **Continuation within activation transaction:** the transaction does more work (create run, create history, create trace shell, create root span, update chain links, cleanup old run). This increases the transaction duration. Mitigation: all operations are single-row inserts/updates with indexed lookups; the overhead is comparable to a normal activation commit.

- **Engine-module trace bootstrap drift:** the continuation path now needs to reproduce the same trace-shell/root-span shape that `StartRun` creates. Mitigation: keep the continuation bootstrap narrowly scoped, assert the inherited fields and shell checkpoint state in activation/integration tests, and defer any shared-package refactor until it is clearly worth the extra module work.

- **Unbounded continuation chains:** a workflow that always calls `ContinueAsNew` creates an unbounded number of runs. Mitigation: each old run is terminal and eligible for retention/purge; the Python SDK has a `max_continuations` guard; operators can use retention to bound chain length.

- **Presentation field inheritance drift:** if the previous trace's presentation fields were set by a StartRun request that no longer reflects the desired state, the continuation inherits stale values. Mitigation: the workflow can set custom status to communicate state; field overrides on continuation can be added in a future change.

- **Terminal-status sweep misses:** multiple packages currently hard-code the terminal run set. Mitigation: call this out as a dedicated task bucket covering read APIs, purge, retention, and projector helpers, rather than relying on incidental edits.

- **Projector complexity:** the projector must handle `continued_as_new` as a terminal status and map it to `completed` on the trace/root-span. Mitigation: this follows the existing pattern for `cancelled` and `terminated` mapping.

## Migration Plan

- One engine migration adding `continued_as_new` to `engine.run_lifecycle_status` and adding `continued_from_run_id`, `continued_to_run_id` columns to `engine.runs`.
- One platform migration updating the `public.traces.engine_run_status` check constraint to allow `continued_as_new`.
- API response field additions are strictly additive (nullable fields).
- Trace chain IDs are derived in read APIs; no platform trace-link columns are added.
- OpenAPI changes extend both contracts that already exist today: uppercase engine run statuses on engine endpoints, lowercase `engine_run_status` values on trace-search filters.
- No existing client contract is broken.
- Rollback: continuation fields are nullable and unused if the feature is disabled.
