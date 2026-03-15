# WebSocket schema status

## What exists
- `contracts/websocket/events.ts` defines Zod schemas for server events and client messages
- `contracts/websocket/events.schema.json` is generated from those schemas

## What does not exist
- a live `internal/ws` runtime
- mounted WebSocket routes in the platform server
- frontend subscription hooks using those schemas in production code

## Current debugger behavior instead
- trace detail uses `GET /api/traces/{id}/events`
- `web/src/pages/useTraceTimeline.ts` bootstraps the full timeline and then polls every 3 seconds for running traces
- timeline rows combine:
  - explicit `span_events`
  - synthetic span lifecycle events

## How to treat WebSocket work
- The contract file is useful as schema history, not proof of a working runtime.
- If a task is to add real-time push behavior, treat it as new capability work.
- Verify whether the existing `events.ts` schema still matches the desired product semantics before reusing it as-is.
