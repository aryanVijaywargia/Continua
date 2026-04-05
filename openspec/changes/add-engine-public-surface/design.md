## Context

Phase 11 delivered a dark-launched engine runtime inside `continua-engine` with three worker loops, single-transaction activation, deterministic replay, and CLI-only access. Phase 12 exposes this runtime through a public REST API hosted by `cmd/continua`, projects engine execution into the existing platform trace model, and surfaces engine state in the debugger UI.

The system keeps its two-binary architecture (`cmd/continua` for the platform server, `continua-engine` for the execution runtime) sharing a single Postgres instance. Engine tables live in the `engine` schema; platform tables live in `public`.

### Stakeholders
- Platform server (`cmd/continua`): hosts the public engine API, reads projected engine traces
- Engine runtime (`continua-engine`): owns execution, runs the projector loop
- Debugger UI (`web/src`): displays engine-backed traces alongside ingest traces

## Goals / Non-Goals

**Goals:**
- Public REST API for starting, inspecting, signaling, and canceling engine runs
- Engine execution state projected into existing platform `traces`/`spans`/`span_events` tables
- Engine-backed traces searchable, viewable, and comparable in the existing debugger
- Clean import boundary: platform imports `engine/db/gen/go` only, never `engine/internal/*`
- Start-request definition validation comes from a runtime-owned source of truth that both binaries can see
- Feature-gated rollout with preview-header protection on mutating routes

**Non-Goals:**
- `Session.engine` columns or session-list engine aggregate badges
- Automated projection cleanup (retention stays default-disabled during preview)
- Multi-primitive waits, ContinueAsNew, subworkflows
- SDK control surface (Python/TypeScript engine client)
- WebSocket-based live engine event streaming

## Decisions

### D1: Import boundary — platform uses public engine DB package

**Decision:** `cmd/continua` (and `internal/api`) import `engine/db/gen/go` (the sqlc-generated query package) to access engine tables for the public API. They never import `engine/internal/*`.

**Why:** The Go module boundary already prevents importing `engine/internal/`, but the generated query package is public by design. This lets the platform server wrap `enginedb.Queries` with project-scoped API semantics without duplicating engine SQL.

**Alternatives considered:**
- Hand-copy engine CAS SQL into platform queries — rejected: drift risk, dual maintenance
- Add a public Go API package in `engine/pkg/control` — rejected: over-abstraction for Phase 12 scope; the generated queries are already the right level

**How to apply:** When new engine-table queries are needed for the public API, extend `engine/db/queries/*.sql` and regenerate. The root-side control layer wraps `enginedb.Queries` with project-scoped validation.

### D2: Runtime-published definition catalog and public history DTOs

**Decision:** `continua-engine` publishes its in-process workflow registry into an engine-owned table (`engine.definition_catalog`) at startup before serving runtime traffic. `cmd/continua` validates `definition_name` + `definition_version` against that table inside the start transaction before claiming dedupe or creating any engine/public rows. Event type constants and payload DTOs that root-side code must construct or parse move to a public engine package (for example `engine/pkg/history`); root-side code does not import `engine/internal/history`.

**Why:** The live registry currently exists only inside `continua-engine` internals, so the platform server has no direct source of truth for “registered in the engine runtime.” The start handler also needs to append `workflow.started`, which currently depends on internal-only event constants and payload types.

**Alternatives considered:**
- Relax pre-validation and let activation fail later — rejected: contradicts the Phase 12 API contract and creates avoidable churn in idempotency/session/trace shells
- Import `engine/internal/workflow` or `engine/internal/history` from platform code — rejected: blocked by Go `internal/` boundaries and contrary to the intended module contract
- Hard-code definitions in both binaries — rejected: duplicates runtime configuration and invites drift

### D3: Shared-DB projection model

**Decision:** The engine projector writes directly into `public.*` tables (`traces`, `spans`, `span_events`) using handwritten cross-schema SQL. This projection SQL is isolated to one engine-local package/file group with header comments citing the authoritative platform schema inputs.

**Why:** Keeping projection inside the engine binary lets the engine own the mapping from history events to platform-visible state. The platform read path doesn't need to know about engine internals — it reads projected rows like any other trace.

**Alternatives considered:**
- Platform-side projection worker — rejected: requires platform to import engine internals or maintain a parallel event interpreter
- Event bus / message queue between binaries — rejected: unnecessary infrastructure for a shared-Postgres deployment

### D4: Two-writer ordering contract

**Decision:** Three writers may touch `public.traces` for an engine-backed trace, with strict ownership:

1. **Start handler** (in `cmd/continua`): Creates engine run + projected shells (trace, root span, session) in one transaction. Owns initial metadata seeding. Sets projection state to `up_to_date`.
2. **Terminal activation** (in `continua-engine`): Within the activation transaction that completes/fails/cancels the run, writes terminal summary fields on the projected trace and root span. Authoritative for terminal status, `end_time`, output/failure summary.
3. **Async projector** (in `continua-engine`): Owns journal detail rows (per-activity spans, semantic events), counters, and `engine_last_projected_history_id`. Must not overwrite terminal summary fields on terminal traces; uses SQL guards to only advance detail/counter fields.

**Why:** The start handler must seed shells synchronously so the trace is immediately visible. The terminal activation must set status atomically with the engine state transition. The async projector handles the detail-heavy work that can lag slightly.

**Ordering invariant:** Once a trace is terminal, the projector's SQL guards prevent regression of `traces.status`, `traces.end_time`, or terminal output/failure summary.

### D5: Per-trace projection state tracking

**Decision:** Projection freshness is tracked per-trace using `engine_latest_history_id` and `engine_last_projected_history_id` columns, not via a project-wide checkpoint alone.

**Why:** Per-trace tracking lets the read path determine projection freshness for individual traces without scanning the full project history. It also handles the common case where most traces are `up_to_date` while one or two are `catching_up`.

**State machine:**
```
up_to_date ──(new history appended)──> catching_up
catching_up ──(projector catches up)──> up_to_date
up_to_date ──(retention cleanup)──> summary_only
summary_only ──(journal expired)──> journal_expired
```

### D6: Deterministic projected identifiers

**Decision:** Engine-backed traces and spans use formulaic external IDs rather than random UUIDs:
- `traces.trace_id = "engine:" + run_id`
- root span `span_id = "engine:root:" + run_id`
- per-activity span `span_id = "engine:activity:" + run_id + ":" + activity_key`
- `sessions.external_id = request.session.key` when provided, else `instance_key`

**Why:** Deterministic IDs make projection idempotent (safe to re-project) and debuggable (IDs reveal their source). They also prevent duplicate rows on projector restart.

**Phase-12 limit:** The current engine history/schema model does not expose a stable attempt identity at `activity.scheduled` time. Phase 12 therefore projects one span per logical `activity_key`. A later phase may split retries into attempt-specific spans only after the engine model carries stable attempt IDs.

### D7: Span kind/name contract for debugger parity

**Decision:** Phase 12 locks projected span shape to the current debugger’s existing span taxonomy:
- root workflow span: `kind = CHAIN`, `name = projected trace name`
- projected activity span: `kind = TOOL`, `name = activity_type`

**Why:** The current API contract only permits `LLM | TOOL | CHAIN | AGENT | CUSTOM`, and existing debugger heuristics already treat `TOOL` spans as external work and `CHAIN` spans as orchestration. Leaving kind/name open would create churn as soon as compare, stall analysis, or span search is implemented.

### D8: Semantic projection emits only contract-complete payloads

**Decision:** The projector only emits semantic `decision` / `effect` / `wait` events when it can satisfy the existing debugger payload contract exactly. Phase 12 mapping is:
- `activity.scheduled` emits:
  - `effect` with `{ effect_kind: "activity", has_external_side_effect: true, effect_id: "activity:" + activity_key }`
  - `wait` with `{ wait_kind: "activity", phase: "entered", wait_id: "activity:" + activity_key }`
- `activity.completed` emits:
  - `decision` with `{ question: "activity:" + activity_key + ":outcome", chosen: "completed" }`
  - `wait` with `{ wait_kind: "activity", phase: "resolved", wait_id: "activity:" + activity_key, resolution: "completed" }`
- `activity.failed` emits:
  - `decision` with `{ question: "activity:" + activity_key + ":outcome", chosen: "failed", reasoning: error_message }`
  - `wait` with `{ wait_kind: "activity", phase: "resolved", wait_id: "activity:" + activity_key, resolution: "failed" }`
- `timer.scheduled` emits `wait` with `{ wait_kind: "timer", phase: "entered", wait_id: "timer:" + timer_key }`
- `timer.fired` emits `wait` with `{ wait_kind: "timer", phase: "resolved", wait_id: "timer:" + timer_key, resolution: "fired" }`
- `signal.received`, `cancel.requested`, `custom_status.updated`, and other rows that do not have a full wait/effect/decision contract stay `custom` with the original engine event type preserved in metadata

**Why:** The current compare/debugger code already keys semantic pairing and wait resolution off `question`, `effect_id`, `wait_id`, `phase`, and `resolution`. Merely classifying history rows is not enough.

**Phase-12 limit:** The current engine history model does not record a signal-wait “entered” row, so live signal waits are surfaced through `TraceDetail.engine.wait_state` fallback rather than projected timeline `wait` rows.

### D9: Rollout gating

**Decision:** Two layers of gating:
- `ENGINE_PUBLIC_API_ENABLED=true` env var enables the entire `/v1/engine/*` route group
- Mutating routes (`POST .../runs`, `POST .../signal`, `POST .../cancel`) additionally require `X-Continua-Engine-Preview: 1` header

**Why:** The env var is a kill switch. The preview header prevents accidental client usage during early rollout without requiring a separate auth mechanism.

## Risks / Trade-offs

- **Cross-schema SQL drift**: The projector's handwritten SQL depends on the platform schema. Mitigated by header comments citing source schemas and integration tests that exercise the projection path.
- **Two-writer race on terminal traces**: If the projector runs after the terminal activation but before the trace becomes visible, it could attempt to overwrite terminal fields. Mitigated by SQL guards that check terminal status before writing.
- **Projection lag visibility**: Users may see stale projection state. Mitigated by `catching_up` state and UI banners.
- **Signal wait history gap**: The current engine history only records signal resolution (`signal.received`), not signal wait entry. Mitigated in Phase 12 by surfacing live wait state through the trace-detail engine fallback and keeping non-contract-complete timeline rows as `custom`.
- **Engine binary must know platform schema**: The projector package will import or reference platform table shapes. This is acceptable because both binaries share the same Postgres instance and deployment lifecycle.

## Migration Plan

1. **12a** — Add trace linkage migration, publish an engine-owned definition catalog, expose public engine history DTOs, establish root-side engine query wrappers, close Phase 11 hardening tests
2. **12b** — Add OpenAPI routes, implement handlers in `internal/api/`, wire through existing auth
3. **12c** — Add projector loop, extend read schemas, add UI surfaces
4. Rollback: disable `ENGINE_PUBLIC_API_ENABLED`, drop linkage columns via down migration

## Open Questions

- **Instance reuse policy**: Phase 11 treats each `instance_key` as single-run (second start with different `request_key` returns `instance_conflict`). If future phases need multi-run instances (ContinueAsNew), the start handler's conflict semantics will need revision. Phase 12 preserves the Phase 11 single-run behavior.
- **Session metadata merge depth**: The spec defines shallow-merge for `session.metadata` on upsert. If nested metadata becomes common, a deeper merge strategy may be needed. Phase 12 starts with shallow-merge to match the existing platform upsert pattern.
- **Projection cleanup trigger**: `summary_only` and `journal_expired` states are modeled but automated cleanup stays default-disabled. The trigger mechanism (TTL-based vs explicit API) is deferred.
- **Signal-wait timeline parity**: Full signal wait lifecycle events in the timeline are deferred until the engine history model records wait entry (or another stable source of wait-entry truth is introduced).
- Session-level engine metadata and automated retention cleanup are explicitly deferred.
