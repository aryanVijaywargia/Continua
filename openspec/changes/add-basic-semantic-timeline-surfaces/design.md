## Context

The timeline in `Timeline.tsx` currently renders `state_change` and `decision` events with dedicated preview components (`StateChangePreview`, `DecisionPreview`) extracted via helpers in `eventSemantics.ts`. All other explicit event types, including the newly accepted `effect` and `wait`, fall through to a generic text summary. This change adds extraction, preview rendering, and filtering for effect/wait events while preserving the existing rendering chain and component patterns.

The Python SDK (`span.py`) is the payload source of truth. The verified payload keys are:
- **effect**: `effect_kind` (string, required), `has_external_side_effect` (boolean, required), `effect_id` (string, optional), `idempotent` (boolean, optional), `idempotency_key` (string, optional)
- **wait**: `wait_kind` (string, required), `phase` (string, required), `resolution` (string, optional), `wait_id` (string, optional)

## Goals / Non-Goals

- **Goals**: Surface effect/wait semantics in the timeline with the same fidelity as state_change/decision; let users filter the timeline to semantic or effect/wait events only
- **Non-Goals**: Causal linking between effect_id/wait_id pairs, session-level aggregation, retry-safety classification, stuck-wait detection, cost surfaces, StateDiffViewer changes

## Decisions

### Extraction pattern: mirror existing eventSemantics.ts helpers
- `getEffectDetails()` and `getWaitDetails()` follow the same guard-clause pattern as `getStateChangeDetails()` and `getDecisionDetails()`: check `event_type`, check `payload`, validate required fields, return typed object or `null`
- **Why**: Consistency with established codebase pattern; zero new abstractions

### Preview rendering: new components in Timeline.tsx
- `EffectPreview` and `WaitPreview` are private components in `Timeline.tsx`, parallel to `StateChangePreview` and `DecisionPreview`
- Rendering chain in `TimelineRow`: `stateChange ? <StateChangePreview> : decision ? <DecisionPreview> : effectDetails ? <EffectPreview> : waitDetails ? <WaitPreview> : <generic summary>`
- **Why**: Keeps the rendering chain flat and readable; new event types slot in without restructuring

### Effect preview: kind + mutating/read-only only
- Primary display: kind label (e.g. `model_call`, `tool_call`, `api_call`) + mutating/read-only badge based on `has_external_side_effect`
- Opaque identifiers (`effect_id`, `idempotency_key`) and the `idempotent` flag are accessible in the expanded payload panel, not surfaced as collapsed-row badges
- **Why**: Users need to quickly see what kind of effect happened and whether it mutated external state; opaque IDs add visual noise in the collapsed row without aiding at-a-glance triage. Deeper causal linking is deferred

### Wait preview: kind + phase + optional resolution
- Primary display: kind label (e.g. `human_approval`, `external`, `timer`) + phase badge (`entered`, `resolved`)
- Resolution shown as accent pill when present
- `wait_id` is accessible in the expanded payload panel, not surfaced as a collapsed-row badge
- **Why**: Phase is the key differentiator for wait events at a glance; resolution provides the outcome. Opaque IDs belong in the detail view

### Segmented filter: custom radiogroup control
- Three modes: `All` (default), `Semantic`, `Effects & waits`
- Implemented as a `<div role="radiogroup">` with `<button role="radio">` children, supporting arrow-key navigation per WAI-ARIA radio group pattern
- **Why**: Native `<select>` would break the visual consistency with the existing `Errors only` toggle; radiogroup semantics are the correct ARIA pattern for a mutually exclusive choice set
- **Alternative considered**: Dropdown menu — rejected because three options don't warrant a popover; segmented control is more discoverable

### Filter composition: intersection
- When `Errors only` is active alongside a segmented filter, the filters compose by intersection: an event must match both the segmented filter AND be an error event
- **Why**: Intersection is the intuitive mental model ("show me only errors among effects & waits")

### Filter state: local, not URL-backed
- Filter state lives in `Timeline` component state via `useState`
- Persists across inspector tab switches (component stays mounted when switching Details/Timeline/State tabs in InspectorTabs)
- Resets on trace navigation (component unmounts and remounts with new trace)
- Not part of React Query cache keys
- **Why**: Timeline filters are ephemeral exploration aids, not shareable navigation state; URL params are reserved for cross-page state like selected span

### Empty-state messages: fixed strings
- Six combinations of segmented filter × errors-only produce six distinct messages
- Messages are statically defined, not template-generated
- **Why**: Fixed strings are easier to test and localize; six cases don't warrant a template system

## Risks / Trade-offs

- **Rendering chain length**: Adding two more branches to the `TimelineRow` conditional increases cyclomatic complexity. Mitigation: each branch is a simple component call; the chain remains flat
- **Filter discoverability**: Users may not notice the segmented control. Mitigation: positioned directly beside the existing `Errors only` toggle in the header bar
- **Payload drift**: If the SDK adds new payload keys, the extractors will ignore them. Mitigation: extractors are additive; new keys can be surfaced in future phases without breaking existing rendering

## Open Questions

None — the scope is fully constrained by the SDK payload contract and the existing Timeline component patterns.
