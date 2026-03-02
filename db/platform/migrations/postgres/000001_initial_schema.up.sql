-- =============================================================================
-- Continua Platform Schema v1.0
-- Ingestion Pipeline with Multi-tenancy Support
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Projects table (Multi-tenancy)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------------------------------
-- Sessions table
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT,
    user_id TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------------------------------
-- Traces table
-- External trace_id (TEXT) + internal id (UUID) for FK references
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS traces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id UUID REFERENCES sessions(id) ON DELETE SET NULL,
    trace_id TEXT NOT NULL,                    -- External identifier from SDK
    name TEXT,
    user_id TEXT,
    tags TEXT[] DEFAULT '{}',
    environment TEXT,
    release TEXT,
    metadata JSONB DEFAULT '{}',               -- Shallow-merged on update
    input JSONB,                               -- Replaced on update
    output JSONB,                              -- Replaced on update
    status TEXT NOT NULL DEFAULT 'running',    -- running, ok, error, cancelled
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duration_ms BIGINT,
    -- Denormalized aggregates (updated by rollup)
    total_spans INTEGER DEFAULT 0,
    total_tokens BIGINT DEFAULT 0,
    total_cost NUMERIC(12, 6) DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    version INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, trace_id)
);

-- -----------------------------------------------------------------------------
-- Spans table
-- External span_id (TEXT) + internal id (UUID)
-- parent_span_id is TEXT (no FK) for out-of-order tolerance
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS spans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    span_id TEXT NOT NULL,                     -- External identifier from SDK
    parent_span_id TEXT,                       -- External parent ID (no FK)
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'custom',       -- llm, tool, retrieval, agent, custom
    status TEXT NOT NULL DEFAULT 'running',    -- running, ok, error, cancelled
    status_message TEXT,
    level TEXT NOT NULL DEFAULT 'default',     -- debug, default, warning, error
    start_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_time TIMESTAMPTZ,
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duration_ms BIGINT,
    -- Input payload
    input JSONB,
    input_truncated BOOLEAN DEFAULT FALSE,
    input_original_size_bytes BIGINT,
    input_truncation_reason TEXT,
    -- Output payload
    output JSONB,
    output_truncated BOOLEAN DEFAULT FALSE,
    output_original_size_bytes BIGINT,
    output_truncation_reason TEXT,
    -- Thinking snapshot (for LLM spans)
    thinking TEXT,
    thinking_truncated BOOLEAN DEFAULT FALSE,
    -- LLM-specific fields
    model TEXT,
    provider TEXT,
    prompt_tokens BIGINT,
    completion_tokens BIGINT,
    total_tokens BIGINT,
    total_cost NUMERIC(12, 6) DEFAULT 0,
    -- Metadata
    metadata JSONB DEFAULT '{}',               -- Shallow-merged on update
    sequence INTEGER,                          -- Ordering within parent
    depth INTEGER DEFAULT 0,                   -- Nesting depth
    version INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(trace_id, span_id)
);

-- -----------------------------------------------------------------------------
-- Span Events table (append-only, no FK to spans for out-of-order tolerance)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS span_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    span_id TEXT NOT NULL,                     -- External span ID (no FK)
    event_type TEXT NOT NULL,                  -- span.start, llm.thinking, tool.call, etc.
    level TEXT NOT NULL DEFAULT 'info',        -- debug, info, warn, error
    event_ts TIMESTAMPTZ,                      -- Client-provided timestamp
    server_ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sequence INTEGER,                          -- Ordering within span
    message TEXT,
    payload JSONB,
    truncated BOOLEAN DEFAULT FALSE,
    original_size_bytes BIGINT,
    truncation_reason TEXT,
    idempotency_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial unique index for idempotency (only when key is provided)
CREATE UNIQUE INDEX IF NOT EXISTS idx_span_events_idempotency
    ON span_events(project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- -----------------------------------------------------------------------------
-- Ingest Batches table (idempotency tracking)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ingest_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    batch_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'processing', -- processing, accepted, rejected
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_completed_at TIMESTAMPTZ,
    trace_count INTEGER DEFAULT 0,
    span_count INTEGER DEFAULT 0,
    event_count INTEGER DEFAULT 0,
    accepted_count INTEGER DEFAULT 0,
    rejected_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, batch_key)
);

-- -----------------------------------------------------------------------------
-- Payloads table (stores request/response bodies - legacy, kept for compatibility)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS payloads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    span_id UUID NOT NULL REFERENCES spans(id) ON DELETE CASCADE,
    direction TEXT NOT NULL,                   -- 'request' or 'response'
    content_type TEXT,
    body JSONB,
    truncated BOOLEAN DEFAULT FALSE,
    original_size INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Indexes
-- =============================================================================

-- Projects
CREATE INDEX IF NOT EXISTS idx_projects_api_key ON projects(api_key_hash);

-- Sessions
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(project_id, user_id);

-- Traces
CREATE INDEX IF NOT EXISTS idx_traces_project_trace ON traces(project_id, trace_id);
CREATE INDEX IF NOT EXISTS idx_traces_project_session ON traces(project_id, session_id);
CREATE INDEX IF NOT EXISTS idx_traces_status ON traces(project_id, status);
CREATE INDEX IF NOT EXISTS idx_traces_started_at ON traces(start_time DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_traces_server_received ON traces(server_received_at DESC);

-- Spans
CREATE INDEX IF NOT EXISTS idx_spans_trace ON spans(trace_id);
CREATE INDEX IF NOT EXISTS idx_spans_trace_span ON spans(trace_id, span_id);
CREATE INDEX IF NOT EXISTS idx_spans_project ON spans(project_id);
CREATE INDEX IF NOT EXISTS idx_spans_type ON spans(project_id, type);
CREATE INDEX IF NOT EXISTS idx_spans_start_time ON spans(start_time);
CREATE INDEX IF NOT EXISTS idx_spans_parent ON spans(trace_id, parent_span_id);

-- Span Events
CREATE INDEX IF NOT EXISTS idx_span_events_trace ON span_events(trace_id);
CREATE INDEX IF NOT EXISTS idx_span_events_trace_span ON span_events(trace_id, span_id);
CREATE INDEX IF NOT EXISTS idx_span_events_project ON span_events(project_id);
CREATE INDEX IF NOT EXISTS idx_span_events_event_ts ON span_events(event_ts);

-- Ingest Batches
CREATE INDEX IF NOT EXISTS idx_ingest_batches_project_key ON ingest_batches(project_id, batch_key);
CREATE INDEX IF NOT EXISTS idx_ingest_batches_status ON ingest_batches(status);

-- Payloads
CREATE INDEX IF NOT EXISTS idx_payloads_span_id ON payloads(span_id);

-- =============================================================================
-- Default project for v1 (single-tenant mode)
-- =============================================================================
INSERT INTO projects (id, name, api_key_hash)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default Project', 'default')
ON CONFLICT DO NOTHING;
