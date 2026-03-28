## Context

The debugger renders `effect` events with structured semantic payloads (`effect_kind`, `has_external_side_effect`, `idempotent`, `idempotency_key`) via `getEffectDetails()` in `eventSemantics.ts`. This change adds advisory retry-safety guidance derived from those payloads, scoped to failed traces only. The classification is purely client-side — no backend, API, or storage changes.

Key codebase constraints:
- `getEffectDetails()` returns `null` for any malformation (missing required fields, wrong types, malformed optional fields). The classifier must distinguish "well-formed effect with clear signal" from "effect event where extraction fails."
- `useTraceTimeline` emits a new `events` array reference on each poll cycle even when content hasn't changed. Naive `useMemo` on `events` would recompute every poll.
- `buildFailureAnalysis` and `findPrimaryFailedSpan` are separate concerns. Retry-safety must not alter failure-first navigation.
- Span statuses are `SCHEDULED | STARTED | COMPLETED | FAILED` — no cancelled/error states exist.

## Goals / Non-Goals

- Goals:
  - Surface advisory retry-safety guidance on failed traces
  - Classify individual effect events, aggregate per failed span, aggregate per trace
  - Keep classification stable across timeline poll updates that add non-effect events
  - Use existing `getEffectDetails()` — no independent payload parsing

- Non-Goals:
  - Automated retry or resume
  - Backend retry-safety storage or API fields
  - Classification from `wait`, `decision`, `state_change`, or error events
  - Policy enforcement or blocking
  - Classification on non-failed traces

## Decisions

### Decision: Reuse `getEffectDetails()` with null-means-malformed semantics
`getEffectDetails()` already validates required fields and types. A `null` return on an `effect`-typed event means malformed payload → `unknown / malformed_effect_payload`. Well-formed payloads flow through the classification rules.

Alternatives considered:
- Separate retry-safety parser: rejected — duplicates validation logic, risks divergence with `eventSemantics.ts`.
- Extend `getEffectDetails()` to return partial results: rejected — changes existing contract used by Timeline previews.

### Decision: Stable output across irrelevant poll updates
The hook must produce identical output (classifications, reasons, decisive evidence) when timeline polls add events that don't affect the set of failed spans or their effect events. The implementation strategy (signature-based memoization, deep comparison, etc.) is left to the implementer. The contract is output stability, not a specific caching mechanism.

Rejected alternative approaches for context:
- Naive `useMemo` on `events` array: recomputes every poll since `useTraceTimeline` emits a new array reference.
- Debounced recomputation: introduces stale windows, harder to test.

### Decision: Separate from `buildFailureAnalysis`
Retry-safety and failure analysis serve different purposes (safety guidance vs. failure triage). Merging them would couple their evolution and make the failure analysis function harder to test. The retry-safety hook consumes spans and events independently.

### Decision: Badge component is visual-only
`RetrySafetyBadge` takes classification, variant, and optional `aria-label` props and renders a styled label. No `tooltip` prop — if a surface needs hover text later, it can wrap the badge locally. Explanation text, evidence details, and advisory copy live in the consuming components (FailureSummary, SpanDetail) using shared helper functions from `retrySafety.ts`. This keeps the badge reusable and testable in isolation.

### Decision: Prop threading — page-level aggregation, local row classification
`TraceDetailPage` owns the `useRetrySafetyAnalysis` hook and threads aggregated results down via explicit props. Span-indexed surfaces (`SpanTree`, `ExecutionWaterfall`) receive a `Map<string, RetrySafetyAssessment>` keyed by span id. Single-assessment surfaces (`FailureSummary`, `SpanDetail`) receive a nullable `RetrySafetyAssessment`.

`Timeline` does NOT receive event-level assessments as a prop. It already has `traceStatus` and imports `getEffectDetails`, so it classifies effect rows locally using the shared `classifyEffectEvent()` utility. This avoids redundant prop churn and keeps the Timeline self-contained for row-level rendering.

`FailureSummary` reuses its existing `onJumpToPrimaryFailedSpan: (spanId: string) => void` callback for decisive-span navigation rather than introducing a new callback.

New props on leaf display components are optional with null/empty defaults for backward compatibility. `TraceDetailPage` passes them explicitly at the wiring site — optionality is a component-level contract, not an excuse to skip wiring.

## Risks / Trade-offs

- **Incomplete classification**: effects without `idempotent` or `idempotency_key` produce `unknown`, which may feel unhelpful. Mitigation: explanation text clearly states what's missing, encouraging SDK users to provide richer metadata.
- **Effect-only scope**: classification ignores `wait` resolution, error messages, and trace-level status. This is intentional for Phase 4 — expanding to multi-signal classification is a future phase.
- **Poll signature cost**: building the signature requires iterating effect events each poll. In practice, effect events are a small subset of timeline events, so this is negligible.

## Open Questions

None — the plan is fully specified.
