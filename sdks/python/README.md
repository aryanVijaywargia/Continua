# Continua SDK for Python

Python SDK for Continua — the self-hosted durable execution engine with built-in
observability for AI agents. Sends traces, spans, sessions, events, and engine-control
requests to a Continua server.

## Installation

```bash
pip install continua-sdk
```

## Quick Start

Initialize the client once at process startup:

```python
from continua import Continua, span, trace

Continua.init(
    api_key="your-api-key",
    endpoint="https://api.your-continua-host.com",
    ingest_mode="server_default",  # or "sync", "async_v2"
)


@trace(name="answer_question")
def answer_question(question: str) -> str:
    with span("retrieve_context", kind="tool") as s:
        s.set_input({"question": question})
        context = "retrieved context"
        s.set_output({"context": context})

    with span("call_model", kind="llm") as s:
        s.set_input({"question": question, "context": context})
        answer = "model answer"
        s.set_output({"answer": answer})
        return answer
```

For a local or self-hosted Continua server, point the SDK at your server URL:

```python
from continua import Continua

Continua.init(
    api_key="dev-api-key",
    endpoint="http://localhost:8080",
)
```

This package is the client library only. It sends data to a hosted or
self-hosted Continua server; it does not install or run the Go server,
Postgres, River workers, or debugger UI.

## Async Ingest Modes

- `server_default`: defer to the server rollout setting.
- `sync`: send `sync=true` and wait for inline processing.
- `async_v2`: opt into true async ingest with `X-Continua-Async-Version: 2`.

True async is not read-after-write. If your code needs the batch to be fully processed before it reads the ingested data back, use `ingest_mode="sync"` or poll with `wait_for_batch()`:

```python
result = client.wait_for_batch(batch_id, timeout=30, poll_interval=0.5)
```

## Development

```bash
# From sdks/python
uv sync

uv run pytest

uv run mypy src/

uv build
```

## Publishing

The Python package is published from this directory:

```bash
cd sdks/python
uv build
uv publish
```

Prefer PyPI Trusted Publishing in CI for repeatable releases once the project is
ready for public distribution.
