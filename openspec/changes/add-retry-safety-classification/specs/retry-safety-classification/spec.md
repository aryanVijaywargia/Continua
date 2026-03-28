## ADDED Requirements

### Requirement: Event-Level Retry Safety Classification

The system SHALL provide a pure function that classifies a single timeline event into a `RetrySafetyAssessment` containing a `classification` (`retryable | unsafe | unknown`) and a `reason` code, or returns `null` if the event is not an `effect` event.

The function SHALL return `null` without classification when the input event has an `event_type` other than `effect`. Only events with `event_type === 'effect'` SHALL proceed to classification.

The function SHALL use `getEffectDetails()` from `eventSemantics.ts` for semantic validation and SHALL NOT independently parse effect payloads.

Classification rules:
- `has_external_side_effect` is `false` → `retryable` / `read_only_effect`
- `has_external_side_effect` is `true` and `idempotent` is `false` → `unsafe` / `mutating_non_idempotent`
- `has_external_side_effect` is `true` and `idempotent` is `true` and `idempotency_key` is a non-empty string → `retryable` / `mutating_idempotent_with_key`
- `has_external_side_effect` is `true` and `idempotent` is `undefined` → `unknown` / `mutating_missing_idempotent`
- `has_external_side_effect` is `true` and `idempotent` is `true` and `idempotency_key` is `undefined` → `unknown` / `mutating_idempotent_missing_key`
- `event_type` is `effect` but `getEffectDetails()` returns `null` → `unknown` / `malformed_effect_payload`

The assessment SHALL carry optional evidence fields: `decisiveEventId`, `effectKind`, `hasExternalSideEffect`, `idempotent`, and `idempotencyKey`.

#### Scenario: Read-only effect classified as retryable
- **WHEN** a timeline event has `event_type: "effect"` and `getEffectDetails()` returns `{ effectKind: "model_call", hasExternalSideEffect: false }`
- **THEN** the classification is `retryable` with reason `read_only_effect`

#### Scenario: Mutating non-idempotent effect classified as unsafe
- **WHEN** a timeline event has `event_type: "effect"` and `getEffectDetails()` returns `{ effectKind: "api_call", hasExternalSideEffect: true, idempotent: false }`
- **THEN** the classification is `unsafe` with reason `mutating_non_idempotent`

#### Scenario: Mutating idempotent effect with key classified as retryable
- **WHEN** a timeline event has `event_type: "effect"` and `getEffectDetails()` returns `{ effectKind: "api_call", hasExternalSideEffect: true, idempotent: true, idempotencyKey: "key-1" }`
- **THEN** the classification is `retryable` with reason `mutating_idempotent_with_key`

#### Scenario: Mutating effect with missing idempotent flag classified as unknown
- **WHEN** a timeline event has `event_type: "effect"` and `getEffectDetails()` returns `{ effectKind: "tool_call", hasExternalSideEffect: true }` with `idempotent` undefined
- **THEN** the classification is `unknown` with reason `mutating_missing_idempotent`

#### Scenario: Mutating idempotent effect with missing key classified as unknown
- **WHEN** a timeline event has `event_type: "effect"` and `getEffectDetails()` returns `{ effectKind: "tool_call", hasExternalSideEffect: true, idempotent: true }` with `idempotencyKey` undefined
- **THEN** the classification is `unknown` with reason `mutating_idempotent_missing_key`

#### Scenario: Malformed effect payload classified as unknown
- **WHEN** a timeline event has `event_type: "effect"` but `getEffectDetails()` returns `null`
- **THEN** the classification is `unknown` with reason `malformed_effect_payload`

#### Scenario: Non-effect event returns null
- **WHEN** a timeline event has `event_type: "log"` or any type other than `effect`
- **THEN** the function returns `null` without producing an assessment

### Requirement: Span-Level Retry Safety Aggregation

The system SHALL provide a function that aggregates event-level classifications into a single `RetrySafetyAssessment` for a failed span.

The function SHALL evaluate only explicit `effect` events attached to that span. Only spans with `status === 'FAILED'` SHALL be eligible for assessment.

Aggregation precedence SHALL be `unsafe > unknown > retryable`. Within the winning classification class, the function SHALL select the latest matching event (by timeline ordering) as evidence.

If a failed span has no `effect` events, the assessment SHALL be `unknown` with reason `no_effect_events`.

The assessment SHALL carry `decisiveSpanId`, `decisiveSpanName`, and `decisiveEventId` identifying the evidence.

#### Scenario: Single retryable effect on failed span
- **WHEN** a failed span has one effect event classified as `retryable`
- **THEN** the span assessment is `retryable` with the event as evidence

#### Scenario: Mixed effects with unsafe taking precedence
- **WHEN** a failed span has one `retryable` effect event and one `unsafe` effect event
- **THEN** the span assessment is `unsafe` with the unsafe event as evidence

#### Scenario: Mixed effects with unknown taking precedence over retryable
- **WHEN** a failed span has one `retryable` effect event and one `unknown` effect event
- **THEN** the span assessment is `unknown` with the unknown event as evidence

#### Scenario: Failed span with no effect events
- **WHEN** a failed span has no effect events
- **THEN** the span assessment is `unknown` with reason `no_effect_events`

#### Scenario: Non-failed span excluded from assessment
- **WHEN** a span has `status` of `COMPLETED` and has effect events
- **THEN** the span is not eligible for retry-safety assessment

#### Scenario: Latest event wins within same classification
- **WHEN** a failed span has two `unsafe` effect events at different timestamps
- **THEN** the span assessment selects the later event as decisive evidence

### Requirement: Trace-Level Retry Safety Aggregation

The system SHALL provide a function that aggregates span-level assessments into a single `RetrySafetyAssessment` for a failed trace.

The function SHALL aggregate across all failed spans in the trace, not just the primary failed span.

Aggregation precedence SHALL be `unsafe > unknown > retryable`. The assessment SHALL carry the `decisiveSpanId` and `decisiveSpanName` of the span whose assessment determined the trace-level result.

If the trace is failed but contains no assessable failed spans, the assessment SHALL be `unknown`.

#### Scenario: Single failed span determines trace assessment
- **WHEN** a failed trace has one failed span assessed as `retryable`
- **THEN** the trace assessment is `retryable` carrying that span as decisive

#### Scenario: Multiple failed spans with unsafe taking precedence
- **WHEN** a failed trace has one failed span assessed as `retryable` and another assessed as `unsafe`
- **THEN** the trace assessment is `unsafe` carrying the unsafe span as decisive

#### Scenario: Decisive span differs from primary failed span
- **WHEN** a failed trace's primary failed span is assessed as `retryable` but a secondary failed span is assessed as `unsafe`
- **THEN** the trace assessment is `unsafe` and the decisive span is the secondary failed span, not the primary

#### Scenario: Failed trace with no assessable failed spans
- **WHEN** a failed trace has no failed spans (e.g., trace-level failure without span failures)
- **THEN** the trace assessment is `unknown`

### Requirement: Poll-Stable Retry Safety Analysis

The system SHALL provide a page-level hook (recommended name `useRetrySafetyAnalysis`) that computes retry-safety analysis from spans and timeline events.

The hook SHALL produce stable output — identical classifications, reasons, and decisive span/event identifiers — when timeline polling adds events that do not change the set of failed spans or the effect events attached to them.

The hook SHALL produce updated output when the set of failed spans changes or when effect events on failed spans are added, removed, or modified.

#### Scenario: Non-effect poll update produces stable results
- **WHEN** a timeline poll adds a new `log` event but no new effect events and no span status changes
- **THEN** the retry-safety analysis returns the same classifications, reasons, and decisive span/event identifiers as the previous result

#### Scenario: New effect event triggers recomputation
- **WHEN** a timeline poll adds a new `effect` event on a failed span
- **THEN** the retry-safety analysis result is recomputed with the new event included

#### Scenario: Span status change triggers recomputation
- **WHEN** a timeline poll changes a span from `STARTED` to `FAILED`
- **THEN** the retry-safety analysis result is recomputed to include the newly failed span

### Requirement: Retry Safety Badge Component

The system SHALL provide a `RetrySafetyBadge` component in `web/src/components/RetrySafetyBadge.tsx`.

The badge SHALL accept `classification` (`retryable | unsafe | unknown`) and `variant` (`compact | full`) props, plus an optional `aria-label` string.

Visible labels SHALL be single words: `Retryable`, `Unsafe`, `Unknown`. The badge SHALL NOT contain long-form evidence text.

The badge SHALL use Tailwind semantic color classes with dark-mode variants. No hardcoded hex color values.

#### Scenario: Retryable badge renders with appropriate styling
- **WHEN** `RetrySafetyBadge` is rendered with `classification="retryable"` and `variant="compact"`
- **THEN** it displays "Retryable" with a green-family Tailwind color class and dark-mode variant

#### Scenario: Unsafe badge renders with appropriate styling
- **WHEN** `RetrySafetyBadge` is rendered with `classification="unsafe"` and `variant="full"`
- **THEN** it displays "Unsafe" with a red-family Tailwind color class and dark-mode variant

#### Scenario: Unknown badge renders with appropriate styling
- **WHEN** `RetrySafetyBadge` is rendered with `classification="unknown"`
- **THEN** it displays "Unknown" with a yellow/amber-family Tailwind color class and dark-mode variant

#### Scenario: Badge supports accessibility label
- **WHEN** `RetrySafetyBadge` is rendered with an `aria-label` of "Retry safety advisory: retryable. Inferred from effect metadata."
- **THEN** the badge element has the corresponding `aria-label` attribute

### Requirement: Retry Safety Explanation Text

The system SHALL provide a mapping from each `RetrySafetyReason` to fixed human-readable explanation text:

- `read_only_effect`: "Retry would repeat a read-only effect with no recorded external mutation."
- `mutating_non_idempotent`: "Recorded effect mutates external state and is explicitly non-idempotent."
- `mutating_idempotent_with_key`: "Recorded effect mutates external state but is marked idempotent and includes an idempotency key."
- `no_effect_events`: "No effect events were recorded for this failed span."
- `malformed_effect_payload`: "An effect event was recorded, but its retry-safety fields were malformed or incomplete."
- `mutating_missing_idempotent`: "An effect may mutate external state, but no idempotency flag was recorded."
- `mutating_idempotent_missing_key`: "An effect is marked idempotent, but no idempotency key was recorded."

The system SHALL provide a shared accessible summary template: `"Retry safety advisory: {classification}. Inferred from effect metadata."`

#### Scenario: Each reason code maps to explanation text
- **WHEN** the explanation helper is called with any valid `RetrySafetyReason`
- **THEN** it returns the corresponding fixed explanation string

#### Scenario: Accessible summary uses classification
- **WHEN** the accessible summary helper is called with classification `unsafe`
- **THEN** it returns "Retry safety advisory: unsafe. Inferred from effect metadata."

### Requirement: Failure Summary Retry Safety Surface

The `FailureSummary` component SHALL display a `Trace retry safety` badge/stat showing the aggregated trace-level retry-safety assessment when the trace is failed.

The badge SHALL NOT be attached to the primary failed span title, because the decisive span may differ.

The surface SHALL show advisory copy plus a reason-specific explanation string.

When the decisive failed span differs from the primary failed span, the surface SHALL state this explicitly and provide a secondary action to navigate to the decisive span, reusing the existing span-navigation callback (`onJumpToPrimaryFailedSpan`).

#### Scenario: Trace-level retryable assessment in failure summary
- **WHEN** the trace is failed and the aggregated assessment is `retryable` with reason `read_only_effect`
- **THEN** the failure summary shows a `Retryable` badge, the advisory text, and the read-only explanation

#### Scenario: Decisive span differs from primary span
- **WHEN** the trace assessment is `unsafe` and the decisive span is not the primary failed span
- **THEN** the failure summary states the decisive span name and provides an action to navigate to it

#### Scenario: Non-failed trace shows no retry safety
- **WHEN** the trace status is `COMPLETED` or `RUNNING`
- **THEN** the failure summary does not render any retry-safety badge or section

### Requirement: Span Tree Retry Safety Badges

The `SpanTree` component SHALL display compact retry-safety badges on failed spans only.

Existing `Selected`, `Failure path`, and `Failed` chips SHALL remain unchanged.

Non-failed spans SHALL NOT display retry-safety badges regardless of their effect events.

#### Scenario: Failed span shows compact retry badge
- **WHEN** a span in the tree has `status === 'FAILED'` and a span-level assessment of `unsafe`
- **THEN** the span tree node displays a compact `Unsafe` badge alongside existing status indicators

#### Scenario: Completed span shows no retry badge
- **WHEN** a span in the tree has `status === 'COMPLETED'` and effect events exist
- **THEN** the span tree node does not display any retry-safety badge

### Requirement: Execution Waterfall Retry Safety Badges

The execution waterfall SHALL display compact retry-safety badges in the left metadata column for failed spans, never inside the timing bar.

The existing layout constraint SHALL be preserved: span name truncates first, badge stays single-line, no third text line is introduced.

#### Scenario: Failed span waterfall label shows retry badge
- **WHEN** a failed span is rendered in the execution waterfall with a span-level assessment
- **THEN** the compact retry-safety badge is rendered within the left label container, not inside the timing bar element

#### Scenario: Waterfall layout preserves truncation structure
- **WHEN** a failed span has a long name and a retry-safety badge
- **THEN** the span name container retains its truncation CSS class, the badge element has `whitespace-nowrap`, and both remain within the same label row container

### Requirement: Selected Span Detail Retry Safety Section

The `SpanDetail` component SHALL render a `Retry Safety` section for failed selected spans only.

The section SHALL display the retry-safety badge, advisory copy, reason-specific explanation, and supporting semantic fields (`effect_kind`, `has_external_side_effect`, `idempotent`, `idempotency_key`) when present in the assessment.

Non-failed selected spans SHALL NOT render the section.

#### Scenario: Failed span with read-only effect shows retryable detail
- **WHEN** the selected span has `status === 'FAILED'` and a span-level assessment of `retryable` with reason `read_only_effect` and `effectKind: "model_call"`
- **THEN** the detail section shows the `Retryable` badge, advisory text, read-only explanation, and `effectKind: model_call`

#### Scenario: Failed span with mutating non-idempotent effect shows unsafe detail
- **WHEN** the selected span has `status === 'FAILED'` and a span-level assessment of `unsafe` with reason `mutating_non_idempotent`
- **THEN** the detail section shows the `Unsafe` badge, advisory text, and non-idempotent explanation

#### Scenario: Failed span with incomplete semantics shows unknown detail
- **WHEN** the selected span has `status === 'FAILED'` and a span-level assessment of `unknown` with reason `mutating_missing_idempotent`
- **THEN** the detail section shows the `Unknown` badge, advisory text, and missing-idempotent explanation

#### Scenario: Completed span shows no retry safety section
- **WHEN** the selected span has `status === 'COMPLETED'`
- **THEN** no `Retry Safety` section is rendered

### Requirement: Timeline Effect Row Retry Safety Badges

On failed traces only, `effect` timeline rows SHALL render a retry-safety badge alongside existing content. The `Timeline` component SHALL classify effect rows locally using the shared `classifyEffectEvent()` utility and its existing `traceStatus` prop to gate badge rendering — no page-threaded event assessment props are required.

Well-formed effect rows SHALL keep the current effect preview plus the new badge.

Malformed effect rows SHALL keep generic summary text and display an `Unknown` badge. The malformed-payload explanation SHALL NOT appear inline in the collapsed row; it is accessible only via the expanded payload panel.

Non-effect rows SHALL remain unchanged. Non-failed traces SHALL NOT render retry-safety badges on any rows.

#### Scenario: Well-formed retryable effect on failed trace shows badge
- **WHEN** a failed trace's timeline contains an effect row with `getEffectDetails()` returning `{ effectKind: "model_call", hasExternalSideEffect: false }`
- **THEN** the row renders the existing `EffectPreview` content plus a `Retryable` badge

#### Scenario: Malformed effect on failed trace shows unknown badge
- **WHEN** a failed trace's timeline contains an effect row where `getEffectDetails()` returns `null`
- **THEN** the row renders generic summary text and an `Unknown` badge

#### Scenario: Effect row on completed trace shows no badge
- **WHEN** a completed trace's timeline contains an effect row
- **THEN** the row renders the `EffectPreview` content with no retry-safety badge

#### Scenario: Non-effect row on failed trace unchanged
- **WHEN** a failed trace's timeline contains a `log` or `state_change` row
- **THEN** the row renders identically to its current behavior with no retry-safety badge
