# Change: Add Python SDK Effect/Wait Emission

## Why

Phase 1 (`add-effect-wait-semantic-foundation`) added wire-level acceptance of `effect` and `wait` event types with deterministic semantic ID derivation on the server. The SDK internals already support emitting arbitrary event types through the private `_record_event()` method, but there is no public ergonomic API for `effect` or `wait`, no implicit emission from existing span setters, and no client-side `event_ts` or `sequence` metadata. Developers have no supported way to emit effect/wait events and cannot rely on ordering guarantees for semantic ID derivation.

This phase adds the SDK-side ergonomics: explicit `span.effect()` and `span.wait()` helpers, implicit effect emission from `set_llm_response()` and `set_tool_call()`, and per-span monotonic sequencing with RFC 3339 timestamps on all explicit events. Together these make the Phase 1 backend foundation usable from Python without manual payload construction.

## What Changes

### SDK event metadata (cross-cutting)
- `_record_event()` emits top-level `event_ts` (RFC 3339 UTC with Z suffix) and `sequence` (per-span monotonic int32) on every explicit event
- Thread-safe sequence assignment via `_event_lock` scoped narrowly to counter increment + timestamp capture
- Defensive OverflowError at int32 boundary (2,147,483,647)
- Early returns (no trace_id, no client) execute before lock acquisition and do not consume sequence numbers

### Implicit effect emission from existing helpers
- `set_llm_response()` gains `emit_effect: bool = True` keyword-only parameter; emits one `effect` event with `effect_kind: "model_call"` at most once per span
- `set_tool_call()` gains `has_external_side_effect: bool = True` and `emit_effect: bool = True` keyword-only parameters; emits one `effect` event with `effect_kind: "tool_call"` at most once per span
- All new parameters are keyword-only; existing positional callers remain valid

### New explicit helpers
- `span.effect(kind, *, has_external_side_effect, ...)` — emits `event_type="effect"` with reserved-field merge semantics
- `span.wait(kind, *, phase, ...)` — emits `event_type="wait"` with reserved-field merge semantics
- No vocabulary validation beyond rejecting empty strings; preferred vocabularies are documented only

### Documentation
- Expand `docs/event-conventions.md` to cover all 10 explicit platform event types including effect and wait
- Add examples for implicit and explicit emission patterns

## Impact
- Affected specs: `sdk-event-metadata` (new), `python-sdk-effect-wait` (new), `event-conventions` (modified — extends existing doc requirement from `add-debugger-semantics-polish`)
- Affected code:
  - `sdks/python/src/continua/span.py` — new helpers, extended setters, `_record_event()` metadata, lock, counter
  - `sdks/python/tests/test_span.py` — new unit tests for effect/wait/implicit/sequencing/concurrency/empty-string
  - `sdks/python/tests/test_errors.py` — updated existing setter/helper assertions (`set_llm_response`, `set_tool_call`, `log`, `error`, `exception`, `metric`) to verify event_ts/sequence fields
  - `sdks/python/tests/test_integration.py` — new live-server integration tests (sync mode) for effect/wait timeline round-trip
  - `docs/event-conventions.md` — expanded from 8 to 10 event types with implicit/explicit examples
- No backend, migration, UI, TypeScript SDK, or engine changes
- No OpenAPI or codegen changes
- Backward-compatible: all new parameters are keyword-only additions
