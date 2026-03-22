## ADDED Requirements

### Requirement: Shared Expansion State
The workspace SHALL manage span expansion state via a shared `expandedSpanIds` set at the workspace level, replacing per-node local expansion state. The set SHALL initialize with all span IDs that have children so the tree renders fully expanded by default.

#### Scenario: Default expansion matches current behavior
- **WHEN** the trace detail page loads with spans
- **THEN** all tree nodes with children are expanded
- **AND** the visual appearance matches the existing fully-expanded default

#### Scenario: Collapse propagates to waterfall
- **WHEN** the user collapses a span in the tree
- **THEN** the span's descendants are removed from the waterfall visible rows
- **AND** the shared `expandedSpanIds` set no longer contains the collapsed span ID

### Requirement: Shared Selection State
The workspace SHALL manage selected span state at the workspace level so that tree, waterfall, and inspector all reflect the same selection. The `?span=<external-span-id>` URL parameter remains the only persisted UI state.

#### Scenario: Selection syncs across all panels
- **WHEN** the user selects a span from any panel (tree, waterfall, timeline, breadcrumb, parent navigation, failure summary)
- **THEN** the tree highlights the selected span
- **AND** the waterfall highlights the corresponding bar
- **AND** the inspector shows the selected span details
- **AND** the URL updates to `?span=<span-id>`

#### Scenario: Cross-trace navigation resets state
- **WHEN** the user navigates to a different trace
- **THEN** selection, expansion, search, and inspector tab state all reset

### Requirement: Sticky Manual Selection
The workspace SHALL track whether the user has made a deliberate span selection. Once a manual selection is made, failure-first auto-selection SHALL NOT override it during polling updates, timeline refreshes, or re-renders. The sticky flag resets only on cross-trace navigation or when the URL `?span=` parameter is removed via browser back/forward.

#### Scenario: Manual selection is not overridden by failure-first auto-selection
- **WHEN** the user manually selects a non-failed span on a FAILED trace
- **AND** a polling update or timeline refresh occurs
- **THEN** the manually selected span remains selected
- **AND** failure-first auto-selection does not run

#### Scenario: URL-driven span param sets sticky selection
- **WHEN** the page loads with `?span=<valid-span-id>` in the URL
- **THEN** the span is selected and the sticky flag is set
- **AND** failure-first auto-selection does not override it

#### Scenario: Browser back removing span param clears sticky flag
- **WHEN** the user navigates back and the `?span=` parameter is removed
- **THEN** the sticky flag clears
- **AND** failure-first auto-selection re-runs if the trace status is FAILED

### Requirement: Pure Tree Helpers
The workspace SHALL use pure helper functions for tree construction, ancestor lookup, preorder flattening, and visible-row derivation in a dedicated tree utility module. Waterfall time-scale helpers SHALL live in a separate waterfall utility module. Both sets of helpers SHALL be independently testable.

#### Scenario: Preorder flattening respects expansion state
- **WHEN** given a tree and an expansion set
- **THEN** the preorder flattening returns only visible (expanded) rows in depth-first order
- **AND** collapsed subtrees are excluded

#### Scenario: Orphan and cycle handling
- **WHEN** spans contain orphaned nodes (missing parent) or cyclic parent references
- **THEN** orphans are promoted to roots
- **AND** cycles are broken deterministically without infinite loops

### Requirement: Versioned Reveal Signal
The workspace SHALL use a versioned reveal signal (monotonically incrementing counter) so that selecting the same span again still triggers reveal behavior (scroll-into-view and ancestor expansion). The version SHALL increment on every selection, including re-selection of the already-selected span.

#### Scenario: Re-selecting the same span triggers reveal
- **WHEN** span A is already selected
- **AND** the user selects span A again (e.g., via failure summary jump or timeline span button)
- **THEN** the reveal signal version increments
- **AND** the tree re-expands collapsed ancestors of span A and scrolls to it
- **AND** the waterfall scrolls to the corresponding bar

#### Scenario: Reveal signal consumed by tree and waterfall independently
- **WHEN** the reveal signal version increments
- **THEN** each panel (tree, waterfall) independently observes the new version and runs its own scroll-into-view logic

### Requirement: Independent Panel Scrolling
Tree and waterfall SHALL scroll independently. On selection, each panel runs its own `scrollIntoView({ block: 'nearest' })`. No linked scrolling between panels.

#### Scenario: Selecting a span scrolls both panels independently
- **WHEN** the user selects a span
- **THEN** the tree scrolls to reveal the selected span row if needed
- **AND** the waterfall scrolls to reveal the corresponding bar if needed
- **AND** neither panel's scroll position is mechanically linked to the other
