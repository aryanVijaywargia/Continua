---
name: continua-integrations
description: Guide for Continua's integration methods - LLM proxy, Python SDK, TypeScript SDK, and framework adapters. Use when developing SDKs, proxy capture layer, or framework integrations.
---

# Continua Integrations

## Purpose

Continua provides three integration methods for capturing AI agent traces. This skill covers development patterns for each method.

## Shared reference

- Read [../../references/decisions.md](../../references/decisions.md) for contract ownership, generated file boundaries, and architecture references before SDK or proxy changes.

## When to Use This Skill

Activates when working on:
- LLM proxy in `internal/proxy/`
- Python SDK in `sdks/python/`
- TypeScript SDK in `sdks/typescript/`
- Framework adapters (LangChain, CrewAI)

---

## Integration Methods

### 1. Zero-Code LLM Proxy

Intercepts LLM API calls without code changes.

```
Your App → Continua Proxy → OpenAI/Anthropic
              ↓
        Captures traces
```

**Best for:** Quick setup, existing applications

### 2. SDK Instrumentation

Explicit tracing with full control.

```python
with continua.trace("agent_run"):
    with continua.span("llm_call", kind="LLM"):
        response = openai.chat.completions.create(...)
```

**Best for:** Custom agents, granular control

### 3. Framework Adapters

Auto-instrumentation for popular frameworks.

```python
from continua.langchain import ContinuaCallback
chain.invoke(input, config={"callbacks": [ContinuaCallback()]})
```

**Best for:** LangChain, CrewAI, etc.

---

## SDK Design Principles

### 1. Minimal Dependencies

```toml
# sdks/python/pyproject.toml
dependencies = [
    "httpx>=0.24",   # HTTP client
    "pydantic>=2.0", # Validation (optional)
]
```

### 2. Context Propagation

```python
# Thread-local context for trace/span
_context = threading.local()

@contextmanager
def trace(name: str):
    parent = getattr(_context, 'current_trace', None)
    trace_id = create_trace(name, parent_id=parent)
    _context.current_trace = trace_id
    try:
        yield trace_id
    finally:
        complete_trace(trace_id)
        _context.current_trace = parent
```

### 3. Type Safety

```typescript
// sdks/typescript/src/types.ts
export interface SpanOptions {
    name: string;
    kind: SpanKind;
    metadata?: Record<string, unknown>;
}

export type SpanKind = 'LLM' | 'TOOL' | 'CHAIN' | 'AGENT' | 'CUSTOM';
```

### 4. Graceful Degradation

```python
def capture_span(func):
    def wrapper(*args, **kwargs):
        try:
            with span(func.__name__):
                return func(*args, **kwargs)
        except ContinuaError:
            # Continua unavailable, proceed without tracing
            return func(*args, **kwargs)
    return wrapper
```

---

## Quick Reference

| SDK | Location | Test Command |
|-----|----------|--------------|
| Python | `sdks/python/` | `uv run pytest` |
| TypeScript | `sdks/typescript/` | `pnpm test` |

---

## Navigation

| Need to... | Read this |
|------------|-----------|
| Build the LLM proxy | [proxy.md](resources/proxy.md) |
| Develop Python SDK | [python-sdk.md](resources/python-sdk.md) |
| Develop TypeScript SDK | [typescript-sdk.md](resources/typescript-sdk.md) |

---

**Skill Status**: COMPLETE
**Line Count**: ~100
