# Change: Add Basic Semantic Timeline Surfaces

## Why

Phase 1 (`add-effect-wait-semantic-foundation`) added wire-level acceptance of `effect` and `wait` event types with deterministic semantic ID derivation. Phase 2 (`add-python-sdk-effect-wait-emission`) added ergonomic `span.effect()` and `span.wait()` helpers to the Python SDK. However, the debugger frontend currently renders these events through the generic fallback branch, `event.message ?? event.event_type.replace(/_/g, ' ')`, losing the structured payload information that makes them useful for debugging. This phase adds dedicated extraction, preview rendering, and filtering for effect and wait events in the timeline, completing the vertical slice from SDK emission to visual surface.

## What Changes

### Effect and wait payload extraction (`eventSemantics.ts`)
- Add `getEffectDetails()` extracting `effect_kind`, `has_external_side_effect`, `effect_id`, `idempotent`, and `idempotency_key` from event payloads
- Add `getWaitDetails()` extracting `wait_kind`, `phase`, `resolution`, and `wait_id` from event payloads
- Both return `null` when required fields are missing or malformed, preserving generic fallback rendering

### Effect and wait summary text (`timeline.ts`)
- Add `effect` and `wait` branches to `summarizeTimelineEvent()` using extracted payload details
- Preserve existing generic fallback when semantic extraction fails

### Effect and wait preview components (`Timeline.tsx`)
- Add `EffectPreview` component parallel to `StateChangePreview`, showing kind and external/read-only distinction as the collapsed-row surface; opaque IDs (`effect_id`, `idempotency_key`) remain accessible via the expanded payload panel
- Add `WaitPreview` component parallel to `DecisionPreview`, showing kind, phase, and resolution when present; `wait_id` remains accessible via the expanded payload panel
- Wire into `TimelineRow` rendering chain after decision check, before generic fallback
- Preserve all existing row behavior: expand/collapse, payload inspection, span navigation

### Timeline segmented filter (`Timeline.tsx`)
- Add a keyboard-accessible segmented control with `All`, `Semantic`, `Effects & waits` modes
- `Semantic` filters to explicit events with `event_type` in `{state_change, decision, effect, wait}`
- `Effects & waits` filters to explicit events with `event_type` in `{effect, wait}`
- Composes with existing `Errors only` toggle by intersection
- Filter state is local to `Timeline`, persists across inspector tab switches, resets on trace navigation
- Six distinct empty-state messages for all filter combinations

## Impact
- Affected specs: `timeline-ui` (extended, adds effect/wait extraction, summary, and preview rendering), `error-timeline-filter` (extended, adds segmented filter, composition, lifecycle, polling, and empty-state requirements)
- Affected code:
  - `web/src/utils/eventSemantics.ts` for new extractors
  - `web/src/utils/timeline.ts` for new summary branches
  - `web/src/components/Timeline.tsx` for new preview components and segmented filter
  - `web/src/utils/eventSemantics.test.ts` for new extractor tests
  - `web/src/utils/timeline.test.ts` for extended summary tests
  - `web/src/components/Timeline.test.tsx` for extended component and filter tests
  - `web/src/pages/TraceDetailPage.test.tsx` for page-level integration tests for filter lifecycle
- No backend, contract, store, migration, mapper, or engine changes
- No changes to `StateDiffViewer`, `stateChanges.ts`, or session-level surfaces
