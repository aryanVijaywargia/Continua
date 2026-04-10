# Design: Engine Activity Retries

## Context

This is the second change in the Phase 4 runtime lifecycle plan. It adds first-class activity retries to the engine runtime, allowing workflow authors to attach retry policies to activities and have the engine handle backoff scheduling automatically.

The engine currently supports single-attempt activities: `Activity(key, type, input, out)` schedules an activity task, the activity worker executes it, and the result (completed or failed) is delivered to the workflow on the next activation. There is no retry mechanism — all retry logic must be hand-coded in the workflow definition.

The activity worker already operates outside the activation transaction with at-least-once semantics, tracks `attempt_count`, and uses CAS on `claimed_by` for completion/failure. The retry mechanism extends this existing pattern by resetting failed tasks to `queued` with a future `available_at` instead of marking them as terminally failed.

## Goals / Non-Goals

### Goals
- Allow workflow authors to declare retry policies on activities via `ActivityWithOptions`
- Provide deterministic exponential backoff with configurable initial delay, max delay, and multiplier
- Allow activity handlers to signal non-retryable failures via `NonRetryableError`
- Keep `Activity()` as zero-config single-attempt sugar
- Handle retries entirely within the activity worker without waking the workflow
- Persist retry policy durably so it survives worker restarts

### Non-Goals
- Jitter (randomized backoff) — deferred to a future refinement
- Activity heartbeats and heartbeat-based timeout
- Server-side activity timeout (start-to-close timeout)
- Retry policy modification after scheduling
- Workflow-side retry observation (the workflow only sees the final outcome)
- Per-retry custom status updates visible to the workflow

## Decisions

### Decision: Retries are handled entirely by the activity worker, not the workflow
- When an activity task fails and has remaining retry attempts, the activity worker resets the task to `queued` with a new `available_at`. No wake occurs: waiting runs stay `waiting`, and externally suspended runs stay `suspended`.
- Only the final outcome (success or terminal failure) attempts the normal wake path; waiting runs wake immediately, while suspended runs remain suspended until resume.
- This keeps the workflow's replay deterministic: the workflow sees `activity.scheduled` followed by either `activity.completed` or `activity.failed`, with optional `activity.retry_scheduled` events in between that are informational only.
- **Alternatives considered:** delivering each retry outcome to the workflow (the workflow decides whether to retry). Rejected — this couples retry logic to the workflow definition, makes replay dependent on retry timing, and defeats the purpose of declarative retry policies.

### Decision: Non-activation history appenders share one sequence-allocation rule
- `engine.history` sequence numbers remain caller-assigned, so every non-activation appender must serialize sequence allocation per run. The shared rule is: lock the owning run row `FOR UPDATE`, read the latest history row for that run under the same transaction, allocate `next_sequence = last_sequence + 1` (or `1` when empty), then call `AppendHistory`.
- Activity retry events, suspend/resume events, and terminate/control-path events all follow this same rule. Interleaving in commit order is allowed, but duplicate `sequence_no` allocation for the same run is not.
- This matches the existing control-path pattern in `internal/api/engine_control.go` and extends it to activity-worker retry events.
- **Alternatives considered:** introducing a separate SQL allocator immediately. Rejected for this phase — the run-lock allocation rule already matches current code and is sufficient as the shared contract. A dedicated allocator/helper can implement the same rule behind one function later.

### Decision: Retry scheduling uses existing `available_at` on `engine.activity_tasks`
- The existing `ClaimNextActivityTask` query claims tasks where `status = 'queued' AND available_at <= NOW()`. Setting `available_at` to a future timestamp naturally defers the next attempt.
- No new maintenance loop, inbox kind, or timer mechanism is needed.
- This was explicitly called out in the maintenance ownership rules recorded in the previous change.
- **Alternatives considered:** creating a timer inbox item for each retry. Rejected — timers are for workflow-level waits; activity retries are an internal engine concern.

### Decision: RetryPolicy is persisted as typed columns on the activity task row
- `max_attempts INT DEFAULT 1` — total attempts including the initial one. Default 1 means no retries.
- `initial_backoff_ms BIGINT` — initial backoff in milliseconds. NULL for single-attempt tasks.
- `max_backoff_ms BIGINT` — maximum backoff cap in milliseconds. NULL for single-attempt tasks.
- `backoff_multiplier DOUBLE PRECISION` — exponential multiplier. NULL for single-attempt tasks. `DOUBLE PRECISION` is used instead of `NUMERIC` because: sqlc maps `DOUBLE PRECISION` directly to `float64` (no `pgtype.Numeric` conversion needed), the multiplier is only used for Go-side arithmetic and doesn't need SQL-side decimal precision, and it avoids range-overflow concerns entirely.
- Typed columns are preferred over JSONB because: the policy fields are fixed and drive concrete arithmetic; `time.Duration` encoding in JSONB is ambiguous (nanoseconds? string? seconds?); typed columns are queryable and debuggable without deserialization; they avoid duplicating `max_attempts` across a denormalized column and a JSONB blob.
- The policy is set at task creation time from `ActivityOptions` and is immutable for the task's lifetime.
- **Go→DB conversion and validation-on-persisted-representation:** Validation operates on the persisted representation, not the Go-side input, to prevent silent lossy conversions. `time.Duration` values are converted to whole milliseconds via `dur.Milliseconds()`. Validation then checks the *converted* millisecond value: `initial_backoff_ms >= 1` (rejects sub-millisecond durations like `500µs` that would truncate to `0ms`), `max_backoff_ms >= initial_backoff_ms`. `BackoffMultiplier float64` is stored as `DOUBLE PRECISION` (maps directly to `float64` in sqlc — no `pgtype.Numeric` conversion needed). Validation checks `BackoffMultiplier >= 1.0` on the Go-side `float64` value, which is the same type stored. All validation happens fail-fast inside `ActivityWithOptions` before the task is created, preserving the replay-safe contract. One exact-value test SHALL verify that `1500ms` → `1500` and sub-millisecond rejection.
- **Alternatives considered:** `retry_policy JSONB` with a denormalized `max_attempts`. Rejected — JSONB adds encoding ambiguity, especially for duration fields, and duplicates the max_attempts field for no benefit. Typed additive columns are cleaner for a fixed schema.

### Decision: Deterministic backoff formula without jitter, with authoritative attempt_count model
- Raw formula: `min(InitialBackoff * BackoffMultiplier^(attempt_count - 1), MaxBackoff)` where `attempt_count` is the value AFTER the `ClaimNextActivityTask` increment.
- Concrete scheduling rule: the worker computes the raw backoff in `float64` milliseconds, rounds it up with `ceil` to the next whole millisecond, and uses that integer-millisecond delay to derive the next `available_at`. This locks deterministic claim timing for fractional results while ensuring the retry is never scheduled earlier than the formula. Example: `333ms * 1.5 = 499.5ms` schedules `500ms` later.
- **Authoritative model:** `ClaimNextActivityTask` increments `attempt_count` on every claim (including retry claims). After the first claim and failure, `attempt_count = 1`. After the second claim and failure, `attempt_count = 2`. The retry exhaustion check is `attempt_count >= max_attempts`.
- **First-retry delay:** when `attempt_count = 1` (first attempt just failed), backoff = `InitialBackoff * BackoffMultiplier^0 = InitialBackoff`. The first retry waits exactly `InitialBackoff`.
- **Example:** `InitialBackoff=1s, BackoffMultiplier=2.0, MaxAttempts=4`:
  - attempt_count=1 (1st attempt failed): backoff = 1s, retry
  - attempt_count=2 (2nd attempt failed): backoff = 2s, retry
  - attempt_count=3 (3rd attempt failed): backoff = 4s, retry
  - attempt_count=4 (4th attempt failed): exhausted, final failure
- Jitter is deferred because it introduces non-determinism in the retry schedule. Deterministic backoff is sufficient for correctness and simplifies testing.
- **Alternatives considered:** full jitter, decorrelated jitter. Both deferred — they improve cluster-level load distribution but are not needed for correctness.

### Decision: `activity.retry_scheduled` is a non-blocking informational history event
- The event records the retry for auditability and debugger timeline display.
- On replay, the cursor advances past `activity.retry_scheduled` events without blocking or producing an outcome.
- The replay engine treats the event sequence as: `activity.scheduled` → zero or more `activity.retry_scheduled` → `activity.completed` or `activity.failed`.
- **Implementation:** in `replay.go`, after consuming `activity.scheduled` and before checking for the outcome event, advance past any `activity.retry_scheduled` events with a matching `activity_key`.

### Decision: `NonRetryableError` is a Go error wrapper, not a history payload field
- `workflow.NonRetryableError(err)` wraps an error; `workflow.IsNonRetryable(err)` checks the wrapper using `errors.As`.
- The activity handler returns this error, and the activity worker checks it before deciding to retry.
- The non-retryable status is NOT persisted in history — it's a handler-side decision. The history only records `activity.failed` (final) or `activity.retry_scheduled` (intermediate).
- **Scope limitation:** this phase provides only the explicit Go wrapper. Configured `non_retryable_error_codes` (a list of error code strings that automatically bypass retry without wrapper) is deferred. The `ActivityOptions` struct is extensible for this future addition.
- **Alternatives considered:** a `non_retryable: true` flag in `activity.failed` payload. Rejected — the flag is meaningful only to the retry decision, which is already made before the event is written. A `non_retryable_error_codes` field on `ActivityOptions` is the natural next step but adds scope beyond the core retry mechanism.

### Decision: Missing activity handlers remain immediate non-retryable failures
- Handler lookup happens before the retry decision. If no handler is registered for the task's `activity_type`, the worker keeps the current behavior: fail the task immediately with error code `activity_not_registered`, attempt the normal `WakeWaitingRun` path, and do NOT append `activity.retry_scheduled`.
- If the run is still `waiting`, the wake succeeds and the next activation observes `activity.failed`. If the run is `suspended`, `WakeWaitingRun` no-ops because the status is no longer `waiting`, and the failure remains recorded until resume.
- This bypasses retry scheduling even when the task row carries remaining retry attempts. A registry miss is a deployment/configuration mismatch, not a transient handler failure, so retrying would only delay a deterministic failure.
- **Alternatives considered:** letting missing handlers consume retry attempts. Rejected — it hides misconfiguration, creates pointless retry churn, and diverges from the current worker behavior.

### Decision: Retry policy validation is fail-fast at scheduling time, on persisted representation
- Validation happens inside `ActivityWithOptions` before the scheduled event is emitted and before the task is created. Invalid policies produce a deterministic error that is replay-safe.
- When `MaxAttempts == 1`: backoff fields are ignored (single attempt, no retries). No backoff validation is performed. Backoff columns are stored as NULL.
- When `MaxAttempts > 1`: all backoff fields are required and validated on their persisted representation:
  - `MaxAttempts >= 1`. Values <= 0 are rejected.
  - `InitialBackoff.Milliseconds() >= 1` (rejects zero, negative, and sub-millisecond values like `500µs` that would truncate to `0ms`).
  - `MaxBackoff.Milliseconds() >= InitialBackoff.Milliseconds()`. Validated on converted millisecond values.
  - `BackoffMultiplier >= 1.0`. Validated on the Go-side `float64` (same type stored as `DOUBLE PRECISION`).
- **Alternatives considered:** clamping invalid values silently. Rejected — silent clamping hides bugs in workflow definitions and makes debugging harder. Validating on Go-side `time.Duration` without converting first. Rejected — this allows sub-millisecond values that silently become `0ms` after truncation.

### Decision: `Activity()` remains single-attempt sugar with no behavioral change
- `Activity(key, type, input, out)` is equivalent to `ActivityWithOptions(key, type, input, out, ActivityOptions{})` — no retry policy, max_attempts defaults to 1.
- `max_attempts` is 1 on the task row, and `initial_backoff_ms`, `max_backoff_ms`, `backoff_multiplier` are NULL.
- Existing workflows using `Activity()` see zero behavioral change.

### Decision: The activity worker retry path uses RetryActivityTask or FailActivityTask based on a Go-side decision
- Handler lookup happens before the retry branch. If no handler is registered, the worker uses the existing immediate-failure path with `activity_not_registered` and attempts the normal wake path without consulting retry policy.
- After a registered task handler fails, the worker reads the task row (which includes `max_attempts`, `initial_backoff_ms`, `max_backoff_ms`, `backoff_multiplier`) and the current `attempt_count` (already incremented by `ClaimNextActivityTask`).
- If `attempt_count < max_attempts` and the error is not `NonRetryableError`:
  1. Compute the raw backoff from the typed columns: `min(initial_backoff_ms * backoff_multiplier^(attempt_count-1), max_backoff_ms)`.
  2. Convert that raw value to a concrete delay as `ceil(raw_backoff_ms)` whole milliseconds.
  3. Pass that integer-millisecond delay into `RetryActivityTask` as `retry_delay_ms`; the SQL query derives `available_at` from `NOW() + retry_delay_ms * INTERVAL '1 millisecond'` using the database clock and clears claim fields. This avoids worker/database clock skew, which matters because claimability is also evaluated against `NOW()` in `ClaimNextActivityTask`. Does NOT touch `attempt_count`.
  4. Append `activity.retry_scheduled` history event using the shared non-activation history sequencing rule under a run lock in the same transaction.
  5. Commit. Do NOT wake the workflow run.
- If `attempt_count >= max_attempts` or the error is `NonRetryableError`:
  1. `FailActivityTask` as today.
  2. Attempt `WakeWaitingRun` as today. Waiting runs wake immediately; suspended runs do not wake because the CAS requires `status = 'waiting'`, so the terminal outcome is surfaced after resume.
- **Alternatives considered:** a combined `RetryOrFailActivityTask` query. Rejected — separating the decision (Go code) from the mutation (SQL) is clearer and easier to test.

## Risks / Trade-offs

- **No jitter:** multiple retrying tasks on the same activity type may thunder-herd at the same backoff intervals. Mitigation: for this phase, the risk is low because engine workloads are not yet at scale. Jitter can be added as a backward-compatible refinement.

- **Activity worker complexity:** the retry decision adds a code path to the activity worker's completion handler. Mitigation: the decision is a simple check (remaining attempts > 0 AND not non-retryable) with a clear SQL mutation.

- **Replay cursor advancement past retry events:** if `activity.retry_scheduled` events are not correctly skipped, replay will produce a mismatch. Mitigation: add explicit tests with multi-retry histories.

- **`attempt_count` semantics:** `ClaimNextActivityTask` already increments `attempt_count` on every claim (including retry claims). This means `attempt_count` represents total claims, not just retries. The retry decision compares `attempt_count` against `max_attempts`. Mitigation: document that `max_attempts` is the total number of attempts including the initial one, consistent with `attempt_count` semantics.

## Migration Plan

- One migration adding `max_attempts INT DEFAULT 1 NOT NULL`, `initial_backoff_ms BIGINT`, `max_backoff_ms BIGINT`, `backoff_multiplier DOUBLE PRECISION` to `engine.activity_tasks`. The `max_attempts` default of 1 ensures existing tasks behave as single-attempt. The backoff columns default to NULL. `backoff_multiplier` uses `DOUBLE PRECISION` (not `NUMERIC`) so sqlc generates `float64` directly — no `pgtype.Numeric` conversion needed.
- No migration on `public.traces` or `engine.runs`.
- `ActivityWithOptions` is additive to `workflow.Context`; `Activity` behavior is unchanged.
- Existing activity workers handle tasks without retry policies identically to today (max_attempts=1 means exhaustion on first failure).
