## ADDED Requirements

### Requirement: Inspector Tab Navigation
The inspector panel SHALL provide two tabs: Details (default) and Timeline.

#### Scenario: Details tab is active by default with a selected span
- **WHEN** the trace detail page loads with a span selected (via `?span=` or failure-first auto-selection)
- **THEN** the Details tab is active in the inspector
- **AND** the selected span detail, breadcrumb, payload inspectors, and parent-span navigation are visible

#### Scenario: Details tab with no selected span
- **WHEN** the trace detail page loads with no `?span=` parameter and the trace is not FAILED
- **THEN** the Details tab is active in the inspector
- **AND** the Details surface displays a prompt to select a span (matching the existing "Select a span to view details" empty state)
- **AND** Failure Summary and Stale Trace Signal are not rendered

#### Scenario: Switching to Timeline tab
- **WHEN** the user clicks the Timeline tab
- **THEN** the Timeline content becomes visible with existing error-only filtering, payload inspection, and span selection behavior

### Requirement: Tab Content Retention
Both Details and Timeline tabs SHALL remain mounted while tabbed so that tab-local state (e.g., error-only filter, scroll position) survives tab switches.

#### Scenario: Timeline state preserved across tab switches
- **WHEN** the user enables error-only filtering on the Timeline tab
- **AND** switches to the Details tab and back to Timeline
- **THEN** the error-only filter is still enabled

### Requirement: Switch To Details Callback
The inspector SHALL expose a `switchToDetails` callback that external panels (waterfall, timeline, failure summary) can invoke to programmatically switch the active tab to Details.

#### Scenario: Waterfall selection switches to Details
- **WHEN** the user selects a span via the waterfall
- **THEN** the inspector automatically switches to the Details tab to show the selected span
