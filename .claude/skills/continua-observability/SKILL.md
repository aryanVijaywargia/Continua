---
name: continua-observability
description: Domain-specific guide for Continua's trace/span data model, WebSocket events, and observability patterns. Use when working on trace ingestion, span trees, real-time updates, or the replay system.
---

# Continua Observability Domain

## Purpose

Guide development of Continua's core domain - capturing, storing, and replaying AI agent executions. This skill covers the **what** and **why** of Continua's data model, not generic observability concepts.

## When to Use This Skill

Activates when working on:
- Trace or span ingestion
- WebSocket real-time events
- Tree reconstruction queries
- Replay/debugging functionality
- Token counting or cost calculation

---

## Core Concepts

### What Continua Captures

```
Session (optional grouping)
    └── Trace (one agent execution)
        ├── Span (LLM call)
        │   ├── Payload (request)
        │   └── Payload (response)
        ├── Span (tool invocation)
        │   └── child spans...
        └── Span (agent decision)
```

### Data Model

| Entity | Purpose | Key Fields |
|--------|---------|------------|
| **Session** | Groups related traces | `id`, `name`, `metadata` |
| **Trace** | Single agent execution | `id`, `status`, `total_tokens`, `total_cost` |
| **Span** | Operation within trace | `id`, `trace_id`, `parent_span_id`, `kind`, `status` |
| **Payload** | Request/response data | `span_id`, `direction`, `body` |

### Span Kinds

| Kind | Use Case |
|------|----------|
| `LLM` | API call to language model |
| `TOOL` | External tool invocation |
| `CHAIN` | Sequence of operations |
| `AGENT` | Agent decision/reasoning |
| `CUSTOM` | User-defined |

### Status Lifecycle

```
Trace:  RUNNING → COMPLETED | FAILED
Span:   SCHEDULED → STARTED → COMPLETED | FAILED
```

---

## Tree Structure

Spans form a **tree**, not a flat list. The `parent_span_id` creates the hierarchy.

```sql
-- Root spans have NULL parent
INSERT INTO spans (trace_id, parent_span_id, name, kind)
VALUES ($1, NULL, 'agent_run', 'AGENT');

-- Child spans reference parent
INSERT INTO spans (trace_id, parent_span_id, name, kind)
VALUES ($1, $2, 'llm_call', 'LLM');
```

### Querying Trees

```sql
-- Recursive CTE for tree reconstruction
WITH RECURSIVE span_tree AS (
    -- Base: root spans
    SELECT *, 0 as depth, ARRAY[id] as path
    FROM spans
    WHERE trace_id = $1 AND parent_span_id IS NULL

    UNION ALL

    -- Recursive: children
    SELECT s.*, st.depth + 1, st.path || s.id
    FROM spans s
    JOIN span_tree st ON s.parent_span_id = st.id
)
SELECT * FROM span_tree ORDER BY path;
```

---

## WebSocket Events

Real-time updates use Zod-typed events (source of truth: `contracts/websocket/events.ts`).

### Server → Client Events

```typescript
// Span lifecycle
SpanCreatedEvent   // New span started
SpanUpdatedEvent   // Span status/metrics changed

// Streaming
StreamChunkEvent   // LLM streaming chunk

// Trace lifecycle
TraceCompletedEvent  // Trace finished

// Errors
ErrorEvent         // Something went wrong
```

### Client → Server Messages

```typescript
SubscribeMessage   // Start receiving events for trace/session
UnsubscribeMessage // Stop receiving events
```

### Event Flow

```
Client                    Server
   |                         |
   |-- subscribe(traceId) -->|
   |                         |
   |<-- span.created --------|
   |<-- stream.chunk --------|
   |<-- span.updated --------|
   |<-- trace.completed -----|
```

---

## Token Counting & Cost

### Per-Span Metrics

```sql
tokens_in    -- Input tokens (prompt)
tokens_out   -- Output tokens (completion)
cost_usd     -- Cost for this span
latency_ms   -- Duration in milliseconds
```

### Trace Aggregation

When trace completes, aggregate from spans:

```sql
UPDATE traces SET
    total_tokens_in = (SELECT SUM(tokens_in) FROM spans WHERE trace_id = $1),
    total_tokens_out = (SELECT SUM(tokens_out) FROM spans WHERE trace_id = $1),
    total_cost_usd = (SELECT SUM(cost_usd) FROM spans WHERE trace_id = $1)
WHERE id = $1;
```

---

## Navigation

| Need to... | Read this |
|------------|-----------|
| Understand trace data flow | [trace-lifecycle.md](resources/trace-lifecycle.md) |
| Implement WebSocket handlers | [websocket-events.md](resources/websocket-events.md) |
| Work with replay system | [replay.md](resources/replay.md) |

---

**Skill Status**: COMPLETE
**Line Count**: ~140
