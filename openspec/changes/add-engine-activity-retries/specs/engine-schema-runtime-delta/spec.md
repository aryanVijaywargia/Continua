## ADDED Requirements

### Requirement: Activity Task Retry Columns
The `engine.activity_tasks` table SHALL have typed retry columns: `max_attempts INT DEFAULT 1 NOT NULL`, `initial_backoff_ms BIGINT`, `max_backoff_ms BIGINT`, `backoff_multiplier DOUBLE PRECISION`.

`max_attempts` defaults to 1 (single attempt, no retries). The backoff columns are nullable; NULL indicates a single-attempt activity with no retry policy.

Typed columns are used instead of JSONB to avoid duration encoding ambiguity and to make the policy queryable and debuggable without deserialization. `backoff_multiplier` uses `DOUBLE PRECISION` (not `NUMERIC`) so sqlc generates `float64` directly without requiring a `pgtype.Numeric` conversion or a sqlc override.

#### Scenario: Activity task with retry policy
- **WHEN** an activity task is created via `ActivityWithOptions` with a retry policy
- **THEN** `max_attempts` contains the total attempt count
- **AND** `initial_backoff_ms`, `max_backoff_ms`, `backoff_multiplier` contain the policy values

#### Scenario: Activity task without retry policy
- **WHEN** an activity task is created via `Activity()` (no options)
- **THEN** `max_attempts` is 1
- **AND** `initial_backoff_ms`, `max_backoff_ms`, `backoff_multiplier` are NULL

#### Scenario: Existing tasks after migration
- **WHEN** existing activity tasks are read after the migration
- **THEN** `max_attempts` is 1 for all existing rows
- **AND** backoff columns are NULL

### Requirement: RetryActivityTask Query
The engine SHALL provide a `RetryActivityTask` query that resets a claimed activity task to `queued` status with a new `available_at` for the next retry attempt.

The query SHALL use CAS on `id`, `status = 'claimed'`, and `claimed_by` to prevent stale retries.

The query SHALL derive `available_at` in SQL from the database clock as `NOW() + retry_delay_ms * INTERVAL '1 millisecond'` using a passed integer-millisecond delay value, not from a worker-supplied absolute timestamp. This keeps retry scheduling aligned with `ClaimNextActivityTask`, which also evaluates claimability against `NOW()`.

The query SHALL clear `claimed_by`, `claimed_at`, `lease_expires_at` and set `updated_at = NOW()`.

The query SHALL NOT modify `attempt_count` — it was already incremented by `ClaimNextActivityTask` on claim.

Retry-related history appends SHALL use the shared non-activation history sequencing rule: lock the owning run row, compute the next `sequence_no` under that lock, and insert the history row in the same transaction as the task reset.

#### Scenario: Successful retry reset
- **WHEN** the activity worker calls `RetryActivityTask` with a valid claim
- **THEN** the task transitions from `claimed` to `queued` with the new `available_at`
- **AND** the claim fields are cleared
- **AND** `attempt_count` is unchanged

#### Scenario: Retry scheduling uses database clock
- **WHEN** the activity worker calls `RetryActivityTask` with a computed retry delay
- **THEN** the query derives `available_at` from the database clock rather than a worker-supplied absolute timestamp
- **AND** the delay units are integer milliseconds, matching the ceiled backoff computation
- **AND** subsequent claimability is evaluated against that same database clock

#### Scenario: Stale retry rejected
- **WHEN** the activity worker calls `RetryActivityTask` with a stale `claimed_by`
- **THEN** the query returns no rows (CAS fails)
- **AND** the store wrapper returns `ErrStaleClaim`
- **AND** the activity worker logs the stale retry and returns nil (no error propagated), matching the existing stale-complete/stale-fail pattern
