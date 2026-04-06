# Design: Engine Retention, Repair, And Control SDK

## Context

This is the second operator-tooling layer built on top of `add-engine-operational-hardening-core` (Proposal 1). Proposal 1 introduces `TERMINATED`, `workflow.cancelled` / `workflow.terminated` history events, the operator terminate handler, the pending-work read endpoint, authoritative instance status, and projector-owned terminal debugger cleanup. Those contracts MUST be frozen before this change begins.

This proposal adds:

- manual purge (`projection_only` / `full`) for terminal runs
- per-run async repair request
- a platform-side env-driven retention maintenance job with two stages
- additive trace-search filters on existing engine metadata columns
- a separate Python control client that wraps the frozen control surface
- documentation + regression test for `definition_version_mismatch` surfacing (already baseline from Proposal 1, not new work)

It deliberately stays additive: no new trace-shell columns, no trace status enum migrations, no public bulk APIs, no projection schema changes.

## Goals / Non-Goals

### Goals
- Give operators manual purge control with two explicit modes (projection_only, full)
- Give operators manual repair to rebuild detail from retained engine history
- Bound projection and history growth automatically via env-driven retention
- Let operators filter traces by engine instance key, definition name, run status, and projection state from the existing trace list endpoint
- Expose the full frozen control surface to Python as a first-class client module
- Keep non-engine traces, sessions, ingest, and happy-path engine flows unchanged

### Non-Goals
- ContinueAsNew
- suspend/resume
- activity heartbeats
- TypeScript control SDK
- WebSocket runtime
- public bulk backfill APIs
- definition-catalog lookups for mismatch remediation
- `engine_wait_kind` storage column
- `engine_last_error_code` storage column
- `projection_purged_at` storage column
- any new trace status enum values

## Decisions

### Decision: Purge modes use existing projection states as barriers
- Purge `projection_only` → deletes `public.span_events` for the trace + all non-root `public.spans`, keeps trace row + root span, flips `engine_projection_state` to `summary_only`.
- Purge `full` → same projection purge, then deletes `engine.history` rows for the run, flips `engine_projection_state` to `journal_expired`.
- No new state values are introduced; `summary_only` and `journal_expired` already exist in the Proposal 1 baseline.
- **Alternatives considered:** a dedicated `purged` state; or storing `projection_purged_at` as a separate column. Rejected because the existing state machine already captures the two post-purge regimes, and `engine_projection_updated_at` already records when the state last changed. Adding new columns or states would require a migration that this phase explicitly avoids.

### Decision: Projector treats `summary_only` and `journal_expired` as hard write barriers via a centralized write wrapper
- The projector checks `engine_projection_state` before every detailed projection write. If the trace is in `summary_only` or `journal_expired`, the projector skips detail writes and does not advance the checkpoint for that run beyond what the barrier allows.
- **Implementation requirement:** The barrier check MUST be centralized in a single write wrapper that reads/locks the trace row within the same transaction as the write. Today the projector fans out through multiple write helpers (`projectHistoryRows`, terminal projection, etc.) without a shared trace-lock wrapper. This change requires introducing one wrapper that every detail-write path calls, so no code path can accidentally skip the barrier check.
- This matches Proposal 1's existing "projector is the single writer for projection tables" invariant and prevents a late catching_up loop from recreating detail rows that purge just deleted.
- **Alternatives considered:** deleting the detail again on every projector loop (wasteful, racy); a CAS checkpoint that could be advanced past the barrier (breaks determinism); checking the barrier in each write helper individually (error-prone, easy to miss a code path). Rejected.

### Decision: Purge/catching_up race is resolved by barrier CAS under row lock
- Purge opens a transaction, locks the `public.traces` row (`SELECT ... FOR UPDATE`), reads the current `engine_projection_state`, flips it to `summary_only` or `journal_expired`, and deletes the detail rows inside the same transaction. The projector checks the barrier before every write; if the projector was mid-catchup, its next write hits the barrier and aborts that iteration.
- **Alternatives considered:** advisory locks on `run_id`; a separate `purge_pending` flag. Rejected because the existing row lock on `public.traces` is enough to serialize the two writers, and a flag would introduce another column.

### Decision: Purge is terminal-only; non-terminal runs return 409
- Purge rejects runs whose `engine_run_status` is not `completed`, `failed`, `cancelled`, or `terminated`.
- The service reuses the existing typed engine error code `run_not_terminal` rather than inventing a purge-specific variant.
- This preserves the invariant that only terminal runs have a stable final shell; a non-terminal run would lose in-flight detail rows that the projector is still writing.
- **Alternatives considered:** waiting for terminal inside the handler; allowing `waiting` runs. Rejected — operators can terminate first via Proposal 1, then purge.

### Decision: Repair is an async projector-resume request, scoped to catching_up recovery
- The current repo runs the projector loop only inside the separate `continua-engine` process. The platform server cannot synchronously call `engine/internal/projector` without either moving code out of `engine/internal/`, introducing RPC, or duplicating projector logic.
- Repair therefore does NOT synchronously drive catch-up from the API process. Instead, repair on `summary_only` checks whether retained history exists beyond the checkpoint. If it does, the service flips `engine_projection_state` back to `catching_up` and returns an accepted response; the existing `continua-engine` projector loop later reads the resumed trace and rebuilds detail from `engine_last_projected_history_id + 1`.
- If the checkpoint already equals `engine_latest_history_id` (trace was fully `up_to_date` before purge), repair returns `{accepted: false, reason: "no_events_to_project"}` and preserves `summary_only`. This means repair never "undoes" an operator's deliberate purge of a fully-projected trace.
- If the trace is already `catching_up`, repair returns `{accepted: true, reason: "already_catching_up"}` and relies on the existing projector loop already in flight.
- Full checkpoint-rewind (reproject from zero), a synchronous in-process repair entry point, and RPC into `continua-engine` are explicitly deferred to future changes.
- **Alternatives considered:** extracting a callable shared projector package, introducing RPC/HTTP to `continua-engine`, or always rewinding the checkpoint to 0. Rejected for this phase because async resume is the only directly implementable choice against the current runtime split without materially expanding scope.

### Decision: Purge and repair orchestration are extracted into a shared Fx-provided service
- Today `engineControlService` is constructed privately inside `internal/api/newConfiguredServer`, which prevents `internal/jobs` from depending on the same orchestration logic.
- This change therefore requires extracting purge/repair orchestration into a shared service that is provided separately through Fx (for example `internal/enginecontrol/`), and then injecting that service into both the API server and the retention worker.
- The API layer remains responsible for auth, request parsing, and HTTP error mapping. The shared service owns the transaction, store coordination, and typed domain errors for purge/repair.
- **Alternatives considered:** keep the service private to `internal/api` and have jobs call HTTP, or duplicate the logic in jobs. Rejected because both add coupling and duplicate failure modes for no benefit.

### Decision: Retention is env-only with fail-fast validation, owned by the platform server
- `ENGINE_PROJECTION_RETENTION_AFTER` and `ENGINE_HISTORY_RETENTION_AFTER` are env durations (Go `time.ParseDuration`-compatible strings like `168h`, `720h`).
- Empty or `0` disables that stage. `ENGINE_HISTORY_RETENTION_AFTER` without `ENGINE_PROJECTION_RETENTION_AFTER`, or history ≤ projection, is a startup error.
- Retention config lives in `internal/config/config.go`, not in `engine/internal/config/config.go`.
- The current repo runs the platform server from `cmd/continua` and the engine maintenance/projector loops from the separate `continua-engine` binary. Because purge logic already lives root-side in `internal/api/engine_control.go` and touches both `public.traces` and `engine.history`, retention must run in the same root process instead of trying to inject a callback across a process boundary.
- **Alternatives considered:** database-backed retention config, per-project overrides, engine-side maintenance via callback injection, engine-to-platform HTTP self-calls. Rejected for this phase — env config is sufficient, and root-side ownership is the only approach that is directly implementable against the current binary split without introducing RPC.

### Decision: Retention runs as a single platform-side River maintenance job with two stages
- One periodic maintenance job in the platform server scans terminal runs and performs stage 1 purge on runs past the projection window, then stage 2 purge on runs past the history window.
- The job runs on the existing River maintenance surface already owned by `internal/jobs`, not inside `continua-engine`.
- For each candidate, the worker calls the shared root-side purge service directly in-process. It does NOT self-call the public HTTP endpoint and does NOT duplicate purge SQL in a separate retention path.
- In this phase, the periodic retention job runs once every 24 hours, with `RunOnStart` disabled.
- The periodic job also uses active-state uniqueness on its job args so duplicate enqueues from multiple River clients collapse to one active job, while an advisory lock (or equivalent DB-backed serialization guard) still gates execution so multiple platform instances cannot run overlapping retention iterations even if a duplicate slips through.
- **Alternatives considered:** two separate workers, per-run cron schedules, engine-side maintenance, HTTP self-call to the platform purge endpoint. Rejected — a single platform-side job is simpler, matches the current runtime split, and reuses the existing maintenance infrastructure.

### Decision: Retention candidate selection lives in root-side handwritten store code
- Candidate selection needs both `engine.runs.completed_at` and `public.traces.engine_projection_state`, so it spans the engine and platform schemas.
- The current sqlc inputs are intentionally schema-local: `engine/db/sqlc.yaml` loads only engine migrations, and `db/platform/sqlc.yaml` loads only platform migrations. The retention join therefore SHOULD live in root-side handwritten store code (`internal/store/`) rather than adding cross-schema sqlc coupling in this phase.
- The engine sqlc layer still owns the engine-only `DeleteHistoryByRun` query.
- **Alternatives considered:** adding `public` schema inputs to engine sqlc, or adding engine schema inputs to platform sqlc. Rejected for this phase — both increase generation coupling for a single cross-schema retention query that is straightforward to express in handwritten root-side SQL.

### Decision: Retention idempotency is driven by projection state
- Stage 1 only touches runs whose current `engine_projection_state` is `up_to_date` or `catching_up`. After stage 1, a run is `summary_only`; re-entering the loop is a no-op for that run.
- Stage 2 only touches runs whose current `engine_projection_state` is `summary_only` (or `up_to_date`/`catching_up` — stage 2 can apply `full` purge directly for runs older than the history window and skip stage 1). After stage 2, a run is `journal_expired`; re-entering the loop is a no-op.
- This makes crash/restart safe without additional tracking tables.
- **Alternatives considered:** a dedicated retention log table. Rejected — unnecessary complexity.

### Decision: Trace search filters are additive in the handwritten builder
- The existing `internal/store/search.go` handwritten dynamic SQL builder is the only changed surface. Four filters (`engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`) are added as optional predicates.
- Queries without engine filters produce identical SQL to today.
- Queries with engine filters naturally exclude non-engine traces because those columns are `NULL` and the predicates are equality.
- **Alternatives considered:** a dedicated engine-traces endpoint; a view. Rejected — additive filters on the existing endpoint preserve the current URL-driven UX and keep the SQL path coherent.

### Decision: `engine_history_expired` is a typed marker distinguishing purged-empty from never-had-events
- The existing `GET /v1/engine/runs/{run_id}/history` endpoint already returns HTTP 200 with whatever history rows exist (empty `events: []` when none match). Today there is no way to distinguish "this run had history that was purged" from "this run hasn't produced events yet or is fully paginated."
- This change adds an explicit `expired: true` marker in the response body when the run's `engine_projection_state` is `journal_expired`. Non-expired runs omit the marker or return `expired: false`.
- The 404 case is unchanged: it means the run does not exist or belongs to a different project.
- **Alternatives considered:** HTTP 410 Gone; a separate `/v1/engine/runs/{id}/history-status` endpoint. Rejected — 410 is opaque to many clients, and a separate endpoint would double the round-trip cost for no gain.

### Decision: Mismatch UX reuses existing fields
- The engine run detail response already carries `definition_name`, `definition_version`, and a `failure` object (per Proposal 1). When the failure code is `definition_version_mismatch`, clients can surface the mismatch without a new endpoint.
- No definition-catalog lookups, no "available versions" hint, no mismatch-specific route.
- **Alternatives considered:** a dedicated `/v1/engine/runs/{id}/mismatch` endpoint; a catalog endpoint. Deferred to a later phase.

### Decision: Python control client is a separate module
- `sdks/python/src/continua/engine_control.py` (or `control/engine.py`) is a standalone client class that takes `endpoint` + `api_key` (consistent with the existing ingest `Client`) and exposes engine control methods.
- It does NOT share state with `Client` (the ingest batching client) and does NOT inject itself into batching paths.
- **Alternatives considered:** add control methods to `Client`. Rejected — ingest batching and engine control have different lifetimes, auth reuse is fine but batching semantics are irrelevant.

### Decision: `wait_for_terminal()` is polling-only with a default 1s interval
- No WebSocket/SSE dependency. Polls `GET /v1/engine/runs/{run_id}/result` (or the run detail endpoint) every 1s by default.
- Accepts an optional timeout; raises a timeout error when the deadline is exceeded.
- Returns the final run summary/detail object (the same shape as `get_result()` for a terminal run).
- **Alternatives considered:** server-side long-poll. Deferred — polling is the baseline the platform already supports.

## Risks / Trade-offs

- **Lossy repair for terminal-then-up_to_date runs:** after projection_only purge on a trace that was fully projected, the checkpoint is at the terminal row. Repair returns `{accepted: false, reason: "no_events_to_project"}` and leaves the trace as `summary_only`. Full checkpoint rewind is explicitly deferred.
  - **Mitigation:** document that fully-projected-then-purged traces retain the summary shell only; checkpoint rewind is a separate future proposal if operators request it.

- **Async repair latency:** a successful repair request only flips the trace back to `catching_up`; detail rebuild completes later when the separate `continua-engine` projector loop reaches that run.
  - **Mitigation:** make the async contract explicit in the API/SDK, return `projection_state: catching_up` on accepted repair requests, and direct callers to poll the existing read endpoints for completion.

- **Retention scan load:** the platform-side retention worker scans terminal runs by completion time. On a large database this could be expensive if the index does not exist.
  - **Mitigation:** this phase includes an index-only migration on `engine.runs(completed_at) WHERE status IN ('completed','failed','cancelled','terminated')`. The current engine indexes stop at instance/claim paths (see `000001_engine_foundation.up.sql`); the retention index is explicitly required. Schedule retention at low-traffic intervals.

- **Projector barrier enforcement correctness:** the projector must check the barrier on every write. If a code path skips the check, purge can be silently undone.
  - **Mitigation:** centralize the barrier check in a single projector-write wrapper. Add tests that run catching_up + purge concurrently and assert detail stays deleted.

- **Search filter SQL drift:** adding filters to the handwritten builder risks SQL injection or planner regressions if done carelessly.
  - **Mitigation:** follow the existing bind-variable pattern in `search.go`; validate enum inputs against known values before binding; add EXPLAIN-based tests for the new filter combinations.

- **Python control client drift:** the control client wraps endpoints that Proposal 1 introduced. If those endpoints change shape during Proposal 1's implementation, the client must track.
  - **Mitigation:** the client builds on the frozen OpenAPI surface; regenerate types from the same OpenAPI schema via `make generate` and reuse them in the client.

- **Purge vs late activation race:** a terminate/cancel commit could in theory race with a purge on the same run.
  - **Mitigation:** purge is terminal-only; by the time a run is terminal, no further activation can commit. Terminate + purge are sequenced by the operator (terminate first, wait for terminal, then purge). The barrier CAS + row lock handle any residual projector write race.

- **History endpoint contract expansion:** clients that relied on 404 for missing history may need to handle the new `expired: true` marker.
  - **Mitigation:** the existing 404 case (run does not exist) is preserved; only `journal_expired` runs get the new marker. Document the marker in OpenAPI; the marker is additive to an existing response body.

## Migration Plan

- No table schema migrations on `public.traces` or `engine.*` tables in this phase. One index-only migration on `engine.runs(completed_at)` filtered by terminal statuses is required for retention candidate queries.
- Retention env config lives in `internal/config/config.go`: operators must set `ENGINE_PROJECTION_RETENTION_AFTER` / `ENGINE_HISTORY_RETENTION_AFTER` explicitly to opt in; default behavior is unchanged (retention disabled).
- Purge / repair / filters / Python client are additive; no existing client contract is broken.
- Rollback: remove retention env vars, redeploy; purge/repair handlers may remain in place but are unused. If the projector barrier check needs to be disabled for emergency rollback, flag it via a runtime config override (out of scope for this phase; document the code location).

## Resolved Questions

- **Repair on fully-projected-then-purged traces:** Decided: leave the checkpoint in place and return `{accepted: false, reason: "no_events_to_project"}`. Repair never "undoes" an operator's deliberate purge. Full checkpoint rewind is deferred to a separate proposal.
- **Session-pinning for retention:** Decided: retention is purely time-based. Session-pinning is out of scope.
- **Purge for non-engine traces:** Decided: no — the endpoint is under `/v1/engine/runs/{run_id}`, and non-engine traces do not have a run id. Non-engine trace cleanup is a separate concern.
- **Retention on boot:** Decided: schedule only, to avoid thundering-herd on boot. Crash-safety is handled by idempotent state transitions, not boot scans.
