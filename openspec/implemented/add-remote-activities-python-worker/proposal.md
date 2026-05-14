# Change: Add Remote Activities And Python Worker

## Why

Activity execution is currently in-process only. Workflows that need to call Python ML models, external APIs with Python-specific SDKs, or long-running computations must embed that logic in the Go server. Remote activity workers allow external processes to claim, execute, and complete activity tasks over REST, keeping the engine's activity retry and lease semantics intact while enabling polyglot execution.

## What Changes

### Engine Remote Activity Protocol
- Add `workflow.ActivityOptions.ExecutionTarget` with values `local` (default) and `remote`
- Add `engine.activity_tasks.execution_target TEXT NOT NULL DEFAULT 'local'`
- Local claim queries only claim `local` tasks; remote claim is a separate endpoint
- Add preview-gated REST endpoints: claim, heartbeat/renew lease, complete, and fail
- Claim returns immediately with zero or more tasks (short-polling, no long-poll in v1)
- Server clamps lease duration: min 10s, default 60s, max 5m
- Complete/fail requires matching `worker_id` lease ownership; stale, already-terminal, local-target, or otherwise wrong-state calls return conflicts
- Failure reuses existing retry logic with non-retryable flag support

### Engine Remote Activity Routing
- No `task_queue` in v1; routing is execution target plus supported activity types
- Existing in-process workers continue handling `local` tasks with no migration break

### Python Remote Activity Worker
- Handler registry for sync Python callables, short-polling loop, configurable concurrency limit
- Heartbeat loop at half of the effective lease duration returned by claim
- Graceful shutdown with in-flight task completion
- Complete/fail calls with typed errors and non-retryable error support
- No Python workflow authoring; no TS/JS SDK

## Impact
- Affected specs: engine-remote-activity-protocol (new), engine-remote-activity-routing (new), python-remote-activity-worker (new)
- Affected code: `engine/pkg/workflow/`, `engine/internal/store/`, `engine/internal/activity/`, `engine/db/`, `contracts/openapi/`, `internal/api/`, `sdks/python/`
