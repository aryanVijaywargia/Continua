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

**Workspace provider**
The single provider seam of the trace-detail workspace (`TraceDetailWorkspaceProvider`
plus `useTraceDetailWorkspace()` in `web/src/pages/traceDetail/`). It owns everything
*derived* from loaded trace data — span index, span tree, analyses, URL-derived
selection, the expansion open set, and workspace actions — while the page keeps
fetching (react-query and the polling timeline) and mounts the provider only after
data loads, so the provider never sees loading/error states.

> Decision (2026-07-05): one provider, drawn at "loaded trace data + everything derived
> from it." The provider does not fetch. Selection stays URL-derived (see **Selected
> span**) and expansion stays owned by the expansion reducer — the provider re-exports
> their surfaces, never forks their state. Reusable, individually tested leaves
> (TreeRail, ExecutionWaterfall, SpanDetail, FailureSummary) keep props as their real
> interface; the provider exists to delete pass-through props in the intermediate
> layers, not to make every component a context consumer. Do not add a second context
> or move fetching into this one.

## Request scoping

**Scope**
The set of projects a single request is allowed to read. A first-class value *produced*
by scope resolution (API layer) and *consumed* by scope enforcement (store). It has
exactly two shapes:
- **Bound(projectID)** — the request may touch exactly one project. API-key and
  public-demo callers, and an operator who named a project on a list route.
- **Unbounded** — the request may touch any project. Operator/admin requests only.

> Decision (2026-05-30): scoping is **one resolved value threaded across two seams**,
> replacing three ad-hoc handler idioms (`projectIDOrUnauthorized`,
> `selectedProjectIDFromRequest` + post-fetch `projectMatchesSelection`, and
> `engineRunProjectIDOrUnauthorized`). The API layer *resolves* a `Scope`; the store
> *enforces* it in SQL (`WHERE … AND ($scope::uuid IS NULL OR project_id = $scope)`), so
> a cross-project read returns not-found **by construction**. Child reads (spans,
> span_events) enforce the same predicate on their own `NOT NULL project_id` column
> (no join to `traces`). The
> post-fetch recheck and its dead `*uuid.UUID` nil-branch are deleted. Do not
> reintroduce ownership checks in handlers after a row is already in memory — that
> convention is the cross-tenant-read bug class this removes.

**Operator (instance-admin)**
An Auth0-authenticated caller. Treated as an **instance administrator with Unbounded
scope**, not a project-scoped user. On operator routes, `project_id` is a
selection/filter (which tenant to view), never a per-project authorization boundary.

> Decision (2026-05-30): operators are cross-tenant by design. There is no
> operator→project grant model and we will not add one now; Auth0 operator auth is
> expected to be **removed** later as a local-first simplification, so investing in
> per-project operator authorization would be wasted effort. A future proposal to scope
> operators to projects is a product reversal, not a refactor.
