# Change: Complete Engine Debugger Operator Surface

## Why
The backend engine control, pending-work, repair, purge, and continuation APIs are all landed, but the React debugger does not yet expose them. Operators cannot filter traces by engine attributes, inspect pending work, control runs, navigate continuation chains, or see projection mismatch warnings without direct API calls.

## What Changes
- Add URL-driven engine filters (`engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`) to the traces page in a collapsible "Engine filters" section beneath the existing filter grid
- Add `engine_projection_state` helper text identifying it as an operator-oriented/advanced filter
- Add a separate pending-work query keyed as `['enginePendingWork', runId]` that polls on `TIMELINE_POLL_INTERVAL_MS` for active engine runs only
- Add trace-detail controls for signal, cancel, suspend, resume, terminate, purge, and repair with state-gated enablement, confirmation where needed, and structured feedback
- Add a mismatch warning banner when `trace.engine.failure.error_code === 'definition_version_mismatch'`
- Add ContinueAsNew navigation links using `continued_from_trace_id` and `continued_to_trace_id`
- Update `EngineRunStatus` to include `SUSPENDED`, `TERMINATED`, and `CONTINUED_AS_NEW`
- Update `EngineRunSummary` to include continuation link fields

## Impact
- Affected specs: engine-debugger-filters, engine-debugger-controls, engine-debugger-pending-work, engine-debugger-operator-ux
- Affected code: `web/src/pages/TracesPage.tsx`, `web/src/pages/TraceDetailPage.tsx`, `web/src/utils/tracesSearchParams.ts`, `web/src/api/client.ts`, and page-local components colocated with the trace detail page (not shared components unless reuse is demonstrated)
