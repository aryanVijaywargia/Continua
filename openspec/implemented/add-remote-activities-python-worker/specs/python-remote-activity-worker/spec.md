## ADDED Requirements

### Requirement: Activity Worker Handler Registry
The Python SDK MUST provide an `ActivityWorker` class with a handler registry. Handlers MUST be registered via `worker.register(activity_type, handler_fn)` or a decorator form `@worker.activity("type_name")`.

V1 handlers MUST be synchronous Python callables. Coroutine functions and async handler execution are out of scope for v1 and MUST be rejected at registration time with a clear error.

Each handler MUST receive the task input (deserialized JSON) and return a result (serializable to JSON) or raise an exception. For ergonomics, one-argument handlers MUST be supported. Handlers MAY also accept a second `ActivityTaskContext` argument. `ActivityTaskContext` MUST expose `task_id`, `activity_key`, `activity_type`, and a cancellation/lost-lease indicator that handlers can poll before performing side effects.

#### Scenario: Register handler with method
- **WHEN** a developer calls `worker.register("send_email", send_email_handler)`
- **THEN** the worker dispatches `send_email` activity tasks to `send_email_handler`

#### Scenario: Register handler with decorator
- **WHEN** a developer decorates a function with `@worker.activity("process_image")`
- **THEN** the worker dispatches `process_image` activity tasks to the decorated function

#### Scenario: Unregistered activity type
- **WHEN** the worker receives a task with an activity type that has no registered handler
- **THEN** the worker fails the task with an appropriate error code and message

#### Scenario: Handler receives task context
- **WHEN** a registered handler accepts `(input, context)`
- **THEN** the worker passes deserialized input as the first argument
- **AND** passes an `ActivityTaskContext` containing task ID, activity key, and activity type as the second argument

#### Scenario: Async handler rejected in v1
- **WHEN** a developer attempts to register an async coroutine handler
- **THEN** registration fails with a clear error explaining that remote activity worker v1 supports sync handlers only

### Requirement: Polling Loop
The `ActivityWorker` MUST implement a polling loop that calls the claim endpoint at a configurable interval. The worker MUST pass its registered activity types in the claim request.

The worker MUST have a stable `worker_id` for the worker instance lifetime. `ActivityWorker` MUST accept a configurable `worker_id`; if omitted, it MUST generate one non-empty value no longer than 128 characters once during worker construction or before the first claim, for example `py-<uuid4>`. The same `worker_id` value MUST be used for claim, heartbeat, complete, and fail calls for all tasks claimed by that worker instance. The worker MUST NOT generate `worker_id` per request or per task.

By default, the worker MUST set claim `max_tasks` to the number of currently available execution slots (`concurrency_limit - active_in_flight_task_count`) and MUST skip claim calls when no execution slots are available. The worker MUST NOT keep a local claimed-but-not-running backlog by default; any configurable prefetch/backlog option MUST be explicit, bounded, and documented as starting leases before handler execution.

When tasks are claimed, the worker MUST dispatch them immediately to registered synchronous handlers on a bounded thread pool, up to a configurable concurrency limit. The worker MUST use the effective lease duration returned by the claim response, not local reimplementation of server clamp logic, to schedule default heartbeat intervals.

#### Scenario: Worker polls and executes tasks
- **WHEN** the worker is started with `worker.run()`
- **THEN** it repeatedly calls the claim endpoint
- **AND** uses the same `worker_id` value on claim, heartbeat, complete, and fail calls
- **AND** sets claim `max_tasks` to the current number of available execution slots
- **AND** dispatches claimed tasks immediately to registered handlers
- **AND** calls the complete or fail endpoint based on the handler outcome

#### Scenario: Worker ID generated once
- **WHEN** an `ActivityWorker` is created without a configured `worker_id`
- **THEN** the worker generates one non-empty `worker_id` no longer than 128 characters
- **AND** reuses that same value for all claim, heartbeat, complete, and fail calls during the worker instance lifetime

#### Scenario: Concurrency limit respected
- **WHEN** the worker has a concurrency limit of 5 and no active in-flight tasks
- **THEN** the next claim request uses `max_tasks: 5`
- **WHEN** the worker already has 5 active in-flight tasks
- **THEN** it skips claiming until a slot opens
- **AND** at most 5 tasks execute concurrently
- **AND** execution uses worker threads rather than asyncio tasks

#### Scenario: Backlog disabled by default
- **WHEN** the worker has a concurrency limit of 5
- **AND** the server has 10 matching tasks available
- **THEN** the worker claims at most 5 tasks by default
- **AND** at most 5 tasks execute concurrently
- **AND** does not hold the remaining tasks in a local claimed-but-not-running backlog

### Requirement: Heartbeat Loop
For each in-flight task, the worker MUST send heartbeats at half of the effective lease duration by default. The heartbeat interval MUST be configurable.

If a heartbeat fails with 409 Conflict or 404 Not Found, the worker SHOULD signal cancellation through `ActivityTaskContext` for handlers that accepted a context argument and MUST NOT call complete or fail for that task. The worker MUST treat both responses as no-longer-owned task outcomes and MUST NOT attempt to force-kill a running sync handler thread.

#### Scenario: Heartbeat sent at half lease interval
- **WHEN** a task is claimed with effective lease of 60 seconds
- **THEN** the worker sends a heartbeat after 30 seconds
- **AND** continues heartbeating at 30-second intervals until the task completes

#### Scenario: Lost lease detected via heartbeat
- **WHEN** a heartbeat returns 409 Conflict
- **THEN** the worker marks the task context as cancelled/lost-lease if the handler accepted context
- **AND** does not call complete or fail for that task

#### Scenario: Missing task detected via heartbeat
- **WHEN** a heartbeat returns 404 Not Found
- **THEN** the worker marks the task context as cancelled/lost-lease if the handler accepted context
- **AND** does not call complete or fail for that task

### Requirement: Graceful Shutdown
The worker MUST support graceful shutdown. On shutdown signal (SIGINT, SIGTERM), the worker MUST:
1. Stop claiming new tasks
2. Continue heartbeating still-owned in-flight tasks while waiting for handlers to complete (up to a configurable timeout)
3. Call complete or fail only for completed handlers whose task lease is still owned and not marked lost-lease
4. Exit cleanly

#### Scenario: Graceful shutdown completes in-flight work
- **WHEN** the worker receives SIGINT with 3 tasks in flight
- **THEN** the worker stops polling for new tasks
- **AND** continues heartbeating still-owned in-flight tasks while waiting for the 3 in-flight handlers to complete
- **AND** calls complete or fail only for tasks whose lease remains owned
- **AND** leaves tasks that timed out or were marked lost-lease for natural reclaim
- **AND** exits cleanly

#### Scenario: Shutdown timeout exceeded
- **WHEN** the worker receives SIGINT and in-flight tasks exceed the shutdown timeout
- **THEN** the worker exits after the timeout
- **AND** does not call complete or fail for unfinished tasks after the timeout
- **AND** uncompleted tasks have their leases expire naturally for reclaim

### Requirement: Error Mapping
The Python SDK MUST provide a `NonRetryableError` exception class. When a handler raises `NonRetryableError`, the worker MUST call the fail endpoint with `non_retryable: true`.

Unhandled exceptions from handlers MUST be mapped to a generic error code with a bounded error message (exception type and message, truncated to 4096 characters) and `non_retryable: false` (retryable by default). The full traceback MAY be logged locally but MUST NOT be sent as the error message, since error messages become durable engine history data.

#### Scenario: NonRetryableError maps correctly
- **WHEN** a handler raises `NonRetryableError("Invalid input format")`
- **THEN** the worker calls fail with `error_code: "non_retryable_error"`, `error_message: "Invalid input format"`, `non_retryable: true`

#### Scenario: Unhandled exception is retryable with bounded message
- **WHEN** a handler raises `ConnectionError("timeout")`
- **THEN** the worker calls fail with `error_code: "activity_error"`, `error_message: "ConnectionError: timeout"` (truncated to 4096 chars), and `non_retryable: false`
- **AND** the full traceback is logged locally
- **AND** the engine retries the task if retry attempts remain
