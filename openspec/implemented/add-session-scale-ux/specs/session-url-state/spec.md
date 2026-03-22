## ADDED Requirements

### Requirement: Sessions Page URL-Driven State
The `/sessions` page SHALL manage all filter, sort, and pagination state via URL search parameters. The URL SHALL include `q`, `user_id`, `sort_by`, `sort_dir`, `limit`, and `offset`. Any filter, sort, or page-size change SHALL reset `offset` to 0. Malformed parameters SHALL be normalized away and replaced in the URL.

#### Scenario: Deep-link rehydrates controls
- **WHEN** a user navigates to `/sessions?q=test&user_id=user-42&sort_by=trace_count&sort_dir=desc&limit=50&offset=100`
- **THEN** the search input shows "test"
- **AND** the user_id filter shows "user-42"
- **AND** sorting is set to trace_count descending
- **AND** page size is 50
- **AND** the table shows results from offset 100

#### Scenario: Filter change resets pagination
- **WHEN** the user changes the search query while on page 3
- **THEN** the offset resets to 0
- **AND** the URL updates to reflect the new query and `offset=0`

#### Scenario: Malformed params are normalized
- **WHEN** a user navigates to `/sessions?sort_by=invalid&limit=999`
- **THEN** `sort_by=invalid` is stripped from the URL
- **AND** `limit=999` is normalized down to the nearest lower allowed UI page-size option (20, 50, or 100), defaulting to 20 if the value is below all options, and replaced in the URL

### Requirement: FetchSessionsParams Web Client Type
The web API client SHALL define a `FetchSessionsParams` interface mirroring the object-parameter pattern used by `FetchTracesParams`, supporting `q`, `user_id`, `sort_by`, `sort_dir`, `limit`, and `offset`.

#### Scenario: Sessions fetch uses typed params
- **WHEN** the sessions page fetches data
- **THEN** it passes a `FetchSessionsParams` object to the API client
- **AND** only non-default parameters are included in the request URL

### Requirement: Session Detail Trace Table URL State
The session detail page (`/sessions/:id`) SHALL manage trace table state (`offset`, `limit`, `sort_by`, `sort_dir`) via URL search params on the session detail route. This enables exact restoration of table state via `returnTo` links. Today `SessionDetailPage` keeps `offset` in local `useState`, which is lost on navigation; Phase 10 moves this state to the URL.

#### Scenario: Session detail trace table state in URL
- **WHEN** a user sorts traces on session detail and navigates to page 3
- **THEN** the URL updates to `/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890?sort_by=started_at&sort_dir=asc&limit=20&offset=40`

#### Scenario: Session detail URL rehydrates trace table state
- **WHEN** a user navigates directly to `/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890?sort_dir=asc&offset=40`
- **THEN** the trace table renders with ascending sort and offset 40

### Requirement: Session Detail Trace Navigation
Session detail trace links SHALL pass `state={{ returnTo: currentSessionDetailUrl }}` when linking to trace detail, where `currentSessionDetailUrl` includes the full path and search params. The trace detail `returnTo` validator SHALL accept paths starting with `/sessions/` in addition to existing allowed paths.

#### Scenario: Session detail trace link includes returnTo with table state
- **WHEN** a user clicks a trace link on `/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890?sort_by=started_at&sort_dir=asc&offset=20`
- **THEN** the trace detail page receives `returnTo = "/sessions/a1b2c3d4-e5f6-7890-abcd-ef1234567890?sort_by=started_at&sort_dir=asc&offset=20"` via location state

#### Scenario: Trace detail accepts session returnTo
- **WHEN** the trace detail page receives a `returnTo` starting with `/sessions/`
- **THEN** the back navigation uses this URL instead of the default traces list
- **AND** navigating back restores the exact session detail table state

### Requirement: Session Detail Trace Table UX
The session detail page SHALL provide trace sorting (by `started_at`), page-size control, keep-previous-data behavior, and stale-offset repair for the trace table. It SHALL NOT include the full trace filter bar.

#### Scenario: Session detail trace sorting
- **WHEN** the user clicks the Started column header on the session detail trace table
- **THEN** the trace sort order toggles, the URL updates, and the table re-fetches

#### Scenario: Session detail does not show trace filter bar
- **WHEN** the session detail page renders its trace table
- **THEN** no search input, status filter, or other trace filter controls are displayed
