## Context

Phase 1 (`add-effect-wait-semantic-foundation`) established server-side acceptance and deterministic semantic ID derivation for `effect` and `wait` events. The derivation function reads `event_ts` and `sequence` from `EventInput` when present, falling back to payload hashing when absent.

This phase adds the Python SDK surface: explicit helpers, implicit emission from existing setters, and per-span event metadata (`event_ts`, `sequence`) that improves derivation determinism and ordering fidelity.

The SDK is the only system touched. No backend, frontend, migration, or TypeScript SDK changes are included.

## Goals / Non-Goals

- **Goals:**
  - Emit `event_ts` and `sequence` on every explicit SDK event so server-side semantic ID derivation has stable inputs
  - Provide `span.effect()` and `span.wait()` helpers with documented vocabulary and reserved-field merge
  - Extend `set_llm_response()` and `set_tool_call()` with implicit, at-most-once effect emission
  - Maintain full backward compatibility for existing SDK callers
  - Thread-safe sequence assignment without serializing event construction or enqueue

- **Non-Goals:**
  - Client-side derivation of `effect_id` or `wait_id` (server derives these)
  - Full SpanContext thread safety for general span mutation
  - Context-manager API for wait enter/resolve lifecycle
  - Dedicated `message()` or `custom()` helpers
  - Backend, UI, or TypeScript SDK changes

## Decisions

### D1: Per-span monotonic sequence counter with int32 bound

**Decision:** Use a single per-span counter initialized to 0, pre-incremented on each emission so the first event receives sequence 1. Shared across all event types emitted through `_record_event()`. Raise `OverflowError` at 2,147,483,647.

**Alternatives considered:**
- Per-event-type counters: rejected because server derivation uses a single `sequence` field and expects monotonic ordering across all events within a span.
- Unbounded counter: rejected because `sequence` is stored as int32 server-side. Silent wrap would corrupt ordering and semantic ID derivation.
- Saturate at max: rejected because saturation causes duplicate sequences, which corrupts semantic ID uniqueness.

### D2: Narrow lock scope in _record_event()

**Decision:** `_event_lock` protects only: (1) incrementing the sequence counter, (2) enforcing the int32 upper bound, (3) capturing the timestamp string. The lock is released before building the event dict and before calling `client.add_event()`.

**Rationale:** The enqueue path (`BatchQueue.add_event`) already holds its own lock. Holding `_event_lock` through enqueue would serialize all event emission per span, which is unnecessary. Out-of-order enqueue is acceptable because the `sequence` and `event_ts` fields carry the ordering truth.

**Trade-off:** Events may arrive at the batch queue out of sequence order. This is documented as intentional — the server uses `sequence` for ordering, not arrival order.

### D3: Early returns before lock acquisition

**Decision:** `_record_event()` checks `self.trace_id is None` and `_get_client_if_initialized() is None` before acquiring `_event_lock`. Disabled tracing does not consume sequence numbers or timestamps.

**Rationale:** Consuming sequence numbers during no-op paths would create gaps that are confusing to debug and would shift deterministic derivation inputs when tracing is conditionally enabled.

### D4: Implicit emission flags are independent per setter

**Decision:** `_implicit_llm_effect_emitted` and `_implicit_tool_effect_emitted` are separate boolean flags. Each setter's implicit emission is at-most-once and independent of the other. Explicit `span.effect()` / `span.wait()` calls never read or write these flags.

**Rationale:** A developer may call `set_llm_response()` and `set_tool_call()` on the same span (e.g., an LLM span that also invokes a tool). Each should produce its own implicit effect. The `emit_effect=False` parameter suppresses the implicit event without consuming the flag, so a subsequent call with the default `emit_effect=True` can still emit.

### D5: Reserved-field merge — helper args win

**Decision:** `effect()` and `wait()` start from a shallow copy of the caller's `payload` dict, then write helper-owned fields last. Helper-owned keys whose value is `None` are omitted; `False` is preserved.

**Rationale:** Ensures the semantic structure of effect/wait payloads is always correct regardless of what the caller passes in `payload`. Shallow copy prevents mutation of the caller's dict. Omitting `None` keys keeps payloads clean while preserving semantically meaningful `False` values (e.g., `has_external_side_effect=False`).

### D6: No runtime validation of kind/phase vocabulary, but reject empty strings

**Decision:** The helpers accept any non-empty string for `kind` and `phase`. Empty strings raise `ValueError`. Preferred vocabularies (`model_call`, `tool_call`, `api_call`, `custom` for effects; `human_approval`, `external`, `timer`, `custom` for waits; `entered`, `resolved` for phases) are documented but not enforced.

**Rationale:** Forward compatibility. New kinds and phases can be introduced without SDK updates. Runtime validation would force SDK releases for vocabulary expansion. Empty-string rejection is a minimal guard that prevents silent emission of semantically invalid events — a developer passing `""` is almost certainly a bug, not an intentional forward-compatible kind.

## Risks / Trade-offs

- **Out-of-order enqueue under concurrency:** Mitigated by `sequence` and `event_ts` carrying ordering truth. Documented as intentional.
- **OverflowError on extreme sequence values:** Intentional fail-loud. A span emitting 2B+ events indicates a bug, not a legitimate workload.
- **No client-side ID derivation:** Relies on Phase 1 server derivation. If the server is unavailable or buggy, IDs won't be derived. Acceptable for v1 — server is the single source of truth for semantic IDs.

## Open Questions

None — all QA feedback from the planning phase has been addressed in this design.
