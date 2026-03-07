# Phase 4 Spec 3: Long Polling

## Implemented Polling Model

Files:

- `web/src/pages/TraceDetailPage.tsx`
- `web/src/utils/timeline.ts`

Behavior implemented:

- initial timeline load fetches the full timeline by paging through `GET /api/traces/{id}/events`
- active traces poll every 3 seconds with TanStack Query `refetchInterval`
- polling advances with `poll_cursor`, which always represents the last event included in the previous response
- new polling results are merged into the existing timeline with event-ID deduplication
- merged client-side results are display-sorted by event timestamp, source, explicit sequence, synthetic phase, and ID
- when polling reports a terminal trace status, the UI performs one final cursorless full refresh and then stops polling

## Cursor Handling

The response now exposes:

- `next_cursor` for continuing multi-page bootstrap pagination when `has_more=true`
- `poll_cursor` for incremental polling from the tail of the already-loaded timeline

- no duplicate timeline rows are rendered
- polling advances incrementally from the current tail instead of refetching the head or last full page
- final terminal refresh restores full ordering from a cursorless snapshot

## Live Indicator

The timeline header shows:

- `LIVE / polling every 3s` while active polling is enabled
- terminal status text once polling stops

## Verification

Command run:

- `make type-check`

Observed result:

- polling state, timeline merge helpers, and page integration all type-checked successfully
