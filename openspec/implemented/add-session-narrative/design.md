## Context

The session detail page (`/sessions/:id`) shows a paginated trace table but provides no session-level summary or trace-to-trace storyline. This change adds a single read-only endpoint (`GET /api/sessions/{id}/narrative`) that assembles a capped narrative view of the session and a frontend surface that renders it above the existing table.

Key constraints:
- No new migrations (reads existing schema)
- No new routes or session lifecycle model
- Reuses existing `TimelineEvent` schema for semantic events
- Hard cap of 100 traces per narrative response (server constant, not query param)
- Explicit lineage metadata is required, but isolated enough to layer onto the core inference algorithm as a final slice

## Goals / Non-Goals

**Goals:**
- Provide session-level aggregate metrics (cost, tokens, timing, trace counts by status)
- Show chronological trace storyline with lineage relationships
- Surface semantic event snippets per trace for at-a-glance debugging
- Keep the narrative query fully independent from existing session/trace queries

**Non-Goals:**
- Session control behavior (pause, resume, cancel)
- New session status model or state machine
- Span projections in narrative response
- Per-trace semantic event counts (frontend derives if needed)
- WebSocket-driven narrative updates (polling only)
- Configurable trace limit (hardcoded at 100)

## Decisions

### Query Plan: Up To 3 Queries

The narrative endpoint uses up to 3 SQL queries. The successful `200` path uses a 3-query default shape. Query 1 is session-anchored so it can distinguish an existing zero-trace session from a missing/cross-project session without a fourth existence lookup, and the handler may return immediately after Query 1 for `404` cases.

**Query 1 — Session summary (all traces):**
```sql
SELECT
  s.id AS session_id,
  COUNT(t.id) AS total_trace_count,
  COUNT(t.id) FILTER (
    WHERE COALESCE(LOWER(t.status), '') NOT IN ('completed', 'ok', 'failed', 'error', 'cancelled')
  ) AS running_trace_count,
  COUNT(t.id) FILTER (
    WHERE COALESCE(LOWER(t.status), '') IN ('completed', 'ok')
  ) AS completed_trace_count,
  COUNT(t.id) FILTER (
    WHERE COALESCE(LOWER(t.status), '') IN ('failed', 'error', 'cancelled')
  ) AS failed_trace_count,
  COALESCE(SUM(t.total_tokens_in), 0) AS total_tokens_in,
  COALESCE(SUM(t.total_tokens_out), 0) AS total_tokens_out,
  COALESCE(SUM(t.total_cost), 0) AS total_cost_usd,
  MIN(COALESCE(t.start_time, t.server_received_at)) AS started_at,
  MAX(CASE
    WHEN t.id IS NULL THEN NULL
    ELSE GREATEST(
      t.server_received_at,
      COALESCE(t.end_time, '1970-01-01'::timestamptz)
    )
  END) AS last_activity_at
FROM sessions s
LEFT JOIN traces t
  ON t.session_id = s.id
 AND t.project_id = s.project_id
WHERE s.id = $1 AND s.project_id = $2
GROUP BY s.id;
```

This computes over the full uncapped session. If Query 1 returns no row, the session is missing or outside the authenticated project and the handler returns the same `404` result as `GetSession`. If Query 1 returns a row with `total_trace_count = 0`, the session exists and the endpoint returns `200` with an empty narrative. The `CASE WHEN t.id IS NULL` guard is required so the `LEFT JOIN` zero-trace row does not collapse to the epoch via PostgreSQL `GREATEST` null-handling. The summary `last_activity_at` is trace-level only (uses `server_received_at` and `end_time`, not span/event timestamps). This is a deliberate cost trade-off — scanning spans and events for the entire uncapped session would be expensive. The per-trace `latest_activity_at` in Query 2 is the authoritative activity timestamp and is computed from all sources (spans, events). The mapper or API docs should note that the summary `last_activity_at` is approximate.

The status buckets deliberately mirror `internal/api/mapper.go:mapTraceStatus`: `completed|ok => completed`, `failed|error|cancelled => failed`, and any other or null status counts as running. This keeps summary counts and frontend polling aligned with how trace cards render today.

**Query 2 — Capped trace detail (oldest 100):**
```sql
WITH capped_traces AS (
  SELECT id, trace_id, name, status, user_id,
    COALESCE(start_time, server_received_at) AS started_at,
    end_time AS ended_at,
    metadata,
    server_received_at,
    total_tokens_in, total_tokens_out, total_cost,
    error_count
  FROM traces
  WHERE session_id = $1 AND project_id = $2
  ORDER BY COALESCE(start_time, server_received_at) ASC, id ASC
  LIMIT 100
),
span_activity AS (
  SELECT s.trace_id,
    MAX(GREATEST(s.start_time, COALESCE(s.end_time, '1970-01-01'::timestamptz))) AS latest_span_ts
  FROM spans s
  WHERE s.trace_id IN (SELECT id FROM capped_traces)
  GROUP BY s.trace_id
),
event_activity AS (
  SELECT se.trace_id,
    MAX(GREATEST(
      COALESCE(se.event_ts, '1970-01-01'::timestamptz),
      se.server_ingested_at
    )) AS latest_event_ts
  FROM span_events se
  WHERE se.trace_id IN (SELECT id FROM capped_traces)
  GROUP BY se.trace_id
)
SELECT ct.*,
  GREATEST(
    ct.server_received_at,
    COALESCE(ct.ended_at, '1970-01-01'::timestamptz),
    COALESCE(sa.latest_span_ts, '1970-01-01'::timestamptz),
    COALESCE(ea.latest_event_ts, '1970-01-01'::timestamptz)
  ) AS latest_activity_at,
  EXTRACT(EPOCH FROM (ct.ended_at - ct.started_at)) * 1000 AS duration_ms
FROM capped_traces ct
LEFT JOIN span_activity sa ON sa.trace_id = ct.id
LEFT JOIN event_activity ea ON ea.trace_id = ct.id
ORDER BY ct.started_at ASC, ct.id ASC;
```

**Query 3 — Semantic events for returned traces:**
```sql
SELECT se.id, se.trace_id, se.span_id,
  s.name AS span_name,
  se.event_type, COALESCE(se.event_ts, se.server_ingested_at) AS timestamp,
  'explicit' AS source, se.level, se.sequence, se.message, se.payload
FROM span_events se
LEFT JOIN spans s ON s.trace_id = se.trace_id AND s.span_id = se.span_id
WHERE se.trace_id = ANY($1)
  AND se.event_type IN ('decision', 'effect', 'wait')
ORDER BY COALESCE(se.event_ts, se.server_ingested_at) ASC, se.sequence ASC NULLS LAST;
```

The `$1` parameter is the array of internal trace UUIDs from Query 2.

**Important:** `span_events.span_id` is an external text identifier, not a UUID FK to `spans.id`. The correct join to get span metadata (e.g., `span_name`) is `(trace_id, span_id)` as used in `db/platform/queries/events.sql:39`. Use `LEFT JOIN` so orphan events (whose span hasn't been ingested yet) are still included. When span metadata is not needed (as in the `event_activity` CTE in Query 2), query `span_events` directly via `trace_id` without joining `spans`.

Because Query 3 reuses `TimelineEvent`, nested `semantic_events[].trace_id` remains the internal trace UUID from that schema, while `SessionNarrativeTrace.trace_id` is the external trace identifier.

Implementation note: after the default 3-query shape ships, a future optimization could fuse Queries 2 and 3. Do not optimize this before the stable 3-query shape is implemented and validated.

### Lineage Resolution (Go-side, post-query)

Lineage is resolved in Go after Query 2 returns the capped trace set with `latest_activity_at`.

**Inference-only (core deliverable):**
For each trace in chronological order:
1. The first trace is always `unlinked`.
2. For trace at index `i`, the candidate parent is the trace at index `i-1`.
3. Infer a link when:
   - `child.started_at > predecessor.latest_activity_at` (clean gap)
   - No other trace in the set with `started_at < child.started_at` has `latest_activity_at >= child.started_at` (no overlapping activity)
4. If timestamps are missing (`started_at` or `latest_activity_at` is null) or the clean-gap test fails, mark `unlinked`.
5. Never infer cycles or multi-parent links.

**Explicit metadata (required cut-safe final slice):**
Before inference, check each trace's `metadata` JSONB for `__continua_lineage`:
```json
{
  "__continua_lineage": {
    "parent_trace_id": "<external trace_id>",
    "trigger_span_id": "<optional external span_id>",
    "link_kind": "<optional string>"
  }
}
```
- `parent_trace_id` must resolve to a trace in the same returned session set.
- Malformed metadata (wrong types, missing `parent_trace_id`) is silently ignored.
- Links pointing outside the returned set or outside the project are ignored.
- When explicit and inferred links conflict, explicit wins.
- This slice is part of the approved scope for this change; it is not a skip-if-time-allows task.

### `latest_activity_at` Computation

Must consider all of:
- `trace.server_received_at`
- `trace.end_time` (nullable)
- `span.start_time` (per related span)
- `span.end_time` (per related span, nullable)
- `span_events.event_ts` (nullable)
- `span_events.server_ingested_at`

This is computed inside Query 2's CTEs, not as a separate query.

### Frontend Independence

The narrative query (`['session-narrative', sessionId]`) is fully independent from the existing `['session', sessionId]` and `['session-traces', ...]` queries. It has its own loading/error states and never blocks or replaces the existing session header or trace table.

The storyline links reuse the existing trace-detail navigation pattern, including `returnTo` preservation. The zero-trace narrative state should be compact and contextual above the existing trace-table empty state, not a second full-page-style empty panel.

Polling: `refetchInterval: 30_000` only while normalized `running_trace_count > 0`, so unknown raw trace statuses continue polling in the same cases where the UI would still render those traces as running.

### Lineage Counts

`explicit_link_count`, `inferred_link_count`, and `unlinked_trace_count` are computed over the returned narrative set (up to 100 traces), not the full uncapped session. This avoids an expensive second pass over all session traces just for counts. The frontend must label these as counts for the shown narrative only, and when truncated, explicitly as counts for the first 100 traces.

## Risks / Trade-offs

- **Approximate session-level `last_activity_at`**: Query 1 only uses trace-level columns (not span/event activity) for the session summary `last_activity_at`. This is a deliberate simplification — the per-trace `latest_activity_at` from Query 2 is the authoritative source for lineage. Improving session-level accuracy would require scanning spans/events for all traces, which is expensive for large sessions.
- **Hard cap at 100**: Sessions with >100 traces show a truncation banner. The storyline covers the first 100 chronologically; the full table below remains the complete browser. This is acceptable for v1.
- **Inference limitations**: The clean-gap rule is conservative — overlapping or concurrent traces are marked `unlinked` rather than guessed. This is by design to avoid misleading lineage.
- **No migration**: The endpoint reads existing schema. If lineage metadata conventions need enforcement in ingest, that would be a separate change.

## Open Questions

None — the proposal spec is sufficiently detailed for implementation.
