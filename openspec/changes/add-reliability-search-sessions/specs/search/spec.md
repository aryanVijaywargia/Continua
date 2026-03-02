## ADDED Requirements

### Requirement: Full-Text Search on Traces

The system SHALL provide full-text search on trace name and user_id fields.

#### Scenario: Search by trace name
- **WHEN** a user searches for "checkout flow"
- **THEN** traces with names containing both "checkout" and "flow" are returned
- **AND** results are ranked by relevance (name matches weighted higher)

#### Scenario: Search by user_id
- **WHEN** a user searches for "user123"
- **THEN** traces with user_id containing "user123" are returned

#### Scenario: Search performance
- **WHEN** searching across 100,000 traces
- **THEN** results are returned in under 200ms
- **AND** the GIN index is used (verified via EXPLAIN ANALYZE)

### Requirement: Search Query Parsing

The system SHALL parse `q` with `plainto_tsquery('english', q)` and treat empty or whitespace-only values as no full-text filter.

#### Scenario: Empty query
- **WHEN** `q` is empty or whitespace
- **THEN** no full-text predicate is applied
- **AND** results are filtered only by non-search filters

#### Scenario: Query parsing
- **WHEN** `q` contains multiple words
- **THEN** the system uses `plainto_tsquery('english', q)` for parsing
- **AND** results match all query terms (AND semantics)

### Requirement: Trace Filtering

The system SHALL support filtering traces by status, time range, user, session, errors, and duration.

#### Scenario: Filter by status
- **WHEN** `status=COMPLETED` is passed as query param
- **THEN** traces with stored status matching "completed" are returned (case-insensitive)

#### Scenario: Status filter maps to stored values
- **WHEN** `status=COMPLETED` is passed
- **THEN** traces with stored status in ("completed", "ok") are returned (case-insensitive mapping)

#### Scenario: Status filter for FAILED
- **WHEN** `status=FAILED` is passed
- **THEN** traces with stored status in ("failed", "error") are returned (case-insensitive)

#### Scenario: Status filter for RUNNING
- **WHEN** `status=RUNNING` is passed
- **THEN** traces with stored status "running" are returned (case-insensitive)

#### Scenario: Cancelled status handling
- **WHEN** a trace has stored status "cancelled"
- **THEN** it is treated as FAILED unless a new API status is added

#### Scenario: Filter by time range
- **WHEN** `start_time_from` and `start_time_to` are passed
- **THEN** only traces started within that range are returned

#### Scenario: Filter by time range with missing start_time
- **WHEN** a trace has start_time NULL
- **THEN** time-range filtering uses COALESCE(start_time, server_received_at)

#### Scenario: Filter by user_id
- **WHEN** `user_id=abc123` is passed
- **THEN** only traces for that user are returned

#### Scenario: Filter by session_id
- **WHEN** `session_id=sess_xyz` is passed
- **THEN** only traces in that session are returned

#### Scenario: Filter by has_errors
- **WHEN** `has_errors=true` is passed
- **THEN** only traces with error_count > 0 are returned

#### Scenario: has_errors is eventually consistent
- **WHEN** rollups are processed asynchronously
- **THEN** `has_errors=true` MAY lag behind the most recent span ingest until the rollup job completes

#### Scenario: Filter by minimum duration
- **WHEN** `min_duration_ms=5000` is passed
- **THEN** only traces with duration >= 5000ms are returned
- **AND** running traces use COALESCE(end_time, now()) for duration calculation
- **AND** start_time uses COALESCE(start_time, server_received_at) when start_time is NULL

### Requirement: Pagination and Total Semantics

The system SHALL return `total` as the count of distinct traces matching the active search and filters, and apply `limit/offset` after de-duplication.

#### Scenario: Multiple matching spans
- **WHEN** a trace has multiple spans that match the search query
- **THEN** the trace appears once in results
- **AND** `total` counts it once

#### Scenario: Pagination with span join
- **WHEN** searching with span name filter
- **THEN** results use SELECT DISTINCT on traces
- **AND** pagination applies after de-duplication

### Requirement: Search Ordering

The system SHALL order results by combined relevance across trace and span matches, with deterministic tie-breakers.

#### Scenario: Span-only match ordering
- **WHEN** `q` matches only span names (no trace field match)
- **THEN** traces are ordered by span match relevance
- **AND** ties fall back to COALESCE(start_time, server_received_at) DESC

#### Scenario: Trace match outranks span-only match
- **WHEN** `q` matches a trace name/user_id for one trace and only a span name for another
- **THEN** the trace match ranks higher than the span-only match

### Requirement: Combined Search and Filter

The system SHALL allow combining full-text search with filters.

#### Scenario: Search with status filter
- **WHEN** `q=checkout&status=FAILED` is passed
- **THEN** only failed traces matching "checkout" are returned

### Requirement: Search Vector Column

The system SHALL maintain a generated tsvector column on traces for efficient search.

#### Scenario: Search vector updated on insert
- **WHEN** a new trace is inserted
- **THEN** the search_vector column is automatically populated
- **AND** no trigger is required (PostgreSQL generated column)

#### Scenario: Search vector updated on name change
- **WHEN** a trace name is updated
- **THEN** the search_vector column is automatically recalculated

### Requirement: Search Index Backfill

The system SHALL ensure existing traces and spans are searchable immediately after the migration.

#### Scenario: Existing data searchable after migration
- **WHEN** the search migration is applied
- **THEN** previously ingested traces and spans are returned by search

### Requirement: Span Name Search

The system SHALL include span names in trace search results using an indexed tsvector on spans.

#### Scenario: Search finds trace by span name
- **WHEN** a user searches for "openai_chat"
- **THEN** traces containing spans with that name are returned
- **AND** the query uses the spans search index and joins on trace_id with project_id scoped
- **AND** each trace appears once

#### Scenario: Span search vector maintained
- **WHEN** a span is inserted or updated
- **THEN** spans.search_vector is automatically populated via generated column
