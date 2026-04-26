## ADDED Requirements

### Requirement: Session Trace Comparison Endpoint

The system SHALL expose a `GET /api/sessions/{id}/compare` endpoint that accepts `baseline_trace_id` and `candidate_trace_id` query parameters and returns a deterministic comparison payload for two terminal traces within the same session.

#### Scenario: Successful comparison of two terminal traces
- **WHEN** a valid request provides two distinct trace IDs that both belong to the specified session and project
- **AND** `NormalizeTraceStatus(status)` returns `completed` or `failed` for both traces
- **THEN** the response status is `200`
- **AND** the response includes session header, baseline trace header, candidate trace header, summary counts and aggregate deltas, and ordered span diff rows with inline semantic detail groups

#### Scenario: Missing required query parameters
- **WHEN** either `baseline_trace_id` or `candidate_trace_id` is absent
- **THEN** the response status is `400`

#### Scenario: Identical trace IDs
- **WHEN** `baseline_trace_id` equals `candidate_trace_id`
- **THEN** the response status is `400`

#### Scenario: Session not found or cross-project access
- **WHEN** the session does not exist or belongs to a different project than the authenticated API key
- **THEN** the response status is `404`

#### Scenario: Trace not in session
- **WHEN** either trace does not belong to the specified session
- **THEN** the response status is `404`

#### Scenario: Non-terminal trace rejected
- **WHEN** `NormalizeTraceStatus(status)` returns `running` for either trace
- **THEN** the response status is `400`

#### Scenario: Comparison too large
- **WHEN** either trace exceeds 500 spans or 1,000 semantic events of types decision, effect, or wait
- **THEN** the response status is `422` with error code `comparison_too_large`
- **AND** the response body uses a dedicated `ComparisonTooLargeError` schema containing a flat `detail` object with `baseline_span_count`, `candidate_span_count`, `baseline_semantic_count`, `candidate_semantic_count`, `max_spans`, and `max_semantic_events`
- **AND** the frontend preserves the `detail` object through the error path so the compare page can display the exceeded counts

### Requirement: Span Matching Algorithm

The system SHALL match spans across baseline and candidate traces using a two-pass deterministic algorithm with explicit provenance metadata on every matched pair.

#### Scenario: Exact span ID match
- **WHEN** a span's external `span_id` exists in both traces
- **THEN** the spans are matched with `match_source: "stable_id"` and `match_reason: "exact_span_id"`

#### Scenario: Heuristic span match within matched parent scope
- **WHEN** an unmatched span has a unique counterpart in the other trace within the same matched-parent scope (or root scope) sharing normalized name, exact kind, and sibling ordinal (using Q2 row order as the canonical ordinal)
- **THEN** the spans are matched with `match_source: "heuristic"` and `match_reason: "name_kind_ordinal"`

#### Scenario: Ambiguous heuristic match leaves spans unmatched
- **WHEN** multiple candidate spans in a scope share the same normalized name, kind, and ordinal as a baseline span
- **THEN** all ambiguous spans remain unmatched rather than guessing

#### Scenario: Unmatched spans reported with correct side
- **WHEN** a span has no match in the other trace
- **THEN** it appears as `baseline_only` or `candidate_only` with null match metadata

### Requirement: Decision Pairing

The system SHALL pair decision events within matched span pairs using normalized question string matching. Only decisions with a non-empty string `question` field in their payload participate in pairing; decisions with missing, non-string, or blank-after-normalization questions are always unpaired.

#### Scenario: Decision with missing or blank question is always unpaired
- **WHEN** a decision event has no `question` field, a non-string `question`, or a `question` that normalizes to an empty string
- **THEN** the decision is never paired and appears as baseline_only or candidate_only

#### Scenario: Unique question match produces a paired decision
- **WHEN** a decision's normalized question (trim, collapse whitespace, lowercase) is non-empty and appears exactly once on both sides of a matched span pair
- **THEN** the decisions are paired and `changed_fields` lists any differing fields among chosen, reasoning, alternatives, and message

#### Scenario: Changed question wording leaves decisions unpaired
- **WHEN** a decision's normalized question does not appear on the other side of the matched span pair
- **THEN** the decision remains unpaired and is shown as baseline_only or candidate_only

#### Scenario: Duplicate questions cause ambiguity
- **WHEN** the same normalized question appears more than once on either side of a matched span pair
- **THEN** all decisions with that question remain unpaired

### Requirement: Effect Pairing

The system SHALL pair effect events within matched span pairs using exact `effect_id` matching only.

#### Scenario: Matching unique effect_id produces a paired effect
- **WHEN** an effect has an `effect_id` in its payload and the same `effect_id` appears exactly once on each side of the matched span pair
- **THEN** the effects are paired and `changed_fields` lists any differing fields

#### Scenario: Duplicate effect_id causes ambiguity
- **WHEN** the same `effect_id` appears more than once on either side of a matched span pair
- **THEN** all effects with that `effect_id` remain unpaired

#### Scenario: Missing effect_id prevents pairing
- **WHEN** an effect lacks an `effect_id` in its payload
- **THEN** the effect is never paired and appears as baseline_only or candidate_only

### Requirement: Wait Pairing

The system SHALL pair wait events within matched span pairs using exact `wait_id` matching when the ID is present and unique within the span pair.

#### Scenario: Matching unique wait_id produces a paired wait
- **WHEN** a wait has a `wait_id` in its payload and the same `wait_id` appears exactly once on each side of the matched span pair
- **THEN** the waits are paired and `changed_fields` lists any differing fields

#### Scenario: Duplicate wait_id causes ambiguity
- **WHEN** the same `wait_id` appears more than once on either side of a matched span pair
- **THEN** all waits with that `wait_id` remain unpaired

#### Scenario: Missing wait_id prevents pairing
- **WHEN** a wait lacks a `wait_id` in its payload
- **THEN** the wait is never paired and appears as baseline_only or candidate_only

### Requirement: Changed Fields Precomputation

The system SHALL precompute `changed_fields` arrays for all matched span and semantic event pairs so the UI does not need to diff raw payloads.

#### Scenario: Matched span with field differences
- **WHEN** two matched spans differ in status, latency_ms, tokens_in, tokens_out, cost_usd, or semantic_count
- **THEN** only the names of differing fields appear in the span row's `changed_fields` array

#### Scenario: Matched span with no differences
- **WHEN** two matched spans have identical values for all compared fields
- **THEN** the `changed_fields` array is empty and `diff_status` is `unchanged`

### Requirement: Session Detail Compare Selection

The session detail page SHALL allow users to select baseline and candidate traces for comparison via URL-backed state.

#### Scenario: Setting baseline and candidate from trace table
- **WHEN** a user selects "Set as baseline" and "Set as candidate" on two different trace rows
- **THEN** the URL updates with `baseline_trace_id` and `candidate_trace_id` params
- **AND** a compare bar appears showing selections with Swap, Clear, and Open comparison actions

#### Scenario: Setting baseline and candidate from storyline cards
- **WHEN** a user selects "Set as baseline" or "Set as candidate" on a storyline card
- **THEN** the URL updates with the corresponding param and the compare bar reflects the selection

#### Scenario: Same trace assigned to both roles
- **WHEN** a user attempts to set the same trace as both baseline and candidate
- **THEN** the second assignment replaces the first — the trace occupies only the newly assigned role and the other role is cleared

#### Scenario: Compare to parent shortcut
- **WHEN** a storyline row's lineage exposes a parent trace in the narrative set
- **AND** both the parent trace and the current trace are terminal (`NormalizeTraceStatus` returns `completed` or `failed`)
- **THEN** a "Compare to parent" action is available that sets the parent as baseline and the current trace as candidate

#### Scenario: Compare to parent hidden for running traces
- **WHEN** either the storyline row's trace or its lineage parent is not terminal
- **THEN** the "Compare to parent" action is not shown

#### Scenario: Running traces cannot be selected for comparison
- **WHEN** a trace has `NormalizeTraceStatus(status)` returning `running`
- **THEN** the "Set as baseline" and "Set as candidate" actions are disabled on both storyline cards and table rows
- **AND** a tooltip or inline hint indicates the trace must complete before it can be compared

#### Scenario: Selected trace outside the current loaded slice is hydrated via bounded lookup
- **WHEN** compare params are loaded from the URL or a user keeps a selected trace while paging away from the selected trace's current table slice
- **AND** the selected trace is not present in the current narrative or paginated table data
- **THEN** the session detail page performs at most one existing trace lookup for that missing selected trace
- **AND** if the lookup resolves to a trace in the current session, the compare bar shows the full selection and the pair remains usable across pagination or refresh
- **AND** "Open comparison" is disabled only while a selected trace is still loading or non-terminal

#### Scenario: Stale selected compare param is removed after bounded lookup
- **WHEN** a bounded selected-trace lookup returns `404` or resolves to a trace whose `session_id` does not match the current session
- **THEN** the corresponding compare param is stripped from the session detail URL

#### Scenario: Malformed UUID in compare params
- **WHEN** a `baseline_trace_id` or `candidate_trace_id` URL param is not a valid UUID
- **THEN** the invalid param is silently stripped from the URL

#### Scenario: Selection works independently of narrative state
- **WHEN** the narrative is loading, errored, or truncated
- **THEN** trace selection via the table remains fully functional

#### Scenario: Existing table URL state preserved alongside compare params
- **WHEN** compare params are added to the URL
- **THEN** existing `sort_by`, `sort_dir`, `limit`, and `offset` params are preserved
- **AND** the query-string canonicalization path (currently `serializeTracesParams`) MUST be extended to include compare params so they survive table-state changes and re-renders

### Requirement: Compare Page

The system SHALL provide a dedicated compare page at `/sessions/:id/compare` that renders the comparison response as an overview section and an ordered span diff section.

#### Scenario: Overview section displays aggregate comparison
- **WHEN** the compare page loads successfully
- **THEN** it shows session identity, baseline and candidate trace headers with status badges, duration/tokens/cost deltas (computed as `candidate - baseline`; positive means candidate is larger/longer), heuristic match count, and links to open each source trace

#### Scenario: Empty diff section
- **WHEN** the comparison response contains zero span diff rows (e.g. both traces have no spans)
- **THEN** the diff section shows an informative empty state message (not a blank section)
- **AND** the overview section still renders normally with aggregate data

#### Scenario: Span diff rows show match status and provenance
- **WHEN** the diff section renders with one or more span diff rows
- **THEN** each span row displays diff status, provenance badge, changed-field chips, and an expand control only when the row contains semantic content

#### Scenario: Inline semantic expansion
- **WHEN** a user expands a span row with semantic content
- **THEN** paired and unpaired decision, effect, and wait items render inline below the row

#### Scenario: No expand affordance for empty semantic rows
- **WHEN** a span row has zero semantic content
- **THEN** no expand control is shown and no misleading empty panel renders

#### Scenario: Candidate-only root branches appear after the baseline tree
- **WHEN** candidate-only spans have no matched parent (root branches)
- **THEN** they appear as a visually distinct group after the baseline-rooted diff rows, preserving their own subtree structure

#### Scenario: All trace-detail links pass returnTo
- **WHEN** a user clicks any trace-detail link from the compare page (overview header links or span-row deep links)
- **THEN** each link uses `/traces/<internal-trace-uuid>` for the pathname
- **AND** `returnTo` is passed via React Router `location.state` pointing back to the current compare URL
- **AND** span-row links additionally include `?span=<external_span_id>` in the query string

#### Scenario: Compare page back navigation fallback
- **WHEN** the compare page's `returnTo` state is missing, malformed, or fails the whitelist (e.g. direct load, refresh, bookmark)
- **THEN** the back button navigates to the parent session detail URL with the current valid `baseline_trace_id` and `candidate_trace_id` params preserved from the compare page URL

#### Scenario: Mobile layout
- **WHEN** the page renders on a narrow viewport
- **THEN** it uses a single-column stacked layout with overview cards first and accordion-style span rows with inline semantic expansion
