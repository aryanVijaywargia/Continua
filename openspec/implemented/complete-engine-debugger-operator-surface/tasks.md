## 1. Type and Client Updates
- [x] 1.1 Update `EngineRunStatus` type in `web/src/api/client.ts` to include `SUSPENDED`, `TERMINATED`, `CONTINUED_AS_NEW`
- [x] 1.2 Add `continued_from_run_id`, `continued_to_run_id`, `continued_from_trace_id`, `continued_to_trace_id` to `EngineRunSummary` interface
- [x] 1.3 Add engine control mutation functions to `web/src/api/client.ts`: `signalEngineRun`, `cancelEngineRun`, `suspendEngineRun`, `resumeEngineRun`, `terminateEngineRun`, `purgeEngineRun`, `repairEngineRun`; all mutation wrappers MUST send `X-Continua-Engine-Preview: 1` header
- [x] 1.4 Add `fetchEnginePendingWork(runId)` API client function calling `GET /v1/engine/runs/{run_id}/pending-work` (no preview header required for this read endpoint)

## 2. Engine Trace Filters
- [x] 2.1 Add `engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state` to `FetchTracesParams` and URL serialization in `tracesSearchParams.ts`; `engine_run_status` uses lowercase query values (`queued`, `running`, etc.) with human-readable labels in the select control
- [x] 2.2 Add filter chip derivation for engine filters in `deriveActiveChips()` with human-readable labels (e.g., "Engine status: Waiting")
- [x] 2.3 Add collapsible "Engine filters" section to TracesPage beneath existing filter grid; auto-expand when any engine filter is active
- [x] 2.4 Add `engine_projection_state` helper text in the Engine filters section identifying it as operator-oriented/advanced
- [x] 2.5 Write Vitest tests for engine filter serialization, hydration, chip derivation, lowercase query values, and section auto-expand behavior

## 3. Pending-Work Query and Display
- [x] 3.1 Add `useEnginePendingWork(runId, engineStatus)` hook with `['enginePendingWork', runId]` query key and `TIMELINE_POLL_INTERVAL_MS` refetchInterval
- [x] 3.2 Gate polling enablement on engine status `QUEUED`, `RUNNING`, `WAITING`, or `SUSPENDED`; do not widen existing timeline/span polling
- [x] 3.3 Add PendingWorkPanel component (colocated with trace detail page, not in shared components) rendering current wait, activities, timers, signals with empty and degraded states
- [x] 3.4 Integrate PendingWorkPanel into TraceDetailPage for engine-backed traces
- [x] 3.5 Write Vitest tests for polling enablement logic, query key structure, and rendering states

## 4. Engine Control Actions
- [x] 4.1 Add EngineControlBar component (colocated with trace detail page) with state-gated action buttons; purge enablement includes `CONTINUED_AS_NEW` alongside other terminal statuses
- [x] 4.2 Add SignalModal with required `signal_name` (non-empty after trim; whitespace-only is rejected) and optional JSON payload validation
- [x] 4.3 Add confirmation dialogs for cancel, terminate, and purge; purge dialog includes mode selection (`projection_only` default, `full` with destructive warning)
- [x] 4.4 Add inline repair feedback in the control area using existing alert/status styling: `repair_requested` success, `already_catching_up`/`already_up_to_date`/`no_events_to_project` info, `history_expired` warning; no global toast system
- [x] 4.5 Add inline purge feedback in the control area: `deleted=true` means applied, `deleted=false` means already satisfied; no global toast system
- [x] 4.6 Add in-flight mutation pending state: disable active action button and modal submit while request is in flight, show submitting indicator
- [x] 4.7 Add mutation failure handling: display inline error in control area for failed mutations, refetch trace detail and pending-work on `409` conflict responses, new error replaces any previous success/info/warning feedback
- [x] 4.8 Add post-mutation TanStack Query invalidation for trace detail, timeline, spans, pending-work, and trace-list queries on success
- [x] 4.9 Integrate EngineControlBar into TraceDetailPage for engine-backed traces
- [x] 4.10 Write Vitest tests for action state gating (including CONTINUED_AS_NEW for purge), signal trim validation, purge mode selection, confirmation flows, single-slot feedback model (success replacing error, error replacing success, info replacing warning), in-flight pending state, 409 conflict error handling, and query invalidation

## 5. Operator UX
- [x] 5.1 Add definition version mismatch banner to TraceDetailPage with exact-match on `error_code === 'definition_version_mismatch'`; banner copy: "This run failed because the engine definition version could not be matched during activation."
- [x] 5.2 Add ContinueAsNew navigation links from `continued_from_trace_id` and `continued_to_trace_id`; preserve `returnTo` location state through continuation hops
- [x] 5.3 Add `definition_version_mismatch` as a documented reserved value in the `EngineFailureSummary.error_code` OpenAPI schema description to keep contract and spec aligned
- [x] 5.4 Run `make generate` after the OpenAPI description change and verify generated code compiles
- [x] 5.5 Write Vitest tests for mismatch banner exact-match behavior, banner copy, continuation link rendering, and returnTo preservation through continuation hops

## 6. Verification
- [x] 6.1 Run `pnpm --filter web test` and confirm all new and existing web tests pass
- [x] 6.2 Run `go test ./internal/api/...` to confirm the OpenAPI description change does not break backend tests
