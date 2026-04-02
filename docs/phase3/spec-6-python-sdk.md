> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Spec 6: Python SDK Polish

## Overview

Enhanced the Python SDK with custom exceptions, retry with exponential backoff, session context management, and convenient helper methods for LLM tracing.

## Changes

### 1. Custom Exception Hierarchy

**File:** `sdks/python/src/continua/exceptions.py`

Created a hierarchy of exceptions for different error types:

```python
ContinuaError             # Base exception
├── AuthenticationError   # 401 responses
├── RateLimitError        # 429 responses (includes retry_after)
├── ValidationError       # 400 responses (includes details dict)
└── NetworkError          # After retries exhausted (includes retry_count, cause)
```

### 2. Retry with Exponential Backoff

**File:** `sdks/python/src/continua/client.py`

Added configurable retry logic:

- **Default retries:** 3 (4 total attempts)
- **Base delay:** 1.0 seconds
- **Max delay:** 30.0 seconds
- **Jitter:** Random 0-1 second added to prevent thundering herd

Retry behavior:
- Retries on: `ConnectError`, `TimeoutException`, 5xx responses
- No retry on: 401 (auth), 400 (validation), 429 (rate limit - raises immediately)
- After retries exhausted: raises `NetworkError` with retry count and original cause

### 3. Session Context Manager

**File:** `sdks/python/src/continua/session.py`

```python
from continua import session, trace

with session("user_123") as sess:
    @trace()
    def my_agent(query):
        # trace automatically inherits session_id
        return "response"
```

Features:
- Auto-generates UUID if session_id not provided
- Supports `user_id` and `metadata` parameters
- All traces within context inherit session_id, user_id, and metadata
- Uses `contextvars` for thread-safe context propagation

### 4. Span Helper Methods

**File:** `sdks/python/src/continua/span.py`

Added convenience methods:

```python
with span("llm_call", kind="llm") as s:
    # Set all LLM metadata in one call
    s.set_llm_response(
        model="gpt-4",
        messages=[{"role": "user", "content": "Hello"}],
        response="Hi there!",
        tokens_in=10,
        tokens_out=5,
        provider="openai",
        cost=0.001
    )

with span("tool_call", kind="tool") as s:
    # Set tool call details
    s.set_tool_call(
        tool_name="search",
        arguments={"query": "weather"},
        result={"temp": 72}
    )

with span("operation") as s:
    # Log events during span execution
    s.log("Starting processing", level="info")
    s.log("Warning: slow response", level="warning", payload={"latency_ms": 500})
```

### 5. Updated Exports

**File:** `sdks/python/src/continua/__init__.py`

All new components exported at package level:

```python
from continua import (
    Continua,
    trace, span, session,
    TraceContext, SpanContext, SessionContext,
    get_current_trace, get_current_span, get_current_session,
    ContinuaError, AuthenticationError, RateLimitError,
    ValidationError, NetworkError,
)
```

## Testing

All tests pass:

```bash
cd sdks/python && uv run pytest tests/ -v
# 35 passed, 12 skipped (integration tests)
```

Test coverage:
- `test_errors.py`: Exception types, retry behavior, session context, span helpers
- `test_client.py`: Singleton pattern, batch queue
- `test_trace.py`: Trace context and decorator
- `test_span.py`: Span context and nesting
- `test_batch.py`: Batch queue behavior

## Usage Example

```python
from continua import Continua, session, trace, span

# Initialize with custom retry settings
Continua.init(
    api_key="sk-...",
    max_retries=5,
    base_delay=0.5
)

# Use session context for grouped traces
with session("user_session_abc", user_id="user_123"):
    @trace()
    def process_request(query: str) -> str:
        with span("openai_call", kind="llm") as s:
            response = call_openai(query)
            s.set_llm_response(
                model="gpt-4",
                messages=[{"role": "user", "content": query}],
                response=response,
                tokens_in=100,
                tokens_out=50
            )
        return response

    process_request("What is 2+2?")

Continua.shutdown()
```

## Specification Reference

Implements: `openspec/implemented/add-reliability-search-sessions/specs/python-sdk-polish/spec.md`
