# Python SDK

## Project Structure

```
sdks/python/
├── pyproject.toml          # Package config (uv)
├── src/continua/
│   ├── __init__.py         # Public exports
│   ├── client.py           # HTTP client
│   ├── tracer.py           # Trace/span management
│   ├── context.py          # Context propagation
│   ├── decorators.py       # @trace, @span decorators
│   ├── types.py            # Type definitions
│   └── integrations/
│       ├── langchain.py    # LangChain callback
│       └── openai.py       # OpenAI wrapper
├── tests/
└── scripts/
    └── generate_types.py   # Generate from OpenAPI
```

## Core API

### Basic Usage

```python
from continua import Continua

client = Continua(api_key="...", base_url="http://localhost:8080")

# Context manager for traces
with client.trace("agent_run") as trace:
    with client.span("thinking", kind="AGENT") as span:
        # Your code
        span.set_metadata({"model": "gpt-4"})
```

### Decorators

```python
from continua import trace, span

@trace("my_agent")
def run_agent(input: str):
    result = think(input)
    return act(result)

@span("thinking", kind="LLM")
def think(input: str):
    return openai.chat.completions.create(...)
```

## Implementation

### Client

```python
# src/continua/client.py
import httpx
from typing import Optional
from .types import Trace, Span, SpanKind

class Continua:
    def __init__(self, api_key: str, base_url: str = "http://localhost:8080"):
        self._client = httpx.Client(
            base_url=base_url,
            headers={"Authorization": f"Bearer {api_key}"}
        )

    def trace(self, name: str, session_id: Optional[str] = None) -> "TraceContext":
        return TraceContext(self, name, session_id)

    def span(self, name: str, kind: SpanKind = "CUSTOM") -> "SpanContext":
        return SpanContext(self, name, kind)

    def _create_trace(self, name: str, session_id: Optional[str]) -> Trace:
        resp = self._client.post("/api/traces", json={
            "name": name,
            "session_id": session_id
        })
        return Trace(**resp.json())

    def _create_span(self, trace_id: str, parent_span_id: Optional[str],
                     name: str, kind: SpanKind) -> Span:
        resp = self._client.post("/api/spans", json={
            "trace_id": trace_id,
            "parent_span_id": parent_span_id,
            "name": name,
            "kind": kind
        })
        return Span(**resp.json())
```

### Context Management

```python
# src/continua/context.py
import threading
from contextvars import ContextVar
from typing import Optional

_current_trace: ContextVar[Optional[str]] = ContextVar('trace', default=None)
_current_span: ContextVar[Optional[str]] = ContextVar('span', default=None)

def get_current_trace() -> Optional[str]:
    return _current_trace.get()

def get_current_span() -> Optional[str]:
    return _current_span.get()

class TraceContext:
    def __init__(self, client: "Continua", name: str, session_id: Optional[str]):
        self._client = client
        self._name = name
        self._session_id = session_id
        self._trace = None
        self._token = None

    def __enter__(self):
        self._trace = self._client._create_trace(self._name, self._session_id)
        self._token = _current_trace.set(self._trace.id)
        return self._trace

    def __exit__(self, exc_type, exc_val, exc_tb):
        status = "FAILED" if exc_type else "COMPLETED"
        self._client._complete_trace(self._trace.id, status)
        _current_trace.reset(self._token)

class SpanContext:
    def __init__(self, client: "Continua", name: str, kind: SpanKind):
        self._client = client
        self._name = name
        self._kind = kind
        self._span = None
        self._token = None

    def __enter__(self):
        trace_id = get_current_trace()
        if not trace_id:
            raise RuntimeError("No active trace")

        parent_span_id = get_current_span()
        self._span = self._client._create_span(
            trace_id, parent_span_id, self._name, self._kind
        )
        self._token = _current_span.set(self._span.id)
        return self._span

    def __exit__(self, exc_type, exc_val, exc_tb):
        status = "FAILED" if exc_type else "COMPLETED"
        self._client._complete_span(self._span.id, status)
        _current_span.reset(self._token)
```

### LangChain Integration

```python
# src/continua/integrations/langchain.py
from langchain.callbacks.base import BaseCallbackHandler
from continua import Continua

class ContinuaCallback(BaseCallbackHandler):
    def __init__(self, client: Continua):
        self.client = client
        self._span_stack = []

    def on_llm_start(self, serialized, prompts, **kwargs):
        span = self.client._create_span(
            trace_id=get_current_trace(),
            parent_span_id=get_current_span(),
            name=serialized.get("name", "llm"),
            kind="LLM"
        )
        self._span_stack.append(span)

    def on_llm_end(self, response, **kwargs):
        span = self._span_stack.pop()
        self.client._complete_span(span.id, "COMPLETED")

    def on_tool_start(self, serialized, input_str, **kwargs):
        span = self.client._create_span(
            trace_id=get_current_trace(),
            parent_span_id=get_current_span(),
            name=serialized.get("name", "tool"),
            kind="TOOL"
        )
        self._span_stack.append(span)

    def on_tool_end(self, output, **kwargs):
        span = self._span_stack.pop()
        self.client._complete_span(span.id, "COMPLETED")
```

## Type Generation

Types are generated from OpenAPI spec:

```bash
# Run from sdks/python/
uv run python scripts/generate_types.py
```

```python
# scripts/generate_types.py
from openapi_python_client import generate

generate(
    url="../../contracts/openapi/openapi.bundle.yaml",
    output_path="src/continua/generated",
)
```

## Testing

```bash
cd sdks/python
uv run pytest
```

```python
# tests/test_tracer.py
import pytest
from continua import Continua

@pytest.fixture
def client():
    return Continua(api_key="test", base_url="http://localhost:8080")

def test_trace_context(client, mock_server):
    with client.trace("test") as trace:
        assert trace.id is not None
        assert trace.status == "RUNNING"

    # After context, trace should be completed
    trace = client.get_trace(trace.id)
    assert trace.status == "COMPLETED"
```

## Async Support

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
