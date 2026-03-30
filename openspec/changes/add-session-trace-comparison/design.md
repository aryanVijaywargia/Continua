## Context

Phase 8 adds a session-scoped trace comparison feature to Continua's debugger platform. Users select two terminal traces within the same session and get a structured diff: matched spans, inline semantic event pairing, precomputed changed-field lists, and match provenance metadata. The feature spans a new REST endpoint, a Go-side comparison builder, and a dedicated React compare page.

The design follows the session narrative pattern (Phase 7): scoped sqlc reads with deterministic Go-side assembly, no new migrations, and a thin handler delegating to a store builder.

### Stakeholders

- Debugger users who rerun agent sessions and need to understand divergence
- Backend: `internal/store`, `internal/api`
- Frontend: `web/src/pages`, `web/src/components`

## Goals / Non-Goals

### Goals
- Deterministic, reproducible comparison output for any two terminal traces in a session
- Clear provenance for every match (stable ID vs heuristic)
- Precomputed diff fields so the UI does not diff payloads client-side
- Graceful rejection of oversized comparisons before doing expensive work
- Mobile-friendly compare page layout

### Non-Goals
- Engine replay comparison
- Running-trace or live-polling compare mode
- Full span input/output payload diffs
- Paginated comparison response
- Dual-workspace overlay or merged tree visualization
- Branch/fork graph UI
- Cross-session comparison

## Decisions

### 1. Directional Baseline/Candidate Semantics

**Decision:** The comparison is directional. Baseline is the reference/original run; candidate is the rerun/alternate. The API requires both as explicit query params — there is no implicit "latest" selection.

**Why:** Directional semantics make diff polarity unambiguous (baseline_only vs candidate_only) and let the UI show clear before/after framing. The user explicitly picks both sides.

### 2. Three-Query Plan

Following the session narrative pattern:

| Query | Purpose | Key filters |
|-------|---------|-------------|
| Q1: Validation + headers | Validate session ownership, trace membership, terminal status via `NormalizeTraceStatus`, size thresholds; returns session header (id, external_id, name) via JOIN and both trace headers | `project_id`, `session_id`, both trace IDs |
| Q2: Spans | Fetch all spans for both traces | Both `trace_id` values, ordered by `COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST, id ASC` (extends `ListSpansByTrace` with a final `id` tie-break for determinism) |
| Q3: Semantic events | Fetch decision/effect/wait events for both traces | Both `trace_id` values, `event_type IN ('decision','effect','wait')`, with span name join |

Q1 runs first. If validation or threshold checks fail, Q2 and Q3 are skipped entirely.

**Why:** Same pattern as narrative — keeps queries simple, moves assembly logic to Go where it's testable and deterministic.

### 3. Threshold-Check Mechanism

**Decision:** Use `COUNT(*)` queries in Q1 for both span count and semantic event count per trace. Not `LIMIT+1` — we need the actual count for the error message and it's a simple index scan.

```sql
-- Span counts (part of Q1 CTE)
SELECT trace_id, COUNT(*) as span_count
FROM spans
WHERE trace_id IN ($baseline_id, $candidate_id)
GROUP BY trace_id

-- Semantic event counts (part of Q1 CTE)
SELECT trace_id, COUNT(*) as semantic_count
FROM span_events
WHERE trace_id IN ($baseline_id, $candidate_id)
  AND event_type IN ('decision', 'effect', 'wait')
GROUP BY trace_id
```

**Zero-count handling:** The grouped COUNT queries return no row for a trace with zero spans or zero semantic events. The store builder MUST treat a missing count row as zero (not as an error). A trace with zero spans is valid and produces an empty diff; a trace with zero semantic events is valid and produces span rows with no semantic groups.

**Thresholds:** >500 spans OR >1,000 semantic events on either trace → `422 comparison_too_large`.

**422 response shape:** A dedicated `ComparisonTooLargeError` schema extending the base `Error` with a `detail` object:
```
{
  code: "comparison_too_large",
  message: "...",
  detail: {
    baseline_span_count: int,
    candidate_span_count: int,
    baseline_semantic_count: int,
    candidate_semantic_count: int,
    max_spans: 500,
    max_semantic_events: 1000
  }
}
```

**Frontend plumbing:** The current `ApiError` class drops fields beyond `code`/`message`. The frontend MUST extend the error path to preserve the `detail` object from `422` responses so the compare page can render the specific counts. This is a prerequisite for the compare page's error state.

**Why:** COUNT on indexed columns is fast. Returning the actual counts in the typed error response helps users understand why comparison was rejected.

### 4. Span Matching Algorithm

Two-pass matching, all in Go after Q2 returns:

**Pass 1 — Stable ID match:** Build maps of `external_span_id → span` for both traces. Any span_id present in both maps is a stable match. Mark `match_source: "stable_id"`, `match_reason: "exact_span_id"`.

**Pass 2 — Heuristic match:** For remaining unmatched spans, group by matched-parent scope (or root scope if parent is unmatched). Within each scope, attempt strict matching using:
1. Normalized span name (trim + collapse whitespace + lowercase)
2. Exact span kind match
3. Sibling ordinal: position among same-scope siblings using Q2 row order as the canonical ordinal (i.e. `COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST, id ASC`)

A match is valid only when exactly one candidate in the scope matches exactly one baseline span by all three criteria. Any ambiguity (multiple candidates with same name+kind+ordinal) leaves both sides unmatched.

Mark heuristic matches as `match_source: "heuristic"`, `match_reason: "name_kind_ordinal"`.

**Why:** Stable IDs are authoritative. Heuristic matching is intentionally conservative — `unmatched` is always safer than a wrong guess. Scoping to matched parents prevents cross-branch false positives.

### 5. Semantic Event Pairing

All pairing happens within already-matched span pairs only. Unmatched spans' events are never paired.

| Event type | Pairing key | Normalization | match_source | match_reason |
|------------|-------------|---------------|--------------|--------------|
| Decision | Normalized question string, unique on both sides; only decisions with a non-empty string `question` field participate — missing/non-string/blank-after-normalization questions are always unpaired | trim + collapse whitespace + lowercase; empty result → excluded | `heuristic` | `unique_normalized_question` |
| Effect | Exact `effect_id` from payload, unique on both sides of the matched span pair | None — must be present and non-empty | `stable_id` | `exact_effect_id` |
| Wait | Exact `wait_id` from payload, unique in span pair | None — must be present and non-empty | `stable_id` | `exact_wait_id` |

Unpaired events are surfaced with their side (`baseline_only` or `candidate_only`) and `match_source: null`, `match_reason: null`.

**Why:** Semantic events lack stable cross-execution IDs (except effect_id/wait_id). Decision pairing by normalized question is heuristic, not stable — the vocabulary reflects this. Conservative pairing prevents misleading diffs.

### 6. Changed-Fields Precomputation

The Go builder computes `changed_fields` for every matched pair by comparing specific fields:

- **Spans:** status, latency_ms, tokens_in, tokens_out, cost_usd, semantic_count (count of paired/unpaired semantic events). `changed_fields` names MUST match the `CompareSpanSummary` API wire names, not DB column names. The builder maps DB columns (`duration_ms` → `latency_ms`, `prompt_tokens` → `tokens_in`, `completion_tokens` → `tokens_out`, `total_cost` → `cost_usd`) before computing the diff. `error_count` is not available on the span model and is out of scope for v1.
- **Decisions:** chosen, reasoning, alternatives, message
- **Effects:** effect_kind, has_external_side_effect, idempotent, idempotency_key, message, payload
- **Waits:** wait_kind, phase, resolution, message, payload

Only field names that actually differ are included in the list. Empty list means unchanged.

**Why:** Client-side diffing of raw payloads is fragile, slow, and non-deterministic. Server precomputation ensures consistency.

### 7. Response Shape

**`CompareTraceHeader`** reuses the scalar fields from `SessionNarrativeTrace` (minus `semantic_events` and `lineage`):
- `id` (internal UUID), `trace_id` (external), `name`, `status` (normalized enum: RUNNING/COMPLETED/FAILED)
- `user_id`, `started_at`, `ended_at`, `duration_ms`
- `error_count`, `total_cost_usd`, `total_tokens_in`, `total_tokens_out`

The OpenAPI schema SHOULD be defined as a standalone `CompareTraceHeader` rather than a `$ref` to `SessionNarrativeTrace`, since the latter includes narrative-specific fields. The field names and types MUST match `SessionNarrativeTrace` for the overlapping set so the frontend can share formatting utilities.

**`CompareSpanSummary`** is a subset of the existing `Span` API schema (defined in `openapi.yaml`), restricted to the fields needed for diff rendering. It does NOT include `input`, `output`, or `metadata` (no payload diff in v1):
- `id` (internal UUID), `span_id` (external), `parent_span_id` (external, nullable)
- `name`, `kind` (enum matching existing `Span.kind`), `status` (enum matching existing `Span.status`)
- `started_at`, `ended_at`, `latency_ms`
- `tokens_in`, `tokens_out`, `cost_usd`
- `error_message` (nullable — replaces the removed `error_count`; surfaces `status_message` from the span row)
- `model` (nullable)

Field names and types MUST match the existing `Span` schema for the overlapping set so the frontend can share formatting utilities. The OpenAPI schema SHOULD be a standalone `CompareSpanSummary` definition, not a `$ref` to `Span`, since `Span` includes payload fields excluded here.

**`CompareSemanticSummary`** is a subset of the existing `TimelineEvent` API schema, restricted to fields needed for semantic diff rendering:
- `id` (event UUID), `span_id` (external, nullable), `span_name` (nullable)
- `event_type` (restricted to `decision | effect | wait`)
- `timestamp`, `message` (nullable)
- `payload` (the full semantic payload object — needed to display question/chosen/alternatives for decisions, effect_kind/idempotent for effects, wait_kind/phase/resolution for waits)

Field names and types MUST match the existing `TimelineEvent` schema for the overlapping set. The OpenAPI schema SHOULD be a standalone `CompareSemanticSummary` definition.

```
CompareResponse {
  session: { id, external_id, name }
  baseline: CompareTraceHeader
  candidate: CompareTraceHeader
  summary: {
    total_spans_baseline, total_spans_candidate,
    matched_spans, unmatched_baseline_spans, unmatched_candidate_spans,
    heuristic_matches,  // span-level heuristic matches only; does not include heuristically paired semantic events
    duration_delta_ms, tokens_in_delta, tokens_out_delta, cost_delta_usd,  // all deltas are (candidate - baseline); positive means candidate is larger/longer
    total_semantic_baseline, total_semantic_candidate
  }
  span_diffs: [SpanDiffRow]
}

SpanDiffRow {
  diff_status: "unchanged" | "changed" | "baseline_only" | "candidate_only"
  match_source: "stable_id" | "heuristic" | null
  match_reason: string | null
  changed_fields: [string]
  baseline_span: CompareSpanSummary | null
  candidate_span: CompareSpanSummary | null
  semantic_groups: [SemanticDiffGroup]
  depth: int  // for indentation
}

SemanticDiffGroup {
  event_type: "decision" | "effect" | "wait"
  diff_status: "unchanged" | "changed" | "baseline_only" | "candidate_only"
  match_source: "stable_id" | "heuristic" | null
  match_reason: string | null   // e.g. "exact_effect_id", "exact_wait_id", "unique_normalized_question"
  changed_fields: [string]
  baseline_event: CompareSemanticSummary | null
  candidate_event: CompareSemanticSummary | null
}
```

Span diff rows use a **baseline-preorder traversal** that preserves parent-child locality:

1. Walk the baseline span tree in preorder (using `COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST, id ASC` within each parent scope).
2. For each matched baseline span, emit the diff row, then recurse into children. Candidate-only children of a matched parent are inserted after the baseline children of that parent, preserving their own candidate-side ordering.
3. Baseline-only spans appear in their natural tree position.
4. After the full baseline tree is emitted, append **unmatched candidate root branches** (candidate-only spans whose parent is also unmatched or absent) as a final block, each with their own subtree in candidate-side preorder.

This keeps `depth` meaningful for indentation and ensures children always appear directly below their parent.

### 8. Compare Page UX Model

**Two sections:**
- **Overview:** Session identity, baseline/candidate trace headers with status badges, aggregate deltas (duration, tokens, cost), match counts, links to source traces.
- **Diff:** Ordered span rows with diff-status coloring, provenance badges, changed-field chips. Rows with semantic content get an expand/collapse control; rows without semantic content render as plain rows.

**Entry flow from session detail:**
- URL params `baseline_trace_id` and `candidate_trace_id` added to session detail URL
- Actions on storyline cards and table rows: "Set as baseline", "Set as candidate"
- Compare bar appears when at least one trace is selected, showing current selections + Swap/Clear/Open comparison
- "Compare to parent" shortcut on storyline rows when lineage exposes a parent

**Compare selection hydration:**

The compare bar needs trace header/status data for the selected IDs, but those traces may not be in the current paginated `fetchTraces` slice or in a truncated/errored narrative. The selection resolution strategy is:

1. **Primary sources (already loaded):** The narrative traces array and the current paginated table slice. Look up each selected ID in both.
2. **Bounded selected-trace lookup:** If a selected trace is not found in the loaded data, the session detail page issues a bounded lookup against the existing `GET /api/traces/{id}` / `fetchTrace(id)` path for that specific trace ID. This is capped at one lookup per selected trace (two total), deduped by trace ID, and cached through React Query. We do not need a dedicated header endpoint in v1 because the existing trace-detail read path is already project-scoped and returns the header fields the compare bar needs.
3. **Lookup result validation:** A bounded lookup is accepted only when the returned trace belongs to the current session (`trace.session_id === sessionId`). If the lookup returns `404` or the trace belongs to another session, the corresponding compare param is stripped from the URL as stale. While a selected-trace lookup is pending, the compare bar stays non-actionable.
4. **No unbounded fetch chains:** The session detail page still does not chase pagination or narrative failures. The only extra fetches allowed are the bounded per-selection lookups above.

**Invalid URL param policy:**

When `baseline_trace_id` or `candidate_trace_id` are hydrated from the URL on page load:
- **Malformed UUIDs:** Silently strip the invalid param from the URL.
- **Same ID in both params:** Clear the `candidate_trace_id` param (matching the same-trace guard behavior).
- **Valid UUID but trace not in loaded data:** Issue the bounded `fetchTrace(id)` lookup. If it resolves and belongs to the current session, use the returned trace header in the compare bar. If it fails session validation or returns `404`, strip the stale compare param.
- **Running trace selected:** If the trace resolves from loaded data or bounded lookup and is running, show it in the compare bar with a warning indicator and disable "Open comparison" for that pair.

**Navigation:**
- Compare page URL: `/sessions/:id/compare?baseline_trace_id=X&candidate_trace_id=Y`
- **All trace-detail links from the compare page** (both overview/header links and span-row deep links) MUST use the internal trace UUID `id` in the `/traces/:id` pathname. The external `trace_id` remains display-only on the compare page. All of these links MUST also pass `returnTo` via React Router `location.state`, pointing back to the current compare URL. This includes:
  - Overview section "open trace" links for baseline and candidate
  - Span-row deep links (which also include `?span=<external_span_id>` in the query string)
- **Compare page `returnTo` handling:** The compare page reads its own `returnTo` from `location.state` for its back button. Validation follows the same pattern as existing pages:
  - Accept `returnTo` values matching `/sessions/*`
  - Fallback to the parent session detail URL with the current canonical compare params preserved (for example `/sessions/:id?baseline_trace_id=X&candidate_trace_id=Y`) when `returnTo` is missing, malformed, or fails the whitelist — e.g. on direct load, refresh, or bookmark
  - Implement this as a `getCompareReturnToDestination(state, sessionId, searchParams)` helper mirroring `getReturnToDestination` and `getSessionsReturnToDestination`

**Mobile:** Single-column stacked layout, overview cards first, accordion-style span rows, inline semantic expansion below each row.

### 9. No New Migrations

All data needed for comparison already exists in the `traces`, `spans`, and `span_events` tables. The comparison builder reads existing columns. No schema changes required.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Large traces hit the 500-span / 1,000-event ceiling | Return informative `422` with counts; future versions can add pagination |
| Heuristic span matching produces unexpected results | Conservative rules + provenance badges make match source transparent |
| Go-side assembly is complex and hard to test | Golden snapshot tests with fixed inputs verify deterministic output |
| Response payload size for max-ceiling traces | 500 spans × 2 sides + 1,000 events × 2 sides is bounded; JSON response stays under a few MB |
| Semantic event payloads may be truncated | Use `changed_fields` precomputation; don't expose raw payload diffs in v1 |

## Open Questions

None — v1 scope is fully constrained by the proposal. Future versions may consider:
- Paginated comparison for larger traces
- Full payload diff inspection
- Cross-session comparison
- Running-trace compare mode
