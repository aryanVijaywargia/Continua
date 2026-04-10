# Change: Add Engine Activity Retries

## Why

Activities today are single-attempt: if an activity handler fails, the workflow immediately sees `activity.failed` and must handle the failure or propagate it. Real-world activities (HTTP calls, external service interactions, database operations) often fail transiently, and requiring every workflow author to hand-code retry loops with exponential backoff is error-prone and produces non-deterministic replay behavior.

This change adds first-class durable activity retries: a workflow author attaches a `RetryPolicy` to an activity via `ActivityWithOptions`, and the engine handles retry scheduling, backoff computation, and final-failure propagation automatically. Retries use the existing `available_at` column on `engine.activity_tasks` for durable scheduling — no new maintenance loop, no new inbox kind, and no workflow wake during retry scheduling.

## What Changes

### Workflow authoring API
- Add `RetryPolicy` struct: `MaxAttempts int`, `InitialBackoff time.Duration`, `MaxBackoff time.Duration`, `BackoffMultiplier float64`
- Add `ActivityOptions` struct: `RetryPolicy *RetryPolicy` (extensible for future options like heartbeat timeout)
- Add `Context.ActivityWithOptions(key, activityType string, input any, out any, opts ActivityOptions) error` — like `Activity()` but with options
- Keep `Activity(key, activityType, input, out)` as single-attempt sugar (equivalent to `MaxAttempts=1`)
- Add `NonRetryableError` wrapper: `workflow.NonRetryableError(err error) error` wraps an error to signal "do not retry"; `workflow.IsNonRetryable(err) bool` checks the wrapper
- Activity handler errors wrapped with `NonRetryableError` bypass retry and fail the activity immediately

### Engine runtime (activity worker side)
- When an activity task fails and has a `RetryPolicy` with remaining attempts:
  - The activity worker resets the task status from `claimed` to `queued`
  - Sets `available_at = NOW() + computed_backoff` using the database clock, not the worker's local wall clock
  - `attempt_count` is NOT modified by the retry decision — it was already incremented by `ClaimNextActivityTask` on claim. The retry exhaustion check uses `attempt_count >= max_attempts` where `attempt_count` reflects the just-completed attempt.
  - The raw backoff is computed in milliseconds as `min(InitialBackoff * BackoffMultiplier^(attempt_count - 1), MaxBackoff)` and then rounded up with `ceil` to the next whole millisecond. That integer-millisecond delay is passed into `RetryActivityTask` as `retry_delay_ms`, and SQL derives the concrete `available_at` from `NOW() + retry_delay_ms * INTERVAL '1 millisecond'`. Example: `333ms * 1.5 = 499.5ms` schedules the retry `500ms` later.
  - Appends `activity.retry_scheduled` history event using the shared non-activation history sequencing rule: lock the owning run row in the same transaction, allocate the next `sequence_no` under that lock, then insert the history row
  - Does NOT wake the owning workflow run. Waiting runs remain `waiting`; suspended runs remain `suspended`
- When retries are exhausted (or `NonRetryableError` is returned):
  - The activity worker marks the task as `failed` (terminal for the task)
  - Attempts the existing `WakeWaitingRun` path. Waiting runs wake immediately; suspended runs remain suspended because the wake CAS no-ops when status is no longer `waiting`
  - The workflow activation sees `activity.failed` immediately for waiting runs, or after resume for suspended runs
- If no handler is registered for the task's `activity_type`, the worker keeps the current behavior: fail immediately with `activity_not_registered`, attempt the normal wake path, and do NOT spend retry attempts. Missing handlers are treated as non-retryable configuration errors, not transient failures. Suspended runs still remain suspended and observe the failure after resume.
- Backoff formula: `min(InitialBackoff * BackoffMultiplier^(attempt_count - 1), MaxBackoff)` where `attempt_count` is the current value AFTER claim increment. First retry (attempt_count=1) uses `InitialBackoff`, second retry (attempt_count=2) uses `InitialBackoff * BackoffMultiplier`, etc. Deterministic, no jitter in this phase.

### Schema
- Add typed retry columns to `engine.activity_tasks`: `max_attempts INT DEFAULT 1`, `initial_backoff_ms BIGINT`, `max_backoff_ms BIGINT`, `backoff_multiplier DOUBLE PRECISION`. All nullable except `max_attempts`. This avoids JSONB encoding ambiguity for `time.Duration` and makes the policy queryable/debuggable without deserialization.
- Add `RetryActivityTask` query that resets `claimed` → `queued` with new `available_at` derived in SQL from `NOW() + retry_delay_ms * INTERVAL '1 millisecond'` (CAS on `claimed_by`). The worker passes a computed integer-millisecond delay, not an absolute timestamp, so retry scheduling stays aligned with the same database clock used by `ClaimNextActivityTask`. Does NOT increment `attempt_count` — it was already incremented by `ClaimNextActivityTask`.
- The existing `ClaimNextActivityTask` already claims `queued` tasks with `available_at <= NOW()`, so retry-delayed tasks are naturally deferred

### Retry policy validation
- `MaxAttempts` MUST be >= 1 (1 means single attempt, no retries). Values <= 0 are rejected at scheduling time.
- When `MaxAttempts == 1`, backoff fields are ignored (no retries needed). Backoff columns are stored as NULL.
- When `MaxAttempts > 1`, all backoff fields are required and validated on their *persisted representation*:
  - `InitialBackoff.Milliseconds() >= 1` — rejects zero, negative, and sub-millisecond values (e.g. `500µs` truncates to `0ms`).
  - `MaxBackoff.Milliseconds() >= InitialBackoff.Milliseconds()`.
  - `BackoffMultiplier >= 1.0`.
- Validation happens in the workflow authoring layer (`ActivityWithOptions`) before the task is created. Invalid policies produce a deterministic workflow error (replay-safe).

### Non-retryable error handling scope
- This phase provides the Go-side `NonRetryableError` wrapper only. Activity handlers that want to bypass retry MUST wrap their errors explicitly.
- Configured `non_retryable_error_codes` (matching error codes from handler failures without explicit wrapping) is deferred to a future refinement. The `ActivityOptions` struct is extensible for this addition.

### History events
- Add `activity.retry_scheduled` — payload: `activity_key`, `activity_type`, `failed_attempt` (the attempt number that just failed, equal to `attempt_count` after claim increment), `next_available_at`, `error_code`, `error_message`
- The existing `activity.scheduled`, `activity.completed`, `activity.failed` events are unchanged
- Non-activation history events use one shared sequencing rule across activity retries and control-surface events: appenders lock the owning run row, compute `next_sequence` from the latest history row under that lock, and insert the new event in the same transaction so `UNIQUE (run_id, sequence_no)` cannot race

### Replay semantics
- `ActivityWithOptions` records `activity.scheduled` with the same key/type/input as `Activity()`; the retry policy is persisted on the task, not in the history event
- On replay, `activity.retry_scheduled` events are consumed as non-blocking informational events (they do not block the workflow and do not produce an outcome)
- The replay cursor advances past `activity.retry_scheduled` events between `activity.scheduled` and `activity.completed`/`activity.failed`

## Impact

- Affected specs (delta per capability):
  - `engine-runtime-execution` (ADDED: retry scheduling, non-retryable bypass, activity worker retry loop, final failure wake)
  - `engine-workflow-api` (ADDED: RetryPolicy, ActivityOptions, ActivityWithOptions, NonRetryableError — new capability)
  - `engine-schema-runtime-delta` (ADDED: typed retry columns — max_attempts, initial_backoff_ms, max_backoff_ms, backoff_multiplier — and RetryActivityTask query)
- Affected code:
  - `engine/pkg/workflow/context.go` — `ActivityWithOptions`, `RetryPolicy`, `ActivityOptions`, `NonRetryableError`, `IsNonRetryable`
  - `engine/pkg/history/history.go` — `activity.retry_scheduled` event type and payload
  - shared non-activation history append path in the engine/control store layer — allocate `sequence_no` under run lock for retry events
  - `engine/db/migrations/postgres/` — add typed retry columns (`max_attempts`, `initial_backoff_ms`, `max_backoff_ms`, `backoff_multiplier`) to `engine.activity_tasks`
  - `engine/db/queries/activity_tasks.sql` — `RetryActivityTask` query; update `CreateActivityTask` params
  - `engine/internal/workflow/replay.go` — advance cursor past `activity.retry_scheduled` events during replay
  - `engine/internal/workflow/activation.go` — pass retry policy from decision to `CreateActivityTask`
  - `engine/internal/activity/worker.go` — retry-or-fail decision after task handler failure
- No API surface change — retries are internal to the engine runtime and workflow authoring layer
- No new maintenance loop — retries use existing `available_at` scheduling on `engine.activity_tasks`
- No change to existing `Activity()` behavior — it remains single-attempt

## Assumptions

- `add-engine-suspend-resume-control` is implemented; the `SUSPENDED` status is frozen
- The existing `attempt_count` column on `engine.activity_tasks` accurately tracks total claims (incremented by `ClaimNextActivityTask` on every claim, including retry claims). The retry exhaustion check uses `attempt_count >= max_attempts` after the claim increment. This means `max_attempts = 3` allows 3 total claim+execute cycles.
- The activity worker operates outside the activation transaction (at-least-once semantics); retry scheduling extends this existing pattern
- Jitter is deferred to a future change; deterministic backoff is sufficient for this phase
- Activity heartbeats are out of scope; `ActivityOptions` is extensible for future addition
- Configured `non_retryable_error_codes` matching is deferred; only explicit `NonRetryableError` wrapping bypasses retry in this phase
