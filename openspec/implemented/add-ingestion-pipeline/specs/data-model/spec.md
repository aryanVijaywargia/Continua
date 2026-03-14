# Spec: Data Model

## Overview

Extended database schema for multi-tenant ingestion with external ID mapping.

---

## ADDED Requirements

### Requirement: Projects Table

The system SHALL have a `projects` table for multi-tenant isolation.

#### Scenario: Project creation
- **Given**: A new project is being onboarded
- **When**: The project is created
- **Then**: A UUID is generated as primary key
- **And**: `name` and `api_key_hash` are stored
- **And**: `created_at` timestamp is set

### Schema

```sql
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

### Requirement: Ingest Batches Table

The system SHALL have an `ingest_batches` table for idempotency tracking.

#### Scenario: Batch record structure
- **Given**: A batch is being processed
- **When**: The batch is claimed
- **Then**: The record includes project_id, batch_key, status, and timestamps

### Schema

```sql
CREATE TABLE ingest_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    batch_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'processing',
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_completed_at TIMESTAMPTZ,
    trace_count INT DEFAULT 0,
    span_count INT DEFAULT 0,
    event_count INT DEFAULT 0,
    accepted_count INT DEFAULT 0,
    rejected_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, batch_key)
);
```

---

### Requirement: Span Events Table

The system SHALL have a `span_events` table for fine-grained event tracking.

#### Scenario: Event without span FK
- **Given**: An event references `span_id: "span-001"`
- **And**: No span with that ID exists yet
- **When**: The event is inserted
- **Then**: The insert succeeds (no FK constraint)
- **And**: The event can be queried as an "orphan"

### Schema

```sql
CREATE TABLE span_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    span_id TEXT NOT NULL,  -- No FK to spans (allows orphans)
    event_type TEXT NOT NULL,
    level TEXT NOT NULL DEFAULT 'info',
    event_ts TIMESTAMPTZ,
    server_ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sequence INT,
    message TEXT,
    payload JSONB,
    truncated BOOLEAN DEFAULT FALSE,
    original_size_bytes BIGINT,
    truncation_reason TEXT,
    idempotency_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_span_events_idempotency
    ON span_events(project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
```

---

## MODIFIED Requirements

### Requirement: Traces External ID

The `traces` table SHALL include an external `trace_id` TEXT column.

#### Scenario: Dual ID system
- **Given**: A trace is created via ingestion
- **When**: The SDK provides `trace_id: "my-trace-123"`
- **Then**: The trace has `id` (internal UUID) and `trace_id` (external TEXT)
- **And**: FKs reference the internal `id`
- **And**: Lookups can use either

### Schema Changes

```sql
ALTER TABLE traces ADD COLUMN trace_id TEXT NOT NULL;
ALTER TABLE traces ADD COLUMN project_id UUID NOT NULL REFERENCES projects(id);
ALTER TABLE traces ADD COLUMN user_id TEXT;
ALTER TABLE traces ADD COLUMN tags TEXT[];
ALTER TABLE traces ADD COLUMN environment TEXT;
ALTER TABLE traces ADD COLUMN release TEXT;
ALTER TABLE traces ADD COLUMN input JSONB;
ALTER TABLE traces ADD COLUMN output JSONB;
ALTER TABLE traces ADD COLUMN version INT DEFAULT 1;
ALTER TABLE traces ADD COLUMN server_received_at TIMESTAMPTZ DEFAULT NOW();

ALTER TABLE traces ADD UNIQUE(project_id, trace_id);
```

---

### Requirement: Spans External ID

The `spans` table SHALL include an external `span_id` TEXT column.

#### Scenario: Span identification
- **Given**: A span is created via ingestion
- **When**: The SDK provides `span_id: "span-001"`
- **Then**: The span has `id` (internal UUID) and `span_id` (external TEXT)
- **And**: `parent_span_id` references external TEXT (not UUID FK)

### Schema Changes

```sql
ALTER TABLE spans ADD COLUMN span_id TEXT NOT NULL;
ALTER TABLE spans ADD COLUMN project_id UUID NOT NULL REFERENCES projects(id);
ALTER TABLE spans ADD COLUMN type TEXT DEFAULT 'custom';
ALTER TABLE spans ADD COLUMN level TEXT DEFAULT 'default';
ALTER TABLE spans ADD COLUMN input JSONB;
ALTER TABLE spans ADD COLUMN input_truncated BOOLEAN DEFAULT FALSE;
ALTER TABLE spans ADD COLUMN input_original_size_bytes BIGINT;
ALTER TABLE spans ADD COLUMN output JSONB;
ALTER TABLE spans ADD COLUMN output_truncated BOOLEAN DEFAULT FALSE;
ALTER TABLE spans ADD COLUMN output_original_size_bytes BIGINT;
ALTER TABLE spans ADD COLUMN model TEXT;
ALTER TABLE spans ADD COLUMN provider TEXT;
ALTER TABLE spans ADD COLUMN prompt_tokens BIGINT;
ALTER TABLE spans ADD COLUMN completion_tokens BIGINT;
ALTER TABLE spans ADD COLUMN total_tokens BIGINT;
ALTER TABLE spans ADD COLUMN total_cost NUMERIC(10, 6) DEFAULT 0;
ALTER TABLE spans ADD COLUMN version INT DEFAULT 1;
ALTER TABLE spans ADD COLUMN server_received_at TIMESTAMPTZ DEFAULT NOW();

-- parent_span_id changes from UUID to TEXT (no FK)
ALTER TABLE spans ALTER COLUMN parent_span_id TYPE TEXT;

ALTER TABLE spans ADD UNIQUE(trace_id, span_id);
```

---

## Index Requirements

### Requirement: Query Performance Indexes

The system SHALL have indexes for common query patterns.

```sql
-- Batch lookup
CREATE INDEX idx_ingest_batches_project_key ON ingest_batches(project_id, batch_key);

-- Trace queries
CREATE INDEX idx_traces_project_trace ON traces(project_id, trace_id);
CREATE INDEX idx_traces_project_session ON traces(project_id, session_id);
CREATE INDEX idx_traces_started_at ON traces(started_at DESC);

-- Span queries
CREATE INDEX idx_spans_trace_span ON spans(trace_id, span_id);
CREATE INDEX idx_spans_project ON spans(project_id);

-- Event queries
CREATE INDEX idx_span_events_trace_span ON span_events(trace_id, span_id);
CREATE INDEX idx_span_events_project ON span_events(project_id);
```

---

## Related Capabilities

- [ingestion](../ingestion/spec.md) - Uses data model for persistence
- [idempotency](../idempotency/spec.md) - Uses ingest_batches table
