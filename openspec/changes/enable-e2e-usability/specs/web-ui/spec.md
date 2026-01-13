## ADDED Requirements

### Requirement: Traces List Page

The web UI SHALL display a paginated list of traces with filtering capabilities.

#### Scenario: Traces table displayed
- **WHEN** user navigates to `/traces`
- **AND** API key is configured
- **THEN** table of traces is displayed
- **AND** table includes columns: name, status, duration, tokens, cost, timestamp

#### Scenario: API key prompt
- **WHEN** user navigates to `/traces`
- **AND** no API key in localStorage
- **THEN** API key input prompt is displayed
- **AND** after entering key, traces are loaded

#### Scenario: Pagination
- **WHEN** more than 20 traces exist
- **THEN** pagination controls are displayed
- **AND** user can navigate between pages

#### Scenario: Status filter
- **WHEN** user selects status filter
- **THEN** table shows only traces with selected status

#### Scenario: Navigate to detail
- **WHEN** user clicks trace row
- **THEN** browser navigates to `/traces/:id`

### Requirement: Trace Detail Page

The web UI SHALL display trace details with an interactive span tree visualization.

#### Scenario: Trace metadata header
- **WHEN** user views trace detail page
- **THEN** header displays trace name, status, duration, tokens, cost

#### Scenario: Span tree displayed
- **WHEN** trace has spans
- **THEN** spans are displayed in hierarchical tree structure
- **AND** parent-child relationships are visualized

#### Scenario: Expand/collapse spans
- **WHEN** span has children
- **THEN** expand/collapse control is displayed
- **AND** clicking toggles visibility of children

#### Scenario: Span selection
- **WHEN** user clicks a span in the tree
- **THEN** span is highlighted
- **AND** detail panel shows span information

### Requirement: Span Detail Panel

The web UI SHALL display detailed span information when a span is selected.

#### Scenario: Span metadata displayed
- **WHEN** span is selected
- **THEN** panel shows name, type, status, duration

#### Scenario: LLM span details
- **WHEN** selected span has type "llm"
- **THEN** panel shows model name and token counts

#### Scenario: Input/output displayed
- **WHEN** span has input or output data
- **THEN** JSON is displayed in scrollable pre-formatted block

### Requirement: Visual Status Indicators

The web UI SHALL provide visual indicators for trace and span status.

#### Scenario: Status badge
- **WHEN** trace status is displayed
- **THEN** colored badge indicates status (green=completed, red=failed, blue=running)

#### Scenario: Error indication
- **WHEN** trace has errors
- **THEN** error count is displayed in red
