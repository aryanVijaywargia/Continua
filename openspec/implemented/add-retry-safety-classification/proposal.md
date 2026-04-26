# Change: Add Retry Safety Classification

## Why

The debugger now surfaces `effect` events with structured semantic payloads (Phase 3: `add-basic-semantic-timeline-surfaces`), but provides no guidance on whether a failed trace can be safely retried. Operators inspecting failures must manually read effect payloads to determine whether external mutations were idempotent. This phase adds a purely advisory, read-only retry-safety classification derived from `effect` metadata, visible only on failed traces and failed spans. No automated retry, no resume, no enforcement — just surfaced guidance to help operators make informed retry decisions.

## What Changes

### Pure classification utility (`web/src/utils/retrySafety.ts`)
- New `RetrySafetyClassification` type: `retryable | unsafe | unknown`
- New `RetrySafetyReason` enum covering seven reason codes
- New `RetrySafetyAssessment` shape carrying classification, reason, decisive span/event evidence, and extracted effect fields
- Event-level classifier using `getEffectDetails()` from `eventSemantics.ts` — no independent payload parsing
- Span-level aggregation across effect events on a single failed span with `unsafe > unknown > retryable` precedence
- Trace-level aggregation across all failed spans with the same precedence, carrying the decisive span identity
- Explanation text mapping from reason codes to fixed human-readable strings

### Stable analysis hook (`web/src/pages/useRetrySafetyAnalysis.ts`)
- Page-level hook backed by the pure utility
- Effect-relevant signature (failed span ids + effect event classifier inputs) prevents recomputation when non-effect timeline events arrive from polling

### Badge component (`web/src/components/RetrySafetyBadge.tsx`)
- Visual-only badge with `Retryable`, `Unsafe`, `Unknown` labels
- Compact and full variants
- Tailwind semantic classes with dark-mode support

### UI surface integration (failure-only)
- Failure summary: aggregated trace retry-safety badge/stat with advisory copy and reason explanation
- Span tree: compact badges on failed spans only
- Execution waterfall: compact badge in left metadata column
- Selected span detail: dedicated `Retry Safety` section for failed spans
- Timeline: effect rows on failed traces render retry-safety badges

### Component prop changes
- `FailureSummary`: new optional `traceRetrySafety: RetrySafetyAssessment | null` prop; reuses existing `onJumpToPrimaryFailedSpan` callback shape for decisive-span navigation (no new callback)
- `SpanTree`: new optional `spanAssessments: Map<string, RetrySafetyAssessment>` prop keyed by span id
- `SpanDetail`: new optional `retrySafety: RetrySafetyAssessment | null` prop
- `ExecutionWaterfall`: new optional `spanAssessments: Map<string, RetrySafetyAssessment>` prop keyed by span id
- `Timeline`: no new props — classifies effect rows locally using the shared `classifyEffectEvent()` utility and its existing `traceStatus` prop to gate badge rendering to failed traces

New props on leaf display components (`SpanTree`, `SpanDetail`, `ExecutionWaterfall`, `FailureSummary`) are optional with null/empty defaults for backward compatibility with existing test harnesses. `TraceDetailPage` passes them explicitly at the wiring site.

## Impact
- Affected specs: `retry-safety-classification` (new capability)
- Affected code:
  - `web/src/utils/retrySafety.ts` — new pure utility
  - `web/src/utils/retrySafety.test.ts` — classifier and aggregation tests
  - `web/src/pages/useRetrySafetyAnalysis.ts` — new hook
  - `web/src/components/RetrySafetyBadge.tsx` — new badge component
  - `web/src/components/FailureSummary.tsx` — trace-level badge integration (new props)
  - `web/src/components/SpanTree.tsx` — failed span badge integration (new props)
  - `web/src/components/SpanDetail.tsx` — retry safety section (new props)
  - `web/src/components/ExecutionWaterfall.tsx` — waterfall label badge integration (new props)
  - `web/src/components/Timeline.tsx` — effect row badge integration (local classification, no new props)
  - `web/src/pages/TraceDetailPage.tsx` — hook wiring and prop threading
  - Component and page-level test files for all touched surfaces
- No backend, API, DB, SDK, ingest, contract, migration, or engine changes
- No changes to `findPrimaryFailedSpan()`, `buildFailureAnalysis`, or failure-first navigation
