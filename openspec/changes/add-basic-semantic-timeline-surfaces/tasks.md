## 1. Pre-Implementation Verification
- [x] 1.1 Verify SDK payload source of truth: read `sdks/python/src/continua/span.py` `effect()` and `wait()` methods, confirm the reserved payload keys match the extractors planned (`effect_kind`, `has_external_side_effect`, `effect_id`, `idempotent`, `idempotency_key`, `wait_kind`, `phase`, `resolution`, `wait_id`)
- [x] 1.2 Verify `docs/event-conventions.md` documents effect and wait payload schemas consistently with `span.py`
- [x] 1.3 Verify current frontend baseline: confirm `effect` and `wait` hit the default branch in `summarizeTimelineEvent()` and render via generic fallback in `TimelineRow`

## 2. Effect and Wait Payload Extraction
- [x] 2.1 Add `EffectDetails` interface and `getEffectDetails()` function to `eventSemantics.ts` — guard on `event_type === 'effect'`, require `effect_kind` (string) and `has_external_side_effect` (boolean), optional `effect_id`, `idempotent`, `idempotency_key`; return `null` on any guard failure
- [x] 2.2 Add `WaitDetails` interface and `getWaitDetails()` function to `eventSemantics.ts` — guard on `event_type === 'wait'`, require `wait_kind` (string) and `phase` (string), optional `resolution`, `wait_id`; return `null` on any guard failure
- [x] 2.3 Add extractor unit tests in `eventSemantics.test.ts`: well-formed payloads with all fields, minimal payloads with required fields only, missing required fields return `null`, wrong event type returns `null`, missing payload returns `null`, incorrect field types return `null`

## 3. Effect and Wait Summary Text
- [x] 3.1 Add `effect` case to `summarizeTimelineEvent()` in `timeline.ts`: format as `"{effectKind} (mutating)"` or `"{effectKind} (read-only)"`; fall back to `event.message ?? 'effect'` when extraction returns `null`
- [x] 3.2 Add `wait` case to `summarizeTimelineEvent()` in `timeline.ts`: format as `"{waitKind} ({phase})"` or `"{waitKind} ({phase}) → {resolution}"`; fall back to `event.message ?? 'wait'` when extraction returns `null`
- [x] 3.3 Add summary unit tests in `timeline.test.ts`: effect mutating, effect read-only, effect fallback to message, effect fallback to type name, wait with resolution, wait without resolution, wait fallback to message, wait fallback to type name

## 4. Effect and Wait Preview Components
- [x] 4.1 Add `EffectPreview` component in `Timeline.tsx`: display effect kind as primary text, mutating/read-only badge from `hasExternalSideEffect`; opaque IDs and idempotency metadata stay in expanded payload panel
- [x] 4.2 Add `WaitPreview` component in `Timeline.tsx`: display wait kind as primary text, phase badge, resolution as accent pill when present; `waitId` stays in expanded payload panel
- [x] 4.3 Wire `EffectPreview` and `WaitPreview` into `TimelineRow` rendering chain: after `DecisionPreview` check, before generic text fallback
- [x] 4.4 Add component tests in `Timeline.test.tsx`: effect preview renders kind and mutating badge, effect preview renders read-only badge, wait preview renders kind and phase, wait preview renders resolution pill, malformed payloads degrade to generic rendering

## 5. Timeline Segmented Filter
- [x] 5.1 Add `TimelineFilterMode` type (`'all' | 'semantic' | 'effects-waits'`) and `filterMode` state to `Timeline` component
- [x] 5.2 Implement `SegmentedFilter` as a `role="radiogroup"` with three `role="radio"` buttons, `aria-checked` state, and arrow-key navigation handler
- [x] 5.3 Wire filter logic: compose `filterMode` with `showErrorsOnly` by intersection to compute `visibleEvents`; `semantic` filters to explicit events with `event_type` in `{state_change, decision, effect, wait}`; `effects-waits` filters to `{effect, wait}`
- [x] 5.4 Implement six empty-state messages based on filter combination (see spec)
- [x] 5.5 Add filter component tests in `Timeline.test.tsx`: segmented control defaults to All, selecting Semantic hides synthetic events, selecting Effects & waits shows only effect/wait, combined Semantic + Errors only intersection, combined Effects & waits + Errors only intersection, all six empty-state messages render correctly, loading/error states take precedence over filtered empty states
- [x] 5.6 Add accessibility tests in `Timeline.test.tsx`: radiogroup role present, aria-checked reflects active option, arrow key moves selection
- [x] 5.7 Add polling-aware filter test in `Timeline.test.tsx`: with a filter active, re-render with new events appended (simulating a poll merge) and verify the filter applies to newly arrived events without user interaction

## 6. Page-Level Integration Tests
- [x] 6.1 Add `TraceDetailPage.test.tsx` test: segmented filter persists across inspector tab switches (Timeline → Details → Timeline), parallel to existing `Errors only` persistence coverage
- [x] 6.2 Add `TraceDetailPage.test.tsx` test: segmented filter resets to `All` on trace navigation (route change triggering page remount)

## 7. Final Verification
- [x] 7.1 Run `pnpm --filter web test` and confirm all new and existing tests pass
- [x] 7.2 Run `go test ./internal/api/...` as backend invariant check (no new backend assertions expected)
