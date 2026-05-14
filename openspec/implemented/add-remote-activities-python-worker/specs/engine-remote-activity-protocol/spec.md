## ADDED Requirements

### Requirement: Remote Activity Claim Endpoint
The engine MUST provide `POST /v1/engine/activities/claim` as an API-key-authenticated, project-scoped, preview-gated endpoint. Requests MUST include `X-Continua-Engine-Preview: 1`. The request MUST include `worker_id` (string) and `activity_types` (non-empty list of strings). The request MAY include `lease_duration` (duration string) and `max_tasks` (integer).

The server MUST trim and validate `worker_id`: it MUST be non-empty after trimming and no longer than 128 characters, otherwise the endpoint MUST return 400 Bad Request. The server MUST reject requests with an empty `activity_types` list, and MUST trim and validate every `activity_types` entry as non-empty and no longer than 256 characters (400 Bad Request).

The server MUST clamp `max_tasks` to minimum 1, default 10, maximum 50. The server MUST clamp `lease_duration` to minimum 10 seconds, default 60 seconds, maximum 5 minutes.

The endpoint MUST return immediately with zero or more claimable tasks matching the requested activity types and `execution_target = 'remote'`. Claimable tasks MUST satisfy the same availability predicate as the local claim path: `(status = 'queued' AND available_at <= now)` OR `(status = 'claimed' AND lease_expires_at IS NOT NULL AND lease_expires_at < now)`. The claim query MUST apply project, `execution_target`, activity type, and availability filters before ordering and applying `max_tasks`. There MUST be no long-polling or connection holding in v1. The response MUST include the effective lease expiry and effective lease duration in milliseconds for each claimed task.

The remote claim transaction MUST atomically set `status = 'claimed'`, `claimed_by`, `claimed_at`, `lease_expires_at`, persist the effective lease duration on each claimed task (as `lease_duration_ms` or equivalent), and increment `attempt_count` by 1. This MUST apply both to queued claims and stale-lease reclaims so retry exhaustion and backoff accounting match the existing local activity claim path.

Stale tasks (where `lease_expires_at` has passed after the last successful claim or heartbeat) MUST be reclaimable by any worker.

#### Scenario: Claim returns available remote tasks
- **WHEN** a worker calls `POST /v1/engine/activities/claim` with `worker_id: "w1"`, `activity_types: ["send_email", "process_image"]`, `max_tasks: 5`
- **THEN** the response contains up to 5 tasks with `execution_target = 'remote'` and matching activity types
- **AND** each task includes task ID, activity key, activity type, input, effective lease expiry, and `effective_lease_duration_ms`
- **AND** each claimed task has `claimed_by = "w1"` and `attempt_count` incremented by 1

#### Scenario: Claim returns empty when no tasks available
- **WHEN** a worker calls claim and no remote tasks match the requested activity types
- **THEN** the response contains an empty task list
- **AND** the response is returned immediately (no blocking)

#### Scenario: Claim respects retry backoff availability
- **WHEN** a remote task is queued for retry with `available_at` in the future
- **THEN** remote claim does not return that task before `available_at <= now`
- **WHEN** `available_at <= now`
- **THEN** remote claim may return the task if its activity type and project match

#### Scenario: Claim request validation
- **WHEN** a worker calls claim with a blank `worker_id`, a too-long `worker_id`, an empty `activity_types` list, a blank activity type, or a too-long activity type
- **THEN** the response is 400 Bad Request

#### Scenario: Lease duration clamping
- **WHEN** a worker requests `lease_duration: "3s"`
- **THEN** the effective lease is clamped to 10 seconds
- **WHEN** a worker requests `lease_duration: "10m"`
- **THEN** the effective lease is clamped to 5 minutes
- **WHEN** a worker omits `lease_duration`
- **THEN** the effective lease defaults to 60 seconds

#### Scenario: Stale lease reclaim
- **WHEN** worker "w1" claims a task but its lease expires without heartbeat
- **THEN** worker "w2" can claim the same task on a subsequent claim call
- **AND** the reclaim updates `claimed_by`, `claimed_at`, `lease_expires_at`, and `lease_duration_ms`
- **AND** increments `attempt_count` by 1

### Requirement: Remote Activity Heartbeat Endpoint
The engine MUST provide `POST /v1/engine/activities/{id}/heartbeat` as an API-key-authenticated, project-scoped, preview-gated endpoint. Requests MUST include `X-Continua-Engine-Preview: 1`. The request MUST include `worker_id`.

The server MUST trim and validate `worker_id`: it MUST be non-empty after trimming and no longer than 128 characters, otherwise the endpoint MUST return 400 Bad Request.

The endpoint MUST validate current remote lease ownership using the same ownership predicate as complete/fail: the task belongs to the authenticated project, `execution_target = 'remote'`, status is `claimed`, `claimed_by` matches `worker_id`, and the lease has not expired. On success, it MUST extend the lease by the stored effective lease duration (persisted at claim time) from now and return the new effective expiry and effective lease duration in milliseconds.

If the task is local-target, queued, terminal, expired, claimed by another worker, or otherwise not currently owned by the requesting remote worker, the endpoint MUST return a 409 Conflict. A missing or cross-project task MUST return 404.

#### Scenario: Heartbeat renews lease
- **WHEN** worker "w1" owns task T (claimed with effective lease 60s) and calls heartbeat
- **THEN** the task's `lease_expires_at` is set to now + 60 seconds
- **AND** the response contains the new effective expiry and `effective_lease_duration_ms`

#### Scenario: Heartbeat from non-owner rejected
- **WHEN** worker "w2" (not the current owner) calls heartbeat for task T
- **THEN** the response is 409 Conflict

#### Scenario: Heartbeat only renews owned remote claimed tasks
- **WHEN** worker "w1" calls heartbeat for a local-target, queued, terminal, expired, or reclaimed task
- **THEN** the response is 409 Conflict
- **AND** the task lease is not extended

#### Scenario: Heartbeat missing or cross-project task
- **WHEN** a worker calls heartbeat for a missing task or a task outside the authenticated project
- **THEN** the response is 404 Not Found

#### Scenario: Heartbeat request validation
- **WHEN** a worker calls heartbeat with a blank or too-long `worker_id`
- **THEN** the response is 400 Bad Request

### Requirement: Remote Activity Complete Endpoint
The engine MUST provide `POST /v1/engine/activities/{id}/complete` as an API-key-authenticated, project-scoped, preview-gated endpoint. Requests MUST include `X-Continua-Engine-Preview: 1`. The request MUST include `worker_id` and `output` (JSON).

The server MUST trim and validate `worker_id`: it MUST be non-empty after trimming and no longer than 128 characters, otherwise the endpoint MUST return 400 Bad Request.

The endpoint MUST validate that the requesting `worker_id` currently owns the task's lease. On success, it MUST complete the task and wake the waiting workflow run.

Current lease ownership requires all of: the task belongs to the authenticated project, `execution_target = 'remote'`, status is `claimed`, `claimed_by` matches `worker_id`, and the lease has not expired.

The endpoint MUST return 409 Conflict instead of silent success when the task is local-target, queued, claimed by another worker, reclaimed after lease expiry, already completed, already failed, or already cancelled. A missing or cross-project task MUST return 404.

#### Scenario: Complete with valid lease ownership
- **WHEN** worker "w1" owns task T and calls complete with output `{"result": "ok"}`
- **THEN** the task transitions to `completed` with the provided output
- **AND** the waiting workflow run is woken

#### Scenario: Duplicate complete after success is rejected
- **WHEN** worker "w1" calls complete for task T after an earlier complete call already transitioned T to `completed`
- **THEN** the response is 409 Conflict
- **AND** the task's completed output is not changed

#### Scenario: Complete with stale lease is rejected
- **WHEN** worker "w1" calls complete for task T but worker "w2" has already reclaimed it
- **THEN** the response is 409 Conflict
- **AND** the task's state is unchanged

#### Scenario: Complete against wrong terminal state is rejected
- **WHEN** worker "w1" calls complete for task T but T is already failed or cancelled
- **THEN** the response is 409 Conflict
- **AND** the task's terminal state is unchanged

#### Scenario: Complete request validation
- **WHEN** a worker calls complete with a blank or too-long `worker_id`
- **THEN** the response is 400 Bad Request

### Requirement: Remote Activity Fail Endpoint
The engine MUST provide `POST /v1/engine/activities/{id}/fail` as an API-key-authenticated, project-scoped, preview-gated endpoint. Requests MUST include `X-Continua-Engine-Preview: 1`. The request MUST include `worker_id`, `error_code` (string), `error_message` (string), and optionally `non_retryable` (boolean).

The server MUST trim and validate `worker_id`: it MUST be non-empty after trimming and no longer than 128 characters, otherwise the endpoint MUST return 400 Bad Request.

The server MUST validate `error_code` before mutating task state: it MUST be non-empty and no longer than 128 characters, otherwise the endpoint MUST return 400 Bad Request. The server MUST enforce a durable `error_message` maximum of 4096 characters by truncating longer messages before storing retry history, terminal state, or workflow wake errors.

If `non_retryable` is true or the task's retry policy is exhausted, the task MUST transition to `failed` and wake the waiting workflow run with the error. Otherwise, the task MUST be requeued with appropriate backoff using existing retry logic.

Current lease ownership requires all of: the task belongs to the authenticated project, `execution_target = 'remote'`, status is `claimed`, `claimed_by` matches `worker_id`, and the lease has not expired.

The endpoint MUST return 409 Conflict instead of silent success when the task is local-target, queued, claimed by another worker, reclaimed after lease expiry, already completed, already cancelled, or already failed. A missing or cross-project task MUST return 404.

#### Scenario: Retryable failure requeues
- **WHEN** worker "w1" fails task T with `non_retryable: false` and the task has remaining retry attempts
- **THEN** the task is requeued with backoff computed from existing retry policy
- **AND** `activity.retry_scheduled` history event is appended

#### Scenario: Non-retryable failure terminates task
- **WHEN** worker "w1" fails task T with `non_retryable: true`
- **THEN** the task transitions to `failed` regardless of remaining retry attempts
- **AND** the waiting workflow run is woken with the error

#### Scenario: Retry exhaustion terminates task
- **WHEN** worker "w1" fails task T and no retry attempts remain
- **THEN** the task transitions to `failed`
- **AND** the waiting workflow run is woken with the error

#### Scenario: Duplicate fail after retry requeue is rejected
- **WHEN** worker "w1" calls fail for task T after an earlier fail call already requeued T for retry
- **THEN** the response is 409 Conflict
- **AND** the task remains queued for retry

#### Scenario: Fail with stale lease is rejected
- **WHEN** worker "w1" calls fail for task T but worker "w2" has already reclaimed it
- **THEN** the response is 409 Conflict
- **AND** the task's state is unchanged

#### Scenario: Fail error fields are bounded
- **WHEN** a worker calls fail with a blank or too-long `worker_id`
- **THEN** the response is 400 Bad Request
- **WHEN** a worker calls fail with an empty `error_code` or an `error_code` longer than 128 characters
- **THEN** the response is 400 Bad Request
- **WHEN** a worker calls fail with an `error_message` longer than 4096 characters
- **THEN** the stored retry or terminal error message is truncated to 4096 characters

#### Scenario: Fail against completed task is rejected
- **WHEN** worker "w1" calls fail for task T but T is already completed
- **THEN** the response is 409 Conflict
- **AND** the task's completed output is unchanged
