# Event Conventions

> **Status: Current**
> These are the current debugger-facing event semantics used by the timeline, state diff surfaces, reasoning summaries, and semantic compare flows.

Continua events are debugger-facing observability signals.

These are debugger semantics, not replay primitives.

Use them to explain what happened during trace execution, not to model checkpoints, resumability, or durable workflow state machines.

## Choosing an Event Type

- Use `state_change` when an observable piece of state changes and you want the debugger to show the transition directly.
- Use `decision` when the system chooses between alternatives and the debugger should show the branch point and rationale.
- Use `effect` when the span performs an external action or model invocation that matters for debugging.
- Use `wait` when progress is blocked on a human, external dependency, timer, or similar resolution outside the current span's direct control.
- Use `snapshot_marker` when you want a first-class debugger milestone with a compact label and category.
- Use `message` for lightweight narrative milestones.
- Use `log`, `error`, and `exception` for operational logging and failures.
- Use `metric` for structured numeric measurements.
- Use `custom` only when no existing semantic type fits or the payload is domain-specific and purely ad hoc.

## Explicit Event Types

| Event type | When to use it | Expected payload fields | Default level |
| --- | --- | --- | --- |
| `log` | Operational log line tied to a span | Any optional structured payload | `info` |
| `error` | Explicit failure signal without exception details | Any optional structured payload | `error` |
| `exception` | Captured exception with structured exception metadata | Common fields: `exception_type`, `exception_message`, `traceback` | `error` |
| `message` | Lightweight narrative milestone or human-readable note | Optional | `info` |
| `metric` | Numeric measurement attached to a span | `metric_name`, `metric_value`, optional `metric_unit` | `info` |
| `custom` | Domain-specific event that does not fit another semantic type | Any optional structured payload | `info` |
| `state_change` | Observable state transition that should render in the State view | `key`, optional `namespace`, optional `old_value`, optional `new_value` | `info` |
| `decision` | Branch point or choice with a selected outcome | `question`, `chosen`, optional `alternatives`, optional `reasoning` | `info` |
| `effect` | External side effect, tool call, or model invocation worth surfacing in the debugger | `effect_kind`, `has_external_side_effect`, optional `effect_id`, optional `idempotent`, optional `idempotency_key` | `info` |
| `wait` | Blocked or deferred progress that depends on later resolution | `wait_kind`, `phase`, optional `wait_id`, optional `resolution` | `info` |
| `snapshot_marker` | Debugger milestone worth surfacing as a dedicated timeline semantic | `marker_kind`, `label`, plus any optional contextual payload | `info` |

`message` and `custom` remain supported event types in this phase, but the Python SDK does not add dedicated `span.message()` or `span.custom()` helpers yet.

## Payload Guidance

### `state_change`

Use `state_change` for state that helps explain execution flow in the debugger.

Recommended payload shape:

```json
{
  "key": "status",
  "namespace": "order",
  "old_value": "pending",
  "new_value": "approved"
}
```

- `key` should be the field name within the namespace, not a whole sentence.
- `namespace` groups related keys in the State tab. Omit it for general state.
- `old_value` and `new_value` may be scalars, arrays, or objects.

Use `state_change` instead of `custom` when you want the debugger to show a before/after transition rather than a raw JSON blob.

Python SDK example:

```python
with span("validate_order") as s:
    s.state_change(
        "status",
        "pending",
        "approved",
        namespace="order",
        message="Order approved after validation",
    )
```

### `decision`

Use `decision` when the system evaluates options and selects one.

Recommended payload shape:

```json
{
  "question": "Which model should handle the request?",
  "chosen": "gpt-4.1",
  "alternatives": ["gpt-4o-mini", "gpt-4.1"],
  "reasoning": "Escalated for higher quality on refund requests"
}
```

- `question` should describe the branch point as a question or decision prompt.
- `chosen` should be the final selected option or outcome.
- `alternatives` should contain meaningful rejected options when available.
- `reasoning` should explain why the choice was made, not restate the choice.

Use `decision` instead of `custom` when the key debugging question is "why did the system choose this path?"

Python SDK example:

```python
with span("route_request") as s:
    s.decision(
        "Which model should handle the request?",
        "gpt-4.1",
        alternatives=["gpt-4o-mini", "gpt-4.1"],
        reasoning="Escalated for higher quality on refund requests",
        message="Escalated to a higher-capability model",
    )
```

### `effect`

Use `effect` when a span performs work whose observable consequence matters outside the current call stack: model calls, tool invocations, outbound API calls, writes to external systems, or similarly meaningful side effects.

Recommended payload shape:

```json
{
  "effect_kind": "api_call",
  "has_external_side_effect": true,
  "effect_id": "effect_123",
  "idempotent": false
}
```

- `effect_kind` is a descriptive category. Common values include `model_call`, `tool_call`, `api_call`, and `custom`.
- `has_external_side_effect` should be `false` for read-only or observational effects and `true` when the action can mutate external state.
- `effect_id` is optional. When omitted, the server may derive it.
- `idempotent` and `idempotency_key` are effect semantics in the payload, not ingest-level deduplication keys.

Implicit Python SDK examples:

```python
with span("draft_reply", kind="llm") as s:
    s.set_llm_response(
        model="gpt-4.1-mini",
        messages=[{"role": "user", "content": "Summarize this ticket"}],
        response={"role": "assistant", "content": "Summary"},
        tokens_in=120,
        tokens_out=48,
    )
```

```python
with span("lookup_weather", kind="tool") as s:
    s.set_tool_call(
        "weather_api",
        {"city": "Berlin"},
        {"temperature_c": 18},
        has_external_side_effect=False,
    )
```

Explicit Python SDK example:

```python
with span("sync_billing") as s:
    s.effect(
        "api_call",
        has_external_side_effect=True,
        effect_id="billing-sync-42",
        payload={"target": "billing"},
    )
```

### `wait`

Use `wait` when the interesting debugging fact is that execution paused pending some later resolution.

Recommended payload shape:

```json
{
  "wait_kind": "human_approval",
  "phase": "entered",
  "wait_id": "wait_123"
}
```

- `wait_kind` is the category of dependency. Common values include `human_approval`, `external`, `timer`, and `custom`.
- `phase` describes the lifecycle point. Common values include `entered` and `resolved`.
- `resolution` is optional and useful when the wait is resolved with a meaningful outcome such as `approved`, `rejected`, or `elapsed`.
- `wait_id` is optional. When omitted, the server may derive it.

Python SDK examples:

```python
with span("review_order") as s:
    s.wait("human_approval", phase="entered", payload={"reviewer": "ops"})
```

```python
with span("review_order") as s:
    s.wait(
        "human_approval",
        phase="resolved",
        wait_id="approval-42",
        resolution="approved",
    )
```

### `snapshot_marker`

Use `snapshot_marker` when the important debugging fact is that execution reached a notable milestone and you want that milestone to stay semantically distinct from generic logs or messages.

Recommended payload shape:

```json
{
  "marker_kind": "milestone",
  "label": "Validation passed"
}
```

- `marker_kind` is a conventionally required non-empty string. Start with `milestone` unless you have a clearer debugger-oriented vocabulary such as `phase` or `handoff`.
- `label` is conventionally required and should be the human-facing marker text shown in the timeline.
- `snapshot_marker` is a debugger milestone event, not a checkpoint, resume token, or replay primitive.

Python SDK example:

```python
with span("checkout_flow") as s:
    s.snapshot_marker(
        "Inventory reserved",
        marker_kind="phase",
        payload={"reservation_id": "res-42"},
    )
```

## Quick Heuristics

- If you want the debugger to show `old → new`, emit `state_change`.
- If you want the debugger to show `question → chosen`, emit `decision`.
- If you want the debugger to show "this span called out to something important", emit `effect`.
- If you want the debugger to show "this span had to wait", emit `wait`.
- If you want the debugger to show "execution reached this milestone", emit `snapshot_marker`.
- If you only need a human-readable sentence, emit `message`.
- If you need a free-form payload with no special UI, emit `custom`.
