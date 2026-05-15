# Python SDK

## Current structure

```
sdks/python/
├── pyproject.toml
├── scripts/generate_types.py
├── src/continua/
│   ├── __init__.py
│   ├── client.py
│   ├── batch.py
│   ├── trace.py
│   ├── span.py
│   ├── session.py
│   ├── exceptions.py
│   └── types.py
└── tests/
```

## Current API shape
- `Continua.init(...)` installs a global singleton client
- `trace`, `span`, and `session` are context-manager/decorator helpers
- batching is local and asynchronous through `BatchQueue`
- explicit event helpers exist on spans:
  - `log`
  - `error`
  - `exception`
  - `metric`
- async ingest support exists through:
  - `ingest_mode`
  - `wait_for_batch()`

## Important behavior
- the client sends batches to `/v1/ingest`
- auth uses `X-API-Key`
- traces/spans/events accumulate in the batch queue and flush automatically
- if the global client is not initialized, trace/span helpers quietly skip emission instead of failing user code
- `session_id` is an external session key, not a server-side UUID the client has to create

## Current ingest modes
- `sync`
- `async_v2`
- `server_default`

These map to request params/headers inside `client.py`. Preserve this behavior unless the contract changes.

## Contract generation
- Python types are generated from the OpenAPI bundle
- repo-wide command: `make generate`
- SDK-local fallback: `cd sdks/python && uv run python scripts/generate_types.py`

## Tests
- `cd sdks/python && uv run pytest`
- relevant suites already cover client behavior, batch queueing, spans, traces, errors, and integration behavior

## Canonical usage

```python
from continua import Continua, span, trace

Continua.init(
    api_key="...",
    endpoint="http://localhost:8080",
    ingest_mode="server_default",  # or "sync", "async_v2"
)


@trace(name="agent_run")
def run() -> None:
    with span("llm_call", kind="llm") as s:
        s.set_input({"prompt": "..."})
        s.set_output({"answer": "..."})
        s.set_model("gpt-4.1-mini")
        s.set_tokens(prompt=120, completion=48)
```

There is no async client class today. `Continua.init()` returns the global singleton; trace/span/session are module-level helpers. Do not invent `client.trace()` or `AsyncContinua` — those are not part of the SDK.
