## 1. OpenAPI Contract

- [x] 1.1 Add `GET /api/sessions/{id}/compare` path with `baseline_trace_id` and `candidate_trace_id` query params to `contracts/openapi/openapi.yaml`
- [x] 1.2 Define `SessionCompareResponse` schema (session header, baseline/candidate trace headers, summary, span_diffs array)
- [x] 1.3 Define `CompareTraceHeader` schema — standalone, reusing scalar field names/types from `SessionNarrativeTrace` (minus `semantic_events` and `lineage`)
- [x] 1.4 Define `SpanDiffRow` schema (diff_status, match_source, match_reason, changed_fields, baseline_span, candidate_span, semantic_groups, depth)
- [x] 1.5 Define `SemanticDiffGroup` schema (event_type, diff_status, match_source, match_reason, changed_fields, baseline_event, candidate_event)
- [x] 1.6 Define `CompareSpanSummary` schema as a subset of `Span` (id, span_id, parent_span_id, name, kind, status, started_at, ended_at, latency_ms, tokens_in, tokens_out, cost_usd, error_message, model — no input/output/metadata). Field names/types MUST match `Span` for the overlapping set.
- [x] 1.7 Define `CompareSemanticSummary` schema as a subset of `TimelineEvent` (id, span_id, span_name, event_type restricted to decision|effect|wait, timestamp, message, payload). Field names/types MUST match `TimelineEvent` for the overlapping set.
- [x] 1.8 Define `CompareSummary` schema (span counts, match counts, aggregate deltas with `candidate - baseline` polarity)
- [x] 1.9 Define error responses: `400` (missing params, identical IDs, non-terminal), `404` (session/trace not found)
- [x] 1.10 Define a dedicated `ComparisonTooLargeError` schema extending the base `Error` with a flat `detail` object containing `baseline_span_count`, `candidate_span_count`, `baseline_semantic_count`, `candidate_semantic_count`, `max_spans` (500), and `max_semantic_events` (1000) — used for the `422` response. Shape must match the design.md example exactly.
- [x] 1.11 Run `make generate` and verify generated code compiles

## 2. Store: Validation and Threshold Queries

- [x] 2.1 Add SQL query `GetCompareValidation` in `db/platform/queries/session_compare.sql`: validate session ownership + project scope, confirm both traces belong to the session, return session header fields (id, external_id, name) and trace status + `CompareTraceHeader` fields (id, trace_id, name, status, user_id, started_at, ended_at, duration_ms, error_count, total_cost, total_tokens_in, total_tokens_out) for both traces. Session fields come from the same query via JOIN — no separate session fetch needed.
- [x] 2.2 Add SQL query `GetCompareSpanCounts`: COUNT(*) per trace for the two trace IDs in `spans` table
- [x] 2.3 Add SQL query `GetCompareSemanticCounts`: COUNT(*) per trace for the two trace IDs in `span_events` filtered to `event_type IN ('decision','effect','wait')`
- [x] 2.4 Run `make generate` to produce sqlc wrappers
- [x] 2.5 Add store method `ValidateCompareEligibility(ctx, projectID, sessionID, baselineTraceID, candidateTraceID)` returning validation result with headers and counts

## 3. Store: Span and Semantic Event Fetch Queries

- [x] 3.1 Add SQL query `ListCompareSpans`: fetch all spans for two trace IDs ordered by `COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST, id ASC` (extends `ListSpansByTrace` with final `id` tie-break for determinism), including id, span_id, parent_span_id, name, type, status, status_message, start_time, end_time, duration_ms, prompt_tokens, completion_tokens, total_cost, depth, server_received_at, sequence
- [x] 3.2 Add SQL query `ListCompareSemanticEvents`: fetch span_events for two trace IDs filtered to `event_type IN ('decision','effect','wait')` with LEFT JOIN on spans for span_name, ordered by `COALESCE(event_ts, server_ingested_at) ASC, sequence ASC NULLS LAST, id ASC`
- [x] 3.3 Run `make generate`

## 4. Store: Comparison Builder

- [x] 4.1 Create `internal/store/session_compare.go` with `BuildSessionComparison(ctx, projectID, sessionID, baselineTraceID, candidateTraceID)` method
- [x] 4.2 Implement Q1 call: validate eligibility and check thresholds; return structured errors for 400/404/422 cases
- [x] 4.3 Implement Q2 call: fetch spans for both traces
- [x] 4.4 Implement Q3 call: fetch semantic events for both traces
- [x] 4.5 Implement Pass 1 span matching: exact external span_id
- [x] 4.6 Implement Pass 2 span matching: heuristic within matched-parent scopes (normalized name + exact kind + sibling ordinal), leaving ambiguous cases unmatched
- [x] 4.7 Implement decision pairing: normalized question matching within matched span pairs
- [x] 4.8 Implement effect pairing: exact effect_id within matched span pairs
- [x] 4.9 Implement wait pairing: exact wait_id (unique in span pair) within matched span pairs
- [x] 4.10 Implement changed_fields precomputation for spans, decisions, effects, and waits
- [x] 4.11 Implement summary aggregation: span counts, match counts, aggregate deltas
- [x] 4.12 Implement span diff ordering: baseline-preorder traversal with candidate-only children inserted under matched parents, unmatched candidate root branches appended at the end
- [x] 4.13 Define comparison result types (SessionComparison, SpanDiffRow, SemanticDiffGroup, etc.)

## 5. API Handler and Mapper

- [x] 5.1 Add compare handler in `internal/api/sessions_handlers.go` (name follows generated operationId): parse params, call store builder, map structured errors to HTTP status codes
- [x] 5.2 Add comparison mapping functions in `internal/api/mapper.go`: store types → OpenAPI response types
- [x] 5.3 Wire handler to generated server interface

## 6. Backend Tests

- [x] 6.1 Integration tests for validation: missing session, cross-project, trace not in session, identical IDs, non-terminal traces
- [x] 6.2 Integration test for `comparison_too_large` threshold rejection
- [x] 6.3 Integration test for zero-span and zero-semantic-event traces (valid comparison with empty diff)
- [x] 6.4 Unit tests for span matching: exact span_id, heuristic match, ambiguous heuristic staying unmatched
- [x] 6.5 Unit tests for decision pairing: unique question match, changed question unpaired, duplicate question ambiguity, missing/blank/non-string question always unpaired
- [x] 6.6 Unit tests for effect pairing: matching unique effect_id, duplicate effect_id ambiguity stays unpaired, missing effect_id stays unpaired
- [x] 6.7 Unit tests for wait pairing: matching unique wait_id, duplicate wait_id ambiguity stays unpaired, missing wait_id stays unpaired
- [x] 6.8 Unit tests for changed_fields precomputation
- [x] 6.9 Golden snapshot tests: nearly identical rerun, changed span structure, changed decision path, effect added/removed with and without effect_id, wait added/removed or resolved differently, ambiguous heuristic case, candidate-only branch

## 7. Frontend: API Client and Types

- [x] 7.1 Add `SessionCompareResponse`, `SpanDiffRow`, `SemanticDiffGroup`, `ComparisonTooLargeError`, and supporting types to `web/src/api/client.ts`
- [x] 7.2 Extend `ApiError` (or add a typed subclass) to preserve the `detail` object from `422` responses so the compare page can display threshold counts
- [x] 7.3 Add `fetchSessionComparison(sessionId, baselineTraceId, candidateTraceId)` function
- [x] 7.4 Add compare route to `web/src/App.tsx`: `/sessions/:id/compare`

## 8. Frontend: Session Detail Compare Selection

- [x] 8.1 Add `baseline_trace_id` and `candidate_trace_id` URL param state to `SessionDetailPage.tsx`. This requires extending `serializeTracesParams` (or its caller) to preserve compare params through the query-string canonicalization path — currently `serializeTracesParams` rebuilds params from a known whitelist and will strip unknown params on first render and on any table-state change
- [x] 8.2 Add "Set as baseline" / "Set as candidate" actions to trace table rows — if the trace already occupies the other role, clear the displaced role (same-trace guard)
- [x] 8.3 Add "Set as baseline" / "Set as candidate" actions to storyline cards — same same-trace guard as table rows
- [x] 8.4 Add "Compare to parent" action on storyline rows — only shown when lineage exposes a parent in the narrative set AND both traces are terminal; handle same-trace-both-roles by clearing the displaced role
- [x] 8.5 Add compare bar component: shows selected baseline/candidate, Swap, Clear, and Open comparison (navigates to compare page, passing `returnTo` via `location.state`)
- [x] 8.6 Disable "Set as baseline" / "Set as candidate" actions on running traces (both storyline cards and table rows) with tooltip indicating trace must complete first
- [x] 8.7 Implement compare selection hydration: resolve selected IDs from narrative + paginated table data first; when a selected trace is missing, issue bounded `fetchTrace(id)` lookups (max two, deduped/cached) to hydrate compare-bar header data; strip malformed UUIDs; clear duplicate same-trace params
- [x] 8.8 Strip stale compare params when a bounded selected-trace lookup returns `404` or a trace whose `session_id` does not match the current session; keep the compare bar disabled while any selected-trace lookup is pending
- [x] 8.9 Compare bar enables "Open comparison" only when both traces are selected AND resolved (from loaded data or bounded lookup) AND terminal

## 9. Frontend: Compare Page

- [x] 9.1 Create `web/src/pages/SessionComparePage.tsx` with React Query fetch using URL params
- [x] 9.2 Implement Overview section: session identity, baseline/candidate headers with status badges, aggregate deltas, heuristic match count, links to source traces
- [x] 9.3 Implement Diff section: ordered span rows with diff-status coloring and provenance badges
- [x] 9.4 Implement changed-field chips on span rows
- [x] 9.5 Implement expand/collapse for inline semantic detail (only when row has semantic content)
- [x] 9.6 Implement inline semantic rendering: paired/unpaired decisions, effects, waits with match metadata
- [x] 9.7 Implement candidate-only branch grouping at the end of the diff list
- [x] 9.8 Implement trace-detail links: ALL links from compare page to trace detail (overview header links AND span-row deep links) MUST use the internal trace UUID `id` in the `/traces/:id` pathname, MUST pass `returnTo` via `location.state` pointing back to the compare URL, and span-row links additionally include `?span=<external_span_id>` in the query string
- [x] 9.9 Implement `getCompareReturnToDestination(state, sessionId, searchParams)` helper: accept `returnTo` matching `/sessions/*`, otherwise fallback to the parent session detail URL while preserving the current canonical compare params from the compare page URL (direct load, refresh, bookmark)
- [x] 9.10 Implement loading, error, and auth states
- [x] 9.11 Implement mobile stacked layout: single-column, overview cards first, accordion-style span rows

## 10. Frontend Tests

- [x] 10.1 Tests for URL-backed baseline/candidate selection on session detail, including cross-page selection and direct-load hydration via bounded selected-trace lookups
- [x] 10.2 Tests for same-trace-both-roles: assigning the same trace to the other role clears the displaced role (table rows and storyline cards)
- [x] 10.3 Tests for compare bar enable/disable logic, including selected-trace lookup pending states and running-trace guards
- [x] 10.4 Tests for running-trace selection disabled with tooltip
- [x] 10.5 Tests for "Compare to parent" action (including terminal-only guard)
- [x] 10.6 Tests for preservation of existing table URL state when compare params change, plus malformed UUID stripping and same-trace param canonicalization
- [x] 10.7 Tests for compare page loading/error/auth states (including 422 with detail display) and empty-diff state (zero span rows, overview still renders)
- [x] 10.8 Tests for swap behavior
- [x] 10.9 Tests for inline semantic expansion (only when content exists, no empty panels)
- [x] 10.10 Tests for provenance badges and changed-field chips rendering
- [x] 10.11 Tests for deep links into source traces: overview links and span-row links use internal trace UUID paths, span deep links preserve external `span_id`, and all links carry `returnTo`
- [x] 10.12 Tests for stacked mobile layout
- [x] 10.13 Tests for stale selected compare params being stripped when bounded selected-trace lookup returns `404` or another session's trace
- [x] 10.14 Tests for compare page back-navigation fallback preserving canonical compare params on direct load, refresh, or bookmark when `returnTo` state is missing or invalid

## 11. Validation

- [x] 11.1 Run `make generate` — verify no drift
- [x] 11.2 Run `go test ./internal/api/... ./internal/store/...` — all pass
- [x] 11.3 Run `pnpm --filter web test` — all pass
- [x] 11.4 Run `make lint` — clean

## Dependencies and Parallelism

- Tasks 1 (contract) must complete before tasks 2–3 (store queries) and 5 (handler) can use generated types
- Tasks 2–3 (queries) can run in parallel once contract is done
- Task 4 (builder) depends on 2–3
- Task 5 (handler/mapper) depends on 4
- Task 6 (backend tests) depends on 4–5
- Task 7 (frontend types) can start after task 1
- Tasks 8–9 (frontend UI) depend on 7
- Task 10 (frontend tests) depends on 8–9
- Task 11 (validation) is final
