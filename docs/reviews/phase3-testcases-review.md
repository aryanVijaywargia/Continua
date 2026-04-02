> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Phase 3 Testcases Review (Deep Analysis)

## Scope Reviewed
- Go tests: `internal/api/middleware/auth_test.go`, `internal/ingest/rollups_test.go`, `internal/store/batch_test.go`, `internal/store/spans_test.go`, `internal/store/search_test.go`, `internal/jobs/rollup_test.go`, `internal/store/performance_test.go`, `internal/api/sessions_test.go`
- Python tests: `sdks/python/tests/test_errors.py`, `sdks/python/tests/test_client.py`, `sdks/python/tests/test_trace.py`, `sdks/python/tests/test_span.py`

## Blocking Issues (Will Not Compile or Will Fail for the Wrong Reason)
1) **SQLC params used with wrong field names and missing required fields**
- `platform.CreateProjectParams` expects `ApiKeyHash`, but tests pass `ApiKey` (compile error) in `internal/api/middleware/auth_test.go`, `internal/ingest/rollups_test.go`, `internal/store/batch_test.go`, `internal/store/search_test.go`, `internal/jobs/rollup_test.go`, `internal/store/performance_test.go`, `internal/api/sessions_test.go`.
- `platform.UpsertTraceParams` requires `SessionID`, `Tags`, `Metadata`, `Input`, `Output`, `Status`, `StartTime`, `EndTime` (all required types), but tests pass only `ProjectID/TraceID/Name` and sometimes `StartTime` as `*time.Time`. This fails to compile (type mismatch) across `internal/ingest/rollups_test.go`, `internal/store/search_test.go`, `internal/jobs/rollup_test.go`, `internal/store/performance_test.go`, `internal/store/spans_test.go`.
- `platform.UpsertSpanParams` requires many required fields (`Type`, `Status`, `Level`, `StartTime`, `EndTime` as `pgtype.Timestamptz`, `TotalCost` as `pgtype.Numeric`, etc.). Tests provide only a few fields and use `TraceID` as a string instead of UUID, so these won’t compile in `internal/ingest/rollups_test.go`, `internal/store/search_test.go`, `internal/jobs/rollup_test.go`, `internal/store/performance_test.go`, `internal/store/spans_test.go`.

2) **Queries/types referenced that do not exist**
- `middleware.NewAuthMiddleware` doesn’t exist; only `middleware.APIKeyAuth` is defined (`internal/api/middleware/auth_test.go`).
- `jobs.NewClient`, `jobs.EnqueueRollup`, `jobs.EnqueueRollupInTx`, `jobs.NewTraceRollupWorker` are referenced but not in the repo yet; if these are intended, the tasks/spec should standardize these names (`internal/jobs/rollup_test.go`).
- `ingest.Service.ComputeRollups` does not exist (`internal/ingest/rollups_test.go`).
- `q.CreateIngestBatch`, `q.GetIngestBatchByKey`, `q.CreateSpanEvent` don’t exist; the actual queries are `ClaimBatch`, `GetBatchByKey`, and `InsertSpanEvent` (`internal/store/batch_test.go`).
- `api.SessionResponse` and `api.SessionListResponse` types don’t exist; OpenAPI generates `api.Session` and `api.SessionList` (`internal/api/sessions_test.go`).
- `api.NewRouter` takes `(server *Server, s *store.Store)` but tests pass `(server, projectID)` (compile error) (`internal/api/sessions_test.go`).

3) **Schema mismatches in tests**
- `CreateSessionParams` has no `SessionID` field; tests pass it, which won’t compile (`internal/api/sessions_test.go`).
- Spans reference the internal trace UUID, but tests use external trace IDs (string) when inserting spans (`internal/ingest/rollups_test.go`, `internal/store/search_test.go`, `internal/jobs/rollup_test.go`, `internal/store/performance_test.go`, `internal/store/spans_test.go`).

4) **Auth middleware tests expect wrong response format**
- Middleware returns `{code, message}` but tests assert `{error: "..."}` (`internal/api/middleware/auth_test.go`).
- Tests create projects with raw API keys, but middleware hashes keys; lookups will fail unless the hash is stored (`internal/api/middleware/auth_test.go`).
- `GetProjectID` returns `(uuid.UUID, bool)` but tests treat it as a single value (`internal/api/middleware/auth_test.go`).

## Logical Mismatches (Tests Enforce Behavior That Conflicts with Current Contracts)
1) **Validation error shape in Python SDK tests**
- `sdks/python/tests/test_errors.py` expects API errors shaped as `{error, details}`, but OpenAPI defines `{code, message}`. This will push the SDK toward parsing a format that doesn’t exist in the contract.

2) **Sessions API tests don’t include authentication**
- The API router enforces `X-API-Key` on `/api/*`; tests do not set headers and would return 401 even if endpoints exist (`internal/api/sessions_test.go`).

## Redundancy / Overlap
1) **Batch idempotency tests duplicated across files**
- `internal/store/batch_test.go` and `internal/store/spans_test.go` both test duplicate batch behavior. Consolidating these avoids drift.

2) **Rollup coverage overlaps between ingest and jobs**
- `internal/ingest/rollups_test.go` tests inline rollups, while `internal/jobs/rollup_test.go` tests async rollups. If Phase 3 fully moves rollups async, consider moving rollup math tests to store/worker and dropping ingest-level rollup tests to avoid duplication.

## Missing Coverage (High-Value Additions)
1) **Search ordering behavior**
- There is no test that enforces the “trace match outranks span-only match” ordering requirement (`internal/store/search_test.go`).

2) **Coalescing behavior for running rollup jobs**
- There is a test that simulates running jobs, but no test for “only one queued follow-up job” across repeated enqueues with uniqueness constraints (partial in `internal/jobs/rollup_test.go`, but it assumes manual state manipulation). Add a test that uses the same enqueue path and verifies exactly one available job exists alongside one running job.

3) **SDK session context manager and span helper methods**
- No tests cover the new session context manager, `span.set_llm_response`, `span.set_tool_call`, or `span.log` helpers (spec requires these). Add to `sdks/python/tests/test_trace.py` and `sdks/python/tests/test_span.py`.

## Recommendations (Actionable)
1) **Stop using raw sqlc inserts in tests**
- Use store/ingest/service helpers that already fill required fields and types, or build small helper constructors that populate required sqlc params (pgtype types, defaults). This fixes most compile errors in `internal/ingest/rollups_test.go`, `internal/store/search_test.go`, `internal/jobs/rollup_test.go`, `internal/store/performance_test.go`, `internal/store/spans_test.go`.

2) **Standardize job API naming in spec/tasks/tests**
- Align on function names for enqueueing rollup jobs (e.g., `jobs.EnqueueRollup`/`jobs.EnqueueRollupInTx`), and update tasks/spec so tests and implementation match (`internal/jobs/rollup_test.go`, `openspec/implemented/add-reliability-search-sessions/tasks.md`).

3) **Fix auth tests to match middleware and OpenAPI**
- Use `middleware.APIKeyAuth`, store hashed API keys, assert `{code,message}` response shape, and handle `(uuid.UUID, bool)` return from `GetProjectID` (`internal/api/middleware/auth_test.go`).

4) **Fix sessions API tests to use actual router and auth**
- Construct router via `api.NewRouter(server, store)` and send `X-API-Key` header; use OpenAPI types `api.Session` and `api.SessionList` (`internal/api/sessions_test.go`).

5) **Fix span/traces ID usage**
- For spans/events, use trace UUID (`trace.ID`) instead of external `trace_id` string in all tests (`internal/ingest/rollups_test.go`, `internal/store/search_test.go`, `internal/jobs/rollup_test.go`, `internal/store/performance_test.go`, `internal/store/spans_test.go`).

6) **Align Python SDK error parsing with OpenAPI**
- Update error parsing tests to expect `{code,message}` or update SDK to support both only if intentionally backward-compatible (`sdks/python/tests/test_errors.py`).

## Suggested Removals (If Keeping Scope Tight)
- Remove redundant batch idempotency tests in `internal/store/spans_test.go` and keep them only in `internal/store/batch_test.go` once query usage is fixed.

## Next Steps
1) Fix compile-level mismatches (sqlc param types and missing function names).
2) Re-run `go test ./internal/...` and `pytest sdks/python/tests` to surface remaining failures.
3) Add the missing tests for SDK session helpers and search ordering once the core compile issues are resolved.
