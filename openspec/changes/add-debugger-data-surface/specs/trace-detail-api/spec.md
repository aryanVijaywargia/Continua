## ADDED Requirements

### Requirement: TraceDetail Schema

The API SHALL define a `TraceDetail` schema using `allOf` composition on `Trace` that exposes additional trace context fields on `GET /api/traces/{id}`.

#### Scenario: TraceDetail extends Trace via allOf
- **WHEN** `TraceDetail` is defined in OpenAPI
- **THEN** it uses `allOf` referencing `Trace` as the base schema
- **AND** adds `trace_id`, `user_id`, `tags`, `environment`, `release`, `input`, `output` as optional properties

#### Scenario: GetTrace returns TraceDetail
- **WHEN** `GET /api/traces/{id}` is called
- **THEN** the response body conforms to `TraceDetail`
- **AND** includes all `Trace` summary fields plus detail fields

#### Scenario: TraceDetail JSON is flat
- **WHEN** `GET /api/traces/{id}` returns JSON
- **THEN** summary fields and detail fields appear at the same level
- **AND** there is no nested `trace` object introduced by allOf serialization

#### Scenario: ListTraces still returns Trace summary
- **WHEN** `GET /api/traces` is called
- **THEN** each item in the response uses the `Trace` summary schema
- **AND** detail-only fields (`trace_id`, `user_id`, `tags`, `environment`, `release`, `input`, `output`) are absent

### Requirement: TraceDetail Field Mapping

The `traceDetailToAPI()` mapper SHALL populate all detail fields from the existing database row.

#### Scenario: External trace_id mapped
- **WHEN** a trace has `TraceID = "abc-123"` in the database
- **THEN** `trace_id` in the API response is `"abc-123"`

#### Scenario: User ID mapped
- **WHEN** a trace has `UserID = "user@example.com"` in the database
- **THEN** `user_id` in the API response is `"user@example.com"`

#### Scenario: Tags mapped when non-empty
- **WHEN** a trace has `Tags = ["prod", "v2"]` in the database
- **THEN** `tags` in the API response is `["prod", "v2"]`

#### Scenario: Tags omitted when empty
- **WHEN** a trace has `Tags = []` (Postgres empty array) in the database
- **THEN** `tags` is absent from the API response (not serialized as `[]`)

#### Scenario: Environment and release mapped
- **WHEN** a trace has `Environment = "production"` and `Release = "v1.2.0"`
- **THEN** `environment` is `"production"` and `release` is `"v1.2.0"` in the API response

#### Scenario: Trace input preserves arbitrary JSON
- **WHEN** a trace has `Input` containing `[1, "hello", false, null]` as JSON bytes
- **THEN** `input` in the API response is `[1, "hello", false, null]`

#### Scenario: Trace output preserves falsy values
- **WHEN** a trace has `Output` containing `0` as JSON bytes
- **THEN** `output` in the API response is `0` (not omitted)

#### Scenario: Trace input/output absent when empty
- **WHEN** a trace has empty `Input` and `Output` bytes in the database
- **THEN** `input` and `output` are absent from the API response

### Requirement: TraceDetail Mapper Composition

The `traceDetailToAPI()` mapper SHALL compose the existing `traceToAPI()` output.

#### Scenario: Summary fields inherited
- **WHEN** `traceDetailToAPI()` is called
- **THEN** all fields produced by `traceToAPI()` are present in the result
- **AND** no summary field mapping logic is duplicated
