## ADDED Requirements

### Requirement: Index/Query Alignment

The system SHALL only replace an existing index if all call sites filter by the new leading columns.

#### Scenario: Span-by-trace queries remain indexed
- **WHEN** spans are queried by trace_id only
- **THEN** an index leading with trace_id remains OR the query is updated to include project_id

#### Scenario: Existing query patterns preserved
- **WHEN** an index is replaced
- **THEN** all queries using that index are verified to include the new leading column

### Requirement: Project-Scoped Indexes for Project-Filtered Queries

The system SHALL add project_id-leading indexes for queries that already filter by project_id.

#### Scenario: Traces list uses project_id + time
- **WHEN** querying traces with project_id and start_time ordering
- **THEN** the planner uses idx_traces_project_started_at

### Requirement: Index Migration Scope

The system SHALL scope index changes to the actual indexes in the current schema.

#### Scenario: Indexes in scope for replacement
- **WHEN** the migration runs
- **THEN** it recreates only these non-project-scoped indexes where queries already include project_id:
  - idx_traces_started_at → idx_traces_project_started_at
  - idx_traces_server_received → idx_traces_project_server_received

#### Scenario: Trace_id-leading indexes preserved
- **WHEN** the migration runs
- **THEN** these indexes are preserved (or only modified if queries are also updated):
  - idx_spans_trace (trace_id)
  - idx_spans_trace_span (trace_id, span_id)
  - idx_span_events_trace (trace_id)
  - idx_span_events_trace_span (trace_id, span_id)

### Requirement: Multi-Tenant Query Performance

The system SHALL maintain query performance for multi-tenant workloads.

#### Scenario: Query performance with project filter
- **WHEN** querying with project_id filter
- **THEN** only relevant rows are scanned
- **AND** query time scales with project data size, not total table size
