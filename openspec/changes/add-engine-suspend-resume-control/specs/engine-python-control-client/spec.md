## ADDED Requirements

### Requirement: SDK Suspend Method
The `EngineControlClient` SHALL expose a `suspend(run_id)` method that calls `POST /v1/engine/runs/{run_id}/suspend` with the engine preview header.

The method SHALL return `EngineRunResponse` (full run detail after transition).

#### Scenario: Suspend via SDK
- **WHEN** a Python client calls `client.suspend(run_id)`
- **THEN** the SDK sends `POST /v1/engine/runs/{run_id}/suspend` with the preview header
- **AND** returns the typed `EngineRunResponse`

### Requirement: SDK Resume Method
The `EngineControlClient` SHALL expose a `resume(run_id)` method that calls `POST /v1/engine/runs/{run_id}/resume` with the engine preview header.

The method SHALL return `EngineRunResponse`.

#### Scenario: Resume via SDK
- **WHEN** a Python client calls `client.resume(run_id)`
- **THEN** the SDK sends `POST /v1/engine/runs/{run_id}/resume` with the preview header
- **AND** returns the typed `EngineRunResponse`

### Requirement: SUSPENDED Status Enum
The `EngineRunStatus` enum in the Python SDK types SHALL include `SUSPENDED`.

#### Scenario: Status enum includes suspended
- **WHEN** a client receives a run response with status `SUSPENDED`
- **THEN** the SDK decodes it as `EngineRunStatus.SUSPENDED`
