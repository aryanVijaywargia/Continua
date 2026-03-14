## ADDED Requirements

### Requirement: Span Breadcrumb Navigation

The system SHALL render a breadcrumb path at the top of the selected span's detail panel. The breadcrumb SHALL show span names from root to the selected span, ordered root-first.

Ancestor segments SHALL be clickable and SHALL trigger span selection via the shared page-level selection callback.

The breadcrumb SHALL be synchronized with the shared selection state: when selection changes, the breadcrumb updates to reflect the new span's path.

#### Scenario: Selected span with ancestors

- **WHEN** a span three levels deep is selected
- **THEN** the breadcrumb shows: grandparent > parent > span, with grandparent and parent clickable

#### Scenario: Root span selected

- **WHEN** a root span is selected
- **THEN** the breadcrumb shows only the root span name, with no clickable ancestors

#### Scenario: Clicking a breadcrumb ancestor

- **WHEN** the user clicks an ancestor segment in the breadcrumb
- **THEN** that ancestor becomes the selected span across the tree, detail panel, and timeline (full synchronization via the shared page-level selection state)

### Requirement: Breadcrumb Keyboard Accessibility

All breadcrumb segments that are interactive (ancestor segments) SHALL be keyboard focusable and activatable with Enter or Space. Each interactive segment SHALL have an explicit accessible name.

#### Scenario: Keyboard navigation through breadcrumb

- **WHEN** a user tabs through the breadcrumb
- **THEN** each clickable ancestor segment receives focus in order

#### Scenario: Keyboard activation of breadcrumb segment

- **WHEN** a user presses Enter or Space on a focused breadcrumb segment
- **THEN** that ancestor becomes the selected span
