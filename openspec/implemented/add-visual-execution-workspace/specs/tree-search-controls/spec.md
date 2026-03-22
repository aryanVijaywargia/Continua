## ADDED Requirements

### Requirement: Tree Search Input
The span tree rail SHALL include a search input that searches the tree non-destructively using case-insensitive matching on span name, kind, span ID, status, model, provider, and inline error preview.

#### Scenario: Search highlights matches and dims non-matches
- **WHEN** the user types a search query
- **THEN** spans matching the query are highlighted
- **AND** non-matching spans are dimmed but remain visible in the tree
- **AND** ancestors of matched spans are auto-expanded

#### Scenario: Clearing search restores prior expansion state
- **WHEN** the user clears the search input
- **THEN** the tree expansion state returns to what it was before the search began
- **AND** no matches are highlighted or dimmed

#### Scenario: Search does not block input
- **WHEN** the user types rapidly in the search input
- **THEN** the search model updates on a deferred render cycle
- **AND** the input remains responsive without blocking

### Requirement: Expand All Control
The span tree rail SHALL include an Expand All button gated by projected visible-row expansion cost with a tunable constant.

#### Scenario: Expand all within cost threshold
- **WHEN** the user clicks Expand All
- **AND** the projected visible-row count is within the cost threshold
- **THEN** all collapsible tree nodes expand

#### Scenario: Expand all exceeds cost threshold
- **WHEN** the user clicks Expand All
- **AND** the projected visible-row count exceeds the cost threshold
- **THEN** the system displays a `window.confirm` dialog warning the user about potential performance impact
- **AND** expands all nodes only if the user confirms
- **AND** does nothing if the user cancels

### Requirement: Collapse All Control
The span tree rail SHALL include a Collapse All button that collapses all tree nodes to show only root spans.

#### Scenario: Collapse all reduces tree to roots
- **WHEN** the user clicks Collapse All
- **THEN** only root-level spans remain visible in the tree
- **AND** the waterfall updates to show only root-level rows

### Requirement: Show Metrics Toggle
The span tree rail SHALL include a Show Metrics toggle that shows or hides additional inline metric hints (token count, cost) on tree rows. Duration and status badge remain always visible regardless of toggle state, as they are already part of the shipped tree row layout.

#### Scenario: Enabling metrics toggle shows additional hints
- **WHEN** the user enables the Show Metrics toggle
- **THEN** each tree row displays additional inline metric hints for token count and cost
- **AND** duration and status badge remain visible as before

#### Scenario: Disabling metrics toggle hides additional hints
- **WHEN** the user disables the Show Metrics toggle
- **THEN** token count and cost hints are hidden from tree rows
- **AND** duration and status badge remain visible

### Requirement: Windowed Tree Rendering
The span tree rail SHALL preserve the full logical visible-row model while allowing off-screen rows to be omitted from the DOM on large traces for performance.

#### Scenario: Large tree windows off-screen rows
- **WHEN** the tree contains more visible rows than fit comfortably in the viewport
- **THEN** rows outside the current viewport plus overscan window may be omitted from the DOM
- **AND** the logical visible-row count, expand/collapse behavior, and match highlighting semantics remain unchanged

#### Scenario: Reveal scrolls a windowed row into view
- **WHEN** selection or search targets a row that is currently outside the rendered window
- **THEN** the tree scrolls that row into view
- **AND** the row becomes rendered before the user inspects it
