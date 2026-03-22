# Navigation Continuity

Unified span selection callback and parent span navigation for consistent cross-surface behavior.

## ADDED Requirements

### Requirement: Unified Selection Callback

All span-selection entry points MUST go through one shared page-level selection callback. The callback is defined in `TraceDetailPage.tsx` and passed down via props.

#### Scenario: Callback responsibilities

Given any span selection event from any entry point
When the unified callback fires
Then it: (1) updates `selectedSpanExternalId`, (2) sets `userHasSelected = true`, (3) computes and sets `revealPath` for tree auto-expand, (4) writes `?span=` with `replace`

#### Scenario: Span tree row selection

Given the user clicks a span in the span tree
When the click handler fires
Then it calls the unified selection callback

#### Scenario: Breadcrumb ancestor selection

Given the user clicks an ancestor in the span breadcrumb
When the click handler fires
Then it calls the unified selection callback

#### Scenario: Failure summary jump

Given the user clicks "Jump to failed span" in the failure summary
When the click handler fires
Then it calls the unified selection callback

#### Scenario: Timeline span button

Given a timeline event with a `span_id` that resolves to a known span in the current span index
When the event renders
Then the span reference is a clickable button (using `span_name` as label when available, falling back to the `span_id` when `span_name` is absent), and clicking it calls the unified selection callback

#### Scenario: Timeline event with unresolvable span_id

Given a timeline event with a `span_id` that does NOT resolve to a known span in the current span index
When the event renders
Then the span reference renders as plain text (not clickable)

#### Scenario: Parent span button

Given the user clicks the parent span navigation button in span detail
When the click handler fires
Then it calls the unified selection callback

### Requirement: Parent Span Navigation

The system MUST render the parent span ID as a navigable button when the parent exists in the current span index.

#### Scenario: Parent exists in index

Given a selected span whose `parent_span_id` references a span in the current trace's span data
When the parent span ID is rendered in SpanDetail
Then it renders as a clickable button that triggers the unified selection callback

#### Scenario: Parent not resolvable

Given a selected span whose `parent_span_id` is a non-null value that does NOT resolve to a span in the current trace's span data
When the parent span ID is rendered in SpanDetail
Then it renders as plain text (not clickable)

#### Scenario: Root span (no parent)

Given a selected span with `parent_span_id` that is null or absent
When SpanDetail renders
Then no parent span row is shown

#### Scenario: Keyboard activation

Given focus on the parent span navigation button
When the user presses Enter or Space
Then the navigation activates as if clicked
