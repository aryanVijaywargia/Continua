## ADDED Requirements

### Requirement: Primary Failed Span Selection

The system SHALL deterministically identify a primary failed span from the trace's span set using the following algorithm:

1. Candidate set: all spans where `status === 'FAILED'`
2. Sort candidates by `ended_at` ascending (spans without `ended_at` sort after those with it)
3. Break ties by `started_at` ascending
4. Break remaining ties by original spans array order
5. The first candidate after sorting is the primary failed span

If the candidate set is empty, the system SHALL report that no primary failed span exists.

#### Scenario: Single failed span

- **WHEN** a trace has exactly one span with `status === 'FAILED'`
- **THEN** that span is identified as the primary failed span

#### Scenario: Multiple failed spans with different end times

- **WHEN** a trace has multiple failed spans with distinct `ended_at` values
- **THEN** the span with the earliest `ended_at` is the primary failed span

#### Scenario: Failed spans with missing ended_at

- **WHEN** some failed spans have `ended_at` and others do not
- **THEN** spans with `ended_at` sort before those without

#### Scenario: Failed trace with no failed spans

- **WHEN** a trace has `status === 'FAILED'` but no spans have `status === 'FAILED'`
- **THEN** the system reports no primary failed span exists

### Requirement: Cycle-Safe Parent Chain Traversal

The system SHALL traverse span parent chains using a visited-set guard to prevent infinite loops. If a cycle or broken parent reference is encountered, traversal SHALL stop and return the longest valid path found. The system SHALL NOT throw exceptions during traversal.

#### Scenario: Normal parent chain

- **WHEN** a span has a valid parent chain to the root
- **THEN** the full path from root to span is returned

#### Scenario: Cyclic parent reference

- **WHEN** a span's parent chain contains a cycle
- **THEN** traversal stops at the cycle boundary and returns the path up to that point

#### Scenario: Broken parent reference

- **WHEN** a span references a `parent_span_id` that does not exist in the span set
- **THEN** the path starts from that span (treated as a root) without throwing

### Requirement: Inline Error Preview Extraction

The system SHALL extract an inline error preview for failed spans using the following priority:

1. `span.error_message` if present and non-empty
2. The `message` field of the first timeline event matching `event_type` of `error` or `exception` for that span
3. No preview (null)

Preview formatting: trim whitespace, take the first non-empty line, truncate to 120 characters with ellipsis if needed.

Note: The preview predicate is intentionally narrower than the error-only timeline filter. It targets only `error` and `exception` event types (not `span_failed` or `level === 'error'`) because `span_failed` synthetic events produce redundant messages like "X failed" (already conveyed by the status badge), and generic `level === 'error'` log events are not span-specific error messages suitable for inline preview.

#### Scenario: Span with error_message

- **WHEN** a failed span has a non-empty `error_message`
- **THEN** the preview is derived from `error_message`

#### Scenario: Span with error timeline event but no error_message

- **WHEN** a failed span has no `error_message` but has an `error` type timeline event with a message
- **THEN** the preview is derived from that event's message

#### Scenario: Span with no error information

- **WHEN** a failed span has no `error_message` and no matching timeline events
- **THEN** no preview is shown

#### Scenario: Long error message truncation

- **WHEN** the extracted preview text exceeds 120 characters after taking the first line
- **THEN** the text is truncated to 120 characters with an ellipsis appended

### Requirement: Breadcrumb Path Generation

The system SHALL generate a breadcrumb path from the root span to a given span using the span index and parent chain traversal. Each segment SHALL contain the span's `name` and `span_id`. The path SHALL be ordered root-first, target-last.

#### Scenario: Nested span breadcrumb

- **WHEN** a span is three levels deep in the tree
- **THEN** the breadcrumb contains three segments: grandparent, parent, span

#### Scenario: Root span breadcrumb

- **WHEN** a span has no parent
- **THEN** the breadcrumb contains only that span

### Requirement: Failure Summary Computation

The system SHALL compute a failure summary object containing: primary failed span (if any), failed span count, error event count, error preview (if any), failure timestamp (primary failed span's `ended_at`), and breadcrumb path to primary failed span (if any).

Error event count SHALL use the same predicate as the error-only timeline filter (`isTimelineErrorEvent`): events with `event_type` of `error`, `exception`, or `span_failed`, or with `level === 'error'`.

#### Scenario: Failed trace with primary failed span

- **WHEN** a trace is failed and a primary failed span exists
- **THEN** the summary includes all fields populated from the primary failed span

#### Scenario: Failed trace with no failed spans

- **WHEN** a trace is failed but no spans are failed
- **THEN** the summary has null primary failed span, zero failed span count, and no breadcrumb path

#### Scenario: Error event count matches timeline filter predicate

- **WHEN** a trace has events of types `error`, `exception`, `span_failed`, and `level === 'error'`
- **THEN** the error event count includes all of them, consistent with the error-only timeline filter
