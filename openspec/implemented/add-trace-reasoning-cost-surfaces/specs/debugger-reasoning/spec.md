## ADDED Requirements

### Requirement: Trace-Level Reasoning Tab
The debugger workspace SHALL provide a Reasoning tab that displays all valid `decision` timeline events across the entire trace, ordered using the existing `compareTimelineEvents()` comparator from `timeline.ts`.

Each row SHALL display: timestamp, originating span name (from `event.span_name`, falling back to span lookup only when absent), question, chosen value, and optional reasoning and alternatives.

The Reasoning tab SHALL appear in both the desktop inspector tab bar and the mobile workspace tab bar.

#### Scenario: Trace with multiple decision events across spans
- **WHEN** a trace contains valid decision events in three different spans
- **THEN** the Reasoning tab lists all three decisions sorted by timestamp
- **AND** each row shows the span name, question, and chosen value

#### Scenario: Trace with no valid decision events
- **WHEN** a trace contains no valid decision events
- **THEN** the Reasoning tab displays an empty-state message

#### Scenario: Malformed decision events are excluded
- **WHEN** a trace contains decision events with missing `question` or `chosen` fields
- **THEN** those events are excluded from the Reasoning tab
- **AND** they remain visible in the generic timeline view

#### Scenario: Decision events with tied timestamps
- **WHEN** two decision events share the same timestamp
- **THEN** both appear in the list ordered by `compareTimelineEvents()` (source rank, then sequence, then event id)

### Requirement: Reasoning Tab Navigation
Activating a row in the Reasoning tab SHALL select the originating span through the shared selection path and switch the view to the Details tab. Each row SHALL be rendered as a `<button>` element to match the existing keyboard-accessible selection pattern used in Timeline and ExecutionWaterfall.

#### Scenario: Click reasoning row on desktop
- **WHEN** the user clicks a reasoning row on a desktop layout
- **THEN** the originating span is selected
- **AND** the inspector switches to the Details tab via `switchToDetailsRef`

#### Scenario: Click reasoning row on mobile
- **WHEN** the user clicks a reasoning row on a mobile layout
- **THEN** the originating span is selected
- **AND** the active mobile workspace tab switches to `details`

#### Scenario: Keyboard activation of reasoning row
- **WHEN** the user focuses a reasoning row and presses Enter or Space
- **THEN** the same navigation behavior fires as a click

### Requirement: Reasoning Derivation Source
The Reasoning tab SHALL derive its entries exclusively from `getDecisionDetails()` applied to `timeline.events`. No additional backend queries or data sources are introduced.

#### Scenario: Derivation uses existing timeline data
- **WHEN** the trace detail page loads timeline events
- **THEN** the reasoning entries are computed client-side via `useMemo` over the existing events array
- **AND** no additional API calls are made for the Reasoning tab
