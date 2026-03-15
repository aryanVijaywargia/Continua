## ADDED Requirements

### Requirement: Execution Waterfall Visualization
The workspace SHALL display an execution waterfall that renders timing bars for visible spans, with row order mirroring the visible tree preorder exactly.

#### Scenario: Waterfall rows match visible tree order
- **WHEN** the tree has visible (expanded) spans
- **THEN** the waterfall renders one row per visible span in the same preorder as the tree
- **AND** collapsing a tree node removes its descendants from the waterfall

#### Scenario: Waterfall has a sticky time axis
- **WHEN** the user scrolls the waterfall vertically
- **THEN** the time axis header remains fixed at the top of the waterfall panel

#### Scenario: Waterfall bars are keyboard-focusable
- **WHEN** the user tabs through the waterfall
- **THEN** each timing bar can receive keyboard focus
- **AND** pressing Enter or Space on a focused bar selects the corresponding span

### Requirement: Running Span Handling
The waterfall SHALL handle running spans deterministically by rendering them as bars extending to the current trace boundary or a visual marker indicating in-progress status.

#### Scenario: Running span renders with in-progress indicator
- **WHEN** a span has no ended_at timestamp
- **THEN** the waterfall renders the bar extending to the trace time boundary
- **AND** a visual indicator distinguishes it from completed spans

### Requirement: Minimum Visible Width
Very short spans SHALL have a minimum visible width in the waterfall so they remain clickable and visible.

#### Scenario: Sub-pixel duration span is still visible
- **WHEN** a span's duration is too short to render at the current time scale
- **THEN** the waterfall renders the bar at a minimum visible width
- **AND** the bar remains interactive (clickable and focusable)

### Requirement: Waterfall Tooltip
Each waterfall bar SHALL display a minimal tooltip on hover showing span name, status, and duration.

#### Scenario: Hovering a bar shows tooltip
- **WHEN** the user hovers over a waterfall timing bar
- **THEN** a tooltip displays the span name, status, and duration

### Requirement: Waterfall Selection Integration
Selecting a waterfall bar SHALL use the existing unified selection path and trigger `switchToDetails` on the inspector.

#### Scenario: Clicking a waterfall bar selects the span
- **WHEN** the user clicks a waterfall timing bar
- **THEN** the span is selected across tree, waterfall, and inspector
- **AND** the URL updates to include `?span=<span-id>`
- **AND** the inspector switches to the Details tab

### Requirement: Windowed Waterfall Rendering
The execution waterfall SHALL preserve the logical visible-row order while allowing off-screen rows to be omitted from the DOM on large traces for performance.

#### Scenario: Large waterfall windows off-screen rows
- **WHEN** the waterfall contains more visible rows than fit in the current viewport
- **THEN** rows outside the current viewport plus overscan window may be omitted from the DOM
- **AND** the rendered rows still follow the same preorder as the logical visible tree model

#### Scenario: Reveal scrolls a windowed waterfall row into view
- **WHEN** selection targets a waterfall row that is outside the rendered window
- **THEN** the waterfall scrolls that row into view
- **AND** the timing bar becomes rendered before the user interacts with it
