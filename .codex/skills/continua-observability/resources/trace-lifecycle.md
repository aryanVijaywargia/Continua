# Trace lifecycle

## Current ingest path

```
Python SDK or custom client
    -> POST /v1/ingest
    -> auth / project scope
    -> validate request shape
    -> claim batch idempotency row
    -> sync write path or async batch acceptance
    -> River rollup jobs
    -> debugger UI reads traces, spans, sessions, timeline
```

## What is persisted
- traces
- spans
- explicit span events
- async ingest batch rows and stored payloads

There is no active payload table for span request/response bodies. Trace and span payloads are stored on the `traces` and `spans` rows themselves.

## Batch lifecycle

### Sync path
- handler calls `ingest.Service.Ingest`
- service validates via `Processor.Validate`
- service claims batch idempotency inside a transaction
- processor upserts traces, spans, and events
- service enqueues trace rollup jobs in the same transaction
- batch is marked completed before commit

### True async path
- handler calls `ingest.Service.AcceptAsync`
- request is validated before acceptance
- batch row is claimed
- compressed request payload is stored in `ingest_batch_payloads`
- River ingest job is enqueued
- worker later marks the batch `processing`, runs `Processor.ProcessBatch`, enqueues rollups, deletes payload on success, and marks terminal state

## Identity model
- project: internal UUID, derived from API key
- session: internal UUID plus external `external_id`
- trace: internal UUID plus external `trace_id`
- span: internal UUID plus external `span_id`
- span tree parent link: external `parent_span_id`

## Status mapping
- ingest input status values are lower-case (`running`, `completed`, `failed`)
- API trace statuses are mapped to `RUNNING`, `COMPLETED`, `FAILED`
- API span statuses are mapped to `SCHEDULED`, `STARTED`, `COMPLETED`, `FAILED`

## Timeline model
- explicit events come from `span_events`
- synthetic lifecycle events are derived from span start/end/status data
- `/api/traces/{id}/events` merges both and returns cursor-based pages
- the frontend polls this endpoint every 3 seconds for running traces

## Rollups
- trace totals are computed from spans
- River rollup jobs coalesce by trace ID and rerun in-process if the trace version changes mid-rollup
