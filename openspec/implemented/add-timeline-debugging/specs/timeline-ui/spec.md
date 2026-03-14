## ADDED Requirements

### Requirement: Timeline View in Trace Detail

The trace detail page SHALL include a timeline view showing events in strict chronological order.

#### Scenario: Timeline tab or section visible
- **WHEN** a user navigates to a trace detail page
- **THEN** a timeline view is accessible (as a tab, section, or panel)

#### Scenario: Events displayed chronologically
- **WHEN** the timeline view is active
- **THEN** events are displayed in ascending chronological order

#### Scenario: Empty timeline
- **WHEN** a trace has no explicit events and no spans
- **THEN** the timeline shows an empty state message

### Requirement: Visual Distinction of Event Types

The timeline UI SHALL visually distinguish between explicit events, synthetic lifecycle markers, and error events.

#### Scenario: Explicit events styled distinctly
- **WHEN** an explicit event (log, message, metric, custom) is rendered
- **THEN** it is visually distinguishable from synthetic lifecycle markers

#### Scenario: Synthetic lifecycle markers styled distinctly
- **WHEN** a synthetic event (span_started, span_completed, span_failed) is rendered
- **THEN** it is visually distinguishable from explicit events

#### Scenario: Error and failure events highlighted
- **WHEN** an event has type `error`, `exception`, or `span_failed`
- **THEN** it is visually highlighted as an error (e.g., red styling)

### Requirement: Payload Inspection

The timeline UI SHALL support expanding event details and payloads.

#### Scenario: Expandable event details
- **WHEN** an event has a payload or message
- **THEN** the user can expand the event to see full details

#### Scenario: Payload collapse
- **WHEN** an expanded event is clicked again
- **THEN** the details collapse

### Requirement: Span Navigation from Timeline

The timeline UI SHALL support navigating to the related span from a timeline event.

#### Scenario: Click span reference
- **WHEN** a timeline event references a span (via span_id/span_name)
- **THEN** clicking the span reference navigates to or highlights that span in the span tree view

### Requirement: Timeline API Client Integration

The web API client SHALL include types and fetch functions for the timeline endpoint.

#### Scenario: Timeline types defined
- **WHEN** the timeline feature is used
- **THEN** `TimelineEvent` and `TimelineResponse` types are defined in `web/src/api/client.ts`

#### Scenario: Timeline fetch function available
- **WHEN** the timeline data is needed
- **THEN** a `fetchTimelineEvents(traceId, options)` function is available that calls `GET /api/traces/{id}/events`
