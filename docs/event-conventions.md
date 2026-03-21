# Event Conventions

Continua events are debugger-facing observability signals.

These are debugger semantics, not replay primitives.

Use them to explain what happened during trace execution, not to model checkpoints, resumability, or durable workflow state machines.

## Choosing an Event Type

- Use `state_change` when an observable piece of state changes and you want the debugger to show the transition directly.
- Use `decision` when the system chooses between alternatives and the debugger should show the branch point and rationale.
- Use `custom` only when no existing semantic type fits or the payload is domain-specific and purely ad hoc.
- Use `message` for lightweight narrative milestones.
- Use `log`, `error`, and `exception` for operational logging and failures.
- Use `metric` for structured numeric measurements.

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

## Quick Heuristics

- If you want the debugger to show `old → new`, emit `state_change`.
- If you want the debugger to show `question → chosen`, emit `decision`.
- If you only need a human-readable sentence, emit `message`.
- If you need a free-form payload with no special UI, emit `custom`.
