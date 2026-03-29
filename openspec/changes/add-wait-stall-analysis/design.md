## Context

The debugger's running-trace experience currently consists of a single binary stale-trace warning panel. Phase 5 replaces this with a multi-level advisory classifier that explains *why* a running trace appears to be in its current state. The classifier is frontend-only and advisory — it never blocks user actions, never triggers backend changes, and always prefers explicit or inferred evidence over heuristic guesses.

Key constraints:
- No backend changes; all data already arrives via existing timeline and span APIs
- Must produce stable classifications during polling (timing fields update every cycle, but the classification itself must not flicker when evidence has not changed)
- Must compose with the existing stale heuristic, not replace or refactor it
- Must avoid recomputation on irrelevant poll churn (the `useRetrySafetyAnalysis` signature-cache pattern is a good default, but the mechanism is an implementation choice)

## Goals / Non-Goals

**Goals:**
- Surface advisory running-state explanations in trace detail
- Parse `wait` events for declared wait detection
- Infer wait state from open span kinds (LLM, TOOL)
- Distinguish active execution from genuine stalls
- Improve timeline rendering for `wait` event rows

**Non-Goals:**
- Automated stall recovery or retry
- Backend storage of classification state
- Tree/waterfall/list-page badge surfaces (deferred)
- Incremental wait pairing optimization (deferred unless perf proves necessary)
- Changing stale threshold values

## Decisions

### Classification precedence is fixed, not configurable
The six-level precedence (declared wait > model > tool > active > stale > unknown) is hardcoded. Explicit evidence always wins over inferred, which always wins over heuristic. This avoids configuration complexity and makes the classifier deterministic.

**Alternatives considered:** Weighted scoring, user-configurable thresholds. Rejected because the classifier is advisory and the precedence is already well-motivated by evidence strength.

### Wait pairing uses `wait_id` only, never fuzzy matching
A resolved wait must carry the same `wait_id` as the entered wait to close it. Waits without `wait_id` are never paired. This is strict but predictable.

**Alternatives considered:** Fuzzy matching by `wait_kind` + span affinity. Rejected because false closures are worse than unclosed waits for an advisory classifier.

### Open generic spans stay `actively_executing` even when stale thresholds are exceeded
An `AGENT`, `CHAIN`, or `CUSTOM` span that is still `STARTED` is stronger evidence of activity than the absence of recent events. Only when there are no open spans at all does the stale heuristic become the deciding factor.

**Alternatives considered:** Letting stale thresholds override open generic spans. Rejected because this would produce confusing flip-flops between `actively_executing` and `possibly_stalled` for long-running agent spans.

### `decisiveEventId` is kept on the assessment for UI label resolution
Instead of copying wait payload fields into the assessment, the assessment carries `decisiveEventId` so the panel can look up the event from the current event list. This keeps the assessment shape stable and avoids payload-field drift between the utility and the UI.

### Add span refetching for running traces
The current `TraceDetailPage` fetches spans once and only invalidates on terminal status. Span-based classifications (`waiting_on_model`, `waiting_on_tool`, `open_generic_span`) would be stale during live execution without a refresh path. The fix is minimal: add `refetchInterval` to the spans query matching the existing timeline poll cadence, gated on `RUNNING` status.

**Alternatives considered:** Deriving span state from synthetic timeline events instead of refetching. Rejected because synthetic events don't carry `span.status` or `span.kind`, which the classifier needs.

### Wait pairing sorts internally
`computeOpenWaits()` sorts events using the existing timeline comparator before pairing, so callers are not required to pass pre-sorted input. This makes the utility safe for standalone tests and future callers.

**Alternatives considered:** Requiring callers to pre-sort. Rejected because it's a fragile precondition for a pure utility that should work correctly regardless of input order.

### Reuse `evaluateStaleTraceSignal` unchanged
The existing stale helper is called as a black box. The classifier copies its timing fields (`latestActivityAt`, `runtimeMs`, `inactivityMs`) and uses only `shouldDisplay` to decide the `possibly_stalled` branch. This avoids touching a well-tested helper and ensures backward compatibility for any other consumers.

## Risks / Trade-offs

- **Full recomputation on every analysis pass:** Acceptable for v1 since wait event counts are typically low. If wait-heavy traces emerge, incremental pairing can be added as a follow-up.
- **Stale panel replacement requires test migration:** Existing test assertions referencing "Experimental stale trace signal" will need updating since the new panel subsumes the old one. This is internal debugger copy, not a public API surface.
- **`unknown` may be confusing to users:** Mitigated by conservative copy ("the debugger cannot yet explain where it is waiting") that sets correct expectations.

## Open Questions

None — the plan is fully specified.
