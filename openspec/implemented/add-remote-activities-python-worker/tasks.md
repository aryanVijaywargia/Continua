## 0. Prerequisite Gate
- [x] 0.1 Close prior-phase validation debt: retries, suspend/resume, ContinueAsNew, generated drift, and debugger tests must be green before starting

## 1. Engine Remote Activity Routing

### 1.1 Schema
- [x] 1.1.1 Create engine migration: add `execution_target TEXT NOT NULL DEFAULT 'local'` with CHECK constraint `execution_target IN ('local', 'remote')`, and `lease_duration_ms BIGINT` to `engine.activity_tasks`; add partial indexes for local and remote claim queries
- [x] 1.1.2 Update local claim queries to filter `WHERE execution_target = 'local'`
- [x] 1.1.3 Add remote claim query: filter `WHERE execution_target = 'remote'` with `activity_type IN (...)`, the local availability predicate (`queued` with `available_at <= NOW()` or expired `claimed` lease), and `max_tasks` limit; atomically set claimed_by/claimed_at/lease_expires_at/lease_duration_ms and increment attempt_count for queued claims and stale-lease reclaims
- [x] 1.1.4 Add sqlc queries for heartbeat/renew lease, remote complete, remote fail, and conflict classification for stale/reclaimed/local-target/wrong-terminal-state calls
- [x] 1.1.5 Run `make generate`

### 1.2 Activity Options
- [x] 1.2.1 Add `ExecutionTarget string` to `workflow.ActivityOptions` with normalized values `local` and `remote`
- [x] 1.2.2 Update `NormalizeActivityOptions` to validate and default `ExecutionTarget`
- [x] 1.2.3 Pass `execution_target` through to `CreateActivityTask` params
- [x] 1.2.4 Verify existing in-process activity workers only claim `local` tasks (no behavior change)

## 2. Engine Remote Activity Protocol

### 2.1 REST Endpoints
- [x] 2.1.1 Add preview-gated OpenAPI paths requiring `X-Continua-Engine-Preview: 1`: `POST /v1/engine/activities/claim`, `POST /v1/engine/activities/{id}/heartbeat`, `POST /v1/engine/activities/{id}/complete`, `POST /v1/engine/activities/{id}/fail`
- [x] 2.1.2 Define request/response schemas: claim (bounded non-empty worker_id, non-empty bounded activity_types, lease_duration, max_tasks; task responses include lease_expires_at and effective_lease_duration_ms), heartbeat (bounded non-empty worker_id; response includes lease_expires_at and effective_lease_duration_ms), complete (bounded non-empty worker_id, output), fail (bounded non-empty worker_id, bounded error_code, bounded/truncated error_message, non_retryable)
- [x] 2.1.3 Run `make generate`
- [x] 2.1.4 Implement claim handler: validate bounded non-empty worker_id and activity_types entries, clamp lease (min 10s, default 60s, max 5m), claim/reclaim only queued tasks whose `available_at <= now` or claimed tasks whose lease expired, update attempt_count atomically, return tasks with effective lease expiry and effective lease duration in milliseconds
- [x] 2.1.5 Implement heartbeat handler: validate bounded non-empty worker_id and remote lease ownership (`execution_target = 'remote'`, status `claimed`, matching worker, unexpired lease), return 409 for local-target/queued/terminal/expired/reclaimed/non-owned tasks, return 404 for missing/cross-project tasks, renew lease, return effective expiry and effective lease duration in milliseconds
- [x] 2.1.6 Implement complete handler: validate bounded non-empty worker_id ownership, complete task, wake waiting run, and return 409 for stale/reclaimed/local-target/already-terminal/wrong-state calls, including duplicate complete calls after a successful response
- [x] 2.1.7 Implement fail handler: validate bounded non-empty worker_id ownership, bound `error_code`, truncate `error_message`, fail or retry task based on retry policy and non_retryable flag, and return 409 for stale/reclaimed/local-target/already-terminal/wrong-state calls, including duplicate fail calls after retry requeue or terminal failure

### 2.2 Auth And Scoping
- [x] 2.2.1 Remote activity endpoints MUST be API-key authenticated and project-scoped (same middleware as existing engine endpoints)
- [x] 2.2.2 Claim, heartbeat, complete, and fail operations MUST only access tasks belonging to the authenticated project

## 3. Python Remote Activity Worker

### 3.1 Worker Module
- [x] 3.1.1 Create `sdks/python/src/continua/worker.py` with `ActivityWorker` class and configurable `worker_id` (generated once per worker instance when omitted)
- [x] 3.1.2 Implement handler registry for synchronous callables: `worker.register(activity_type, handler_fn)` and decorator form `@worker.activity(type_name)`; reject coroutine handlers in v1
- [x] 3.1.3 Implement polling loop: configurable poll interval, calls claim endpoint with `max_tasks` set to available execution slots, skips claiming when no slots are available, dispatches claimed tasks immediately to registered handlers
- [x] 3.1.4 Implement concurrency limit with a bounded thread pool: cap on concurrent sync task executions and avoid a local claimed-but-not-running backlog by default
- [x] 3.1.5 Implement heartbeat loop: heartbeat at half of `effective_lease_duration_ms` for each in-flight task; treat heartbeat 409 and 404 as lost/no-longer-owned and signal the task context
- [x] 3.1.6 Implement complete/fail calls: serialize output or map errors, call complete/fail endpoints with the same worker-instance `worker_id`, and suppress terminal calls for tasks marked lost/no-longer-owned
- [x] 3.1.7 Implement graceful shutdown: stop claiming, continue heartbeating still-owned in-flight tasks while waiting, complete/fail only still-owned completed tasks, and leave timeout/lost-lease tasks for reclaim
- [x] 3.1.8 Implement non-retryable error support: `NonRetryableError` exception class, mapped to `non_retryable: true` on fail
- [x] 3.1.9 Implement optional `ActivityTaskContext` second handler argument with task_id, activity_key, activity_type, and lost-lease/cancellation indicator

### 3.2 Error Taxonomy
- [x] 3.2.1 Map Python exceptions to error codes and messages
- [x] 3.2.2 `NonRetryableError` maps to `non_retryable: true`
- [x] 3.2.3 Unhandled exceptions map to a generic error code with bounded exception type/message; full traceback is logged locally only and is not sent as the durable error message

## 4. Tests

### 4.1 Engine Routing Tests
- [x] 4.1.1 Test: local activity claim only returns `local` tasks
- [x] 4.1.2 Test: remote activity claim only returns `remote` tasks
- [x] 4.1.3 Test: claim with empty response (no available tasks) returns immediately
- [x] 4.1.4 Test: remote type filtering (claim only returns tasks matching requested activity_types)

### 4.2 Protocol Tests
- [x] 4.2.1 Test: lease clamping (below min clamped to 10s, above max clamped to 5m)
- [x] 4.2.2 Test: heartbeat renewal extends lease expiry only for owned remote claimed tasks; local-target/queued/terminal/expired/reclaimed/non-owned tasks return 409 without extending the lease
- [x] 4.2.3 Test: stale lease reclaim (task reclaimed after lease expiry)
- [x] 4.2.4 Test: complete with matching worker_id succeeds
- [x] 4.2.5 Test: duplicate complete after successful response returns 409 and does not mutate completed output
- [x] 4.2.6 Test: complete with stale/reclaimed worker_id returns 409 and does not mutate task state
- [x] 4.2.7 Test: complete/fail against local-target or wrong terminal state returns 409 and does not mutate task state
- [x] 4.2.8 Test: duplicate fail after retry requeue or terminal failure returns 409 and does not mutate task state
- [x] 4.2.9 Test: fail with retryable error requeues task with backoff
- [x] 4.2.10 Test: fail with non-retryable error or exhausted retry policy terminates task immediately
- [x] 4.2.11 Test: auth, preview gating, and project scoping (router-level auth/preview coverage plus cross-project claim returns nothing)
- [x] 4.2.12 Test: heartbeat for missing/cross-project tasks returns 404 without leaking task existence
- [x] 4.2.13 Test: fail rejects invalid or too-long `error_code` and truncates too-long `error_message`
- [x] 4.2.14 Test: remote claim increments attempt_count and persists claimed_by/claimed_at/lease_expires_at/lease_duration_ms for queued claims and stale-lease reclaims
- [x] 4.2.15 Test: all remote endpoints reject blank or too-long worker_id, and claim rejects blank, too-long, or too many activity_types entries
- [x] 4.2.16 Test: remote claim respects `available_at` retry backoff and does not claim queued retry tasks before they become available
- [x] 4.2.17 Test: complete and fail return 404 for missing/cross-project task IDs without leaking task existence

### 4.3 Python Worker Tests
- [x] 4.3.1 Test: handler registration and dispatch
- [x] 4.3.2 Test: polling loop claims and executes tasks
- [x] 4.3.3 Test: concurrency limit respected with sync thread-pool execution, slot-based `max_tasks`, and no default local claimed-but-not-running backlog
- [x] 4.3.4 Test: heartbeat sent at half of claim response `effective_lease_duration_ms`
- [x] 4.3.5 Test: graceful shutdown heartbeats still-owned tasks, completes/fails only still-owned finished tasks, and leaves timeout/lost-lease tasks for reclaim
- [x] 4.3.6 Test: NonRetryableError mapped correctly
- [x] 4.3.7 Test: unhandled exception mapped to generic error
- [x] 4.3.8 Test: optional `ActivityTaskContext` second argument receives task metadata and lost-lease cancellation signal
- [x] 4.3.9 Test: coroutine handlers are rejected in v1
- [x] 4.3.10 Test: worker_id is stable for the worker instance lifetime and reused across claim, heartbeat, complete, and fail
- [x] 4.3.11 Test: heartbeat 404/409 marks the task lost/no-longer-owned, signals context cancellation, and suppresses complete/fail

### 4.4 Validation
- [x] 4.4.1 Run `make generate` and verify no drift
- [x] 4.4.2 Run targeted Go tests for engine/store/API changes
- [x] 4.4.3 Run `cd sdks/python && uv run pytest`
