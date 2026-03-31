# Continua SDK for Python

Python SDK for the Continua AI Agent Observability Platform.

## Installation

```bash
pip install continua
```

## Quick Start

```python
from continua import Continua

client = Continua(
    api_key="your-api-key",
    endpoint="http://localhost:8080",
    ingest_mode="server_default",  # or "sync", "async_v2"
)
```

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
# Install with dev dependencies
uv sync

# Run tests
uv run pytest

# Type check
uv run mypy src/
```
