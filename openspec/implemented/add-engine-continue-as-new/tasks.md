## 1. Schema and Generated Types

- [x] 1.1 Add an engine migration: `ALTER TYPE engine.run_lifecycle_status ADD VALUE IF NOT EXISTS 'continued_as_new'`
- [x] 1.2 Add an engine migration: add `continued_from_run_id UUID REFERENCES engine.runs(id)` and `continued_to_run_id UUID REFERENCES engine.runs(id)` to `engine.runs` (both nullable)
- [x] 1.3 Add a platform migration updating `traces_engine_run_status_check` so `public.traces.engine_run_status` accepts lowercase `continued_as_new`
- [x] 1.4 Run `make generate` so engine/platform sqlc output includes the new enum value and run-chain columns

**Validation:** migrations apply cleanly; generated types compile; existing rows are unaffected; platform traces accept `continued_as_new`

## 2. Workflow Authoring API and History Event

- [x] 2.1 Add `ErrContinueAsNew` in `engine/pkg/workflow/context.go` following the replay-aware sentinel pattern used by `ErrCancelled`
- [x] 2.2 Add `ContinueAsNew(input any) error` that marshals the input and returns a sentinel carrying the payload
- [x] 2.3 Ensure `errors.Is(err, ErrContinueAsNew)` works through wrapping and the payload is recoverable via a typed error / accessor
- [x] 2.4 Add `EventWorkflowContinuedAsNew = "workflow.continued_as_new"` plus payload registration/decoding in `engine/pkg/history/history.go`
- [x] 2.5 Add unit tests for sentinel creation/extraction and history event encoding/decoding

**Validation:** the sentinel round-trips correctly through wrapped errors; the continuation input is recoverable; the event serializes and deserializes correctly

## 3. Engine Queries and Latest-Run Ordering

- [x] 3.1 Add `TransitionRunToContinuedAsNew :one` in `engine/db/queries/runs.sql`: CAS on `status = 'running' AND claimed_by = $2`, set `status = 'continued_as_new'`, `completed_at = NOW()`, `continued_to_run_id = $3`, clear wait/claim fields, and return the updated row
- [x] 3.2 Add `continued_from_run_id` support for the new run, either directly in `CreateRun` or through a dedicated update query
- [x] 3.3 Update `GetLatestRunByInstance` to order by `run_number DESC, id DESC`
- [x] 3.4 Update `ListRunsByInstance` to order by `run_number DESC, id DESC`
- [x] 3.5 Run `make generate` and update any engine CLI/runtime callers that assume created-at ordering

**Validation:** sqlc generation succeeds; continuation CAS queries compile; per-instance "latest run" lookups return the highest run number

## 4. Replay and Activation Continuation Commit

- [x] 4.1 Add `decisionContinuedAsNew` to the replay/activation decision kinds
- [x] 4.2 In `workflowRunner.execute()`, detect `ErrContinueAsNew`, extract the continuation input, append `workflow.continued_as_new`, and return `decisionContinuedAsNew`
- [x] 4.3 On replay, if history already contains `workflow.continued_as_new`, require the workflow to return the same continuation sentinel/input using the existing `equalJSON` rule (trimmed byte equality first, otherwise decoded semantic JSON equality), or emit a replay mismatch
- [x] 4.4 Add a `decisionContinuedAsNew` branch in `engine/internal/workflow/activation.go` that, in one transaction:
- [x] 4.5 Cancels open activity tasks on the old run and discards open inbox items on the old run
- [x] 4.6 Transitions the old run to `continued_as_new`
- [x] 4.7 Creates run N+1 on the same instance with `run_number = old + 1`, the same `definition_version`, and `continued_from_run_id = old_run.id`
- [x] 4.8 Appends `workflow.started` on the new run with the continuation input
- [x] 4.9 Keeps the instance `active` instead of moving it to a terminal instance status
- [x] 4.10 Add replay and activation tests for first execution, replay match, replay mismatch, JSON-semantic replay equality (for example different object key order), and bidirectional run-chain linkage

**Validation:** replay tests pass; continuation remains atomic; the old run is terminal and the new run is immediately current

## 5. Continuation Trace Bootstrap

- [x] 5.1 In the activation transaction, read the old run's projected trace shell to inherit session and presentation fields (`name`, `user_id`, `tags`, `environment`, `release`, `metadata`); if that trace row is unexpectedly missing, fail the activation as an invariant violation rather than creating a degraded fallback shell
- [x] 5.2 Create the new trace shell and root span from the engine module continuation path using the exact live-shell fields that `StartRun` establishes: trace status `running`, root-span status `running`, `output = null`, trace/root input set to the continuation input, `engine_run_status = queued`, nil wait/custom status, zero pending counts, `engine_instance_key`, and `engine_definition_name` / `engine_definition_version`; do not require a helper extracted from `internal/api` or `internal/enginecontrol`
- [x] 5.3 Set the new trace shell to `engine_projection_state = up_to_date` and use the new `workflow.started` history ID as both `engine_latest_history_id` and `engine_last_projected_history_id`
- [x] 5.4 Add activation/integration tests covering inherited presentation fields, preserved session link, immediate trace-shell/root-span creation, the exact live-shell field values above, and invariant-violation behavior when the previous trace shell is missing

**Validation:** continuation creates the new trace shell immediately; the new trace inherits the previous run's presentation fields; no cross-module helper refactor is required

## 6. Terminal-Status Plumbing

- [x] 6.1 Update terminal-status helpers in `internal/api/engine_control.go` and `internal/enginecontrol/service.go` so stored status `continued_as_new` is treated as terminal in read APIs and purge eligibility
- [x] 6.2 Update retention candidate selection in `internal/store/engine_projection.go` so `continued_as_new` is eligible after the retention window
- [x] 6.3 Update terminal projection helpers in `engine/pkg/projection/terminal.go` and `engine/internal/projector/` so `continued_as_new` maps to trace/root-span `completed`, keeps terminal trace/root `output = null`, uses a continuation-specific cleanup reason for discarded activity/timer waits, and clears projected signal wait state without leaving stale wait UI
- [x] 6.4 Add tests covering terminal shells, purge/retention eligibility, projected terminal mapping for continued runs, null terminal output for `continued_as_new`, resolved activity/timer wait events under the continuation cleanup reason, and signal-wait cleanup semantics

**Validation:** continued runs behave like terminal runs everywhere terminal-status sets are currently checked

## 7. OpenAPI and Engine Response Mapping

- [x] 7.1 Add uppercase `CONTINUED_AS_NEW` to the `EngineRunStatus` enum in `contracts/openapi/openapi.yaml`
- [x] 7.2 Add nullable `continued_from_run_id`, `continued_to_run_id`, `continued_from_trace_id`, and `continued_to_trace_id` to `EngineRunSummary`, `EngineRunResponse`, and `EngineRunResultResponse`
- [x] 7.3 Derive `continued_from_trace_id` / `continued_to_trace_id` from the corresponding run IDs using `engine:<run_id>` rather than adding `public.traces` columns
- [x] 7.4 Keep `EngineRunResultResponse.result` reserved for the stored workflow result payload; `CONTINUED_AS_NEW` responses return `result = null`
- [x] 7.5 Ensure `GET /v1/engine/instances/{instance_key}` returns the highest `run_number` as `current_run`
- [x] 7.6 Add handler/mapper tests for continuation fields on `EngineRunSummary`, trace detail `engine`, instance `current_run`, run detail, and run result; also verify uppercase status output, null result on continued runs, and current-run selection after a continuation
- [x] 7.7 Run `make generate` and verify generated Go, TypeScript, and Python bindings stay in sync

**Validation:** OpenAPI generation succeeds; engine endpoints return uppercase `CONTINUED_AS_NEW`; continuation fields are present and trace IDs are derived correctly

## 8. Trace Search Filter and Constraint Coverage

- [x] 8.1 Add lowercase `continued_as_new` to the `/api/traces` `engine_run_status` query parameter enum in `contracts/openapi/openapi.yaml`
- [x] 8.2 Update `internal/store/search.go` validation to accept lowercase `continued_as_new`
- [x] 8.3 Add a trace search test showing `engine_run_status=continued_as_new` returns only continued runs

**Validation:** the public trace-search contract and platform constraint both accept `continued_as_new`

## 9. Python SDK

- [x] 9.1 Add `CONTINUED_AS_NEW` to the generated/manual `EngineRunStatus` enum surface in `sdks/python/src/continua/types.py`
- [x] 9.2 Add `EngineRunContinuationDepthError` in `sdks/python/src/continua/exceptions.py`
- [x] 9.3 Extend engine run result/response models with continuation run IDs and derived trace IDs
- [x] 9.4 Extend `wait_for_terminal` to accept `follow_continuations=False` and `max_continuations=32`
- [x] 9.5 When `follow_continuations=True` and status is `CONTINUED_AS_NEW`, follow `continued_to_run_id` until a non-continuation terminal status is reached or the depth limit is exceeded
- [x] 9.6 Add unit tests for default no-follow behavior, one-hop and multi-hop follow behavior, and depth-limit failure
- [x] 9.7 Run `cd sdks/python && uv run pytest`

**Validation:** Python callers can opt into continuation following without changing the default per-run semantics

## 10. Integration and Regression

- [x] 10.1 Integration test: workflow calls `ContinueAsNew(newInput)` and run N becomes `CONTINUED_AS_NEW` while run N+1 is created with `run_number = old + 1`
- [x] 10.2 Integration test: run-chain linkage is correct on both runs, instance reads return run N+1 as `current_run`, and trace detail `engine` summary exposes continuation chain fields without requiring a separate run-detail fetch
- [x] 10.3 Integration test: run N+1 trace inherits session and presentation fields from run N
- [x] 10.4 Integration test: run N+1 starts with fresh history (`workflow.started` only) and the continuation input
- [x] 10.5 Integration test: run N's open activity tasks are cancelled and inbox items are discarded on continuation
- [x] 10.6 Integration test: multi-hop continuation (run1 -> run2 -> run3) preserves run-chain linkage and highest-run ordering
- [x] 10.7 Integration test: `GET /v1/engine/runs/{run_id}/result` on a continued run returns `CONTINUED_AS_NEW`, null `result`, and continuation chain fields
- [x] 10.8 Integration test: purge/retention treats `CONTINUED_AS_NEW` as terminal
- [x] 10.9 Integration test: `/api/traces?engine_run_status=continued_as_new` filters correctly
- [x] 10.10 Integration test: Python `wait_for_terminal(follow_continuations=True)` follows a multi-hop chain and returns the final result
- [x] 10.11 Integration test: Python `wait_for_terminal(..., max_continuations=1)` raises `EngineRunContinuationDepthError` on a deeper chain
- [x] 10.12 Run `cd engine && go test ./...`
- [x] 10.13 Run `go test ./internal/api/... ./internal/ingest/... ./internal/store/... ./internal/jobs/...`
- [x] 10.14 Run `pnpm --filter web test`
- [x] 10.15 Re-run `cd sdks/python && uv run pytest`

**Validation:** continuation works end-to-end against real Postgres and does not regress existing engine/platform behavior

---

### Parallelization Notes

- Tasks 1 and 2 can start in parallel
- Task 3 depends on Task 1
- Task 4 depends on Tasks 2 and 3
- Task 5 depends on Task 4
- Task 6 depends on Tasks 1, 4, and 5
- Tasks 7 and 8 depend on Task 1 and can proceed in parallel with late engine work once the contracts are settled
- Task 9 depends on Task 7
- Task 10 is final
