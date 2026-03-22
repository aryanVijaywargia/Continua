## Context
The debugger currently has URL-driven filtering and pagination on `/traces` but not on `/sessions`. Session list has no search, filtering, or sorting. Trace reads don't surface session external IDs. Pagination everywhere is minimal. Phase 10 needs to bring sessions to parity with traces for scale UX while keeping the scope contained.

## Goals / Non-Goals

**Goals:**
- Sessions list with search, filtering, sorting, and URL-driven state
- Trace reads include `session_external_id` from a join
- Consistent advanced pagination across all list views
- External-first session identity throughout the debugger

**Non-Goals:**
- Cursor pagination (offset stays in Phase 10)
- Session mutation APIs
- Separate batch lookup API for session external IDs
- Trigram search or Postgres FTS for sessions
- Broader trace search/filter expansion

## Decisions

### Traces keep sqlc with two static queries for sort direction; sessions get handwritten queries
- **Decision**: Trace fast-path queries stay in sqlc. Since sqlc cannot parameterize SQL keywords like `ASC`/`DESC`, the implementation uses two static queries: the existing `ListTraces` (DESC, the default) and a new `ListTracesAsc` (ASC). The same applies to `ListTracesBySession` / `ListTracesBySessionAsc`. The handler picks the correct query based on the parsed `sort_dir` param. Sessions get a handwritten dynamic query file parallel to `search.go`.
- **Why**: sqlc binds values, not SQL keywords, so a single parameterized query cannot toggle sort direction. Two static queries is the simplest sqlc-compatible approach and avoids introducing a handwritten path for traces when the only variation is sort direction. Sessions need handwritten SQL because they have dynamic WHERE clauses and dynamic ORDER BY (`q`, `user_id`, `trace_count` sorting), which sqlc is not a good fit for.

### `trace_count` via derived-table aggregate join
- **Decision**: Implement `trace_count` sorting with `LEFT JOIN (SELECT session_id, COUNT(*) AS cnt FROM traces WHERE project_id = $1 GROUP BY session_id) tc ON tc.session_id = s.id`, using `COALESCE(tc.cnt, 0)`.
- **Why**: Joining traces directly on the outer query would cause row multiplication that breaks pagination and counting. A derived table keeps the session row cardinality stable.

### Search overrides user sorting
- **Decision**: When `q` is present on either traces or sessions, `sort_by` and `sort_dir` are ignored server-side and relevance/rank ordering is used instead.
- **Why**: Relevance ordering is the correct behavior during search. Allowing user sort on top of search results produces confusing results where exact matches appear far from the top.

### Session search ranking tiers
- **Decision**: `external_id exact match > external_id prefix match > all other ILIKE matches > created_at desc`. No trigram indexes or FTS.
- **Why**: Sessions are lower cardinality than traces, `external_id` is the primary lookup key, and the three-tier ranking is implementable with a simple CASE expression in ORDER BY.

### Single `session_external_id` field on trace reads
- **Decision**: Add `session_external_id` as an optional string field on both `Trace` and `TraceDetail` API schemas, populated by a `LEFT JOIN sessions` in every trace read query.
- **Why**: Avoids a separate API call to resolve session external IDs, and the join is cheap because `session_id` is already an FK on traces with an existing index.

### One new index only
- **Decision**: Add `(project_id, created_at DESC, id DESC)` on sessions. Reuse existing `(project_id, user_id)` for `user_id` filtering. No new trace indexes.
- **Why**: The existing `idx_sessions_project` is a single-column index that can't serve sorted pagination efficiently. The composite index with `id DESC` as tiebreaker ensures stable offset pagination. Trace queries sort on `COALESCE(start_time, server_received_at)`, which does not have an exact composite index today, but Phase 10 intentionally adds no new trace index unless measurement shows regression from the new sort direction support.

## Risks / Trade-offs
- **Offset pagination at scale**: Large offset values have O(N) seek cost. Acceptable for Phase 10 given session cardinality is low; cursor pagination is a known future improvement.
- **`trace_count` subquery cost**: The derived-table join scans the traces table grouped by session_id. For projects with many traces, this may be slow. Mitigation: the `project_id` filter on the subquery uses the existing `idx_traces_project_session` index.
- **Session search without FTS**: ILIKE-based search won't scale to millions of sessions. Acceptable for current usage; FTS can be added later if needed.

### Session detail trace table state in URL params
- **Decision**: The session detail page (`/sessions/:id`) SHALL manage its trace table state (`offset`, `limit`, `sort_by`, `sort_dir`) via URL search params on the session detail route itself (e.g., `/sessions/abc-def?sort_by=started_at&sort_dir=asc&limit=50&offset=20`). The `returnTo` link captures the full URL including these params, enabling exact restoration of table state when navigating back from trace detail.
- **Why**: Today `SessionDetailPage` keeps `offset` in local `useState`, which is lost on navigation. Moving state to URL params is required for the `returnTo` round-trip promise to be achievable.

### Pagination limits are UI defaults, not API contract changes
- **Decision**: The page-size options `20`, `50`, `100` are UI-only defaults and selector choices. The backend API continues to accept any `limit` value within the existing range (`defaultPageLimit=50`, `maxPageLimit=200`). The UI defaults its page size to `20` but the API does not enforce this whitelist.
- **Why**: Restricting the API to only three limit values would be an unnecessary contract change that could break existing API consumers. The UI can present curated options without constraining the API.

## Open Questions
- None. All decisions are locked per the proposal spec.
