## 0. Prep: Baseline Stabilization and Decision Record

- [x] 0.1 Restore `pnpm --filter web test` baseline: fix any broken Vitest tests or TypeScript type errors caused by engine trace-shell column additions in the regenerated TypeScript client; run `pnpm --filter web test` and confirm green
- [x] 0.2 Record maintenance ownership rules in `.codex/references/decisions.md`: engine maintenance owns due-timer wakeups for non-suspended runs and request-dedupe expiry; activity retries use durable `available_at` on activity tasks, not a new maintenance loop; root-side maintenance owns retention and bulk backfill triggering
- [x] 0.3 Run `make generate` and confirm no drift after prep changes

**Validation:** `pnpm --filter web test` passes; decisions.md includes the maintenance ownership section; `make generate` is clean

## 1. Schema Migration

- [x] 1.1 Add engine migration adding `suspended` to `engine.run_lifecycle_status` enum via `ALTER TYPE engine.run_lifecycle_status ADD VALUE IF NOT EXISTS 'suspended'`
- [x] 1.2 Run `make generate` for the engine sqlc layer and confirm the generated Go enum includes `EngineRunLifecycleStatusSuspended`
- [x] 1.3 Confirm that the existing `ClaimNextRun` query (which claims `queued` or expired-lease `running`) does NOT match `suspended` runs without any query change

**Validation:** migration applies cleanly; generated enum includes `suspended`; claim query naturally skips suspended runs

## 2. Engine Queries

- [x] 2.1 Add `TransitionRunToSuspended :one` in `engine/db/queries/runs.sql`: CAS on `status IN ('queued', 'waiting')`, transition to `suspended`, clear `claimed_by`/`claimed_at`/`lease_expires_at`, set `updated_at = NOW()`, return the updated row
- [x] 2.2 Add `TransitionRunToQueuedFromSuspended :one` in `engine/db/queries/runs.sql`: CAS on `status = 'suspended'`, transition to `queued`, clear `waiting_for`/`claimed_by`/`claimed_at`/`lease_expires_at`, set `ready_at = NOW()`, `updated_at = NOW()`, return the updated row
- [x] 2.3 Widen `TransitionRunToTerminated` CAS predicate from `status IN ('queued', 'running', 'waiting')` to `status IN ('queued', 'running', 'waiting', 'suspended')` so operators can terminate suspended runs without resuming first
- [x] 2.4 Run `make generate` and confirm the generated Go query functions compile

**Validation:** generate succeeds; all three queries compile; CAS conditions are correct; terminate accepts suspended runs

## 3. History Event Types

- [x] 3.1 Add `EventWorkflowSuspended = "workflow.suspended"` and `EventWorkflowResumed = "workflow.resumed"` constants in `engine/pkg/history/history.go`
- [x] 3.2 Add `WorkflowSuspendedPayload struct{}` and `WorkflowResumedPayload struct{}` (empty payloads, matching `WorkflowCancelledPayload` pattern)
- [x] 3.3 Register both event types in `payloadTarget()` and `DecodePayload()` functions
- [x] 3.4 Add unit tests for encoding/decoding the new event types

**Validation:** event types serialize and deserialize correctly; `DecodePayload` handles empty payloads

## 4. OpenAPI Contract Updates

- [x] 4.1 Add `POST /v1/engine/runs/{run_id}/suspend` to `contracts/openapi/openapi.yaml` with no request body and response `EngineRunResponse` (callers get authoritative post-transition state)
- [x] 4.2 Add `POST /v1/engine/runs/{run_id}/resume` with no request body and response `EngineRunResponse`
- [x] 4.3 Document that suspend returns 409 for `running` or terminal runs; resume returns 409 for terminal runs; both return 200 with current state for idempotent no-ops
- [x] 4.4 Add `SUSPENDED` to the `EngineRunStatus` enum in the OpenAPI schema (uppercase, matching the existing `QUEUED`, `RUNNING`, `WAITING`, `COMPLETED`, `FAILED`, `CANCELLED`, `TERMINATED` convention)
- [x] 4.5 Run `make generate` and verify Go server bindings, TypeScript client, and Python types regenerate without drift

**Validation:** `make generate` succeeds; generated bindings include suspend/resume operations and `SUSPENDED` status; response schema is `EngineRunResponse`

## 5. Suspend Handler + Service

- [x] 5.1 Add `SuspendRun` on the shared engine control service: open transaction, load run by project+id (FOR UPDATE), verify status is `queued` or `waiting` (return typed `run_not_suspendable` 409 for `running`, `run_terminal` 409 for terminal, no-op 200 with current `EngineRunResponse` for already `suspended`), call `TransitionRunToSuspended`, append `workflow.suspended` history event using the shared non-activation history sequencing rule (compute `next_sequence` while holding the run lock), update `engine_latest_history_id` on the projected trace, call `syncProjectedTraceSummary` to immediately reflect `engine_run_status=suspended` on `public.traces`, commit, return `EngineRunResponse`
- [x] 5.2 Wire the HTTP route with `ENGINE_PUBLIC_API_ENABLED=true` gating, `X-Continua-Engine-Preview: 1` header requirement, API key authentication, and project scoping
- [x] 5.3 Add handler tests: suspend from queued (success, response shows `SUSPENDED`), suspend from waiting (success), suspend from running (409 with typed error code `run_not_suspendable`), suspend from terminal (409 with typed error code `run_terminal`), suspend already-suspended (no-op 200 with `SUSPENDED` state), missing run (404), cross-project (404)

**Validation:** handler tests pass; response body is `EngineRunResponse` with correct status; `engine_run_status` on `public.traces` is updated immediately

## 6. Resume Handler + Service

- [x] 6.1 Add `ResumeRun` on the shared engine control service: open transaction, load run by project+id (FOR UPDATE), verify status is `suspended` (return typed `run_terminal` 409 for terminal, no-op 200 with current `EngineRunResponse` for `queued`/`running`/`waiting`), call `TransitionRunToQueuedFromSuspended`, append `workflow.resumed` history event using the shared non-activation history sequencing rule (compute `next_sequence` while holding the run lock), update `engine_latest_history_id` on the projected trace, call `syncProjectedTraceSummary` to immediately reflect `engine_run_status=queued` on `public.traces`, commit, return `EngineRunResponse`
- [x] 6.2 Wire the HTTP route with `ENGINE_PUBLIC_API_ENABLED=true` gating, `X-Continua-Engine-Preview: 1` header requirement, API key authentication, and project scoping
- [x] 6.3 Add handler tests: resume from suspended (success → queued, response shows `QUEUED`), resume from non-suspended active (no-op 200), resume from terminal (409 with typed error code `run_terminal`), missing run (404), cross-project (404)

**Validation:** handler tests pass and assert the typed error codes; resumed run is claimable with `ready_at = NOW()`; `engine_run_status` on `public.traces` is updated immediately

## 7. Projector Mapping

- [x] 7.1 Update the projector's run-status-to-trace-status mapping: `suspended` → trace `running`, root-span `running`
- [x] 7.2 Ensure `engine_run_status` on `public.traces` carries the string `suspended` for engine-specific filtering (note: the control-path summary sync already writes this; the projector mapping covers the async catch-up path)
- [x] 7.3 Add projector tests: a `workflow.suspended` event does not change the projected trace/root-span from `running`; a `workflow.resumed` event does not change the projected trace/root-span status

**Validation:** projector tests pass; suspended runs show as `running` in the debugger

## 8. Trace Search Filter Update

- [x] 8.1 Add `suspended` (lowercase) to the allowed values for `engine_run_status` filter in `internal/store/search.go` validation (matching the existing lowercase filter convention: `queued`, `running`, `waiting`, etc.)
- [x] 8.2 Add a search test: filter by `engine_run_status=suspended` returns only suspended engine runs

**Validation:** search test passes; invalid enum values are still rejected

## 9. Python SDK

- [x] 9.1 Add `SUSPENDED` to the `EngineRunStatus` enum in `sdks/python/src/continua/types.py`
- [x] 9.2 Add `suspend(run_id)` method to `EngineControlClient`: `POST /v1/engine/runs/{run_id}/suspend` with preview header, returns `EngineRunResponse`
- [x] 9.3 Add `resume(run_id)` method to `EngineControlClient`: `POST /v1/engine/runs/{run_id}/resume` with preview header, returns `EngineRunResponse`
- [x] 9.4 Add unit tests for suspend/resume methods (mocked HTTP)
- [x] 9.5 Run `cd sdks/python && uv run pytest` and confirm green

**Validation:** Python tests pass; SDK methods match the OpenAPI contract; return type is `EngineRunResponse`

## 10. Integration Tests

- [x] 10.1 Integration test: start run → suspend from queued → verify run status is `SUSPENDED` → resume → verify run is `QUEUED` and claimable → workflow completes normally
- [x] 10.2 Integration test: start run → run becomes `waiting` (activity scheduled) → suspend → deliver signal during suspension → resume → next activation sees the signal in frontier
- [x] 10.3 Integration test: start run → run becomes `waiting` (timer scheduled) → suspend → timer fires during suspension (maintenance skips it) → resume → next activation sees the fired timer
- [x] 10.4 Integration test: suspend → cancel during suspension → resume → next activation sees `CancellationRequested() == true` before any wait resumes
- [x] 10.5 Integration test: suspend → activity completes during suspension → resume → next activation sees activity outcome
- [x] 10.6 Integration test: suspend idempotency (double suspend is no-op 200)
- [x] 10.7 Integration test: resume idempotency (resume on queued/waiting is no-op 200)
- [x] 10.8 Integration test: suspend from `running` returns 409
- [x] 10.9 Integration test: suspend/resume on terminal run returns 409
- [x] 10.10 Integration test: terminate on suspended run succeeds (widened CAS) and transitions to `TERMINATED`
- [x] 10.11 Integration test: verify `engine_run_status` on `public.traces` is updated immediately after suspend/resume (not deferred to projector)

**Validation:** all integration tests pass against real Postgres

## 11. Regression Guard

- [x] 11.1 Run `make generate` and verify no drift
- [x] 11.2 Run `cd engine && go test ./...`
- [x] 11.3 Run `go test ./internal/api/... ./internal/ingest/... ./internal/store/... ./internal/jobs/...`
- [x] 11.4 Run `pnpm --filter web test`
- [x] 11.5 Run `cd sdks/python && uv run pytest`

**Validation:** all suites pass


---

### Parallelization Notes

- Task 0 (prep) must complete first
- Tasks 1, 3, 4 can run in parallel (schema migration, history events, OpenAPI)
- Task 2 (engine queries) depends on Task 1 (enum migration)
- Tasks 5, 6 (handlers) depend on Tasks 1–4
- Task 7 (projector) depends on Task 3 (history events)
- Task 8 (search filter) depends on Task 1 (enum migration)
- Task 9 (Python SDK) depends on Task 4 (OpenAPI)
- Tasks 10, 11 (integration + regression) are final
