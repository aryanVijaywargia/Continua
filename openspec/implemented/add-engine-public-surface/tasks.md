## Slice 12a: Foundation, Boundaries, And Projection State

### Phase 11 hardening prerequisite
- [x] 1.1 Close the three remaining Phase 11 restart-hardening tests (prerequisite before 12a is considered complete)

### Trace linkage migration
- [x] 1.2 Create platform migration `000013_engine_trace_linkage.up.sql` adding `engine_run_id` (UUID, unique index), `engine_definition_name`, `engine_definition_version`, `engine_projection_state`, `engine_latest_history_id`, `engine_last_projected_history_id`, `engine_projection_updated_at` to `traces`
- [x] 1.3 Create corresponding `.down.sql` dropping the linkage columns
- [x] 1.4 Run `make generate` to regenerate sqlc models with new columns
- [x] 1.4a Add follow-up platform migration `000014_engine_instance_key.up.sql` to persist projected `instance_key` for `TraceDetail.engine`
- [x] 1.4b Add follow-up platform migration `000015_engine_projected_summary.up.sql` to persist projected run summary fields required by `TraceDetail.engine`
- [x] 1.4c Add follow-up platform migration `000016_engine_projected_summary_nullable.up.sql` to restore NULL engine-summary counters for non-engine traces
- [x] 1.5 Test: verify non-engine trace creation leaves all engine columns NULL

### Definition catalog and public boundary
- [x] 1.6 Create engine migration `000003_definition_catalog.up.sql` adding `engine.definition_catalog` keyed by `(definition_name, definition_version)` with publication timestamps
- [x] 1.7 Create corresponding `.down.sql`
- [x] 1.8 Add engine sqlc queries for publishing and reading the definition catalog; run `make generate`
- [x] 1.9 Add a public engine history package (for example `engine/pkg/history`) exposing shared event constants and payload DTOs needed by root-side start/read paths
- [x] 1.10 Update `continua-engine` bootstrap to publish the currently registered definitions into `engine.definition_catalog` before serving runtime traffic
- [x] 1.11 Test: published catalog matches the runtime registry and root-side code can validate definitions / construct `workflow.started` without importing `engine/internal/*`

### Root-side engine query wrappers
- [x] 1.12 Audit existing `engine/db/queries/*.sql` for reusable queries; add only missing project-scoped queries needed for root-side control (including definition-catalog reads and project-scoped instance/run lookups not already covered by existing generated methods)
- [x] 1.13 Run `make generate` to regenerate engine sqlc package
- [x] 1.14 Add root-side engine control wrapper (in `internal/api/` or `internal/engine/`) that imports `engine/db/gen/go` and wraps `enginedb.Queries` with project-scoped validation
- [x] 1.15 Test: root-side wrappers correctly enforce project scoping against engine dedupe/CAS semantics

### Projection state contract
- [x] 1.16 Implement projection-state transition logic: `up_to_date` ↔ `catching_up` via history-id comparison, and model `up_to_date` → `summary_only` → `journal_expired` for deferred retention cleanup
- [x] 1.17 Test: active projection-state transitions and per-trace history-id advancement behave correctly across start, activation, and projector operations; retention-state helpers are unit-tested while automated cleanup remains deferred

## Slice 12b: Public Engine APIs

### OpenAPI and route registration
- [x] 2.1 Add engine API routes to `contracts/openapi/openapi.yaml`: `POST /v1/engine/runs`, `GET /v1/engine/instances/{instance_key}`, `GET /v1/engine/runs/{run_id}`, `GET /v1/engine/runs/{run_id}/result`, `GET /v1/engine/runs/{run_id}/history`, `POST /v1/engine/runs/{run_id}/signal`, `POST /v1/engine/runs/{run_id}/cancel`
- [x] 2.2 Add request/response schemas for each engine endpoint
- [x] 2.3 Run `make generate`

### Rollout gating
- [x] 2.4 Add `ENGINE_PUBLIC_API_ENABLED` env var to `internal/config/config.go`
- [x] 2.5 Implement engine route availability gating conditional on the env var (pre-auth 404 behavior; middleware or conditional registration acceptable)
- [x] 2.6 Implement `X-Continua-Engine-Preview: 1` header check middleware for mutating routes
- [x] 2.7 Test: env gating returns 404 when disabled; preview header returns 400 when missing on POST routes; GET routes work without preview header

### Start run handler
- [x] 2.8 Implement `POST /v1/engine/runs` handler with atomic transaction: definition-catalog validation, dedupe claim, instance create, run create, `workflow.started` history append via public engine history DTOs, session upsert, trace create, root span shell create
- [x] 2.9 Set deterministic projected identifiers: `trace_id = "engine:" + run_id`, `span_id = "engine:root:" + run_id`, `session.external_id` from request or `instance_key`
- [x] 2.10 Set engine linkage columns on the projected trace: `engine_run_id`, `engine_definition_name`, `engine_definition_version`, `engine_projection_state = up_to_date`, `engine_latest_history_id = engine_last_projected_history_id = workflow.started.id`
- [x] 2.11 Test: start dedupe replay returns original response; instance conflict returns 409; unregistered definition returns 400 before creating rows; session upsert merges name/metadata on existing session; atomic transaction creates engine rows and projected shells together; missing required fields return 400

### Read handlers
- [x] 2.12 Implement `GET /v1/engine/instances/{instance_key}` returning scoped instance + latest run summary
- [x] 2.13 Implement `GET /v1/engine/runs/{run_id}` returning run detail
- [x] 2.14 Implement `GET /v1/engine/runs/{run_id}/result` returning terminal result or 409 for non-terminal
- [x] 2.15 Implement `GET /v1/engine/runs/{run_id}/history` with cursor pagination (`after`, `limit` with default 100 / max 1000, `has_more`)
- [x] 2.16 Test: cross-project 404 on all engine routes; run detail, result, history content correctness

### Signal and cancel handlers
- [x] 2.17 Implement `POST /v1/engine/runs/{run_id}/signal` creating inbox signal and waking if waiting
- [x] 2.18 Implement `POST /v1/engine/runs/{run_id}/cancel` creating inbox cancel with dedupe key `"cancel:" + run_id` and waking if waiting
- [x] 2.19 Test: signal/cancel on waiting run triggers wake; signal/cancel on terminal run returns 409 `run_terminal`; duplicate cancel on active run is idempotent; cross-project 404

## Slice 12c: Projection And Debugger Integration

### Projector loop
- [x] 3.1 Add projector polling loop to `continua-engine serve` (fourth loop alongside workflow, activity, maintenance)
- [x] 3.2 Projector polls for traces where `engine_last_projected_history_id < engine_latest_history_id`, reads new history, and projects into `public.*`
- [x] 3.3 Isolate all cross-schema projection SQL in one engine-local package/file group with header comments citing platform schema dependencies

### Projection event mapping
- [x] 3.4 Implement `activity.*` mapping: create/update per-activity spans with deterministic IDs (`engine:activity:run_id:activity_key`), `kind = TOOL`, and `name = activity_type`
- [x] 3.5 Implement root workflow span shape: `kind = CHAIN`, `name = projected trace name`, and deterministic `span_events.idempotency_key` values derived from `(run_id, history_id, projection_variant)`
- [x] 3.6 Implement explicit semantic payload mapping that conforms to the current debugger contract:
  - `activity.scheduled` => `effect` (`effect_id = activity:<activity_key>`) plus `wait entered`
  - `activity.completed` / `activity.failed` => `decision` (`question = activity:<activity_key>:outcome`) plus `wait resolved`
  - `timer.scheduled` / `timer.fired` => `wait entered` / `wait resolved`
- [x] 3.7 Map `signal.received`, `cancel.requested`, `custom_status.updated`, and other non-contract-complete engine history rows through `custom` with original engine event type in payload metadata; surface live signal waits through `TraceDetail.engine.wait_state` fallback instead of projected wait rows in Phase 12

### Two-writer ordering
- [x] 3.8 Terminal activation transaction writes terminal summary fields on projected trace/root span within the activation transaction
- [x] 3.9 Projector SQL uses guards: do not overwrite `traces.status`, `traces.end_time`, or terminal output/failure summary on terminal traces
- [x] 3.10 Test: writer-ordering regression — terminal projection cannot be overwritten by later async catch-up

### Checkpoint advancement
- [x] 3.11 Advance `engine_last_projected_history_id` after successful projection; transition `engine_projection_state` to `up_to_date` when caught up
- [x] 3.12 Test: projector checkpoint monotonicity and restart safety

### Extended read schemas
- [x] 3.13 Add `Trace.engine` (optional, minimal: `run_id`, definition info, `projection_state`) to OpenAPI `Trace` schema
- [x] 3.14 Add `TraceDetail.engine` (full engine summary, wait state, pending work counts, custom status, terminal result/failure summary) to OpenAPI `TraceDetail` schema
- [x] 3.15 Add `CompareTraceHeader.engine` (minimal: `run_id`, definition info, `projection_state`) to compare header schema
- [x] 3.16 Add `TimelineResponse.engine` (projection-state metadata only) to timeline schema
- [x] 3.17 Run `make generate` and update API mappers
- [x] 3.18 Implement trace-detail fallback: read engine summary via root-side helper keyed by `engine_run_id` when `engine_projection_state` is `catching_up` or `summary_only`; serve shell-only for `journal_expired`
- [x] 3.19 Test: projected activity/timer semantic events deserialize through the existing timeline contract; live signal waits remain visible through `TraceDetail.engine.wait_state`; engine-backed traces appear in trace/session/detail/compare flows; non-engine traces unchanged

### Debugger UI
- [x] 3.20 Trace list: display engine badge on traces with non-null `engine` object
- [x] 3.21 Trace detail: show engine wait-state summary when run is `waiting`
- [x] 3.22 Trace detail: show projection-state banners for `catching_up`, `summary_only`, `journal_expired`
- [x] 3.23 Session detail: engine-backed traces appear in existing trace table with engine badges
- [x] 3.24 Compare: show engine metadata in compare headers when present
- [x] 3.25 Verify: non-engine traces, sessions, and existing flows remain visually unchanged
