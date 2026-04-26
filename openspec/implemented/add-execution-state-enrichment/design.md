## Context

Phase 9 enriches two existing debugger surfaces without introducing new backend schema, migrations, or runtime semantics. The two tracks — unresolved-wait rows and `snapshot_marker` — are independent but ship together because both extend the trace-detail experience for running traces.

Existing foundations:
- `computeOpenWaits()` in `web/src/utils/waitStallAnalysis.ts` already derives paired/unpaired open waits from timeline events
- `classifyRunningTrace()` and `useWaitStallAnalysis` already power the `RunningStatePanel` summary
- The semantic event pipeline (contract -> mapper -> timeline -> frontend) is proven for `state_change`, `decision`, `effect`, and `wait`

## Goals / Non-Goals

- Goals:
  - Surface all unresolved waits in the running-state panel so operators see the full wait picture
  - Add `snapshot_marker` as a first-class debugger milestone type end-to-end
  - Keep changes read-only and debugger-scoped

- Non-Goals:
  - Turn Lens or turn-boundary semantics
  - Session-level marker surfaces or compare-time marker logic
  - DB schema changes or new store queries
  - Checkpoint/resume/replay semantics for markers
  - Ingest-time validation warnings for snapshot_marker payloads
  - Waterfall overlay or milestone aggregation views

## Decisions

### 1. `snapshot_marker` is convention-documented, not schema-enforced
- `marker_kind` and `label` are documented as conventionally required but not OpenAPI-required
- Raw callers omitting them get generic rendering, not ingest rejection
- This matches the existing permissive ingest model for `effect`, `wait`, `state_change`, `decision`
- Alternative considered: schema-required fields — rejected because it would break the established permissive pattern and add ingest-time validation for a debugger-only event type

### 2. Unresolved-wait rows reuse `computeOpenWaits()` unchanged
- The existing function already handles `wait_id` pairing, anonymous waits, and phase filtering
- `TraceDetailPage` derives `openWaits` via `useMemo` and passes the array to `RunningStatePanel`
- Alternative considered: new dedicated hook — rejected because the computation is a simple `useMemo` over already-available data and doesn't need its own stability/caching logic beyond what `useMemo` provides

### 3. Open-wait duration refresh uses existing poll cadence only
- No `setInterval` or `requestAnimationFrame` for duration updates
- Durations refresh when the timeline poll returns new data and triggers re-render
- This avoids timer proliferation and matches the existing `useWaitStallAnalysis` timing model
- Alternative considered: per-second timer for live-updating durations — rejected because it adds complexity and visual noise without improving debugging actionability

### 4. `snapshot_marker` included in Semantic filter, no new filter mode
- `snapshot_marker` joins `state_change`, `decision`, `effect`, `wait` in the existing `Semantic` filter
- No dedicated "Milestones" filter mode
- Alternative considered: separate filter — rejected because milestone events are expected to be infrequent and adding a fourth filter mode fragments the UI for minimal benefit

### 5. No backend ingest warnings for `snapshot_marker`
- `state_change` and `decision` have processor warnings for missing semantic fields
- `snapshot_marker` skips this because: marker_kind/label are convention, not contract; the frontend gracefully degrades malformed markers; and adding ingest-time log warnings for a single optional type adds maintenance cost without user benefit
- Alternative considered: parity with state_change warnings — rejected for the reasons above

## Risks / Trade-offs

- **Risk:** `snapshot_marker` payloads vary widely across teams
  - Mitigation: well-formed/malformed distinction in frontend extraction; generic fallback rendering for anything that doesn't have both `marker_kind` and `label`
- **Risk:** many open waits could make `RunningStatePanel` tall
  - Mitigation: waits are newest-first with the current gate prominent; in practice, traces rarely accumulate many concurrent waits; future phases can add collapsing if needed

## Open Questions

None — scope is intentionally narrow and builds entirely on proven patterns.
