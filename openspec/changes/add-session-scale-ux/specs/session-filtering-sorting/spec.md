## ADDED Requirements

### Requirement: Session List Filtering
The `/api/sessions` endpoint SHALL accept optional `q` and `user_id` query parameters. `q` SHALL perform fuzzy search on `external_id` and `name`. `user_id` SHALL be an exact-match structured filter and is not part of fuzzy ranking. Both filters MAY be combined.

#### Scenario: Filter sessions by search query
- **WHEN** a client requests `GET /api/sessions?q=conv`
- **THEN** sessions whose `external_id` or `name` match "conv" are returned in ranked order

#### Scenario: Filter sessions by exact user_id
- **WHEN** a client requests `GET /api/sessions?user_id=user-42`
- **THEN** only sessions with `user_id = 'user-42'` are returned

#### Scenario: Combine search and user_id filter
- **WHEN** a client requests `GET /api/sessions?q=test&user_id=user-42`
- **THEN** only sessions matching both the search query and exact `user_id` are returned
- **AND** results are ordered by search ranking

#### Scenario: Empty query returns all sessions
- **WHEN** a client requests `GET /api/sessions` without `q` or `user_id`
- **THEN** all sessions for the project are returned in default order

### Requirement: Session List Sorting
The `/api/sessions` endpoint SHALL accept optional `sort_by` and `sort_dir` query parameters. `sort_by` SHALL accept `created_at` or `trace_count`. `sort_dir` SHALL accept `asc` or `desc` and default to `desc`. When `q` is present, the server SHALL ignore `sort_by` and `sort_dir` and use search ranking instead.

#### Scenario: Sort sessions by created_at ascending
- **WHEN** a client requests `GET /api/sessions?sort_by=created_at&sort_dir=asc`
- **THEN** sessions are returned ordered by `created_at ASC`

#### Scenario: Sort sessions by trace_count descending
- **WHEN** a client requests `GET /api/sessions?sort_by=trace_count&sort_dir=desc`
- **THEN** sessions are returned ordered by trace count descending
- **AND** trace count is computed via a derived-table aggregate join

#### Scenario: Default sort when no sort params
- **WHEN** a client requests `GET /api/sessions` without `sort_by` or `sort_dir`
- **THEN** sessions are returned in default order: `created_at DESC`

#### Scenario: Search overrides user sorting on sessions
- **WHEN** a client requests `GET /api/sessions?q=test&sort_by=created_at&sort_dir=asc`
- **THEN** the server ignores `sort_by` and `sort_dir`
- **AND** sessions are returned in search ranking order

### Requirement: Session List Pagination
The `/api/sessions` endpoint SHALL accept `limit` and `offset` query parameters. The API SHALL use the existing server-wide pagination defaults (`defaultPageLimit=50`, `maxPageLimit=200`) and SHALL NOT restrict `limit` to a whitelist of values. The response SHALL include a `total` field reflecting the count after filters are applied. The UI page-size selector offers `20`, `50`, and `100` as options with a UI default of `20`, but these are UI-only choices and are not enforced by the API.

#### Scenario: Paginated session list with filters
- **WHEN** a client requests `GET /api/sessions?q=test&limit=50&offset=50`
- **THEN** the response contains up to 50 sessions starting at offset 50
- **AND** `total` reflects the total count of sessions matching the search query

#### Scenario: API accepts arbitrary limit within server max
- **WHEN** a client requests `GET /api/sessions?limit=75`
- **THEN** the server accepts the limit and returns up to 75 sessions

### Requirement: Session Sorting UI
The sessions list page SHALL render `Traces` and `Created` column headers as sortable. Clicking a sortable header SHALL toggle sort direction and update URL search params. `Session`, `User ID`, and `Name` columns SHALL remain visible but non-sortable. When search is active, sortable headers SHALL become non-clickable and visually muted.

#### Scenario: Click Traces header to sort by trace_count
- **WHEN** the user clicks the `Traces` column header
- **THEN** the URL updates to include `sort_by=trace_count` with toggled `sort_dir`
- **AND** the table re-fetches with the new sort order

#### Scenario: Sort headers disabled during search
- **WHEN** the user has an active search query
- **THEN** the `Traces` and `Created` sortable headers are visually muted and non-clickable

### Requirement: Dynamic Session Query Path
The store layer SHALL use a handwritten dynamic query file for session search, user_id filtering, and non-default sorting. The default session listing (no search, no user_id, default sort) MAY continue to use sqlc.

#### Scenario: trace_count sorting uses derived-table aggregate
- **WHEN** sorting sessions by `trace_count`
- **THEN** the query uses `LEFT JOIN (SELECT session_id, COUNT(*) AS cnt FROM traces WHERE project_id = $1 GROUP BY session_id) tc ON tc.session_id = s.id`
- **AND** uses `COALESCE(tc.cnt, 0)` as the sort value
- **AND** never joins traces directly on the outer query

### Requirement: Session Index for Sorted Pagination
A database index `(project_id, created_at DESC, id DESC)` SHALL be added on the `sessions` table to support efficient sorted pagination. The existing `(project_id, user_id)` index SHALL be reused for `user_id` filtering.

#### Scenario: Index supports default session listing
- **WHEN** sessions are listed with default `created_at DESC` sorting
- **THEN** the query uses the composite index for efficient sequential access
