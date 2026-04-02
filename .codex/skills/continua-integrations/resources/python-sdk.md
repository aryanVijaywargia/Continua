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
  - `state_change`
  - `decision`
  - `effect`
  - `wait`
  - `snapshot_marker`
- async ingest support exists through:
  - `ingest_mode`
  - `wait_for_batch()`
- `set_llm_response(...)` and `set_tool_call(...)` emit one implicit `effect` event per helper type per span unless disabled

## Important behavior
- the client sends batches to `/v1/ingest`
- auth uses `X-API-Key`
- traces/spans/events accumulate in the batch queue and flush automatically
- if the global client is not initialized, trace/span helpers quietly skip emission instead of failing user code
- `session_id` is an external session key, not a server-side UUID the client has to create
- the generated `types.py` includes current compare/read models from OpenAPI, but there is not a higher-level Python compare client surface yet

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

```python
# src/continua/async_client.py
import httpx

class AsyncContinua:
    def __init__(self, api_key: str, base_url: str):
        self._client = httpx.AsyncClient(...)

    async def trace(self, name: str):
        return AsyncTraceContext(self, name)

# Usage
async with client.trace("agent") as trace:
    async with client.span("llm") as span:
        response = await openai.chat.completions.create(...)
```
