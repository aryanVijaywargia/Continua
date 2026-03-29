## 1. OpenAPI Contract

- [x] 1.1 Add `GET /api/sessions/{id}/narrative` path to `contracts/openapi/openapi.yaml`
- [x] 1.2 Add `SessionNarrativeResponse`, `SessionNarrativeSummary`, `SessionNarrativeTrace`, `SessionNarrativeLineage` schemas (reuse `TimelineEvent` via `$ref`), with schema descriptions clarifying that `SessionNarrativeTrace.trace_id` is external while nested `semantic_events[].trace_id` remains the internal trace UUID from `TimelineEvent`
- [x] 1.3 Run `make generate` and verify generated Go/TS types

## 2. Store: BuildSessionNarrative

- [x] 2.1 Add `BuildSessionNarrative(ctx, projectID, sessionID, limit)` to `internal/store/`
- [x] 2.2 Implement Query 1 as `sessions LEFT JOIN traces`, so zero-trace sessions and missing/cross-project sessions are distinguishable without a separate existence query; status buckets must mirror `mapTraceStatus` normalization
- [x] 2.3 Implement Query 2: capped trace query with per-trace `latest_activity_at` via span/event CTEs, ordered by `COALESCE(start_time, server_received_at) ASC, id ASC` (join `span_events` directly via `trace_id`, not through `spans`; see `events.sql:39` for the `(trace_id, span_id)` join convention when span metadata is needed)
- [x] 2.4 Implement Query 3: semantic events for returned trace IDs, filtered to `decision|effect|wait` (use `LEFT JOIN spans ON (trace_id, span_id)` for `span_name`, following `events.sql:39`)
- [x] 2.5 Define Go result types for narrative summary, narrative traces, and semantic events

## 3. Lineage Resolution (inference core)

- [x] 3.1 Implement chronological inference lineage resolver in Go
- [x] 3.2 Clean-gap rule: child.started_at > predecessor.latest_activity_at, no overlapping activity
- [x] 3.3 First trace always unlinked; null timestamps produce unlinked
- [x] 3.4 Compute `explicit_link_count`, `inferred_link_count`, `unlinked_trace_count` over returned set

## 4. Handler and Mapper

- [x] 4.1 Add `GetSessionNarrative` handler in `internal/api/sessions_handlers.go`, mapping a no-row Query 1 result to the same `404` behavior as `GetSession`
- [x] 4.2 Wire handler to generated server interface
- [x] 4.3 Add narrative mapping functions in `internal/api/mapper.go` (note: summary `last_activity_at` is trace-level approximate; add a mapper comment; summary status counts must stay aligned with `mapTraceStatus`)
- [x] 4.4 Verify 404 for missing session and cross-project access, while zero-trace existing sessions still return `200`

## 5. Backend Tests

- [x] 5.1 Missing session returns 404
- [x] 5.2 Cross-project access returns 404
- [x] 5.3 Zero-trace session returns 200 with empty narrative
- [x] 5.4 Summary aggregation across multiple traces (cost, tokens, status counts)
- [x] 5.5 Unknown/raw trace statuses normalize into `running_trace_count`
- [x] 5.6 `latest_activity_at` includes trace.end_time, span timestamps, and event timestamps
- [x] 5.7 Capped response returns oldest 100 traces, ordered by `started_at ASC, id ASC`, sets `truncated=true`
- [x] 5.8 Semantic event inclusion and ordering for `decision|effect|wait`
- [x] 5.9 Inference lineage for clean sequential traces
- [x] 5.10 No inferred lineage when overlapping trace activity exists

## 6. Frontend: API Client, Query, and Sections

- [x] 6.1 Add `SessionNarrative` types and `fetchSessionNarrative(sessionId)` to `web/src/api/client.ts`
- [x] 6.2 Add narrative React Query in `SessionDetailPage` with key `['session-narrative', sessionId]`, polling every 30s while `running_trace_count > 0`
- [x] 6.3 Add summary section above trace table (returned/total counts, status breakdown, cost/tokens, timing, lineage coverage) with skeleton loading and inline error states, reusing existing `SessionDetailPage` containment patterns
- [x] 6.4 Add storyline section: trace cards oldest-first with name, trace_id, status badge, timing/duration, cost/tokens/errors, lineage badge, semantic snippet
- [x] 6.5 Label lineage coverage as applying to the shown narrative only and, when truncated, to the first 100 traces
- [x] 6.6 Truncation banner when `truncated=true`; compact empty narrative placeholder for zero-trace sessions
- [x] 6.7 Storyline navigation uses same trace-detail linking as table, preserves `returnTo`

## 7. Frontend Tests

- [x] 7.1 Narrative loading state is local to the summary/storyline area and does not block the existing session header or trace table
- [x] 7.2 Narrative error state is inline/contained and does not break the existing session header or trace table
- [x] 7.3 Summary renders above unchanged trace table; storyline oldest-first ordering remains stable when timestamps tie
- [x] 7.4 Lineage badges (explicit/inferred/unlinked), disclosure label, truncation banner, zero-trace compact placeholder
- [x] 7.5 Polling only while running traces exist, including normalized unknown-status traces
- [x] 7.6 Storyline navigation preserves `returnTo`; existing trace-table URL-state unchanged

## 8. Explicit Lineage Metadata (required final slice)

- [x] 8.1 Add explicit metadata parsing in lineage resolver: `metadata.__continua_lineage`
- [x] 8.2 Validate `parent_trace_id` resolves within returned session set; ignore malformed and out-of-session
- [x] 8.3 Explicit links take precedence over inferred
- [x] 8.4 Backend tests: valid explicit lineage, malformed ignored, out-of-session ignored

## 9. Final Validation

- [x] 9.1 `make generate` (no drift)
- [x] 9.2 `go test ./internal/api/... ./internal/store/...`
- [x] 9.3 `pnpm --filter web test`
- [ ] 9.4 `make lint`

Note: `make lint` is currently blocked in this environment because `golangci-lint` is not installed.

### Parallelization Notes

- Tasks 2 and 6.1 can proceed in parallel once 1.3 is complete.
- Tasks 5 and 6-7 can proceed in parallel (backend tests vs frontend).
- Task 8 depends on the core lineage resolver from Task 3, but stays isolated enough to implement after the inference core.
- Task 9 is the final gate and depends on all prior tasks.
