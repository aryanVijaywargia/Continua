## 1. Wait parsing foundation
- [x] 1.1 Add `WaitDetails` interface and `getWaitDetails(event)` to `web/src/utils/eventSemantics.ts`
- [x] 1.2 Add unit tests for `getWaitDetails` in `web/src/utils/eventSemantics.test.ts` (well-formed, minimal, malformed, non-wait)

## 2. Pure classification utility
- [x] 2.1 Create `web/src/utils/waitStallAnalysis.ts` with types: `WaitStallClassification`, `WaitStallBasis`, `WaitStallReason`, `WaitStallAssessment`
- [x] 2.2 Implement `computeOpenWaits()` wait-pairing function using `wait_id`-only matching on entered/resolved phases; utility sorts events internally using the existing timeline comparator
- [x] 2.3 Implement `classifyRunningTrace()` with the six-level fixed-precedence classifier calling `evaluateStaleTraceSignal` for timing and stale detection
- [x] 2.4 Add comprehensive tests in `web/src/utils/waitStallAnalysis.test.ts`:
  - open entered wait → `declared_wait`
  - entered + resolved with same `wait_id` → wait closes
  - resolved before entered self-heals on recomputation
  - waits without `wait_id` never close other waits
  - duplicate `wait_id`: one resolve closes earliest unmatched enter (FIFO), later enter remains open
  - duplicate `wait_id`: two resolves close both enters, zero open waits
  - unsorted input produces correct pairing (utility sorts internally)
  - unsupported phases do not affect pairing
  - open `LLM` span → `waiting_on_model`
  - open `TOOL` span → `waiting_on_tool`
  - open `AGENT`/`CHAIN`/`CUSTOM` span → `actively_executing` with `open_generic_span`
  - no open spans but recent activity → `actively_executing` with `recent_activity_without_open_span`
  - zero spans and zero events → `unknown`
  - scheduled-only trace → `unknown`
  - stale heuristic true with no stronger signal → `possibly_stalled`
  - precedence: declared wait beats model/tool, model beats tool, generic span beats stale, stale beats unknown
  - tie-break: two open spans of same kind with equal `started_at` selects last in input array
- [x] 2.5 Add decisive evidence assertions to classification tests:
  - `declared_wait` sets `decisiveEventId` (and `decisiveSpanId`/`decisiveSpanName` when the wait belongs to a known span)
  - `waiting_on_model` sets `decisiveSpanId` and `decisiveSpanName` to the latest-started open `LLM` span
  - `waiting_on_tool` sets `decisiveSpanId` and `decisiveSpanName` to the latest-started open `TOOL` span
  - `actively_executing` with `open_generic_span` sets `decisiveSpanId`/`decisiveSpanName`; with `recent_activity_without_open_span` leaves them unset
  - `possibly_stalled` and `unknown` leave `decisiveSpanId`, `decisiveSpanName`, and `decisiveEventId` unset
- [x] 2.6 Add null-gating tests:
  - hook returns `null` when trace status is `COMPLETED`
  - hook returns `null` when trace status is `FAILED`
  - hook returns `null` when initial timeline snapshot has not loaded

## 3. Classification-stable hook
- [x] 3.1 Create `web/src/pages/useWaitStallAnalysis.ts` with hook that recomputes timing on every cycle but produces stable classifications when evidence has not changed
- [x] 3.2 Add classification-stability and recomputation tests:
  - classification stays stable when no evidence changes and stale heuristic has not flipped
  - timing fields (`runtimeMs`, `inactivityMs`) update on every poll cycle even without data changes
  - time advance that flips stale heuristic transitions classification to `possibly_stalled`
  - new explicit event (e.g., `log`) triggers recomputation and can change classification (e.g., `unknown` → `actively_executing`)
  - new wait event triggers recomputation
  - span status change triggers recomputation

## 4. Running-trace span freshness
- [x] 4.1 Add `refetchInterval` to the spans query in `TraceDetailPage.tsx` while the trace is `RUNNING`, matching the existing timeline poll cadence (`TIMELINE_POLL_INTERVAL_MS`)
- [x] 4.2 Verify refetching stops once trace reaches terminal status
- [x] 4.3 Add test coverage: new span or span status change becomes visible within one poll cycle during live execution

## 5. Timeline wait row rendering
- [x] 5.1 Implement wait row rendering (well-formed → phase-only fallback → generic fallback) in the appropriate location (summary helper and/or `Timeline.tsx` component)
- [x] 5.2 Add helper-level tests for wait summary text (well-formed waits, phase-only waits, malformed waits, non-wait rows)
- [x] 5.3 Add component-level timeline tests verifying that `wait` event rows render the correct text in the actual `Timeline.tsx` component, covering well-formed, phase-only, and malformed cases

## 6. Running-state panel in trace detail
- [x] 6.1 Replace `StaleTraceSignalPanel` with `RunningStatePanel` in `web/src/pages/TraceDetailPage.tsx`
- [x] 6.2 Wire `useWaitStallAnalysis` hook into `TraceDetailPage` and pass assessment to `RunningStatePanel`
- [x] 6.3 Implement panel UI: classification label, basis, advisory copy, timing, decisive span jump action, declared-wait label resolution from `decisiveEventId`

## 7. Test updates for replaced stale panel
- [x] 7.1 Update or remove existing test assertions referencing "Experimental stale trace signal", the old stale-only copy, or `StaleTraceSignalPanel`
- [x] 7.2 Add trace detail tests:
  - running-state panel appears only for running traces after initial snapshot
  - panel does not appear for completed or failed traces
  - declared wait resolves wait-specific text from decisive event
  - model/tool states expose correct jump-to-span action using `decisiveSpanId`
  - active-between-spans renders `actively_executing`, not `unknown`
  - stale running trace with no stronger state renders `possibly_stalled`
  - unknown renders conservative copy
  - panel correctly handles `null` assessment (non-running or pre-snapshot)

## 8. Final verification
- [x] 8.1 Run `pnpm --filter web test` and confirm all tests pass
- [x] 8.2 Run `pnpm --filter web build` and confirm no build errors
