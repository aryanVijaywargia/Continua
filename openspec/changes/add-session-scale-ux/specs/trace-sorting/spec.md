## ADDED Requirements

### Requirement: Trace List Sorting
The `/api/traces` endpoint SHALL accept optional `sort_by` and `sort_dir` query parameters. `sort_by` SHALL accept only `started_at`. `sort_dir` SHALL accept `asc` or `desc` and default to `desc`. When `q` is present, the server SHALL ignore `sort_by` and `sort_dir` and use relevance ordering. The implementation SHALL use two static sqlc queries (`ListTraces` for DESC, `ListTracesAsc` for ASC) rather than dynamic SQL, since sqlc cannot parameterize SQL keywords.

#### Scenario: Sort traces by started_at ascending
- **WHEN** a client requests `GET /api/traces?sort_by=started_at&sort_dir=asc`
- **THEN** traces are returned ordered by `COALESCE(start_time, server_received_at) ASC`

#### Scenario: Sort traces by started_at descending (explicit)
- **WHEN** a client requests `GET /api/traces?sort_by=started_at&sort_dir=desc`
- **THEN** traces are returned ordered by `COALESCE(start_time, server_received_at) DESC`

#### Scenario: Default sort when no sort params
- **WHEN** a client requests `GET /api/traces` without `sort_by` or `sort_dir`
- **THEN** traces are returned in the default order: `COALESCE(start_time, server_received_at) DESC`

#### Scenario: Search overrides user sorting
- **WHEN** a client requests `GET /api/traces?q=something&sort_by=started_at&sort_dir=asc`
- **THEN** the server ignores `sort_by` and `sort_dir`
- **AND** traces are returned in relevance order as defined by the existing search ranking

#### Scenario: Invalid sort_by value
- **WHEN** a client requests `GET /api/traces?sort_by=invalid_field`
- **THEN** the server ignores the invalid `sort_by` and uses default ordering

### Requirement: Trace List Sortable UI Header
The traces list page SHALL render the `Started` column header as sortable. Clicking the header SHALL toggle between ascending and descending order and update the URL search params. When search is active, the sortable header SHALL be non-clickable and visually muted.

#### Scenario: Click Started header to sort ascending
- **WHEN** the user clicks the `Started` column header while traces are in default descending order
- **THEN** the URL updates to include `sort_by=started_at&sort_dir=asc`
- **AND** the table re-fetches with ascending order

#### Scenario: Sort header disabled during search
- **WHEN** the user has an active search query (`q` is non-empty)
- **THEN** the `Started` column header is visually muted and non-clickable
