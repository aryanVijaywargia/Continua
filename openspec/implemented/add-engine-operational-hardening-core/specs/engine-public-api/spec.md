# Capability: engine-public-api

Operational hardening layer on top of the existing engine public API: introduces a forceful terminate endpoint, an exact pending-work read endpoint, an explicit cooperative cancel contract, and adds `TERMINATED` to the run status enum.

Related capabilities: [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-history-events](../engine-history-events/spec.md), [engine-trace-projection](../engine-trace-projection/spec.md)

## ADDED Requirements

### Requirement: EngineRunStatus includes TERMINATED

The `EngineRunStatus` enum returned by engine API responses MUST include `TERMINATED` as an additive terminal value.

#### Scenario: Enum values
- **WHEN** the engine API returns `EngineRunStatus` in any response
- **THEN** the enum values are `QUEUED`, `RUNNING`, `WAITING`, `COMPLETED`, `FAILED`, `CANCELLED`, `TERMINATED`

#### Scenario: Terminal status classification
- **WHEN** API handlers check whether a run is terminal
- **THEN** `COMPLETED`, `FAILED`, `CANCELLED`, and `TERMINATED` are all classified as terminal
- **THEN** terminal-guarded endpoints (`cancel`, `signal`, `terminate`) uniformly reject `TERMINATED` the same way they reject other terminal statuses

#### Scenario: Additive change for existing clients
- **WHEN** an existing client receives a response that contains `TERMINATED`
- **THEN** the change is additive; the client sees a new enum value but no existing value changes meaning

---

### Requirement: Terminate engine run

`POST /v1/engine/runs/{run_id}/terminate` MUST forcefully stop an active run and is not inbox-mediated.

#### Scenario: Terminate route registration and gating
- **WHEN** `ENGINE_PUBLIC_API_ENABLED=true`
- **THEN** `POST /v1/engine/runs/{run_id}/terminate` is registered
- **WHEN** a terminate request lacks the `X-Continua-Engine-Preview: 1` header
- **THEN** the server returns 400 with a message indicating the preview header is required
- **WHEN** `ENGINE_PUBLIC_API_ENABLED` is not set or is `false`
- **THEN** the route returns 404

#### Scenario: Terminate authentication and project scoping
- **WHEN** a terminate request arrives with a valid API key
- **THEN** `project_id` is taken from the API key and all engine operations are scoped to that project
- **WHEN** no valid API key is provided
- **THEN** the server returns 401

#### Scenario: Terminate response schema
- **WHEN** a terminate request returns HTTP 200
- **THEN** the response body conforms to the existing `EngineRunResultResponse` schema: `{run_id, status, result, failure}`
- **THEN** a new schema is NOT introduced for terminate responses

#### Scenario: Terminate active run
- **WHEN** a terminate request targets a run with status `queued`, `running`, or `waiting`
- **THEN** the handler locks the run row, appends `workflow.terminated` to history, transitions the run to `terminated`, seals open activity tasks and inbox items, and responds with the new terminal state
- **THEN** the response body is an `EngineRunResultResponse` with `status=TERMINATED`, `result=null`, and `failure={error_code:"terminated", error_message:"run terminated by operator"}`

#### Scenario: Terminate on already-terminal run (idempotent)
- **WHEN** a terminate request targets a run whose status is already `completed`, `failed`, `cancelled`, or `terminated`
- **THEN** no new history rows, transitions, or cleanup are produced
- **THEN** the response returns the existing terminal `EngineRunResultResponse` with HTTP 200 (same shape as `GET /result` returns for the same run)

#### Scenario: Terminate on missing or cross-project run
- **WHEN** the `run_id` does not exist or belongs to a different project
- **THEN** the server returns 404

#### Scenario: Terminate is not inbox-mediated
- **WHEN** a terminate request is processed
- **THEN** no `cancel` inbox row is created and no activation is required for the transition to take effect

#### Scenario: Terminate races with activation
- **WHEN** terminate and an activation both target the same run
- **WHEN** activation commits first
- **THEN** terminate observes a terminal status under lock and returns it unchanged
- **WHEN** terminate commits first
- **THEN** the subsequent activation exits via the existing stale-claim path and does not revive the run

---

### Requirement: Pending-work read endpoint

`GET /v1/engine/runs/{run_id}/pending-work` MUST return the run's current wait and the durable rows the run is still holding.

#### Scenario: Endpoint authentication and gating
- **WHEN** `ENGINE_PUBLIC_API_ENABLED=true`
- **THEN** `GET /v1/engine/runs/{run_id}/pending-work` is registered
- **THEN** the preview header is NOT required for this GET route
- **WHEN** no valid API key is provided
- **THEN** the server returns 401

#### Scenario: Response shape
- **WHEN** the endpoint returns a successful response
- **THEN** the body is an `EnginePendingWorkResponse` with all of: `run_id` (uuid), `current_wait` (nullable), `activities` (array of `EnginePendingActivityItem`), `timers` (array of `EnginePendingTimerItem`), `signals` (array of `EnginePendingSignalItem`), `pending_activity_tasks` (integer), `pending_inbox_items` (integer)
- **THEN** these seven fields are the complete response body; no additional wrapping envelope is added

#### Scenario: current_wait synthesis
- **WHEN** `runs.waiting_for` is populated
- **THEN** `current_wait` is derived directly from `runs.waiting_for` and may overlap with a durable activity or timer row
- **WHEN** `runs.waiting_for` is NULL
- **THEN** `current_wait` is null

#### Scenario: activities array
- **WHEN** the endpoint reads `engine.activity_tasks` for the run
- **THEN** the returned array includes rows with status `queued` or `claimed`
- **THEN** there is no `available_at <= NOW()` filter; future and claimed rows both appear
- **THEN** rows are ordered by `available_at ASC, id ASC`

#### Scenario: activities item schema
- **WHEN** an activity row appears in `activities`
- **THEN** each item exposes `task_id`, `activity_key`, `activity_type`, `status`, `available_at`, and `attempt_count`
- **THEN** `status` mirrors the durable activity-task row state (`queued` or `claimed`)
- **THEN** this endpoint does NOT require `history_id`, raw `input`, or raw `output` fields for activities

#### Scenario: timers array
- **WHEN** the endpoint reads `engine.inbox` for the run
- **THEN** the returned `timers` array includes rows with `kind = 'timer'` and status `pending` or `claimed`
- **THEN** there is no `available_at <= NOW()` filter
- **THEN** rows are ordered by `available_at ASC, id ASC`

#### Scenario: timers item schema
- **WHEN** a timer inbox row appears in `timers`
- **THEN** each item exposes `inbox_id`, `timer_key`, `status`, and `available_at`
- **THEN** `status` mirrors the durable inbox row state (`pending` or `claimed`)
- **THEN** `timer_key` is decoded from the row payload using the existing `timer.scheduled` payload contract
- **THEN** this endpoint does NOT require `history_id` or raw timer payload fields

#### Scenario: signals array
- **WHEN** the endpoint reads `engine.inbox` for the run
- **THEN** the returned `signals` array includes rows with `kind = 'signal'` and status `pending` or `claimed`
- **THEN** delivered-but-unconsumed signals appear here
- **THEN** rows are ordered by `available_at ASC, id ASC`

#### Scenario: signals item schema
- **WHEN** a signal inbox row appears in `signals`
- **THEN** each item exposes `inbox_id`, `signal_name`, `status`, and `available_at`
- **THEN** `status` mirrors the durable inbox row state (`pending` or `claimed`)
- **THEN** `signal_name` is decoded from the row payload using the existing `signal.received` payload contract
- **THEN** this endpoint does NOT require `history_id` or raw signal payload fields

#### Scenario: cancel inbox rows are excluded
- **WHEN** the endpoint reads `engine.inbox` for the run
- **THEN** rows with `kind = 'cancel'` are not included in any array

#### Scenario: Pure signal wait with no inbox rows
- **WHEN** a run is waiting on a signal and no matching inbox row has been delivered yet
- **THEN** `current_wait` is populated from `runs.waiting_for`
- **THEN** `activities`, `timers`, and `signals` arrays are empty
- **THEN** the summary counts `pending_activity_tasks` and `pending_inbox_items` are zero

#### Scenario: Summary counts match durable rows
- **WHEN** the endpoint returns the response
- **THEN** `pending_activity_tasks` equals the number of rows in the `activities` array
- **THEN** `pending_inbox_items` equals the combined number of rows in the `timers` and `signals` arrays (cancel inbox rows are never counted)
- **THEN** these counts match what existing run-summary responses return for the same run at the same moment, because `CountOpenInboxByRun` is updated in this change to also exclude `kind='cancel'` (see engine-schema-runtime-delta)

#### Scenario: Run not found or cross-project
- **WHEN** the `run_id` does not exist or belongs to a different project
- **THEN** the server returns 404

---

### Requirement: Cancel contract is explicit and cooperative

`POST /v1/engine/runs/{run_id}/cancel` MUST be documented and behave as purely cooperative; its success does not guarantee the run becomes `CANCELLED`.

#### Scenario: Cancel request alone remains cooperative
- **WHEN** a cancel request is accepted for an active run
- **THEN** the handler creates a `cancel` inbox row and optionally wakes the run, and the final terminal outcome is determined by the workflow code on the next activation

#### Scenario: Cancel + workflow returning nil
- **WHEN** a workflow observes cancellation and returns `nil` (no error)
- **THEN** the run may still end with terminal status `COMPLETED`
- **THEN** the run is not forcibly moved to `CANCELLED`

#### Scenario: Cancel + workflow returning workflow.ErrCancelled
- **WHEN** a workflow observes cancellation and returns `workflow.ErrCancelled`
- **THEN** the run ends with terminal status `CANCELLED`
- **THEN** `workflow.cancelled` is appended to history as the terminal event

#### Scenario: Cancel is not a terminate shortcut
- **WHEN** a caller needs to guarantee a forceful stop
- **THEN** the caller MUST use `POST /terminate`, not `POST /cancel`

---

### Requirement: Terminal result response for cancelled and terminated

`GET /v1/engine/runs/{run_id}/result` MUST return explicit, documented shapes for `CANCELLED` and `TERMINATED` runs.

#### Scenario: Cancelled terminal response
- **WHEN** a run has status `CANCELLED`
- **THEN** the endpoint returns HTTP 200
- **THEN** the response body has `result=null` and a populated `failure` object with `error_code=cancelled` and `error_message=workflow cancelled`

#### Scenario: Terminated terminal response
- **WHEN** a run has status `TERMINATED`
- **THEN** the endpoint returns HTTP 200
- **THEN** the response body has `result=null` and a populated `failure` object with `error_code=terminated` and `error_message=run terminated by operator`

#### Scenario: Non-terminal run response unchanged
- **WHEN** a run is not yet terminal
- **THEN** the endpoint returns HTTP 409 as before
