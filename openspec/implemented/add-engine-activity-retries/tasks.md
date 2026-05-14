## 1. Schema Migration

- [x] 1.1 Add engine migration adding typed retry columns to `engine.activity_tasks`: `max_attempts INT DEFAULT 1 NOT NULL`, `initial_backoff_ms BIGINT`, `max_backoff_ms BIGINT`, `backoff_multiplier DOUBLE PRECISION` (all nullable except `max_attempts`)
- [x] 1.2 Run `make generate` for the engine sqlc layer and confirm the generated Go types include the new columns
- [x] 1.3 Verify that the default `max_attempts = 1` means existing tasks behave as single-attempt without any code change
- [x] 1.4 Add a unit test verifying Go→DB conversion: `1500*time.Millisecond` → `1500` (initial_backoff_ms), `2.5` → `2.5` (backoff_multiplier as DOUBLE PRECISION / float64), and that sub-millisecond durations are rejected by validation (not silently truncated to 0ms)

**Validation:** migration applies cleanly; generated types compile; existing `CreateActivityTask` calls work without modification

## 2. Workflow Authoring API

- [x] 2.1 Add `RetryPolicy` struct to `engine/pkg/workflow/`: `MaxAttempts int`, `InitialBackoff time.Duration`, `MaxBackoff time.Duration`, `BackoffMultiplier float64`
- [x] 2.2 Add `ActivityOptions` struct: `RetryPolicy *RetryPolicy`
- [x] 2.3 Add `ActivityWithOptions(key, activityType string, input any, out any, opts ActivityOptions) error` to the `Context` interface
- [x] 2.4 Add retry policy validation inside `ActivityWithOptions` (fail-fast, replay-safe). When `MaxAttempts == 1`, skip backoff validation (backoff columns stored as NULL). When `MaxAttempts > 1`, validate on persisted representation: `InitialBackoff.Milliseconds() >= 1` (rejects sub-ms values), `MaxBackoff.Milliseconds() >= InitialBackoff.Milliseconds()`, `BackoffMultiplier >= 1.0`. Always: `MaxAttempts >= 1`. Invalid policies return a deterministic error from the Context method.
- [x] 2.5 Add `NonRetryableError(err error) error` function that wraps an error with a non-retryable marker
- [x] 2.6 Add `IsNonRetryable(err error) bool` function that checks the wrapper via `errors.As`
- [x] 2.7 Verify that `Activity(key, type, input, out)` remains unchanged and is equivalent to `ActivityWithOptions` with empty options
- [x] 2.8 Add unit tests for `NonRetryableError`/`IsNonRetryable` wrapping and unwrapping
- [x] 2.9 Add unit tests for retry policy validation: valid policy accepted, MaxAttempts=0 rejected, sub-millisecond InitialBackoff rejected (e.g. 500µs → 0ms), MaxBackoff < InitialBackoff rejected, BackoffMultiplier < 1.0 rejected, MaxAttempts=1 with zero-value backoff fields accepted (backoff ignored)

**Validation:** interface compiles; `NonRetryableError` roundtrips correctly; `Activity` signature is unchanged; invalid policies rejected deterministically

## 3. History Event Type

- [x] 3.1 Add `EventActivityRetryScheduled = "activity.retry_scheduled"` constant in `engine/pkg/history/history.go`
- [x] 3.2 Add `ActivityRetryScheduledPayload` struct: `ActivityKey string`, `ActivityType string`, `FailedAttempt int32` (the attempt number that just failed, equal to `attempt_count` after claim increment), `NextAvailableAt time.Time`, `ErrorCode string`, `ErrorMessage string`
- [x] 3.3 Register the event type in `payloadTarget()` and `DecodePayload()`
- [x] 3.4 Add unit tests for encoding/decoding

**Validation:** event type serializes and deserializes correctly

## 4. Engine Queries

- [x] 4.1 Update `CreateActivityTask` params in `engine/db/queries/activity_tasks.sql` to accept `max_attempts INT`, `initial_backoff_ms BIGINT`, `max_backoff_ms BIGINT`, `backoff_multiplier DOUBLE PRECISION`
- [x] 4.2 Add `RetryActivityTask :one` query: CAS on `id = $1 AND status = 'claimed' AND claimed_by = $2`, set `status = 'queued'`, `available_at = NOW() + (sqlc.arg(retry_delay_ms)::bigint * INTERVAL '1 millisecond')`, clear `claimed_by`/`claimed_at`/`lease_expires_at`, `updated_at = NOW()`, return the updated row. The worker passes an integer-millisecond delay value, not an absolute timestamp, so retry scheduling uses the same database clock that `ClaimNextActivityTask` uses for claimability and the units match the ceiled `raw_backoff_ms` computation. Does NOT modify `attempt_count`. The store wrapper SHALL return `store.ErrStaleClaim` when the CAS matches no rows, matching the existing `CompleteActivityTask`/`FailActivityTask` pattern.
- [x] 4.3 Run `make generate` and confirm the generated Go functions compile with the new parameters

**Validation:** generate succeeds; `RetryActivityTask` compiles with a `retry_delay_ms` parameter; retry scheduling is DB-clock-based; `CreateActivityTask` accepts retry columns

## 5. Replay Engine Updates

- [x] 5.1 Implement `ActivityWithOptions` on `workflowRunner` in `engine/internal/workflow/replay.go`: validate retry policy, extract retry fields from options, pass them through the `activationDecision` for task creation
- [x] 5.2 Add retry column fields to the `activationDecision.NewActivity` struct (or a parallel struct) so the activation commit can pass `max_attempts`, `initial_backoff_ms`, `max_backoff_ms`, `backoff_multiplier` to `CreateActivityTask`
- [x] 5.3 Update the replay cursor to advance past `activity.retry_scheduled` events between `activity.scheduled` and `activity.completed`/`activity.failed`: after consuming `activity.scheduled` and before checking for the outcome, loop and skip any `activity.retry_scheduled` events with matching `activity_key`
- [x] 5.4 Add replay tests: history with zero retries (existing behavior), history with one retry event between scheduled and completed, history with multiple retry events, history with retry events followed by failed (exhausted)

**Validation:** replay tests pass; cursor correctly advances past retry events; retry columns propagate to decision

## 6. Activation Commit

- [x] 6.1 Update `commitDecision` in `engine/internal/workflow/activation.go` to pass `max_attempts`, `initial_backoff_ms`, `max_backoff_ms`, `backoff_multiplier` to `CreateActivityTask` when the decision includes a new activity with options
- [x] 6.2 Ensure `Activity()` (no options) passes `max_attempts=1` and NULL for the backoff columns

**Validation:** tasks created by `ActivityWithOptions` have the correct retry columns; tasks created by `Activity()` have max_attempts=1 and NULL backoff columns

## 7. Activity Worker Retry Logic

- [x] 7.1 In `engine/internal/activity/worker.go`, keep the current handler-lookup ordering explicit: if no handler is registered for `activity_type`, fail immediately with `activity_not_registered`, attempt the normal `WakeWaitingRun` path, and skip retry-policy evaluation entirely. After a registered handler fails, read the task's `max_attempts`, `initial_backoff_ms`, `max_backoff_ms`, `backoff_multiplier`, and current `attempt_count` (already incremented by `ClaimNextActivityTask`); check if `attempt_count < max_attempts` and the error is not `NonRetryableError`
- [x] 7.2 If retries remain: compute raw backoff as `min(initial_backoff_ms * backoff_multiplier^(attempt_count-1), max_backoff_ms)` milliseconds, round it up with `ceil` to the next whole millisecond, and call `RetryActivityTask` with that ceiled integer-millisecond delay value (`retry_delay_ms`) rather than an absolute timestamp. `RetryActivityTask` SHALL derive `available_at` from the database clock (`NOW() + retry_delay_ms * INTERVAL '1 millisecond')` so retry scheduling cannot drift under worker/DB clock skew and the units match the computed backoff. Example lock-in: `333ms * 1.5 = 499.5ms` schedules the retry `500ms` later. If `RetryActivityTask` returns `ErrStaleClaim`, log and return nil (matching the existing stale-complete/stale-fail pattern in worker.go). Otherwise, append `activity.retry_scheduled` history event to the engine history, commit WITHOUT waking the workflow run
- [x] 7.3 If retries exhausted or `NonRetryableError`: call `FailActivityTask` and attempt `WakeWaitingRun` as today. Waiting runs wake immediately; suspended runs remain suspended because `WakeWaitingRun` no-ops when the status is no longer `waiting`
- [x] 7.4 Ensure the retry history event is appended in the same transaction as the task status reset and uses the shared non-activation history sequencing rule: lock the owning run row `FOR UPDATE`, compute `next_sequence` from the latest history row under that lock, then call `AppendHistory`
- [x] 7.5 Add activity worker tests: single-attempt failure (max_attempts=1, no retry, normal wake path attempted), retryable failure with remaining attempts (retry scheduled, no wake), exhausted retries (final failure, normal wake path attempted), `NonRetryableError` bypass (immediate failure regardless of remaining attempts, normal wake path attempted), missing handler remains immediate `activity_not_registered` failure even when `max_attempts > 1` (no retry scheduled, normal wake path attempted), stale-claim on retry (RetryActivityTask CAS miss → log and return nil, no error propagated)

**Validation:** worker tests pass; retry scheduling uses correct ceiled backoff and the DB clock; retry history sequencing does not race control-path events; waiting runs stay waiting during retries and wake on terminal activity failure; suspended runs remain suspended until resume; non-retryable errors skip retry; missing handlers stay immediate failures

## 8. Backoff Formula Tests

- [x] 8.1 Add unit tests for the backoff computation: initial backoff (attempt_count=1 → InitialBackoff), exponential growth (attempt_count=2 → InitialBackoff * Multiplier), max backoff cap, edge cases (MaxAttempts=1, BackoffMultiplier=1.0), and a fractional-millisecond result that rounds up via `ceil` (for example `333ms * 1.5 = 499.5ms` schedules as `500ms`)
- [x] 8.2 Verify determinism: same attempt count always produces the same backoff duration
- [x] 8.3 Verify first-retry delay: after first claim and failure (attempt_count=1), backoff = exactly InitialBackoff (not InitialBackoff * Multiplier)

**Validation:** backoff formula tests pass; results are deterministic; first-retry delay is correct; fractional results round up to the next whole millisecond

## 9. Integration Tests

- [x] 9.1 Integration test: `ActivityWithOptions` with `MaxAttempts=3`, activity fails twice then succeeds on third attempt → workflow completes with activity output
- [x] 9.2 Integration test: `ActivityWithOptions` with `MaxAttempts=2`, activity fails twice → workflow sees `activity.failed` and can handle the error
- [x] 9.3 Integration test: `Activity()` (no options), activity fails → workflow sees `activity.failed` immediately (no retry)
- [x] 9.4 Integration test: `NonRetryableError` with `MaxAttempts=3` → activity fails on first attempt with no retry
- [x] 9.5 Integration test: retry backoff delays are respected — second attempt is not claimable before `available_at`
- [x] 9.6 Integration test: history contains `activity.retry_scheduled` events between `activity.scheduled` and `activity.completed`/`activity.failed`
- [x] 9.7 Integration test: replay with retry history — workflow replays correctly past retry events
- [x] 9.8 Integration test: invalid retry policy (MaxAttempts=0) returns deterministic error from `ActivityWithOptions`
- [x] 9.9 Integration test: suspend a run while it is waiting on an activity, let the activity reach terminal failure during suspension (for example retries exhausted), verify the run remains `SUSPENDED`, then resume and confirm the first post-resume activation observes `activity.failed`
- [x] 9.10 Integration test: suspend a run while it is waiting on an activity, let the activity hit a retryable failure with attempts remaining, verify the run remains `SUSPENDED`, `activity.retry_scheduled` is appended without waking the run, and the run still observes the final activity outcome only after resume

**Validation:** all integration tests pass against real Postgres

## 10. Regression Guard

- [x] 10.1 Run `make generate` and verify no drift
- [x] 10.2 Run `cd engine && go test ./...`
- [x] 10.3 Run `go test ./internal/api/... ./internal/ingest/... ./internal/store/... ./internal/jobs/...`
- [x] 10.4 Run `pnpm --filter web test`
- [x] 10.5 Run `cd sdks/python && uv run pytest`
- [x] 10.6 Confirm suspend/resume tests from the previous change still pass without modification

**Validation:** all suites pass


---

### Parallelization Notes

- Tasks 1, 2, 3 can run in parallel (schema, API types, history events)
- Task 4 (queries) depends on Task 1 (schema migration)
- Task 5 (replay) depends on Tasks 2, 3 (API types, history events)
- Task 6 (activation commit) depends on Tasks 4, 5 (queries, replay)
- Task 7 (activity worker) depends on Tasks 4, 5, 6 (queries, replay, activation)
- Task 8 (backoff tests) can run in parallel with Tasks 5–7 (pure computation, no DB)
- Tasks 9, 10 (integration + regression) are final
