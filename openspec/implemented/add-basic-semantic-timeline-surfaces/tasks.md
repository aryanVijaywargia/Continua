## 1. Pre-Implementation Verification
- [x] 1.1 Verify SDK payload source of truth: read `sdks/python/src/continua/span.py` `effect()` and `wait()` methods, confirm the reserved payload keys match the planned extractors (`effect_kind`, `has_external_side_effect`, `effect_id`, `idempotent`, `idempotency_key`, `wait_kind`, `phase`, `resolution`, `wait_id`)
- [x] 1.2 Verify `docs/event-conventions.md` documents effect and wait payload schemas consistently with `span.py`
- [x] 1.3 Verify current frontend baseline: confirm `effect` and `wait` hit the default branch in `summarizeTimelineEvent()` and render via generic fallback in `TimelineRow`

## 2. Effect and Wait Payload Extraction
- [x] 2.1 Add `EffectDetails` interface and `getEffectDetails()` function to `eventSemantics.ts`, guarding on `event_type === 'effect'`, requiring `effect_kind` (string) and `has_external_side_effect` (boolean), with optional `effect_id`, `idempotent`, `idempotency_key`
- [x] 2.2 Add `WaitDetails` interface and `getWaitDetails()` function to `eventSemantics.ts`, guarding on `event_type === 'wait'`, requiring `wait_kind` (string) and `phase` (string), with optional `resolution` and `wait_id`
- [x] 2.3 Add extractor unit tests in `eventSemantics.test.ts` covering well-formed payloads, minimal payloads, missing required fields, wrong event type, missing payload, and incorrect field types

## 3. Effect and Wait Summary Text
- [x] 3.1 Add `effect` handling to `summarizeTimelineEvent()` in `timeline.ts`, formatting as `"{effectKind} (mutating)"` or `"{effectKind} (read-only)"`, with fallback to `event.message ?? 'effect'`
- [x] 3.2 Add `wait` handling to `summarizeTimelineEvent()` in `timeline.ts`, formatting with structured semantics while preserving fallback behavior when extraction fails
- [x] 3.3 Add summary unit tests in `timeline.test.ts` for effect summaries plus existing wait behavior

## 4. Effect and Wait Preview Components
- [x] 4.1 Add `EffectPreview` to `Timeline.tsx`, displaying effect kind and mutating/read-only badge while leaving opaque metadata in the payload panel
- [x] 4.2 Add `WaitPreview` to `Timeline.tsx`, displaying wait kind, phase badge, and optional resolution pill while leaving `wait_id` in the payload panel
- [x] 4.3 Wire `EffectPreview` and `WaitPreview` into `TimelineRow` after decision handling and before the generic fallback
- [x] 4.4 Add component tests in `Timeline.test.tsx` for effect previews, wait previews, malformed payload fallback, payload inspection, and span navigation

## 5. Timeline Segmented Filter
- [x] 5.1 Add `TimelineFilterMode` type and `filterMode` state to `Timeline`
- [x] 5.2 Implement `SegmentedFilter` as a `role="radiogroup"` with three `role="radio"` buttons, `aria-checked` state, and arrow-key navigation
- [x] 5.3 Wire filter logic so `filterMode` composes with `showErrorsOnly` by intersection
- [x] 5.4 Implement the six empty-state messages based on filter combination
- [x] 5.5 Add filter component tests in `Timeline.test.tsx` for default state, filter predicates, intersections, empty states, and loading/error precedence
- [x] 5.6 Add accessibility tests in `Timeline.test.tsx` for radiogroup semantics and arrow-key behavior
- [x] 5.7 Add a polling-aware filter test in `Timeline.test.tsx` so active filters apply to newly appended events

## 6. Page-Level Integration Tests
- [x] 6.1 Add a `TraceDetailPage.test.tsx` test confirming the segmented filter persists across inspector tab switches
- [x] 6.2 Add a `TraceDetailPage.test.tsx` test confirming the segmented filter resets on trace navigation

## 7. Final Verification
- [x] 7.1 Run `pnpm --filter web test` and confirm all new and existing tests pass
- [x] 7.2 Run `go test ./internal/api/...` as a backend invariant check
