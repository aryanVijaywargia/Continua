## Context

The engine's activity task system currently only supports in-process Go workers. Remote workers need to claim, execute, and complete tasks over REST without direct access to engine DB tables. This proposal adds the remote activity protocol as a preview-gated extension and ships a Python SDK worker implementation.

### Stakeholders
- Engine runtime: execution target routing, remote claim/complete paths
- Engine store: new column, updated claim queries
- Platform API: new REST endpoints for remote workers
- Python SDK: new worker module

## Goals / Non-Goals

### Goals
- Remote activity workers that never touch engine DB tables
- Backward-compatible: existing in-process workers unchanged
- Lease-based execution with heartbeat renewal
- Python SDK worker with handler registry and graceful shutdown
- Non-retryable error support for remote failures

### Non-Goals
- Task queues or named routing groups (deferred to v2)
- Long-polling or WebSocket-based claim (v1 is short-poll only)
- TypeScript/JS SDK worker
- Python workflow authoring
- Sticky worker affinity or caching
- Native async handler execution in the Python worker

## Decisions

### Decision: Short-polling claim, not long-poll
Claim returns immediately with zero or more tasks. Workers poll on their own interval.

**Why:** Simplest correct implementation. Long-poll adds server-side hold complexity and connection management. Short-poll is sufficient for preview and easy to upgrade later.

**Alternatives considered:** Long-poll (holds connection until task available or timeout). Rejected for v1 complexity; can be added later as a non-breaking enhancement.

### Decision: No task_queue in v1
Remote routing is `execution_target = 'remote'` plus `activity_types` filter on claim. No named queue routing.

**Why:** Named queues add schema, API, and SDK complexity without clear v1 use cases. Execution target + type filter covers the preview need (route specific activity types to Python workers).

**Alternatives considered:** Adding `task_queue TEXT` column and requiring queue names. Rejected because it front-loads routing complexity that isn't needed until multiple distinct worker pools exist.

### Decision: Lease clamping
Server clamps lease duration to min 10s, default 60s, max 5m. Python worker heartbeats at half the effective lease.

**Why:** Prevents workers from holding tasks indefinitely (max 5m) or losing them too quickly (min 10s). Half-lease heartbeat is a standard pattern that ensures renewal happens before expiry.

### Decision: Complete/fail conflict matrix, not post-terminal idempotency
Complete/fail uses a strict ownership rule. A valid call must target a remote activity task that is still claimed by the requesting `worker_id` under an unexpired lease. Calls from stale workers, tasks reclaimed by another worker, local-target tasks, already terminal tasks, queued retry tasks, and terminal calls that conflict with the existing state return explicit conflicts instead of silent success.

**Why:** Current activity completion/failure clears `claimed_by`, so the server cannot later prove that a post-terminal duplicate came from the same worker and represented the same operation. Returning 409 for post-success duplicates is simpler than adding terminal operation fingerprints in v1, and it prevents a Python worker from reporting success when its output or failure did not become the authoritative engine result.

### Decision: Sync Python handlers on a bounded thread pool
The Python worker v1 supports synchronous handler callables executed on a bounded `ThreadPoolExecutor`, using the SDK's existing sync `httpx.Client` style for REST calls. Async coroutine handlers are out of scope for v1 and should be rejected at registration time with a clear error.

**Why:** The current Python SDK is sync-first and already depends on `httpx`. Supporting both sync and async execution would double the scheduler, heartbeat, shutdown, and cancellation surface before the remote protocol proves itself.

### Decision: Preview gate
Remote activity endpoints are preview-gated using the existing `X-Continua-Engine-Preview: 1` header, consistent with the mutating `/v1/engine` routes already in the repo.

**Why:** This is a new external surface. Gating allows iteration without stability commitments. Reusing the existing preview header avoids introducing a second gating mechanism.

## Risks / Trade-offs

- **Polling overhead:** Short-poll at scale produces empty claim requests. Acceptable for preview; long-poll or push would reduce this for production.
- **Lease expiry race:** A worker completing just as its lease expires could race with stale reclaim. The conflict matrix prevents false success from stale workers, but the task may still be double-executed after reclaim. Activity handlers should use the task context's stable identifiers for idempotency when they perform remote side effects.
- **Python SDK surface:** The worker module is the first Python code that interacts with engine-specific endpoints. SDK structure and error taxonomy set precedent for future polyglot workers.

## Open Questions

None. All clarifications resolved in the proposal phase.
