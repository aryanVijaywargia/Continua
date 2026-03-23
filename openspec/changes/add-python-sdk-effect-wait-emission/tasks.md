## 1. Event Metadata Foundation
- [x] 1.1 Add `_event_seq` counter (int, initialized to 0, pre-incremented to 1 on first emission) and `_event_lock` (threading.Lock) to `SpanContext.__init__`
- [x] 1.2 Update `_record_event()`: early returns before lock; acquire lock to pre-increment counter, enforce int32 bound, capture `event_ts`; release lock; add `sequence` and `event_ts` to event dict
- [x] 1.3 Add unit tests: sequential events get increasing sequence starting at 1; per-span independence; OverflowError at int32 boundary; early-return path does not consume sequence
- [x] 1.4 Add targeted concurrency test: spawn N threads each emitting one event on the same span, collect all sequence values, assert they form a duplicate-free contiguous set {1..N} (verify by sorting after collection, not by observation order)
- [x] 1.5 Update existing event helper tests in `test_errors.py` (set_llm_response, set_tool_call, log, error, exception, metric) and `test_span.py` (state_change, decision) to assert `event_ts` exists and matches RFC 3339 UTC Z pattern, and `sequence` exists as expected integer

## 2. Implicit Effect Emission from Existing Helpers
- [x] 2.1 Add `_implicit_llm_effect_emitted` and `_implicit_tool_effect_emitted` flags to `SpanContext.__init__`
- [x] 2.2 Extend `set_llm_response()` with `emit_effect: bool = True` keyword-only param; emit at-most-once effect event with `effect_kind: "model_call"`
- [x] 2.3 Extend `set_tool_call()` with `has_external_side_effect: bool = True` and `emit_effect: bool = True` keyword-only params; emit at-most-once effect event with `effect_kind: "tool_call"`
- [x] 2.4 Add unit tests: implicit LLM effect emission, deduplication, emit_effect=False suppression without flag consumption
- [x] 2.5 Add unit tests: implicit tool effect emission, deduplication, has_external_side_effect override, emit_effect=False suppression

## 3. Explicit Effect and Wait Helpers
- [x] 3.1 Implement `span.effect()` with reserved-field merge, underscore normalization for default message, None-omission, empty-string omission for ID fields, and `ValueError` on empty `kind`
- [x] 3.2 Implement `span.wait()` with reserved-field merge, underscore normalization for default message, None-omission, empty-string omission for `wait_id`, and `ValueError` on empty `kind` or `phase`
- [x] 3.3 Add unit tests: effect event type, default message, caller payload merge, reserved-field precedence, caller dict not mutated, None omission, False preservation, empty-string effect_id omitted, empty kind raises ValueError, `idempotency_key` stays payload-only (not mapped to top-level event dedup field), empty-string `idempotency_key` is omitted from payload
- [x] 3.4 Add unit tests: wait event type, default message with phase capitalization, resolution round-trip, caller payload merge, wait_id round-trip, empty-string wait_id omitted, empty kind/phase raises ValueError
- [x] 3.5 Add unit tests: explicit helpers independent of implicit flags (effect after emit_effect=False, explicit does not block implicit)
- [x] 3.6 Add unit tests: quiet no-op behavior — `span.effect()` and `span.wait()` emit nothing and raise no error when `trace_id` is `None` or no client is initialized; implicit effect from `set_llm_response()` and `set_tool_call()` likewise emits nothing (while still mutating span fields) when tracing is inactive

## 4. Documentation
- [x] 4.1 Expand `docs/event-conventions.md` to cover all 10 explicit event types including effect and wait, with expected payload fields and default levels
- [x] 4.2 Add Python SDK examples in `docs/event-conventions.md` for implicit model-call/tool-call effects via `set_llm_response()`/`set_tool_call()` and explicit wait enter/resolve patterns via `span.wait()`
- [x] 4.3 State in `docs/event-conventions.md` that `message` and `custom` remain supported without dedicated helpers in this phase

## 5. Integration Tests
- [x] 5.1 Add live-server integration test in `sdks/python/tests/test_integration.py` using `ingest_mode="sync"`: emit LLM span with `set_llm_response()`, fetch timeline, assert effect event present with expected payload
- [x] 5.2 Add live-server integration test using `ingest_mode="sync"`: emit tool span with `set_tool_call()`, fetch timeline, assert effect event present
- [x] 5.3 Add live-server integration test using `ingest_mode="sync"`: emit explicit `span.wait()`, fetch timeline, assert wait event present with expected fields
- [x] 5.4 Assert server-derived `effect_id`/`wait_id` appear when omitted by SDK; caller-provided IDs round-trip unchanged
- [x] 5.5 Assert timeline ordering: span_started < explicit effect/wait < span_completed for each span
- [x] 5.6 Assert event metadata round-trip from live server: fetched timeline events for effect/wait SHALL contain non-null `timestamp` (mapped from `event_ts` via `mapper.go`) parseable as RFC 3339 `date-time`, and integer `sequence` values preserving SDK-assigned ordering. Do not assert the SDK's original microsecond Z format verbatim — the server may normalize the timestamp representation

## 6. Final Verification
- [x] 6.1 Run full SDK unit test suite: `cd sdks/python && uv run pytest`
- [x] 6.2 Run integration tests against live server with `ingest_mode="sync"`: `cd sdks/python && uv run pytest tests/test_integration.py -v`
- [x] 6.3 Confirm no backend, UI, migration, or TypeScript SDK changes were introduced
