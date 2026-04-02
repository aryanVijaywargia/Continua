# Data Model

> **Status: Current**
> This is the current platform data model used by the Go server, Postgres schema, and debugger UI.

## Core Persisted Entities

### Project

`projects` scope all protected data access. API-key auth resolves a request into a project before reading or writing platform data.

### Ingest Batch

`ingest_batches` store durable idempotency and async lifecycle state for `POST /v1/ingest`.

Important semantics:
- one batch is identified externally by `batch_key`
- async acceptance and polling are backed by persisted batch rows
- legacy compatibility may exist in status handling, but the active async path uses the true-async batch model

### Ingest Batch Payload

`ingest_batch_payloads` store compressed request payloads for accepted async batches until the worker processes and cleans them up.

### Session

`sessions` group related traces. Each row has:
- an internal UUID primary key
- a user-facing `external_id`
- optional narrative and workflow-facing metadata used by the debugger

### Trace

`traces` represent individual executions within a session or standalone context. Each row has:
- an internal UUID primary key
- an external `trace_id`
- aggregate rollup fields such as duration, status, cost, token counts, and error counts
- trace-level input/output payload fields

### Span

`spans` represent operations within a trace. Each row has:
- an internal UUID primary key
- an external `span_id`
- an external `parent_span_id` for tree reconstruction
- kind, status, timing, cost/token fields, model/provider fields, metadata, and span-level payloads

### Span Event

`span_events` store explicit append-only events such as logs, errors, exceptions, metrics, and semantic debugger events.

Important semantics:
- explicit events are stored separately from spans
- trace timelines merge explicit events with synthetic lifecycle events derived from spans
- session compare currently focuses on semantic event types such as `decision`, `effect`, and `wait`

## Relationship Shape

```text
projects
  1-* ingest_batches
  1-* sessions
  1-* traces

sessions
  1-* traces

traces
  1-* spans

spans
  1-* span_events
  0..1 parent_span_id -> spans.span_id (external ID relationship)
```

## Identity And Mapping Rules

- `sessions.external_id` is the user-facing session identifier
- `traces.trace_id` is the external trace identifier
- `spans.span_id` and `spans.parent_span_id` are external span identifiers
- internal UUIDs are the persistence identity used inside the database and server code

## Payload And Timeline Notes

- there is no active standalone runtime `payloads` table for trace/span request and response bodies
- trace payloads live on `traces`
- span payloads live on `spans`
- accepted async payload blobs live temporarily in `ingest_batch_payloads`
- the timeline API returns explicit `span_events` plus synthetic lifecycle markers
