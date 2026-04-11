## ADDED Requirements

### Requirement: Continuation Chain Fields in Engine Run Surfaces
The `EngineRunSummary`, `EngineRunResponse`, and `EngineRunResultResponse` schemas SHALL include nullable fields: `continued_from_run_id`, `continued_to_run_id`, `continued_from_trace_id`, `continued_to_trace_id`.

These fields SHALL be populated for runs that are part of a continuation chain and null for non-continued runs.

`continued_from_trace_id` and `continued_to_trace_id` SHALL be derived from the corresponding run IDs using the deterministic `engine:<run_id>` trace ID format.

#### Scenario: Run detail with continuation
- **WHEN** a client calls `GET /v1/engine/runs/{run_id}` on a run that has `continued_to_run_id`
- **THEN** the response includes `continued_to_run_id` and `continued_to_trace_id`

#### Scenario: Trace detail engine summary with continuation
- **WHEN** a client reads a trace detail whose `engine` summary refers to a continued run
- **THEN** the `TraceDetail.engine` object includes the continuation chain fields without requiring a second run-detail fetch

#### Scenario: Run detail without continuation
- **WHEN** a client calls `GET /v1/engine/runs/{run_id}` on a run with no continuation chain
- **THEN** the continuation fields are null

### Requirement: ContinuedAsNew Status in Run Result
`GET /v1/engine/runs/{run_id}/result` SHALL return a response with status `CONTINUED_AS_NEW` for continued runs.

The continuation chain fields SHALL be populated.

The `result` field SHALL continue to represent the stored workflow result payload and SHALL therefore be `null` for continued runs.

#### Scenario: Get result on continued run
- **WHEN** a client calls `GET /v1/engine/runs/{run_id}/result` on a `CONTINUED_AS_NEW` run
- **THEN** the response includes `status: "CONTINUED_AS_NEW"`, `result: null`, and `continued_to_run_id`

### Requirement: Instance Response Shows Highest Run Number
`GET /v1/engine/instances/{instance_key}` SHALL return the run with the highest `run_number`, which for continued instances is the continuation target.

#### Scenario: Instance shows continuation target
- **WHEN** run 1 continued as run 2
- **AND** a client calls `GET /v1/engine/instances/{instance_key}`
- **THEN** the response includes run 2 as the current run

### Requirement: ContinuedAsNew Status in OpenAPI Enum
The `EngineRunStatus` enum in the OpenAPI schema SHALL include `CONTINUED_AS_NEW`.

#### Scenario: Status enum includes continued_as_new
- **WHEN** a client receives a run response with status `CONTINUED_AS_NEW`
- **THEN** the status is valid per the OpenAPI schema

### Requirement: Trace Search Filter Accepts continued_as_new
The `engine_run_status` query parameter on `GET /api/traces` SHALL accept lowercase `continued_as_new`.

#### Scenario: Trace search filters continued runs
- **WHEN** a client calls `GET /api/traces?engine_run_status=continued_as_new`
- **THEN** the request is valid per the OpenAPI schema
- **AND** only continued runs are eligible to match the filter
