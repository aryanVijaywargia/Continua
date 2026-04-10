## ADDED Requirements

### Requirement: Suspend Endpoint
The platform SHALL expose `POST /v1/engine/runs/{run_id}/suspend` behind the engine preview header gate.

The endpoint SHALL accept no request body and return `EngineRunResponse` (the full run detail including status, custom_status, wait_state, and pending work counts after the transition).

The endpoint SHALL return 200 for successful suspension or idempotent no-op (already suspended, returning current state). The endpoint SHALL return 409 with a typed error for `running` (mid-activation) or terminal runs. The endpoint SHALL return 404 for missing or cross-project runs.

#### Scenario: Successful suspend
- **WHEN** a client calls `POST /v1/engine/runs/{run_id}/suspend` on a `queued` or `waiting` run
- **THEN** the response is 200 with `EngineRunResponse` showing `status: "SUSPENDED"`

#### Scenario: Suspend mid-activation
- **WHEN** a client calls `POST /v1/engine/runs/{run_id}/suspend` on a `running` run
- **THEN** the response is 409 with error code `run_not_suspendable`

#### Scenario: Suspend terminal run
- **WHEN** a client calls `POST /v1/engine/runs/{run_id}/suspend` on a terminal run
- **THEN** the response is 409 with error code `run_terminal`

#### Scenario: Suspend already-suspended
- **WHEN** a client calls `POST /v1/engine/runs/{run_id}/suspend` on an already-suspended run
- **THEN** the response is 200 with `EngineRunResponse` showing `status: "SUSPENDED"` (no-op)

### Requirement: Resume Endpoint
The platform SHALL expose `POST /v1/engine/runs/{run_id}/resume` behind the engine preview header gate.

The endpoint SHALL accept no request body and return `EngineRunResponse`.

The endpoint SHALL return 200 for successful resume or idempotent no-op (non-suspended active run, returning current state). The endpoint SHALL return 409 with a typed error for terminal runs. The endpoint SHALL return 404 for missing or cross-project runs.

#### Scenario: Successful resume
- **WHEN** a client calls `POST /v1/engine/runs/{run_id}/resume` on a `suspended` run
- **THEN** the response is 200 with `EngineRunResponse` showing `status: "QUEUED"`

#### Scenario: Resume non-suspended active run
- **WHEN** a client calls `POST /v1/engine/runs/{run_id}/resume` on a `queued`, `running`, or `waiting` run
- **THEN** the response is 200 with `EngineRunResponse` showing the current status (no-op)

#### Scenario: Resume terminal run
- **WHEN** a client calls `POST /v1/engine/runs/{run_id}/resume` on a terminal run
- **THEN** the response is 409 with error code `run_terminal`

### Requirement: Suspended Status in Run Responses
The `EngineRunStatus` enum in the OpenAPI schema SHALL include `SUSPENDED` (uppercase, matching the existing convention).

Run detail responses SHALL return `SUSPENDED` for suspended runs. The run result endpoint (`GET /v1/engine/runs/{run_id}/result`) SHALL return 409 `run_not_terminal` for suspended runs because `SUSPENDED` is not a terminal status.

#### Scenario: Run detail shows suspended status
- **WHEN** a client calls `GET /v1/engine/runs/{run_id}` on a suspended run
- **THEN** the response includes `status: "SUSPENDED"`

### Requirement: Terminate Widened to Include Suspended
The `TransitionRunToTerminated` CAS predicate SHALL be widened to accept `suspended` in addition to `queued`, `running`, and `waiting`.

Operators SHALL be able to terminate a suspended run without first resuming it.

#### Scenario: Terminate suspended run
- **WHEN** an operator calls terminate on a `suspended` run
- **THEN** the run transitions to `terminated` with `completed_at = NOW()`
