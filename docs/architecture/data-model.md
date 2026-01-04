# Data Model

## Core Entities

### Session
A session groups related traces together.

### Trace
A trace represents a single execution of an AI agent.

### Span
A span represents a single operation within a trace (LLM call, tool invocation, etc.).

### Payload
Stores request/response bodies for spans.

## Relationships

```
Session 1-* Trace 1-* Span 1-* Payload
                      |
                      └── parent_span_id (self-referential)
```
