## Context

Phase 4 adds a unified debugging timeline to Continua. The timeline merges two data sources that already exist in the database:
- **Explicit events** in `span_events` (logs, errors, metrics emitted by SDK)
- **Span lifecycle data** in `spans` (start_time, end_time, status)

The challenge is presenting a paginated, incrementally-pollable merged view without introducing new storage or complex infrastructure. For running traces, late-arriving events and span time adjustments (via `LEAST`/`GREATEST` in upserts) mean the timeline is eventually consistent; for terminal traces, a final fetch produces an accurate snapshot.

### Current State

- `span_events` table: `id`, `project_id`, `trace_id`, `span_id`, `event_type`, `level`, `event_ts`, `server_ingested_at`, `sequence`, `message`, `payload`, `idempotency_key`
- `spans` table: `id`, `trace_id`, `span_id`, `name`, `type`, `status`, `start_time`, `end_time`, `duration_ms`
- Existing queries: `ListSpanEventsByTrace` (ordered by `COALESCE(event_ts, server_ingested_at) ASC, sequence NULLS LAST`), `ListSpansByTrace` (ordered by `COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST`)
- No existing timeline or events endpoint on the API
- Trace statuses: DB stores `running`/`ok`/`error`/`cancelled`; API maps to `RUNNING`/`COMPLETED`/`FAILED`
- Span statuses: DB stores `running`/`completed`/`failed`/`error`; API maps to `STARTED`/`COMPLETED`/`FAILED`/`SCHEDULED`
- Project scoping: `middleware.GetProjectID(r.Context())` extracts project from auth; handlers verify ownership

## Goals / Non-Goals

**Goals:**
- Unified chronological timeline for a trace combining explicit events and synthetic lifecycle markers
- Stable cursor pagination over the merged timeline (monotonic for running traces, accurate for terminal traces)
- Incremental polling for running traces that incorporates new events without duplicates while preserving display order
- Improved Python SDK event helpers for common patterns (error, exception, metric)
- Cross-layer type alignment

**Non-Goals:**
- New storage tables or materialized views
- Trace-level events (span_id=null)
- WebSocket/SSE infrastructure
- Frontend generated-types migration
- TypeScript SDK changes

## Decisions

### Decision 1: Timeline Merge Strategy

**What:** Merge explicit events and synthetic lifecycle markers in the Go handler, not in SQL.

**Why:** Both sources use different schemas and orderings. A SQL UNION would require complex type coercion and wouldn't support synthetic event generation from span state. Go-side merge is simpler, testable, and consistent with existing handler patterns.

**Implementation:**
1. Fetch all explicit events for the trace via `ListSpanEventsByTrace`
2. Fetch all spans for the trace via `ListSpansByTrace`
3. Generate synthetic events from spans (span_started, span_completed, span_failed)
4. Merge both lists using a total ordering (see Decision 2 for sort keys)
5. Apply cursor/limit pagination on the merged result

**Display vs cursor ordering:** Events are displayed by `COALESCE(event_ts, server_ingested_at)` for explicit events and `start_time`/`end_time` for synthetic events. Cursor ordering uses a monotonic key (`created_at` for explicit events, `created_at` of the span for synthetic events) to avoid cursor drift from late-arriving or time-shifted events.

### Decision 2: Opaque Cursor Design

**What:** Cursor encodes position in the merged timeline as a base64-encoded JSON object.

**Format:** `base64({"cts":"RFC3339Nano","src":"explicit|synthetic","id":"uuid-or-span_id:type"})`

Where `cts` is the monotonic cursor timestamp (`created_at` for explicit events, span `created_at` for synthetic events), not the display timestamp.

**Why:** A single opaque string that encodes a monotonic timestamp, source type, and unique identifier provides:
- Stable ordering that does not shift when span times are adjusted by upserts
- No duplicates (position is recoverable via monotonic key + source + ID)
- Forward-compatible (can add fields without breaking clients)

**Tie-break resolution:** The cursor payload intentionally omits secondary sort keys (`sequence` for explicit events, lifecycle phase for synthetic events). When multiple events share the same `cts`, the server resolves the exact cursor position by looking up the referenced event/span row by `id` and comparing against the full total ordering. This keeps the cursor compact and avoids encoding mutable or schema-coupled fields.

**Total ordering for cursor comparison:**
1. `cts` (monotonic cursor timestamp) ascending
2. `src`: `explicit` before `synthetic` (explicit events at the same cursor time sort first)
3. For explicit events: `sequence` (preserving ingest order), then `id` (UUID)
4. For synthetic events: lifecycle phase (`span_started` before `span_completed`/`span_failed`), then `span_id`

**Consistency model:**
- For **running traces**, the cursor guarantees no duplicates but may miss late-arriving events whose monotonic timestamp sorts before the cursor. Clients should do a full refresh when the trace becomes terminal.
- For **terminal traces**, a single fetch produces the complete, accurate timeline.

**Alternatives considered:**
- Offset-based: Would break with concurrent inserts during polling
- Timestamp-only: Would duplicate events with identical timestamps
- Event ID: Would not work across explicit + synthetic sources

### Decision 3: Synthetic Lifecycle Rules

**What:** For each span, generate up to 2 synthetic events:
- `span_started`: when `start_time` is non-null
- `span_completed`: when `end_time` is non-null AND status is `completed`
- `span_failed`: when `end_time` is non-null AND status is `failed`/`error`

Synthetic event IDs use format `{span_id}:span_started` or `{span_id}:span_completed` for deterministic identification.

**Why:** Matches existing `mapSpanStatus` conventions in `mapper.go` (which handles `running`, `completed`, `failed`, `error` — not `ok`, which is a trace-only status). Using end_time presence + status avoids showing completion events for still-running spans.

### Decision 4: Long Polling Strategy

**What:** Frontend polls the timeline endpoint with `after` cursor every 3 seconds while trace status is `RUNNING`. The API returns a durable `poll_cursor` for the last event included in each response, so polling can continue incrementally even when `has_more=false`. Stops when response `trace_status` is terminal.

**Why:** Simple, uses TanStack Query's `refetchInterval` capability. No new infrastructure needed. When the trace becomes terminal, the client does one final cursorless fetch to get the complete, accurately-ordered timeline.

### Decision 5: Start with Existing SQLC Queries

**What:** Start by reusing existing `ListSpanEventsByTrace` and `ListSpansByTrace` queries. Timeline merge and pagination happen in Go. Add filtered SQLC queries later if correctness or performance requires them.

**Why:** The merge logic is application-level (generating synthetic events, applying cursor pagination over mixed sources). Adding a SQL-level timeline query would be fragile and harder to maintain. For typical trace sizes (tens to hundreds of spans/events), fetching all and merging in memory is efficient.

**Trade-off:** For very large traces (>10K spans), this fetches everything before trimming. This does not bound memory despite cursor pagination — it bounds the response size only. Acceptable for Phase 4; add SQL-level `WHERE created_at > $cursor` filtering if profiling shows this is a bottleneck.

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| In-memory merge for large traces | Slow response for traces with >10K spans | Default limit of 100 events per page; documented as known limitation |
| Synthetic events change when spans update | Polling may see "new" synthetic events if span status changes | This is correct behavior — the timeline reflects current state |
| Clock skew between client and server timestamps | Events may appear out of chronological order | Use `COALESCE(event_ts, server_ingested_at)` as already done in existing queries |

## Open Questions

None — all design decisions are resolved based on existing codebase patterns.

## Appendix: Event Type Enum Separation

Two distinct enums exist:
- **`IngestEventType`** (ingest contract): `log`, `error`, `exception`, `message`, `metric`, `custom` — these are the only types SDKs can emit
- **`TimelineEventType`** (timeline response): all of the above plus `span_started`, `span_completed`, `span_failed` — synthetic types generated server-side

These MUST NOT be conflated. The ingest contract does not accept synthetic types; the timeline response includes both.
