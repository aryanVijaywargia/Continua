## ADDED Requirements

### Requirement: State Change Viewer
A state change viewer SHALL display emitted `state_change` events grouped by namespace. This is a structured viewer over explicitly emitted events, not a deep-diff engine or snapshot comparison system. Scalar values SHALL be displayed inline (key: old → new). Object/array values SHALL be rendered using the existing JSON tree viewer. Events where `payload.key` is missing SHALL be excluded before rendering.

#### Scenario: Render grouped state changes
- **WHEN** the StateDiffViewer receives state change events with namespaces "user" and "config"
- **THEN** changes are grouped under their namespace headings
- **AND** scalar values show inline old→new transitions

#### Scenario: Render object value changes
- **WHEN** a state change has object values for `old_value` or `new_value`
- **THEN** the values are rendered using `JsonViewer`

#### Scenario: Handle events with missing key
- **WHEN** state change extraction receives events where some lack `payload.key`
- **THEN** those events are excluded from the output
- **AND** only events with `payload.key` present are shown in the viewer

### Requirement: State Change Extraction
A utility SHALL filter timeline events to only `state_change` type events where `payload.key` exists, and return them structured for the state change viewer.

#### Scenario: Filter to state_change events with key
- **WHEN** given a mixed list of timeline events
- **THEN** the utility returns only events with `event_type === "state_change"` and `payload.key` defined

#### Scenario: Empty result for no matching events
- **WHEN** given timeline events with no `state_change` events
- **THEN** the utility returns an empty array

### Requirement: State Tab in InspectorTabs
`InspectorTabs` SHALL support a third "State" tab displaying the `StateDiffViewer`. The tab SHALL show a badge count of state change events only when the count is greater than zero. When the count is zero, no badge or "(0)" SHALL be displayed.

#### Scenario: State tab with changes
- **WHEN** the trace has 3 state_change events with valid keys
- **THEN** the State tab label shows a badge with "3"

#### Scenario: State tab with no changes
- **WHEN** the trace has no state_change events
- **THEN** the State tab label shows "State" with no badge

### Requirement: Span Decisions Section
The span detail view SHALL render a "Decisions" section showing `decision` events associated with the currently selected span. Decision events where `payload.question` or `payload.chosen` is missing SHALL be skipped.

#### Scenario: Show decisions for selected span
- **WHEN** a span is selected that has 2 decision events with valid question and chosen fields
- **THEN** SpanDetail renders a "Decisions" section with both entries

#### Scenario: Skip decisions with missing fields
- **WHEN** a decision event for the selected span is missing `payload.chosen`
- **THEN** that decision is not rendered in the Decisions section

#### Scenario: No decisions section when empty
- **WHEN** the selected span has no valid decision events
- **THEN** the Decisions section is not rendered

### Requirement: Mobile State Tab
The mobile workspace tab set SHALL include a 5th "State" tab alongside the existing Details, Waterfall, Tree, and Timeline tabs.

#### Scenario: State tab in mobile layout
- **WHEN** the debugger is viewed on a mobile viewport
- **THEN** the tab bar includes a "State" tab
- **AND** tapping it shows the StateDiffViewer content
