## ADDED Requirements

### Requirement: Timeline Segmented Filter Control

The `Timeline` component SHALL provide a segmented control with three mutually exclusive modes: `All` (default), `Semantic`, and `Effects & waits`.

The control SHALL be implemented as a `<div role="radiogroup">` containing `<button role="radio">` elements with appropriate `aria-checked` attributes and arrow-key navigation per the WAI-ARIA radio group pattern.

#### Scenario: Default state shows all events
- **WHEN** the Timeline component mounts
- **THEN** the segmented control is set to `All` and all events are visible, subject to the `Errors only` toggle

#### Scenario: Semantic filter shows only semantic explicit events
- **WHEN** the user selects `Semantic`
- **THEN** only explicit events with `event_type` in `{state_change, decision, effect, wait}` are displayed
- **AND** synthetic events (`span_started`, `span_completed`, `span_failed`) are hidden

#### Scenario: Effects & waits filter shows only effect and wait events
- **WHEN** the user selects `Effects & waits`
- **THEN** only explicit events with `event_type` in `{effect, wait}` are displayed

#### Scenario: Keyboard navigation within segmented control
- **WHEN** the segmented control has focus and the user presses the right arrow key
- **THEN** focus moves to the next option and that option becomes selected
- **AND** pressing left arrow moves to the previous option

### Requirement: Filter Composition with Errors Only

The segmented filter SHALL compose with the existing `Errors only` toggle by intersection. When both filters are active, an event MUST satisfy both conditions to be displayed.

#### Scenario: Semantic plus Errors only
- **WHEN** the segmented control is set to `Semantic` and `Errors only` is active
- **THEN** only events matching both conditions are shown: the intersection of the semantic predicate and `isTimelineErrorEvent`, with synthetic rows still excluded by the semantic predicate

#### Scenario: Effects & waits plus Errors only
- **WHEN** the segmented control is set to `Effects & waits` and `Errors only` is active
- **THEN** only effect or wait events that are also error events are shown

#### Scenario: All plus Errors only
- **WHEN** the segmented control is set to `All` and `Errors only` is active
- **THEN** behavior is identical to the existing `Errors only` filter

### Requirement: Timeline Filter State Lifecycle

Timeline filter state, both the segmented control and errors-only toggle, SHALL be local to the `Timeline` component instance. Filter state SHALL NOT be URL-backed and SHALL NOT be included in React Query cache keys.

These lifecycle guarantees operate at the page level through `TraceDetailPage` mount and unmount semantics, not only within the isolated `Timeline` component.

#### Scenario: Filter state persists across inspector tab switches
- **WHEN** the user sets the segmented control to `Semantic`, switches from the Timeline tab to the Details tab, and switches back to the Timeline tab
- **THEN** the segmented control remains set to `Semantic`

#### Scenario: Filter state resets on trace navigation
- **WHEN** the user navigates from one trace to another, triggering a `TraceDetailPage` remount
- **THEN** the segmented control resets to `All` and `Errors only` resets to off

### Requirement: Filter Correctness During Live Polling

When a timeline filter is active on a running trace, newly arrived events from polling updates SHALL be subject to the same filter predicate as existing events. The filter SHALL NOT require user interaction to apply to incrementally merged events.

#### Scenario: Active filter applies to polled events on a running trace
- **WHEN** the segmented control is set to `Effects & waits`, the trace is running, and a poll tick merges new events including an `effect` event and a `log` event
- **THEN** the newly arrived `effect` event is displayed and the newly arrived `log` event is hidden, without the user needing to re-select the filter

### Requirement: Filtered Empty State Messages

The `Timeline` component SHALL display context-appropriate empty-state messages based on the active filter combination when no events match.

Filtered empty-state messages SHALL only be evaluated after the existing loading and initial-error precedence branches. When the component is in a loading state (`isLoading && events.length === 0`) or an error state (`error && events.length === 0`), those states SHALL take precedence over any filter-derived empty-state message.

The messages SHALL be:
- `All` with errors off: `No timeline events recorded for this trace yet.`
- `Errors only` with segmented `All`: `No error events for this trace.`
- `Semantic` with errors off: `No semantic events for this trace.`
- `Effects & waits` with errors off: `No effect or wait events for this trace.`
- `Semantic` plus `Errors only`: `No error-level semantic events for this trace.`
- `Effects & waits` plus `Errors only`: `No error-level effect or wait events for this trace.`

#### Scenario: Semantic filter with no matching events
- **WHEN** the segmented control is set to `Semantic` and no explicit `state_change`, `decision`, `effect`, or `wait` events exist for the trace
- **THEN** the empty state displays `No semantic events for this trace.`

#### Scenario: Effects & waits plus errors only with no matching events
- **WHEN** the segmented control is set to `Effects & waits` and `Errors only` is active and no error-level effect or wait events exist
- **THEN** the empty state displays `No error-level effect or wait events for this trace.`

#### Scenario: Base empty state unchanged
- **WHEN** the segmented control is `All` and `Errors only` is off and no events exist
- **THEN** the empty state displays `No timeline events recorded for this trace yet.`
