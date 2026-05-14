## ADDED Requirements

### Requirement: RetryPolicy Type
The engine workflow package SHALL expose a `RetryPolicy` struct with fields: `MaxAttempts int`, `InitialBackoff time.Duration`, `MaxBackoff time.Duration`, `BackoffMultiplier float64`.

`MaxAttempts` represents the total number of attempts including the initial one. A value of 1 means no retries.

#### Scenario: Retry policy creation
- **WHEN** a workflow author creates a `RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, MaxBackoff: 30 * time.Second, BackoffMultiplier: 2.0}`
- **THEN** the policy allows up to 3 total attempts with exponential backoff from 1s to 30s

### Requirement: RetryPolicy Validation
`ActivityWithOptions` SHALL validate the retry policy at scheduling time and return a deterministic error for invalid policies.

When `MaxAttempts == 1`, backoff fields are ignored (single attempt, no retries). Backoff columns are stored as NULL.

When `MaxAttempts > 1`, all backoff fields are required and validated on their persisted representation: `InitialBackoff.Milliseconds() >= 1` (rejects sub-millisecond values that truncate to 0ms), `MaxBackoff.Milliseconds() >= InitialBackoff.Milliseconds()`, `BackoffMultiplier >= 1.0`.

`MaxAttempts >= 1` is always required. Invalid policies are rejected before the scheduled event is emitted and before the task is created.

#### Scenario: Invalid MaxAttempts
- **WHEN** a workflow calls `ActivityWithOptions` with `MaxAttempts = 0`
- **THEN** a deterministic error is returned (replay-safe)

#### Scenario: Invalid backoff
- **WHEN** a workflow calls `ActivityWithOptions` with `MaxAttempts = 3` and `InitialBackoff = 500µs` (truncates to 0ms)
- **THEN** a deterministic error is returned (replay-safe)

#### Scenario: Missing backoff fields with retries
- **WHEN** a workflow calls `ActivityWithOptions` with `MaxAttempts = 3` and zero-value `InitialBackoff`
- **THEN** a deterministic error is returned (replay-safe)

#### Scenario: MaxAttempts 1 ignores backoff
- **WHEN** a workflow calls `ActivityWithOptions` with `MaxAttempts = 1` and zero-value backoff fields
- **THEN** the activity is scheduled as single-attempt (backoff columns stored as NULL)

### Requirement: ActivityOptions Type
The engine workflow package SHALL expose an `ActivityOptions` struct with field: `RetryPolicy *RetryPolicy`.

`ActivityOptions` is extensible for future options (e.g., heartbeat timeout, non_retryable_error_codes) without breaking the API.

#### Scenario: Options with retry policy
- **WHEN** a workflow author creates `ActivityOptions{RetryPolicy: &RetryPolicy{MaxAttempts: 5, InitialBackoff: time.Second, MaxBackoff: 30 * time.Second, BackoffMultiplier: 2.0}}`
- **THEN** the options are valid and can be passed to `ActivityWithOptions`

### Requirement: ActivityWithOptions Context Method
The `Context` interface SHALL expose `ActivityWithOptions(key, activityType string, input any, out any, opts ActivityOptions) error`.

`ActivityWithOptions` SHALL behave identically to `Activity` for scheduling and replay, with the addition that the retry policy from options is persisted as typed columns on the created activity task.

#### Scenario: Activity with retry options
- **WHEN** a workflow calls `ctx.ActivityWithOptions("fetch", "http", input, &out, opts)` where opts includes a retry policy
- **THEN** the activity is scheduled with the retry policy columns set on the task
- **AND** retries are handled by the activity worker according to the policy

#### Scenario: Activity with empty options
- **WHEN** a workflow calls `ctx.ActivityWithOptions("fetch", "http", input, &out, ActivityOptions{})` with no retry policy
- **THEN** the activity behaves identically to `ctx.Activity("fetch", "http", input, &out)` (single attempt, max_attempts=1)

### Requirement: NonRetryableError Wrapper
The engine workflow package SHALL expose `NonRetryableError(err error) error` and `IsNonRetryable(err error) bool`.

`NonRetryableError` wraps an error to signal that the activity handler failure should not be retried. `IsNonRetryable` checks for the wrapper using `errors.As`.

This phase provides only the explicit Go wrapper. Configured `non_retryable_error_codes` (a list of error code strings that automatically bypass retry) is deferred. The `ActivityOptions` struct is extensible for this future addition.

#### Scenario: Non-retryable wrapping
- **WHEN** an activity handler returns `workflow.NonRetryableError(fmt.Errorf("bad input"))`
- **THEN** `workflow.IsNonRetryable(err)` returns `true`
- **AND** the underlying error message is preserved

#### Scenario: Regular error is retryable
- **WHEN** an activity handler returns `fmt.Errorf("timeout")`
- **THEN** `workflow.IsNonRetryable(err)` returns `false`

### Requirement: Replay Advances Past Retry Events
During replay, the replay engine SHALL advance the history cursor past `activity.retry_scheduled` events that appear between `activity.scheduled` and the final `activity.completed` or `activity.failed` event.

`activity.retry_scheduled` events SHALL NOT block the workflow or produce an outcome.

#### Scenario: Replay with retry history
- **WHEN** a workflow replays history containing `activity.scheduled` → `activity.retry_scheduled` → `activity.retry_scheduled` → `activity.completed`
- **THEN** the replay cursor advances past the retry events and returns the completed output
