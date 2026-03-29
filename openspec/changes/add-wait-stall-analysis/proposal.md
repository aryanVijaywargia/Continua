# Change: Add Wait / Stall Analysis for Running Traces

## Why

The debugger currently shows a single "stale trace" warning when a running trace exceeds both runtime and inactivity thresholds. This is useful but coarse — it cannot distinguish between a trace waiting on a declared external dependency, one blocked on an in-flight model call, one actively executing between spans, and one that is genuinely stalled. Phase 5 adds a frontend-only running-trace classifier that uses wait event pairing, open span inference, and the existing stale heuristic to surface advisory state explanations directly in the trace detail workspace.

This builds on three existing foundations:
- `wait` events already round-trip through ingest and timeline APIs (Phase 3 semantic foundation)
- the debugger already has a stale-running heuristic in `evaluateStaleTraceSignal`
- `useRetrySafetyAnalysis` establishes the poll-stable, signature-cached, client-side classification pattern

## What Changes

### 1. Wait parsing in `eventSemantics.ts`
- Add `WaitDetails` interface and `getWaitDetails(event)` strict parser
- Only `event_type === 'wait'` with non-empty `wait_kind` and `phase` strings qualifies
- Optional `wait_id` and `resolution` fields
- Malformed waits return `null`

### 2. Pure classification utility (`web/src/utils/waitStallAnalysis.ts`)
- `WaitStallClassification`: `declared_wait | waiting_on_model | waiting_on_tool | actively_executing | possibly_stalled | unknown`
- `WaitStallBasis`: `declared | inferred | heuristic`
- `WaitStallReason`: seven reason codes
- `WaitStallAssessment`: classification + basis + reason + decisive evidence + timing fields
- Client-side wait pairing using `wait_id`-only matching on `entered`/`resolved` phases
- Fixed-precedence classifier: declared wait > model span > tool span > active execution > stale heuristic > unknown
- Reuses `evaluateStaleTraceSignal` unchanged for timing and stale detection

### 3. Classification-stable hook (`web/src/pages/useWaitStallAnalysis.ts`)
- Hook that recomputes timing fields on every poll cycle (since `runtimeMs`/`inactivityMs` depend on current time) but produces stable classifications when underlying evidence has not changed
- Returns `WaitStallAssessment | null` (null when trace is not RUNNING or initial snapshot not loaded)
- Recomputation triggers include: span state changes, new wait events, new explicit events (which affect execution evidence and `latestActivityAt`), trace status changes, and stale heuristic flips

### 4. Running-trace span freshness
- Add `refetchInterval` to the spans query in `TraceDetailPage.tsx` while the trace is `RUNNING`, matching the existing timeline poll cadence
- Stops refetching once the trace reaches terminal status (existing invalidation handles that)
- Without this, span-based classifications (`waiting_on_model`, `waiting_on_tool`, `open_generic_span`) would be stale during live execution

### 5. Running-state panel in trace detail
- Replaces `StaleTraceSignalPanel` with generalized `RunningStatePanel`
- Shows classification label, basis, advisory copy, timing, and decisive span jump action
- For `declared_wait`, resolves wait-specific label from `decisiveEventId` + current events

### 6. Timeline wait row rendering
- Well-formed waits: `Entered wait: <kind>` / `Resolved wait: <kind> → <resolution>`
- Phase-only fallback: `<Capitalized phase> wait`
- Generic fallback for malformed waits
- Implementation may use `summarizeTimelineEvent()` or direct component rendering branches; the requirement is behavioral

## Impact
- Affected specs: `wait-stall-classification` (new capability)
- Affected code:
  - `web/src/utils/eventSemantics.ts` — `WaitDetails` + `getWaitDetails()`
  - `web/src/utils/waitStallAnalysis.ts` — new pure utility
  - `web/src/utils/waitStallAnalysis.test.ts` — classifier and pairing tests
  - `web/src/pages/useWaitStallAnalysis.ts` — new hook
  - `web/src/pages/TraceDetailPage.tsx` — replace `StaleTraceSignalPanel` with `RunningStatePanel`; add span refetch interval for running traces
  - `web/src/utils/timeline.ts` and/or `web/src/components/Timeline.tsx` — wait row rendering
  - Component and page test files for touched surfaces
- No backend, API, DB, SDK, ingest, contract, migration, or engine changes
- No changes to `evaluateStaleTraceSignal`, `buildFailureAnalysis`, or failure-first navigation
