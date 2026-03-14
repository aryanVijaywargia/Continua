# Payload Inspector

Replaces the static `JsonViewer` `<pre>` renderer with an interactive JSON tree component used across all payload surfaces.

## ADDED Requirements

### Requirement: Payload Tree Construction

The system MUST build a typed, memoized tree structure from any JSON payload. The tree consists of `ObjectNode`, `ArrayNode`, and `PrimitiveNode` types. The tree is recomputed only when the payload reference changes.

#### Scenario: Object payload

Given a JSON object payload
When the tree is constructed
Then the root is an `ObjectNode` with one child per key

#### Scenario: Array payload

Given a JSON array payload
When the tree is constructed
Then the root is an `ArrayNode` with one child per element

#### Scenario: Primitive payload

Given a JSON primitive payload (string, number, boolean)
When the tree is constructed
Then the root is a `PrimitiveNode` rendered directly without expand/collapse

#### Scenario: JSON null payload

Given a JSON `null` payload
When the tree is constructed
Then the root is a `PrimitiveNode` rendering the literal text `null`

#### Scenario: Empty collection

Given an empty object `{}` or empty array `[]`
When the tree is constructed
Then the root node renders with a count badge showing `0 keys` or `0 items`

#### Scenario: Undefined payload (no data)

Given an `undefined` payload (field absent from the API response)
When the inspector is rendered
Then it displays a placeholder message (e.g., "No data") instead of a tree

### Requirement: Initial Expansion Rules

The system MUST apply deterministic initial expansion rules based on depth and shallow size.

#### Scenario: Root with few children

Given a root object or array with <= 200 immediate children
When the inspector first renders
Then the root is expanded

#### Scenario: Root with many children

Given a root object or array with > 200 immediate children
When the inspector first renders
Then the root is collapsed with a count badge showing the child count

#### Scenario: Depth-1 nodes with many children

Given a depth-1 object or array node with > 200 immediate children
When the inspector first renders
Then the node is collapsed with a count badge

#### Scenario: Nodes at depth >= 2

Given any object or array node at depth >= 2
When the inspector first renders
Then the node is collapsed

### Requirement: Expand and Collapse Interaction

The system MUST allow users to expand and collapse object and array nodes individually.

#### Scenario: Expanding a collapsed node

Given a collapsed object or array node
When the user clicks the expand toggle
Then the node's children are rendered

#### Scenario: Collapsing an expanded node

Given an expanded object or array node
When the user clicks the collapse toggle
Then the node's children are hidden

#### Scenario: Keyboard activation

Given focus on an expand/collapse toggle
When the user presses Enter or Space
Then the toggle activates as if clicked

### Requirement: Inspector Toolbar

Each inspector instance MUST render a toolbar with: search input, next/previous match buttons, expand all, collapse all, and copy full JSON.

#### Scenario: Toolbar layout

Given a rendered inspector
When the user views the inspector
Then the toolbar is visible above the tree with all five controls

#### Scenario: Expand all within threshold

Given a payload with total node count <= 5,000
When the user clicks "Expand all"
Then all object and array nodes expand

#### Scenario: Expand all exceeding threshold

Given a payload with total node count > 5,000
When the user views the toolbar
Then the "Expand all" button is disabled with a tooltip explaining the limit

#### Scenario: Collapse all

Given an inspector with some nodes expanded
When the user clicks "Collapse all"
Then all nodes return to their initial expansion state

### Requirement: Per-Inspector Search

Each inspector instance MUST maintain independent search state. Search covers object keys and scalar values (stringified). No JSONPath or path-segment queries.

#### Scenario: Key match

Given a payload containing key `"temperature"`
When the user types "temperature" in the search input
Then the key is highlighted and its ancestor nodes auto-expand

#### Scenario: Scalar value match

Given a payload containing value `42`
When the user types "42" in the search input
Then the value is highlighted and its ancestor nodes auto-expand

#### Scenario: No match

Given a payload not containing "nonexistent"
When the user types "nonexistent" in the search input
Then no highlights appear and match count shows "0 matches"

#### Scenario: Match navigation

Given a search with 5 matches
When the user clicks "Next"
Then the active match index advances to the next match in document order, cycling after the last

#### Scenario: Active match highlighting

Given a search with multiple matches
When the active match index points to match 3 of 5
Then match 3 has a distinct "active" highlight style (e.g., stronger background) while matches 1, 2, 4, 5 have a standard match highlight style

#### Scenario: Active match scroll into view

Given the active match is outside the visible scroll area of the inspector
When the active match changes (via next/previous or initial search)
Then the inspector scrolls the active match row into view using `scrollIntoView`. DOM focus remains on the search input (not moved to the match element).

#### Scenario: Deferred search input

Given a large payload
When the user types rapidly in the search input
Then the search filtering uses `useDeferredValue` to defer tree re-rendering, keeping the input responsive

#### Scenario: Search expansion restoration

Given the user manually expanded nodes A and B, then searched and the search auto-expanded nodes C and D
When the user clears the search input
Then nodes A and B remain expanded and nodes C and D return to their pre-search state

### Requirement: Value Rendering

The inspector MUST render values according to their type with appropriate formatting.

#### Scenario: Multiline string

Given a string value containing newlines
When the value is rendered
Then it displays in a bounded `white-space: pre-wrap` block with inner scrolling

#### Scenario: Long single-line string

Given a long string value without newlines
When the value is rendered
Then it wraps within the row view

#### Scenario: Object and array count badges

Given a collapsed object with 15 keys
When the row is rendered
Then it shows `{15 keys}` as a count badge

#### Scenario: Leaf value copy

Given a rendered primitive value
When the user activates the row's copy action
Then the raw leaf value is copied to clipboard

#### Scenario: Subtree copy

Given a rendered object or array node
When the user activates the row's copy action
Then the JSON-serialized subtree is copied to clipboard

### Requirement: Inspector Rollout

By the end of this phase, all payload surfaces MUST use the `PayloadInspector` component. The legacy `JsonViewer` `<pre>` renderer MUST NOT remain as a separate divergent component.

#### Scenario: Trace context payloads

Given the trace detail page with trace input and output
When the page renders
Then both trace input and trace output use `PayloadInspector`

#### Scenario: Span detail payloads

Given a selected span with input, output, and metadata
When SpanDetail renders
Then all three sections use `PayloadInspector`

#### Scenario: Timeline event payloads

Given an expanded timeline event with a payload
When the payload section renders
Then it uses `PayloadInspector`

#### Scenario: JsonViewer removal

Given the phase is complete
When auditing the codebase
Then `JsonViewer.tsx` either delegates entirely to `PayloadInspector` or is removed
