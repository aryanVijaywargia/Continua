## ADDED Requirements

### Requirement: Explicit Effect Helper

The Python SDK `SpanContext` class SHALL provide an `effect()` method with the following signature:

```python
def effect(
    self,
    kind: str,
    *,
    has_external_side_effect: bool,
    effect_id: str | None = None,
    idempotent: bool | None = None,
    idempotency_key: str | None = None,
    message: str | None = None,
    payload: dict[str, Any] | None = None,
    level: str = "info",
) -> None
```

The method SHALL emit an event with `event_type="effect"` via `_record_event()`.

The `idempotent` and `idempotency_key` parameters are payload-level semantic metadata for the effect, not event-level deduplication fields:
- `idempotent` (bool): whether replaying or retrying this effect is safe in principle (e.g., a read-only API call is idempotent; sending an email is not).
- `idempotency_key` (str): the concrete business key or operation identifier that makes retries safe (e.g., a payment transaction ID). Meaningful only when `idempotent` is `True`.

Neither field SHALL be confused with the event-level `idempotency_key` in the ingest contract, which is a top-level batch deduplication mechanism. The helper writes both fields into the event payload dict only; it does not set any top-level event dedup key.

The default message SHALL be `"Effect: <kind>"` with underscores in `kind` normalized to spaces (e.g., `"model_call"` becomes `"Effect: model call"`).

The method SHALL NOT modify any span fields (name, model, input, output, metadata).

#### Scenario: Basic effect emission
- **WHEN** `span.effect("api_call", has_external_side_effect=True)` is called
- **THEN** an event is emitted with `event_type="effect"`, `level="info"`, `message="Effect: api call"`, and payload containing `{"effect_kind": "api_call", "has_external_side_effect": True}`

#### Scenario: Effect with caller-supplied payload
- **WHEN** `span.effect("api_call", has_external_side_effect=True, payload={"url": "https://example.com"})` is called
- **THEN** the payload SHALL contain `{"url": "https://example.com", "effect_kind": "api_call", "has_external_side_effect": True}`

#### Scenario: Effect with custom message
- **WHEN** `span.effect("api_call", has_external_side_effect=True, message="Called weather API")` is called
- **THEN** the event message SHALL be `"Called weather API"`

#### Scenario: Caller-supplied effect_id round-trips
- **WHEN** `span.effect("api_call", has_external_side_effect=True, effect_id="my-effect-1")` is called
- **THEN** the payload SHALL contain `"effect_id": "my-effect-1"`

#### Scenario: Empty-string effect_id is treated as absent
- **WHEN** `span.effect("api_call", has_external_side_effect=True, effect_id="")` is called
- **THEN** the payload SHALL NOT contain the key `effect_id`
- **AND** the server SHALL derive the `effect_id` as if the caller omitted it

#### Scenario: None-valued optional fields are omitted from payload
- **WHEN** `span.effect("api_call", has_external_side_effect=True)` is called without `effect_id`, `idempotent`, or `idempotency_key`
- **THEN** the payload SHALL NOT contain keys `effect_id`, `idempotent`, or `idempotency_key`

#### Scenario: Empty-string kind is rejected
- **WHEN** `span.effect("", has_external_side_effect=True)` is called
- **THEN** a `ValueError` SHALL be raised with a message indicating that `kind` must be a non-empty string

#### Scenario: False-valued fields are preserved in payload
- **WHEN** `span.effect("api_call", has_external_side_effect=False, idempotent=False)` is called
- **THEN** the payload SHALL contain `"has_external_side_effect": False` and `"idempotent": False`

---

### Requirement: Explicit Wait Helper

The Python SDK `SpanContext` class SHALL provide a `wait()` method with the following signature:

```python
def wait(
    self,
    kind: str,
    *,
    phase: str,
    resolution: str | None = None,
    wait_id: str | None = None,
    message: str | None = None,
    payload: dict[str, Any] | None = None,
    level: str = "info",
) -> None
```

The method SHALL emit an event with `event_type="wait"` via `_record_event()`.

The default message SHALL be `"<Phase> wait: <kind>"` with underscores in both `phase` and `kind` normalized to spaces and `phase` capitalized (e.g., `phase="entered"`, `kind="human_approval"` becomes `"Entered wait: human approval"`).

The method SHALL NOT modify any span fields (name, model, input, output, metadata).

#### Scenario: Basic wait emission
- **WHEN** `span.wait("human_approval", phase="entered")` is called
- **THEN** an event is emitted with `event_type="wait"`, `level="info"`, `message="Entered wait: human approval"`, and payload containing `{"wait_kind": "human_approval", "phase": "entered"}`

#### Scenario: Wait with resolution
- **WHEN** `span.wait("human_approval", phase="resolved", resolution="approved")` is called
- **THEN** the payload SHALL contain `{"wait_kind": "human_approval", "phase": "resolved", "resolution": "approved"}`
- **AND** the message SHALL be `"Resolved wait: human approval"`

#### Scenario: Wait with caller-supplied payload
- **WHEN** `span.wait("external", phase="entered", payload={"service": "stripe"})` is called
- **THEN** the payload SHALL contain `{"service": "stripe", "wait_kind": "external", "phase": "entered"}`

#### Scenario: Caller-supplied wait_id round-trips
- **WHEN** `span.wait("human_approval", phase="entered", wait_id="wait-abc")` is called
- **THEN** the payload SHALL contain `"wait_id": "wait-abc"`

#### Scenario: Empty-string wait_id is treated as absent
- **WHEN** `span.wait("human_approval", phase="entered", wait_id="")` is called
- **THEN** the payload SHALL NOT contain the key `wait_id`

#### Scenario: None-valued optional fields omitted
- **WHEN** `span.wait("timer", phase="entered")` is called without `resolution` or `wait_id`
- **THEN** the payload SHALL NOT contain keys `resolution` or `wait_id`

#### Scenario: Empty-string kind is rejected
- **WHEN** `span.wait("", phase="entered")` is called
- **THEN** a `ValueError` SHALL be raised with a message indicating that `kind` must be a non-empty string

#### Scenario: Empty-string phase is rejected
- **WHEN** `span.wait("human_approval", phase="")` is called
- **THEN** a `ValueError` SHALL be raised with a message indicating that `phase` must be a non-empty string

---

### Requirement: Reserved-Field Merge Semantics

The `effect()` and `wait()` helpers SHALL merge caller-supplied `payload` with helper-owned reserved fields using the following rule:

1. Start from a shallow copy of the caller's `payload` dict (or empty dict if `None`)
2. Write helper-owned fields last, so helper arguments always win over caller payload keys
3. Omit helper-owned keys whose value is `None`; preserve `False`
4. Treat empty-string (`""`) values for string ID fields (`effect_id`, `wait_id`, `idempotency_key`) as absent and omit them. This matches the server's behavior where empty-string `effect_id`/`wait_id` triggers derivation as if the field were absent, and avoids emitting semantically meaningless empty keys

Effect helper-owned fields: `effect_kind`, `has_external_side_effect`, `effect_id`, `idempotent`, `idempotency_key`.

Wait helper-owned fields: `wait_kind`, `phase`, `wait_id`, `resolution`.

#### Scenario: Helper args override conflicting caller payload keys
- **WHEN** `span.effect("api_call", has_external_side_effect=True, payload={"effect_kind": "wrong"})` is called
- **THEN** the payload SHALL contain `"effect_kind": "api_call"` (helper wins)

#### Scenario: Caller payload dict is not mutated
- **WHEN** a caller passes `payload = {"key": "value"}` to `span.effect()`
- **THEN** the original dict object SHALL NOT be modified after the call

---

### Requirement: Implicit LLM Effect Emission

The `set_llm_response()` method SHALL accept an additional keyword-only parameter `emit_effect: bool = True`.

When `emit_effect` is `True` and the implicit LLM effect has not already been emitted for this span, the method SHALL emit one event with:
- `event_type="effect"`, `level="info"`
- `message="Model call: <model>"` (using the `model` argument)
- payload: `{"effect_kind": "model_call", "has_external_side_effect": False}`

The implicit emission SHALL occur at most once per span, tracked by an `_implicit_llm_effect_emitted` flag.

When `emit_effect` is `False`, the implicit event SHALL be suppressed and the flag SHALL NOT be consumed (a subsequent call with `emit_effect=True` can still emit).

The method SHALL continue to perform all existing span mutations (model, input, output, tokens, provider, cost) exactly as before, regardless of `emit_effect`.

#### Scenario: First set_llm_response emits implicit effect
- **WHEN** `span.set_llm_response("gpt-4", messages, response, 100, 50)` is called for the first time on a span
- **THEN** the span fields SHALL be set as before (model, input, output, tokens)
- **AND** one event SHALL be emitted with `event_type="effect"` and `effect_kind="model_call"`

#### Scenario: Repeated set_llm_response does not duplicate effect
- **WHEN** `span.set_llm_response(...)` is called twice on the same span
- **THEN** only one implicit effect event SHALL be emitted (from the first call)

#### Scenario: emit_effect=False suppresses without consuming flag
- **WHEN** `span.set_llm_response("gpt-4", m, r, 100, 50, emit_effect=False)` is called
- **THEN** no implicit effect event SHALL be emitted
- **AND** a subsequent call with `emit_effect=True` SHALL still emit the implicit effect

#### Scenario: Existing callers remain valid
- **WHEN** `span.set_llm_response("gpt-4", messages, response, 100, 50)` is called with only positional and existing keyword arguments
- **THEN** the call SHALL succeed without errors (backward compatible)

---

### Requirement: Implicit Tool Effect Emission

The `set_tool_call()` method SHALL accept two additional keyword-only parameters: `has_external_side_effect: bool = True` and `emit_effect: bool = True`.

When `emit_effect` is `True` and the implicit tool effect has not already been emitted for this span, the method SHALL emit one event with:
- `event_type="effect"`, `level="info"`
- `message="Tool call: <tool_name>"` (using the `tool_name` argument)
- payload: `{"effect_kind": "tool_call", "has_external_side_effect": <flag>}`

The implicit emission SHALL occur at most once per span, tracked by an `_implicit_tool_effect_emitted` flag.

When `emit_effect` is `False`, the implicit event SHALL be suppressed and the flag SHALL NOT be consumed.

The method SHALL continue to perform all existing span mutations (name, input, output) exactly as before.

#### Scenario: First set_tool_call emits implicit effect
- **WHEN** `span.set_tool_call("search", {"q": "test"}, {"results": []})` is called for the first time on a span
- **THEN** the span fields SHALL be set as before (name, input, output)
- **AND** one event SHALL be emitted with `event_type="effect"`, `effect_kind="tool_call"`, and `has_external_side_effect=True`

#### Scenario: Override has_external_side_effect
- **WHEN** `span.set_tool_call("cache_lookup", args, result, has_external_side_effect=False)` is called
- **THEN** the effect payload SHALL contain `"has_external_side_effect": False`

#### Scenario: Repeated set_tool_call does not duplicate effect
- **WHEN** `span.set_tool_call(...)` is called twice on the same span
- **THEN** only one implicit effect event SHALL be emitted (from the first call)

#### Scenario: emit_effect=False suppresses without consuming flag
- **WHEN** `span.set_tool_call("search", args, result, emit_effect=False)` is called
- **THEN** no implicit effect event SHALL be emitted
- **AND** a subsequent call with `emit_effect=True` SHALL still emit the implicit effect

#### Scenario: Existing callers remain valid
- **WHEN** `span.set_tool_call("search", {"q": "test"}, {"results": []})` is called with the original 3 positional arguments
- **THEN** the call SHALL succeed without errors (backward compatible)

---

### Requirement: Explicit Helpers Independent of Implicit Flags

Explicit `span.effect()` and `span.wait()` calls SHALL NOT read, write, or be blocked by the implicit emission flags (`_implicit_llm_effect_emitted`, `_implicit_tool_effect_emitted`).

#### Scenario: Explicit effect after implicit suppression
- **WHEN** `span.set_llm_response("gpt-4", m, r, 100, 50, emit_effect=False)` is called
- **AND** then `span.effect("model_call", has_external_side_effect=False)` is called
- **THEN** the explicit effect event SHALL be emitted successfully

#### Scenario: Explicit effect does not block later implicit emission
- **WHEN** `span.effect("model_call", has_external_side_effect=False)` is called
- **AND** then `span.set_llm_response("gpt-4", m, r, 100, 50)` is called for the first time
- **THEN** both the explicit effect and the implicit effect SHALL be emitted (two events total)

---

### Requirement: Quiet No-Op When Tracing Is Inactive

The `effect()`, `wait()`, and the implicit effect emission from `set_llm_response()` and `set_tool_call()` SHALL be quiet no-ops when tracing is inactive. This preserves the repo-wide SDK convention where helper calls silently skip when there is no active trace context or no initialized client.

#### Scenario: effect() with no trace context
- **WHEN** `span.effect("api_call", has_external_side_effect=True)` is called on a span with `trace_id = None`
- **THEN** no event SHALL be emitted and no error SHALL be raised

#### Scenario: wait() with no initialized client
- **WHEN** `span.wait("human_approval", phase="entered")` is called and no client is initialized
- **THEN** no event SHALL be emitted and no error SHALL be raised

#### Scenario: Implicit effect with no trace context
- **WHEN** `span.set_llm_response("gpt-4", m, r, 100, 50)` is called on a span with `trace_id = None`
- **THEN** the span fields SHALL still be mutated as normal
- **AND** no implicit effect event SHALL be emitted and no error SHALL be raised

