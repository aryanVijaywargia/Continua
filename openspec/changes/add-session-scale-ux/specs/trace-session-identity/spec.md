## ADDED Requirements

### Requirement: Session External ID on Trace Reads
All trace read paths SHALL return an optional `session_external_id` string field when the trace belongs to a session. This includes `ListTraces`, `ListTracesBySession`, `GetTrace` (detail), and trace search results. The field SHALL be populated via a `LEFT JOIN sessions` on the query layer.

#### Scenario: Trace with session includes session_external_id
- **WHEN** a trace belongs to a session with `external_id = "conv-abc-123"`
- **THEN** the trace API response includes `"session_external_id": "conv-abc-123"`

#### Scenario: Trace without session omits session_external_id
- **WHEN** a trace has no associated session (`session_id` is NULL)
- **THEN** the trace API response omits `session_external_id` (or returns null)

#### Scenario: Trace search results include session_external_id
- **WHEN** a trace search returns results via `GET /api/traces?q=something`
- **THEN** each trace in the results includes `session_external_id` if the trace belongs to a session

#### Scenario: Trace detail includes session_external_id
- **WHEN** a client requests `GET /api/traces/{id}` for a trace with a session
- **THEN** the `TraceDetail` response includes `session_external_id`

### Requirement: Unified Trace Read Model
The store layer SHALL use a single trace read model that carries trace data plus an optional `session_external_id` field. All trace read paths (sqlc fast-path and handwritten search) SHALL populate this model consistently so that API mapping stays uniform.

#### Scenario: Store model consistency across read paths
- **WHEN** the same trace is read via the fast-path `ListTraces` and via search `ListTracesFiltered`
- **THEN** both paths return the same `session_external_id` value for that trace
