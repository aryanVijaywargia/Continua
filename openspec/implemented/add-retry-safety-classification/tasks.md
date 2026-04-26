## 1. Pre-Implementation Verification
- [x] 1.1 Verify `getEffectDetails()` in `eventSemantics.ts` returns `null` for malformed payloads and confirm the `EffectDetails` interface shape matches classifier expectations (`effectKind`, `hasExternalSideEffect`, `idempotent?`, `idempotencyKey?`)
- [x] 1.2 Verify `buildFailureAnalysis` and `findPrimaryFailedSpan` in `failureAnalysis.ts` — confirm retry-safety will not alter these functions or their call sites
- [x] 1.3 Verify `useTraceTimeline` poll behavior — confirm it emits a new `events` array reference on each poll even when content is unchanged, validating the need for an effect-relevant signature
- [x] 1.4 Verify span status enum — confirm only `SCHEDULED | STARTED | COMPLETED | FAILED` are used in `client.ts` and downstream components

## 2. Pure Classification Utility
- [x] 2.1 Create `web/src/utils/retrySafety.ts` with `RetrySafetyClassification` type (`retryable | unsafe | unknown`), `RetrySafetyReason` type (seven reason codes), and `RetrySafetyAssessment` interface
- [x] 2.2 Implement `classifyEffectEvent()` — takes a `TimelineEvent`, returns `null` for non-effect events, calls `getEffectDetails()` for effect events and returns `RetrySafetyAssessment` following the six classification rules
- [x] 2.3 Implement `assessSpanRetrySafety()` — takes a failed span's effect events, aggregates with `unsafe > unknown > retryable` precedence, selects latest event within winning class as evidence, returns `unknown / no_effect_events` when no effects present
- [x] 2.4 Implement `assessTraceRetrySafety()` — takes span-level assessments from all failed spans, aggregates with same precedence, carries decisive span identity
- [x] 2.5 Implement `getReasonExplanation()` — maps each reason code to fixed explanation text
- [x] 2.6 Implement `getAccessibleSummary()` — returns `"Retry safety advisory: {classification}. Inferred from effect metadata."`

## 3. Classification Utility Tests
- [x] 3.1 Add `web/src/utils/retrySafety.test.ts` with event-level tests: all six classification branches including `read_only_effect`, `mutating_non_idempotent`, `mutating_idempotent_with_key`, `mutating_missing_idempotent`, `mutating_idempotent_missing_key`, `malformed_effect_payload`, plus non-effect event returns `null`
- [x] 3.2 Add span-level aggregation tests: single effect, mixed effects with precedence, no effects → `no_effect_events`, latest event wins within same class, non-failed spans excluded
- [x] 3.3 Add trace-level aggregation tests: single failed span, multiple failed spans with precedence, decisive span differs from primary, no assessable failed spans → `unknown`
- [x] 3.4 Add explanation text tests: each reason maps to correct string, accessible summary interpolates classification

## 4. Poll-Stable Analysis Hook
- [x] 4.1 Create `web/src/pages/useRetrySafetyAnalysis.ts` — build effect-relevant signature from failed span ids and per-effect classifier inputs, memoize on signature
- [x] 4.2 Add hook tests verifying: non-effect poll updates produce stable classifications/reasons/evidence, new effect events trigger recomputation, span status changes trigger recomputation

## 5. Badge Component
- [x] 5.1 Create `web/src/components/RetrySafetyBadge.tsx` — visual-only badge with `classification`, `variant` (compact/full), and optional `aria-label` props; Tailwind semantic colors with dark-mode variants
- [x] 5.2 Add badge component tests: renders correct label for each classification, applies correct color classes, supports compact and full variants, passes through aria-label, no hardcoded hex colors in output

## 6. Failure Summary Integration
- [x] 6.1 Add trace-level retry-safety badge/stat to `FailureSummary.tsx` — show only when trace is failed, display advisory copy and reason explanation
- [x] 6.2 When decisive span differs from primary span, show explicit text naming the decisive span and a secondary action to navigate to it via the existing `onJumpToPrimaryFailedSpan` callback
- [x] 6.3 Add failure summary tests: retryable/unsafe/unknown badge renders with correct explanation, decisive-span-differs case shows navigation action, non-failed trace shows no retry-safety surface

## 7. Span Tree Integration
- [x] 7.1 Add compact retry-safety badges to failed span nodes in `SpanTree.tsx` — keep existing `Selected`, `Failure path`, and `Failed` chips unchanged
- [x] 7.2 Add span tree tests: failed span shows compact badge, completed span shows no badge, existing chips remain

## 8. Execution Waterfall Integration
- [x] 8.1 Add compact retry-safety badge to left label container of failed spans in `ExecutionWaterfall.tsx` — badge inside label container not timing bar, span name retains truncation class, badge has `whitespace-nowrap`
- [x] 8.2 Add waterfall tests: badge rendered within label container for failed spans, span name container has truncation class, badge element has non-wrapping class, no badge on non-failed spans

## 9. Selected Span Detail Integration
- [x] 9.1 Add `Retry Safety` section to `SpanDetail.tsx` for failed spans — badge, advisory copy, reason explanation, supporting semantic fields
- [x] 9.2 Add span detail tests: retryable/unsafe/unknown sections with correct content, non-failed span shows no section

## 10. Timeline Effect Row Integration
- [x] 10.1 Add retry-safety badge to effect rows in `Timeline.tsx` on failed traces only — classify locally using `classifyEffectEvent()` and existing `traceStatus` prop; well-formed rows show badge alongside `EffectPreview`, malformed rows show `Unknown` badge with generic text (explanation accessible via expanded panel only)
- [x] 10.2 Add timeline tests: badge on retryable/unsafe/unknown effects on failed traces, malformed effect shows Unknown, no badges on completed/running traces, non-effect rows unchanged

## 11. Page-Level Wiring and Integration Tests
- [x] 11.1 Wire `useRetrySafetyAnalysis` in `TraceDetailPage.tsx` and thread assessments to FailureSummary, SpanTree, SpanDetail, and ExecutionWaterfall (Timeline classifies locally, no prop threading needed)
- [x] 11.2 Add page-level integration tests: consistent classifications across failure summary, span tree, waterfall, span detail, and timeline effect rows
- [x] 11.3 Add poll-stability regression test at page level: aggregate results stable when non-effect events arrive

## 12. Theme and Accessibility Verification
- [x] 12.1 Add theming regression check: badge styles work in both light and dark themes at component level
- [x] 12.2 Verify accessible labels propagate correctly across all surfaces

## 13. Final Verification
- [x] 13.1 Run `pnpm --filter web test` and confirm all new and existing tests pass
