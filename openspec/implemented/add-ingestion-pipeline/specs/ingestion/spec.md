# Spec: Ingestion

## Overview

Batch ingestion capability for accepting traces, spans, and events from SDKs.

---

## ADDED Requirements

### Requirement: Batch Ingestion Endpoint

The system SHALL provide a `POST /v1/ingest` endpoint that accepts batched trace data.

#### Scenario: Successful sync ingestion
- **Given**: A valid ingest request with batch_key, traces, spans, and events
- **And**: Query param `sync=true`
- **When**: The request is sent to `POST /v1/ingest?sync=true`
- **Then**: The response status is 200
- **And**: The response body contains `{"status": "ok", "batch_key": "<batch_key>"}`
- **And**: All traces, spans, and events are persisted

#### Scenario: Successful async ingestion
- **Given**: A valid ingest request
- **And**: Query param `sync=false` or not provided
- **When**: The request is sent to `POST /v1/ingest`
- **Then**: The response status is 202
- **And**: The response body contains `{"status": "accepted", "batch_key": "<batch_key>"}`

#### Scenario: Request too large
- **Given**: An ingest request larger than 5MB
- **When**: The request is sent to `POST /v1/ingest`
- **Then**: The response status is 413
- **And**: The response body contains `{"error": "batch exceeds 5MB limit"}`

---

### Requirement: Trace Upsert Semantics

The system SHALL upsert traces using patch semantics where NULL values do not overwrite existing values.

#### Scenario: Create new trace
- **Given**: A trace with `trace_id: "trace-001"` does not exist
- **When**: An ingest request contains that trace
- **Then**: A new trace is created with an internal UUID
- **And**: The external `trace_id` is stored for lookup

#### Scenario: Update existing trace
- **Given**: A trace with `trace_id: "trace-001"` exists with `name: "Old Name"`
- **When**: An ingest request contains the same trace_id with `name: "New Name"`
- **Then**: The trace name is updated to "New Name"
- **And**: Other fields not provided are preserved

#### Scenario: Error status preserved
- **Given**: A trace exists with `status: "error"`
- **When**: An update attempts to set `status: "ok"`
- **Then**: The status remains "error" (never downgraded)

---

### Requirement: Span Upsert with Trace UUID FK

The system SHALL upsert spans using the internal trace UUID as foreign key.

#### Scenario: Create span for existing trace
- **Given**: A trace with `trace_id: "trace-001"` exists with internal UUID `abc-123`
- **When**: An ingest request contains a span with `trace_id: "trace-001"`, `span_id: "span-001"`
- **Then**: The span is created with `spans.trace_id = abc-123` (UUID FK)

#### Scenario: Span references unknown trace
- **Given**: No trace with `trace_id: "trace-999"` exists
- **When**: An ingest request contains only a span referencing that trace
- **Then**: The request fails with status 400
- **And**: Error message indicates unknown trace reference

---

### Requirement: Span Events Append-Only

The system SHALL insert span events as append-only records with idempotency.

#### Scenario: Insert new event
- **Given**: An event with `idempotency_key: "event-001"` does not exist
- **When**: An ingest request contains that event
- **Then**: The event is inserted

#### Scenario: Duplicate event silently ignored
- **Given**: An event with `idempotency_key: "event-001"` already exists
- **When**: An ingest request contains the same idempotency_key
- **Then**: The duplicate is silently ignored (no error)
- **And**: The insert count reflects 0 new events for that key

---

### Requirement: Invalid JSON Wrapping

The system SHALL wrap invalid JSON payloads instead of rejecting them.

#### Scenario: Valid JSON passthrough
- **Given**: A trace with `input: {"key": "value"}`
- **When**: The trace is ingested
- **Then**: The input is stored as-is

#### Scenario: Invalid JSON wrapped
- **Given**: A trace with `input: "{malformed json"`
- **When**: The trace is ingested
- **Then**: The request succeeds (not rejected)
- **And**: The input is stored as `{"__continua_raw": "{malformed json", "__parse_error": "..."}`

---

### Requirement: Payload Truncation

The system SHALL truncate payloads exceeding 64KB with metadata.

#### Scenario: Payload under limit
- **Given**: A span with input payload of 10KB
- **When**: The span is ingested
- **Then**: The input is stored as-is
- **And**: `input_truncated = false`

#### Scenario: Payload over limit
- **Given**: A span with input payload of 100KB
- **When**: The span is ingested
- **Then**: The input is truncated to ~64KB
- **And**: `input_truncated = true`
- **And**: `input_original_size_bytes = 100000`
- **And**: `input_truncation_reason = "size_limit"`

---

## Related Capabilities

- [idempotency](../idempotency/spec.md) - Batch deduplication
- [data-model](../data-model/spec.md) - Schema requirements
