# Continua SDK for Python

Python SDK for the Continua AI Agent Observability Platform.

## Installation

```bash
pip install continua
```

## Quick Start

```python
from continua import ContinuaClient

client = ContinuaClient(base_url="http://localhost:8080")
```

## Development

```bash
# Install with dev dependencies
uv sync --all-extras

# Run tests
uv run pytest

# Type check
uv run mypy src/
```
