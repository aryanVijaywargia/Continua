# CONTEXT — Domain & Design Vocabulary

Shared vocabulary for Continua's product surfaces. Architecture reviews
(`/improve-codebase-architecture`) and grilling sessions use these terms so
suggestions name the right seams in the project's own language. The *architecture*
vocabulary (module, interface, seam, adapter, depth, deletion test) lives in the
skill; this file is the *domain* language.

## Trace-detail workspace

**Selected span**
The span currently shown in the inspector panel of the trace-detail workspace.
Source of truth: the `span` URL query param — and nothing else. Selecting a span
(by operator click or by system auto-selection) writes the URL; the displayed span
is derived as `spanIndex.get(spanParam)`.

> Decision (2026-05-30): the URL is the **single source of truth** for the selected
> span. An earlier design kept a second copy in React state (`selectedSpanExternalId`)
> and reconciled the two with effects plus a pending-sync ref; that dual-state model
> is being deleted. Auto-selecting the failed span therefore also writes the URL (see
> **Failed-span auto-select**), which makes shared failed-trace links open on the
> failure. Do not reintroduce a second source of truth for span selection.

**Span expansion**
Which expandable spans are open in the tree rail. Stateful and operator-owned:
- preserves the operator's manual expand/collapse across live-poll updates,
- auto-expands branches that newly arrive while a trace is RUNNING,
- reveals (expands) the ancestor path of the selected span.

> Decision (2026-05-30): expansion is the **one deep module** extracted from the
> trace-detail workspace — a pure reducer `(state, event) -> state` over the events
> `toggle`, `syncExpandable`, and `revealAncestors`. It is the only real state left in
> the workspace after selection dissolves into the URL; it carries genuine operator
> intent plus live-poll reconciliation, so it earns a module. The reducer is the test
> surface (testable without React); the hook around it is thin wiring.

**Reveal**
When the selected span changes, the workspace expands its ancestor path (so it is
visible in the tree rail) and scrolls the execution waterfall and tree to it.

> Decision (2026-05-30): reveal is **pure derivation** from the selected span, not a
> command. `revealPath` = ancestors(selectedSpan) ∪ {selectedSpan}; the scroll target
> is the selected span id. Re-selecting the already-selected span is a deliberate
> no-op — no re-scroll, no re-center. The old `revealVersion`/`revealKey` counter that
> re-fired scroll on re-selection is deleted. The selected/highlighted state already
> shows which span is active; to re-center, select another span and back.

**Failed-span auto-select**
On a FAILED trace with no span selected, the workspace opens the primary failed span
once per trace load (a one-shot latch, reset when the trace changes), writing it to
the URL. It must not re-open after the operator closes the panel within the same
trace load.
