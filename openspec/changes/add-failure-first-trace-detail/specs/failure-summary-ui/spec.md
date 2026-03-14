## ADDED Requirements

### Requirement: Failure Summary Display

The system SHALL render a failure summary section above Trace Context when the effective trace status is `FAILED` (where effective status is `timeline.traceStatus ?? trace.status`, consistent with the existing page pattern). The summary SHALL display:

- Primary failed span name and kind, when a primary failed span exists
- Short error preview, when available
- Failure timestamp, when available
- Failed span count
- Error event count
- Breadcrumb path to the primary failed span, when available
- A jump action button that selects the primary failed span

When no primary failed span exists, the summary SHALL display a generic message indicating the trace failed but no failed span was identified, and SHALL omit the jump action.

The summary SHALL NOT render when the effective trace status is not `FAILED`.

#### Scenario: Failed trace with primary failed span

- **WHEN** the effective trace status is `FAILED` and a primary failed span exists
- **THEN** the failure summary renders above Trace Context with all available failure details and a jump action

#### Scenario: Failed trace with no failed spans

- **WHEN** the effective trace status is `FAILED` but no spans have `status === 'FAILED'`
- **THEN** the failure summary renders a generic failure message without a jump action

#### Scenario: Non-failed trace

- **WHEN** the effective trace status is `RUNNING` or `COMPLETED`
- **THEN** no failure summary is rendered

### Requirement: Failure Summary Jump Action

The jump action SHALL select the primary failed span in the page-level selection state when activated. It SHALL be operable via keyboard (focusable, activatable with Enter/Space) and SHALL have an explicit accessible name.

#### Scenario: User activates jump action

- **WHEN** the user clicks or keyboard-activates the jump action
- **THEN** the primary failed span becomes the selected span in the tree, detail panel, and timeline

### Requirement: Auto-Selection of Primary Failed Span

The page SHALL auto-select the primary failed span when:

1. The trace first loads with an effective status of `FAILED` and no user selection exists
2. The effective trace status transitions from `RUNNING` to `FAILED` during polling and the user has not manually selected a span

Auto-selection depends on refreshed spans data. When the effective trace status transitions to `FAILED`, the page SHALL refetch trace and spans data so that failure analysis operates on current state (see Terminal Refresh of Trace and Spans Data requirement).

User selection SHALL be sticky: once the user manually selects any span, auto-selection SHALL NOT override their choice until the page is reloaded or the user navigates to a different trace.

If the selected span disappears from refreshed data, the system SHALL fall back to the primary failed span if one exists, otherwise clear the selection.

#### Scenario: Initial load of failed trace

- **WHEN** a failed trace loads for the first time
- **THEN** the primary failed span is auto-selected

#### Scenario: Running trace transitions to failed

- **WHEN** a running trace transitions to failed during polling and the user has not selected a span
- **THEN** the primary failed span is auto-selected

#### Scenario: User has manually selected a span

- **WHEN** the user has clicked a span and the trace transitions to failed
- **THEN** the user's selection is preserved; auto-selection does not fire

#### Scenario: Selected span removed during refresh

- **WHEN** the currently selected span disappears from the spans array after a data refresh
- **THEN** selection falls back to the primary failed span, or clears if none exists

### Requirement: Span Tree Failure Highlighting

All failed span rows in the span tree SHALL be visually marked as failed using indicators that do not rely on color alone (text labels, badges, or structural cues).

The ancestor branch leading to the primary failed span SHALL be highlighted distinctly from other failed rows.

A `revealPath` signal SHALL cause nodes on the selected span's ancestor path to auto-expand if they are collapsed. Tree expansion state SHALL remain local to individual nodes.

#### Scenario: Failed span row appearance

- **WHEN** a span has `status === 'FAILED'`
- **THEN** its row in the span tree shows a failure indicator visible without relying on color alone

#### Scenario: Primary ancestor branch highlighting

- **WHEN** a primary failed span exists
- **THEN** all ancestor nodes from root to the primary failed span are highlighted distinctly

#### Scenario: Reveal path auto-expansion on auto-selection

- **WHEN** the primary failed span is auto-selected and an ancestor node is collapsed
- **THEN** that ancestor node auto-expands to reveal the path

#### Scenario: Reveal path auto-expansion on manual navigation

- **WHEN** a span is selected via the failure summary jump action, a breadcrumb click, or a timeline span click, and an ancestor node of that span is collapsed
- **THEN** that ancestor node auto-expands to reveal the path to the newly selected span

### Requirement: Span Tree Row Style Precedence

When a span tree row has multiple visual states (selected, failed, on primary ancestor path), the following precedence SHALL apply:

1. Selected state takes highest visual precedence
2. Primary ancestor path highlighting takes second precedence
3. Failed row highlighting takes third precedence

The combined styling SHALL ensure that the active state is always visually distinguishable regardless of overlapping states.

#### Scenario: Selected failed span on ancestor path

- **WHEN** the primary failed span itself is selected
- **THEN** the row shows selected styling, and the failed/ancestor states do not visually conflict

#### Scenario: Unselected failed ancestor

- **WHEN** a span is both failed and on the primary ancestor path but is not selected
- **THEN** the row shows ancestor path highlighting combined with the failed indicator

### Requirement: Inline Error Previews in Span Tree

Failed span rows SHALL display an inline error preview beneath or beside the span name. The preview SHALL use the error preview extraction logic from the failure analysis module.

#### Scenario: Failed span with error preview

- **WHEN** a failed span has an extractable error preview
- **THEN** the preview text appears on the span tree row

#### Scenario: Failed span without error preview

- **WHEN** a failed span has no extractable error information
- **THEN** no inline preview text is shown on that row

### Requirement: Terminal Refresh of Trace and Spans Data

When the effective trace status transitions from `RUNNING` to a terminal state (`COMPLETED` or `FAILED`), the page SHALL refetch the trace and spans queries so that failure analysis, failure summary, span tree highlighting, and header metrics all operate on current data rather than the initial one-shot fetch.

This refetch SHALL be triggered by observing the terminal transition from the timeline polling hook, which already detects non-`RUNNING` status.

#### Scenario: Running trace transitions to failed

- **WHEN** the timeline hook detects that `trace_status` has changed from `RUNNING` to `FAILED`
- **THEN** the `traceQuery` and `spansQuery` are invalidated and refetched
- **AND** the failure summary, auto-selection, and header metrics reflect the refreshed data

#### Scenario: Running trace completes normally

- **WHEN** the timeline hook detects that `trace_status` has changed from `RUNNING` to `COMPLETED`
- **THEN** the `traceQuery` and `spansQuery` are invalidated and refetched so header duration and metrics are current

### Requirement: Cross-Trace State Reset

When the user navigates from one trace to another (the `traceId` route parameter changes), all Phase 7 local state SHALL reset to initial values:

- `selectedSpanExternalId` resets to `null`
- `userHasSelected` resets to `false`
- The `Errors only` timeline filter resets to inactive
- Any reveal-path signal clears

This ensures no stale selection, filter, or highlight state leaks from a previously viewed trace into a newly navigated trace.

#### Scenario: Navigate from trace A to trace B

- **WHEN** the user navigates from `/traces/A` to `/traces/B`
- **THEN** selection state, user-has-selected flag, error filter, and reveal path are all reset
- **AND** auto-selection logic runs fresh against trace B's data

#### Scenario: Navigate back to same trace

- **WHEN** the user navigates away from trace A and later returns to it
- **THEN** state resets as if loading trace A for the first time
