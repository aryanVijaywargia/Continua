## ADDED Requirements

### Requirement: Updated engine run status types
The web frontend `EngineRunStatus` type SHALL include `SUSPENDED`, `TERMINATED`, and `CONTINUED_AS_NEW` in addition to the existing values (`QUEUED`, `RUNNING`, `WAITING`, `COMPLETED`, `FAILED`, `CANCELLED`). The `EngineRunSummary` interface SHALL include `continued_from_run_id`, `continued_to_run_id`, `continued_from_trace_id`, and `continued_to_trace_id` optional string fields matching the OpenAPI `EngineRunSummary` schema.

#### Scenario: EngineRunStatus includes all backend values
- **WHEN** the frontend type definitions are compiled
- **THEN** `EngineRunStatus` includes all nine values: `QUEUED`, `RUNNING`, `WAITING`, `SUSPENDED`, `COMPLETED`, `FAILED`, `CANCELLED`, `TERMINATED`, `CONTINUED_AS_NEW`

### Requirement: Definition version mismatch banner
The trace detail page SHALL render a warning banner when `trace.engine.failure.error_code === 'definition_version_mismatch'`. The comparison SHALL use exact string matching against the `error_code` field. The banner SHALL NOT appear for other error codes or when no failure is present.

#### Scenario: Mismatch banner shown for definition_version_mismatch
- **WHEN** `trace.engine.failure.error_code` is `'definition_version_mismatch'`
- **THEN** a warning banner is displayed stating "This run failed because the engine definition version could not be matched during activation."

#### Scenario: Mismatch banner not shown for other error codes
- **WHEN** `trace.engine.failure.error_code` is `'timeout'`
- **THEN** no mismatch banner is displayed

#### Scenario: No banner when no failure present
- **WHEN** `trace.engine.failure` is undefined
- **THEN** no mismatch banner is displayed

### Requirement: ContinueAsNew navigation links
The trace detail page SHALL render navigation links from `continued_from_trace_id` (link to previous trace) and `continued_to_trace_id` (link to next trace) when those fields are present in the engine run summary. Links SHALL navigate to `/traces/{trace_id}` and preserve the current `returnTo` location state, following the existing debugger navigation pattern for trace, session, and compare flows.

#### Scenario: Previous run link shown
- **WHEN** `engine.continued_from_trace_id` is present
- **THEN** a "Previous run" navigation link is rendered pointing to the previous trace

#### Scenario: Next run link shown
- **WHEN** `engine.continued_to_trace_id` is present
- **THEN** a "Next run" navigation link is rendered pointing to the continued trace

#### Scenario: Continuation link preserves returnTo
- **WHEN** the trace detail page has a `returnTo` location state (e.g., `/sessions/abc`) and the user clicks a continuation link
- **THEN** the navigation to the continued trace carries the same `returnTo` state so the back destination is preserved

#### Scenario: No continuation links when absent
- **WHEN** `engine.continued_from_trace_id` and `engine.continued_to_trace_id` are both absent
- **THEN** no continuation navigation links are rendered

### Requirement: definition_version_mismatch is a stable reserved error code
The `definition_version_mismatch` value SHALL be treated as a stable reserved `EngineFailureSummary.error_code` value for exact-match UI behavior. The `error_code` field itself SHALL remain an open string type that accepts arbitrary values. The OpenAPI schema description for `EngineFailureSummary.error_code` SHALL document `definition_version_mismatch` as a reserved value that clients may match against.

#### Scenario: error_code remains an open string
- **WHEN** an engine failure has `error_code` set to `'custom_application_error'`
- **THEN** the value is accepted without validation because `error_code` is an open string

#### Scenario: OpenAPI documents reserved error code
- **WHEN** the OpenAPI spec is reviewed
- **THEN** the `EngineFailureSummary.error_code` field description lists `definition_version_mismatch` as a stable reserved value
