## ADDED Requirements

### Requirement: Session Narrative Endpoint

The system SHALL expose `GET /api/sessions/{id}/narrative` that returns a capped, chronologically ordered narrative view of a session including a summary, per-trace detail with activity timestamps and lineage, and semantic event snapshots.

The endpoint SHALL be protected by API-key authentication and scoped to the authenticated project.

#### Scenario: Successful narrative for session with traces
- **WHEN** a valid session UUID is provided for a session belonging to the authenticated project
- **THEN** the response status is `200`
- **AND** the response body conforms to the `SessionNarrativeResponse` schema
- **AND** `summary.returned_trace_count` is at most `100`
- **AND** `traces` are ordered oldest-first by `started_at`
- **AND** ties on `started_at` are broken by internal trace `id` ascending

#### Scenario: Session with zero traces
- **WHEN** a valid session UUID is provided but the session has no traces
- **THEN** the response status is `200`
- **AND** `summary.total_trace_count` is `0`
- **AND** `summary.returned_trace_count` is `0`
- **AND** `summary.truncated` is `false`
- **AND** `summary.started_at` is `null`
- **AND** `summary.last_activity_at` is `null`
- **AND** `traces` is an empty array

#### Scenario: Session not found
- **WHEN** the session UUID does not exist or does not belong to the authenticated project
- **THEN** the response status is `404`

#### Scenario: Cross-project access denied
- **WHEN** the session UUID belongs to a different project than the authenticated API key
- **THEN** the response status is `404`

### Requirement: Session Narrative Summary

The `SessionNarrativeSummary` SHALL include aggregate metrics computed over all traces in the session (not just the returned set) for count and cost fields, plus session-level timing.

The summary SHALL contain:
- `total_trace_count`: total traces in the session
- `returned_trace_count`: traces included in the response (at most 100)
- `truncated`: `true` when `total_trace_count > returned_trace_count`
- `running_trace_count`, `completed_trace_count`, `failed_trace_count`: status breakdowns over all session traces using the same normalization semantics as the API mapper (`completed|ok => completed`, `failed|error|cancelled => failed`, any other or null status => running)
- `total_cost_usd`, `total_tokens_in`, `total_tokens_out`: aggregates over all session traces
- `started_at`: earliest `COALESCE(trace.start_time, trace.server_received_at)` across all session traces, or `null` if no traces
- `last_activity_at`: latest trace-level activity across all session traces (computed from `trace.server_received_at` and `trace.end_time` only — does not include span or event timestamps for cost reasons), or `null` if no traces. This is approximate; the per-trace `latest_activity_at` is authoritative
- `explicit_link_count`, `inferred_link_count`, `unlinked_trace_count`: lineage classification counts over the returned narrative set only

#### Scenario: Summary aggregation across multiple traces
- **WHEN** a session contains 5 traces with known cost, token, and status values
- **THEN** `total_trace_count` is `5`
- **AND** `total_cost_usd` is the sum of all 5 traces' costs
- **AND** status counts sum to `5`

#### Scenario: Truncation with more than 100 traces
- **WHEN** a session contains 150 traces
- **THEN** `total_trace_count` is `150`
- **AND** `returned_trace_count` is `100`
- **AND** `truncated` is `true`
- **AND** `traces` contains the oldest 100 traces by `started_at`, with `id` ascending as the tie-breaker

#### Scenario: Unknown raw trace status still counts as running
- **WHEN** a session contains a trace whose raw DB `status` is not one of `completed`, `ok`, `failed`, `error`, or `cancelled`
- **THEN** that trace contributes to `running_trace_count`
- **AND** it does not contribute to `completed_trace_count` or `failed_trace_count`

### Requirement: Session Narrative Trace Detail

Each `SessionNarrativeTrace` in the response SHALL contain per-trace detail including activity timing and lineage classification.

Fields:
- `id`: internal trace UUID
- `trace_id`: external trace identifier
- `name`, `status`, `user_id`: trace metadata
- `started_at`: `COALESCE(trace.start_time, trace.server_received_at)`
- `ended_at`: trace end time (nullable)
- `duration_ms`: computed from `ended_at - started_at` (nullable if `ended_at` is null)
- `error_count`: trace-level error count
- `total_cost_usd`, `total_tokens_in`, `total_tokens_out`: per-trace aggregates
- `latest_activity_at`: the most recent timestamp across `trace.server_received_at`, `trace.end_time`, all related `span.start_time`, `span.end_time`, `span_events.event_ts`, and `span_events.server_ingested_at`
- `semantic_events`: array of `TimelineEvent` objects filtered to explicit `decision`, `effect`, and `wait` event types; because `TimelineEvent` is reused, each nested `semantic_events[].trace_id` remains the internal trace UUID rather than the external trace identifier
- `lineage`: a `SessionNarrativeLineage` object

#### Scenario: latest_activity_at includes span and event timestamps
- **WHEN** a trace has `end_time` at T1, a span ending at T2 > T1, and an event at T3 > T2
- **THEN** `latest_activity_at` is T3

#### Scenario: Semantic events filtered and ordered
- **WHEN** a trace has explicit events of types `decision`, `effect`, `wait`, `log`, and `error`
- **THEN** `semantic_events` contains only the `decision`, `effect`, and `wait` events
- **AND** they are ordered by timestamp ascending, then sequence ascending

#### Scenario: Stable trace ordering when timestamps tie
- **WHEN** two returned traces have the same effective `started_at`
- **THEN** the response orders them by internal trace `id` ascending

### Requirement: Session Narrative Lineage Resolution

The system SHALL classify each returned narrative trace with a lineage type of `explicit`, `inferred`, or `unlinked`.

Lineage precedence:
1. Explicit metadata link from `trace.metadata.__continua_lineage` with a valid `parent_trace_id` that resolves within the returned session set
2. Conservative inference based on chronological ordering
3. Otherwise `unlinked`

#### Scenario: Inference for clean sequential traces
- **WHEN** trace B starts strictly after trace A's `latest_activity_at`
- **AND** no other returned trace has activity overlapping trace B's start
- **THEN** trace B's lineage type is `inferred` with trace A as parent

#### Scenario: No inference when overlapping activity exists
- **WHEN** trace C starts while trace A still has activity (`A.latest_activity_at >= C.started_at`)
- **THEN** trace C's lineage type is `unlinked`

#### Scenario: No inference when timestamps are missing
- **WHEN** a trace has null `started_at` or its predecessor has null `latest_activity_at`
- **THEN** the trace's lineage type is `unlinked`

#### Scenario: First trace is always unlinked
- **WHEN** a trace is the first (oldest) in the returned set
- **THEN** its lineage type is `unlinked`

### Requirement: Explicit Lineage Metadata Convention

The system SHALL support an explicit lineage convention via `trace.metadata.__continua_lineage`.

The expected shape is:
```json
{
  "__continua_lineage": {
    "parent_trace_id": "<external trace_id>",
    "trigger_span_id": "<optional>",
    "link_kind": "<optional>"
  }
}
```

- `parent_trace_id` uses the external `trace_id` (not internal UUID)
- Malformed metadata (wrong types, missing `parent_trace_id`) SHALL be silently ignored
- Links that do not resolve within the same project and returned session set SHALL be ignored
- Explicit links SHALL take precedence over inferred links

#### Scenario: Valid explicit metadata lineage
- **WHEN** trace B has `metadata.__continua_lineage.parent_trace_id` pointing to trace A's external `trace_id`
- **AND** trace A is in the returned session set
- **THEN** trace B's lineage type is `explicit` with trace A as parent

#### Scenario: Malformed metadata ignored
- **WHEN** trace B has `metadata.__continua_lineage` with `parent_trace_id` as a number instead of a string
- **THEN** the explicit metadata is ignored and inference or `unlinked` applies

#### Scenario: Out-of-session metadata ignored
- **WHEN** trace B has `metadata.__continua_lineage.parent_trace_id` pointing to a trace not in the returned session set
- **THEN** the explicit metadata is ignored and inference or `unlinked` applies

### Requirement: Session Narrative Query Efficiency

The `BuildSessionNarrative` store method SHALL use at most 3 SQL round trips in the default implementation. The successful `200` path uses 3 queries by default; missing/cross-project requests may return after Query 1. A later 2-round-trip fused version is acceptable only as a follow-up optimization if it preserves the same semantics and remains maintainable.

The default query shape SHALL be:
1. Session-scoped summary query anchored on `sessions` with a `LEFT JOIN` to `traces`, so zero-trace sessions are distinguishable from missing/cross-project sessions within the same query
2. Capped trace query returning the oldest 100 traces with per-trace `latest_activity_at` computed via span and event CTEs
3. Semantic event query for returned trace IDs only, filtered to `decision`, `effect`, `wait`

#### Scenario: Query plan stays within round-trip budget
- **WHEN** `BuildSessionNarrative` is called for any session
- **THEN** no more than 3 SQL queries are executed against the database in the default implementation

#### Scenario: Missing session may short-circuit after the summary query
- **WHEN** the narrative store method is called for a missing session or a session outside the authenticated project
- **THEN** the implementation may return after Query 1
- **AND** Queries 2 and 3 are not required for the `404` path

#### Scenario: Existing zero-trace session is distinguished from missing session within the query budget
- **WHEN** the narrative store method is called for an existing session with zero traces
- **THEN** the summary query returns a row with `total_trace_count = 0`
- **AND** the API returns `200` with an empty narrative
- **AND** no additional session-existence query is required

### Requirement: Session Narrative Frontend Summary

The `SessionDetailPage` SHALL render a narrative summary section above the existing trace table.

The summary section SHALL display:
- Returned vs total trace counts
- Running, completed, and failed trace counts
- Aggregate cost and token totals
- Session `started_at` and `last_activity_at`
- Lineage coverage (explicit, inferred, unlinked counts), labeled as applying to the shown narrative only and, when truncated, to the first 100 traces

The summary section SHALL have independent loading and error states that do not affect the existing session header or trace table, reusing the page's existing local containment patterns.

#### Scenario: Summary renders above unchanged trace table
- **WHEN** the narrative loads successfully
- **THEN** the summary section appears above the trace table
- **AND** the trace table continues to function with its own query and URL state

#### Scenario: Narrative loading does not block the page
- **WHEN** the narrative query is loading
- **THEN** a skeleton or loading indicator is shown in the summary/storyline area
- **AND** the existing session header and trace table render normally

#### Scenario: Narrative error is contained
- **WHEN** the narrative query fails
- **THEN** an inline error is shown in the summary/storyline area
- **AND** the existing session header and trace table remain functional

#### Scenario: Lineage coverage disclosure on truncated sessions
- **WHEN** `summary.truncated` is `true`
- **THEN** the summary labels lineage coverage as applying to the shown narrative only
- **AND** the copy explicitly indicates the counts cover the first 100 traces

### Requirement: Session Narrative Frontend Storyline

The `SessionDetailPage` SHALL render a storyline section between the summary and the trace table, showing returned traces oldest-first.

Each storyline card SHALL display:
- Trace name and external `trace_id`
- Status badge
- Timing (`started_at`, `ended_at`) and duration
- Cost, token, and error totals
- Lineage badge: `Explicit`, `Inferred`, or `Unlinked`
- Optional one-line latest semantic snippet derived client-side from the returned `semantic_events`

The storyline SHALL NOT include retry-safety badges, wait/stall chips, or other per-card trace-debugger widgets. Storyline links SHALL reuse the existing trace-table navigation pattern, including `returnTo` preservation.

#### Scenario: Storyline ordering oldest-first
- **WHEN** the narrative contains traces with different `started_at` timestamps
- **THEN** storyline cards are rendered in ascending `started_at` order

#### Scenario: Lineage badges rendered correctly
- **WHEN** a trace has lineage type `inferred`
- **THEN** the storyline card shows an `Inferred` lineage badge

#### Scenario: Truncation banner shown
- **WHEN** `summary.truncated` is `true`
- **THEN** a banner appears above the storyline indicating the narrative is limited to the first 100 traces and the table below is the full browser

#### Scenario: Zero-trace empty state
- **WHEN** the session has zero traces
- **THEN** a compact narrative placeholder renders above the existing empty trace-table state
- **AND** the page does not render a second full-size empty-state card

#### Scenario: Storyline navigation preserves returnTo
- **WHEN** a user clicks a trace name in the storyline
- **THEN** they navigate to the trace detail page
- **AND** the current session URL (including search params) is preserved as `returnTo` in navigation state

### Requirement: Session Narrative Polling

The narrative query SHALL poll every 30 seconds only while the latest response reports `running_trace_count > 0`.

#### Scenario: Polling while running traces exist
- **WHEN** the narrative response has `running_trace_count > 0`
- **THEN** the narrative query refetches every 30 seconds

#### Scenario: Polling stops when no running traces
- **WHEN** the narrative response has `running_trace_count` equal to `0`
- **THEN** the narrative query does not automatically refetch

#### Scenario: Polling respects normalized running counts
- **WHEN** the narrative response includes traces whose raw DB statuses normalize to running
- **THEN** those traces are included in `running_trace_count`
- **AND** the polling decision follows that normalized count
