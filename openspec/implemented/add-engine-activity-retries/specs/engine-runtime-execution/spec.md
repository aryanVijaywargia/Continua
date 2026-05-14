## ADDED Requirements

### Requirement: Activity Retry Scheduling
The engine activity worker SHALL support automatic retry scheduling for activity tasks that have remaining attempts.

When a registered activity handler fails and `attempt_count < max_attempts` (and the error is not `NonRetryableError`), the activity worker SHALL reset the task to `queued` status with `available_at` derived from the computed retry delay using the database clock.

`attempt_count` is the value AFTER the `ClaimNextActivityTask` increment and SHALL NOT be modified by the retry decision.

The worker SHALL NOT wake the owning workflow run during retry scheduling. Waiting runs remain `waiting`; suspended runs remain `suspended`.

#### Scenario: Retryable failure with remaining attempts
- **WHEN** an activity task fails with a retryable error
- **AND** `attempt_count < max_attempts`
- **THEN** the task is reset to `queued` with `available_at = NOW() + backoff` using the database clock
- **AND** an `activity.retry_scheduled` history event is appended
- **AND** the owning workflow run is NOT waked

#### Scenario: Retry scheduled while suspended
- **WHEN** an activity task fails with a retryable error
- **AND** `attempt_count < max_attempts`
- **AND** the owning run is `suspended`
- **THEN** the retry is scheduled with a future `available_at`
- **AND** the run remains `suspended`
- **AND** no workflow activation is triggered

#### Scenario: Retries exhausted
- **WHEN** an activity task fails
- **AND** `attempt_count >= max_attempts`
- **THEN** the task is marked as `failed` (terminal)
- **AND** the worker attempts the normal wake path
- **AND** the next activation sees `activity.failed` immediately for waiting runs or after resume for suspended runs

### Requirement: Non-Retryable Error Bypass
When an activity handler returns an error wrapped with `NonRetryableError`, the activity worker SHALL skip retry scheduling and mark the task as `failed` immediately, regardless of remaining attempts.

This phase provides the Go-side `NonRetryableError` wrapper only. Configured `non_retryable_error_codes` matching (bypassing retry based on error code strings without explicit wrapping) is deferred to a future refinement.

#### Scenario: Non-retryable error
- **WHEN** an activity task fails with a `NonRetryableError`
- **AND** the task has remaining attempts
- **THEN** the task is marked as `failed` immediately
- **AND** the worker attempts the normal wake path
- **AND** the next activation sees `activity.failed` immediately for waiting runs or after resume for suspended runs

### Requirement: Missing Activity Handler Fails Immediately
If no handler is registered for a claimed activity task's `activity_type`, the activity worker SHALL treat that as an immediate non-retryable failure instead of consuming retry attempts.

The worker SHALL mark the task as `failed` with error code `activity_not_registered`, SHALL NOT append `activity.retry_scheduled`, and SHALL attempt the normal wake path so the next activation observes `activity.failed` immediately for waiting runs or after resume for suspended runs.

#### Scenario: Missing handler on a retry-configured task
- **WHEN** an activity task is claimed
- **AND** its `activity_type` has no registered handler
- **AND** the task still has remaining retry attempts
- **THEN** the task is marked as `failed` immediately with error code `activity_not_registered`
- **AND** no `activity.retry_scheduled` event is appended
- **AND** the worker attempts the normal wake path

### Requirement: Terminal Activity Failures Respect Suspension
For terminal activity outcomes (single-attempt failure, retries exhausted, `NonRetryableError`, or missing handler), the activity worker SHALL attempt the existing `WakeWaitingRun` path after persisting the terminal task status.

If the run is still `waiting`, the wake succeeds and the next activation observes `activity.failed`. If the run is `suspended`, the wake CAS no-ops because the status is no longer `waiting`, and the failure remains recorded until resume.

#### Scenario: Exhausted retries while suspended
- **WHEN** an activity task reaches terminal failure because retries are exhausted
- **AND** the owning run is `suspended`
- **THEN** the task is marked as `failed`
- **AND** the run remains `suspended`
- **AND** the failure is observed on the first activation after resume

### Requirement: Deterministic Backoff Computation
The raw backoff duration SHALL be computed as `min(initial_backoff_ms * backoff_multiplier^(attempt_count - 1), max_backoff_ms)` where `attempt_count` is the value AFTER the `ClaimNextActivityTask` increment.

The concrete scheduled retry delay SHALL be `ceil(raw_backoff_ms)` whole milliseconds before deriving `available_at`. This rounding rule SHALL be deterministic and SHALL ensure the worker never schedules earlier than the raw formula.

That concrete delay SHALL be passed as an integer-millisecond value and applied against the database clock when setting `available_at`, so retry scheduling and claimability both use the same notion of `NOW()`.

The first retry (attempt_count=1) waits exactly `initial_backoff_ms`. The second retry (attempt_count=2) waits `initial_backoff_ms * backoff_multiplier` before whole-millisecond rounding is applied.

#### Scenario: First retry delay
- **WHEN** `initial_backoff_ms = 1000`, `backoff_multiplier = 2.0`, `max_backoff_ms = 30000`
- **AND** `attempt_count = 1` (first attempt just failed)
- **THEN** backoff is `min(1000 * 2.0^0, 30000) = 1000ms`

#### Scenario: Exponential backoff
- **WHEN** `initial_backoff_ms = 1000`, `backoff_multiplier = 2.0`, `max_backoff_ms = 30000`
- **THEN** attempt_count=2 backoff is `min(1000 * 2.0^1, 30000) = 2000ms`
- **AND** attempt_count=3 backoff is `min(1000 * 2.0^2, 30000) = 4000ms`

#### Scenario: Max backoff cap
- **WHEN** the computed backoff exceeds `max_backoff_ms`
- **THEN** the backoff is capped at `max_backoff_ms`

#### Scenario: Fractional backoff rounds up
- **WHEN** `initial_backoff_ms = 333`, `backoff_multiplier = 1.5`, `max_backoff_ms = 30000`
- **AND** `attempt_count = 2`
- **THEN** the raw backoff is `min(333 * 1.5^1, 30000) = 499.5ms`
- **AND** the concrete scheduled delay is `500ms`

### Requirement: Retry History Events
The engine SHALL append an `activity.retry_scheduled` history event for each retry attempt, containing the activity key, activity type, `failed_attempt` (the attempt number that just failed, equal to `attempt_count` after claim increment), next available_at, and the error that triggered the retry.

The retry history event SHALL be appended in the same transaction as the task status reset.

Its `sequence_no` SHALL be allocated using the shared non-activation history sequencing rule so retry events cannot collide with suspend/resume or other control-path history appends for the same run.

#### Scenario: Retry event recorded
- **WHEN** an activity task is retried
- **THEN** an `activity.retry_scheduled` event is appended with `failed_attempt` (the attempt that just failed) and next available_at

### Requirement: Single-Attempt Activity Unchanged
`Activity(key, type, input, out)` SHALL remain single-attempt with no retry behavior. It is equivalent to `ActivityWithOptions` with empty options (max_attempts = 1).

#### Scenario: Single-attempt failure
- **WHEN** an activity scheduled via `Activity()` fails
- **THEN** the task is marked as `failed` immediately (no retry)
- **AND** the worker attempts the normal wake path

### Requirement: Retry Scheduling Uses Existing Available_At
Activity retry scheduling SHALL use the existing `available_at` column on `engine.activity_tasks` to defer the next attempt. No new maintenance loop, inbox kind, or timer mechanism SHALL be introduced.

The existing `ClaimNextActivityTask` query naturally defers retry-delayed tasks because it filters `available_at <= NOW()`.

#### Scenario: Deferred retry not claimable
- **WHEN** an activity task is reset to `queued` with `available_at` in the future
- **THEN** the task is not claimable until `available_at` passes
