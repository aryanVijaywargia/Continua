# Phase 4 Spec 2: Timeline UI

## Implemented Surface

Files:

- `web/src/api/client.ts`
- `web/src/components/Timeline.tsx`
- `web/src/components/JsonViewer.tsx`
- `web/src/components/SpanDetail.tsx`
- `web/src/pages/TraceDetailPage.tsx`
- `web/src/utils/timeline.ts`

## Client Integration

Added manual client types:

- `TimelineEvent`
- `TimelineResponse`
- `TimelineTraceStatus`
- `fetchTimelineEvents(traceId, options)`

Also aligned the existing session client type with the contract by adding `external_id`.

## UI Behavior

Implemented timeline section on the trace detail page with:

- chronological event list
- explicit vs synthetic visual distinction
- error/failure highlighting
- expandable payload and message inspection
- span navigation back into the span tree selection state
- empty, loading, and error states

The trace detail page now renders:

- top header with trace summary
- span tree panel
- span detail panel
- timeline panel beneath the inspection panels

## Shared UI Reuse

To avoid duplicating payload rendering logic:

- extracted `JsonViewer` from `SpanDetail`
- reused the same component in timeline event detail expansion

## Verification

Command run:

- `make type-check`

Observed result:

- workspace TypeScript type-check passed
