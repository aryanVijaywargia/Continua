## ADDED Requirements

### Requirement: Desktop Split-Panel Layout
The trace detail workspace SHALL render a split-panel layout on desktop viewports using `react-resizable-panels` with a left tree rail and a right workspace containing a waterfall on top and a tabbed inspector below.

#### Scenario: Desktop viewport renders three-region layout
- **WHEN** the trace detail page loads on a desktop viewport
- **THEN** the layout displays a resizable left panel containing the span tree rail
- **AND** a resizable right panel split vertically into a top waterfall region and a bottom inspector region

#### Scenario: Panels are resizable via drag handles
- **WHEN** the user drags a panel resize handle on desktop
- **THEN** the adjacent panels resize proportionally
- **AND** panel sizes are not persisted to the URL or local storage

### Requirement: Non-Desktop Stacked Tabbed Layout
The trace detail workspace SHALL render a single stacked tabbed layout on non-desktop viewports with four tabs: `Waterfall`, `Tree`, `Details`, and `Timeline`. The `Details` tab SHALL be active by default. The `Details` tab content SHALL reuse the same core span-details content component used in the desktop inspector panel (Failure Summary, Stale Trace Signal, selected span detail, breadcrumb, payload inspectors, parent-span navigation). Trace Context placement is a viewport-specific wrapper difference: on desktop it lives above the workspace as a disclosure; on non-desktop it renders as a collapsible section inside the Details tab.

#### Scenario: Non-desktop viewport renders tabbed layout with Details active
- **WHEN** the trace detail page loads on a non-desktop viewport
- **THEN** the layout displays four tabs: Waterfall, Tree, Details, and Timeline
- **AND** the Details tab is active by default
- **AND** only one tab's content is visible at a time

#### Scenario: Non-desktop Details tab shares core content with desktop inspector
- **WHEN** the user views the Details tab on a non-desktop viewport
- **THEN** the core content is the same as the Details tab in the desktop inspector panel
- **AND** includes Failure Summary, Stale Trace Signal, selected span detail, breadcrumb, payload inspectors, and parent-span navigation
- **AND** additionally includes the Trace Context collapsible section (which on desktop renders above the workspace instead)

### Requirement: Trace Context Collapsed By Default
Trace Context SHALL render collapsed by default. On desktop it renders as a disclosure above the workspace. On non-desktop it renders as a collapsible section inside the Details tab.

#### Scenario: Desktop Trace Context starts collapsed
- **WHEN** the trace detail page loads on desktop
- **THEN** the Trace Context disclosure above the workspace is collapsed
- **AND** the user can expand it on demand

#### Scenario: Non-desktop Trace Context inside Details tab
- **WHEN** the user views the Details tab on a non-desktop viewport
- **THEN** Trace Context renders as a collapsible section within the Details tab content
- **AND** it starts collapsed

### Requirement: Failure Summary In Details Surface
Failure Summary SHALL render inside the Details surface as a compact section at the top for failed traces, rather than as a full-width page banner. This applies to the desktop inspector Details tab and the non-desktop top-level Details tab identically.

#### Scenario: Failed trace shows Failure Summary in Details
- **WHEN** the trace status is FAILED
- **THEN** Failure Summary renders at the top of the Details surface content
- **AND** no full-width failure banner appears above the workspace

### Requirement: Stale Trace Signal In Details Surface
StaleTraceSignal SHALL render inside the Details surface, directly below Failure Summary, when applicable. This applies to the desktop inspector Details tab and the non-desktop top-level Details tab identically.

#### Scenario: Stale running trace shows signal in Details
- **WHEN** the trace is running and meets stale criteria
- **THEN** StaleTraceSignal renders in the Details surface below Failure Summary
- **AND** no full-width stale signal banner appears above the workspace
