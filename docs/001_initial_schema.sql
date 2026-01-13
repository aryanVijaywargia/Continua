-- ============================================================================
-- CONTINUA DATABASE SCHEMA v1.0
-- ============================================================================
-- 
-- This schema implements the core Continua observability platform with:
-- - Never-fail payload handling (wrapper approach for invalid JSON)
-- - Append-only span_events for durability and replay
-- - Soft integrity (no FK on span_events for out-of-order tolerance)
-- - Server-trust timestamps for clock-skew protection
-- - Blob reference columns for v1.5 large payload offloading
-- - Thinking capture via events + snapshot column
--
-- Generated: 2025-01-08
-- ============================================================================

-- Required extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================================
-- PROJECTS
-- Multi-tenant isolation unit
-- ============================================================================

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_projects_name ON projects(name);

COMMENT ON TABLE projects IS 'Multi-tenant project isolation unit';
COMMENT ON COLUMN projects.settings IS 'Project-level configuration (retention, limits, features)';

-- ============================================================================
-- API KEYS
-- Project-scoped authentication
-- ============================================================================

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    
    -- Key components (pk_xxx visible, sk_xxx hashed)
    public_key TEXT NOT NULL,
    hashed_secret TEXT NOT NULL,
    
    -- Metadata
    name TEXT,
    description TEXT,
    scopes TEXT[] DEFAULT '{ingest,query}',  -- ingest, query, admin
    
    -- Lifecycle
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_api_keys_public ON api_keys(public_key);
CREATE INDEX idx_api_keys_project ON api_keys(project_id);

COMMENT ON TABLE api_keys IS 'Project-scoped API authentication keys';
COMMENT ON COLUMN api_keys.public_key IS 'Public portion (pk_xxx), sent in Authorization header';
COMMENT ON COLUMN api_keys.hashed_secret IS 'Hashed secret (sk_xxx), never exposed after creation';
COMMENT ON COLUMN api_keys.scopes IS 'Permissions: ingest, query, admin';

-- ============================================================================
-- REDACTION RULES
-- PII and sensitive data handling at ingestion
-- ============================================================================

CREATE TABLE redaction_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    
    -- Rule definition
    name TEXT NOT NULL,
    description TEXT,
    rule_type TEXT NOT NULL,  -- regex, jsonpath, keyword
    pattern TEXT NOT NULL,
    replacement TEXT DEFAULT '[REDACTED]',
    
    -- Targeting
    target_fields TEXT[] DEFAULT '{input,output,metadata}',
    
    -- Status
    enabled BOOLEAN DEFAULT TRUE,
    priority INT DEFAULT 0,  -- Higher = applied first
    version INT NOT NULL DEFAULT 1,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_redaction_rules_project ON redaction_rules(project_id);
CREATE INDEX idx_redaction_rules_active ON redaction_rules(project_id, enabled) 
    WHERE enabled = TRUE;

COMMENT ON TABLE redaction_rules IS 'Rules for redacting PII/sensitive data at ingestion';
COMMENT ON COLUMN redaction_rules.version IS 'Version for audit trail - which rules applied to which data';

-- ============================================================================
-- PAYLOAD BLOBS
-- Content-addressed storage for large payloads (v1.5 feature, table ready now)
-- ============================================================================

CREATE TABLE payload_blobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    
    -- Content addressing
    sha256 TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    
    -- Storage details
    mime_type TEXT DEFAULT 'application/json',
    compression TEXT,  -- gzip, zstd, none
    storage_type TEXT NOT NULL DEFAULT 'inline',  -- inline, s3, local
    storage_path TEXT,  -- S3 key or local path
    inline_data BYTEA,  -- For small blobs stored inline
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_payload_blobs_sha256 ON payload_blobs(project_id, sha256);
CREATE INDEX idx_payload_blobs_project ON payload_blobs(project_id, created_at DESC);

COMMENT ON TABLE payload_blobs IS 'Content-addressed storage for large payloads (v1.5)';
COMMENT ON COLUMN payload_blobs.sha256 IS 'Content hash for deduplication';

-- ============================================================================
-- SESSIONS
-- Group related traces (conversations, user sessions, threads)
-- ============================================================================

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    
    -- External identifier
    session_id TEXT NOT NULL,
    
    -- Context
    user_id TEXT,
    metadata JSONB DEFAULT '{}',
    
    -- Timing (client-provided)
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    
    -- Server timestamp
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Denormalized rollups (updated by workers)
    trace_count INT DEFAULT 0,
    total_cost DECIMAL(20, 10) DEFAULT 0,
    total_tokens BIGINT DEFAULT 0,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE(project_id, session_id)
);

CREATE INDEX idx_sessions_project_time ON sessions(project_id, server_received_at DESC);
CREATE INDEX idx_sessions_project_user ON sessions(project_id, user_id);
CREATE INDEX idx_sessions_start ON sessions(start_time DESC);

COMMENT ON TABLE sessions IS 'Groups related traces into logical sessions (conversations, threads)';

-- ============================================================================
-- TRACES
-- Root of each execution tree
-- ============================================================================

CREATE TABLE traces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id UUID REFERENCES sessions(id) ON DELETE SET NULL,
    
    -- External identifier
    trace_id TEXT NOT NULL,
    
    -- Metadata
    name TEXT,
    user_id TEXT,
    tags TEXT[] DEFAULT '{}',
    environment TEXT,
    release TEXT,
    metadata JSONB DEFAULT '{}',
    
    -- I/O (JSONB - invalid JSON wrapped at ingestion)
    input JSONB,
    output JSONB,
    
    -- Timing (client-provided)
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    
    -- Server-trust timestamp
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Status
    status TEXT DEFAULT 'running',  -- running, ok, error, cancelled
    
    -- Denormalized rollups (updated by workers)
    total_spans INT DEFAULT 0,
    total_cost DECIMAL(20, 10) DEFAULT 0,
    total_tokens BIGINT DEFAULT 0,
    error_count INT DEFAULT 0,
    duration_ms BIGINT,
    
    -- Versioning for optimistic concurrency
    version INT DEFAULT 1,
    
    -- Idempotency
    idempotency_key TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE(project_id, trace_id)
);

CREATE INDEX idx_traces_project_time ON traces(project_id, server_received_at DESC);
CREATE INDEX idx_traces_project_session ON traces(project_id, session_id);
CREATE INDEX idx_traces_project_name ON traces(project_id, name);
CREATE INDEX idx_traces_project_user ON traces(project_id, user_id);
CREATE INDEX idx_traces_project_env ON traces(project_id, environment);
CREATE INDEX idx_traces_project_status ON traces(project_id, status);
CREATE INDEX idx_traces_project_tags ON traces USING GIN(tags);
CREATE INDEX idx_traces_start_time ON traces(start_time DESC);
CREATE UNIQUE INDEX idx_traces_idempotency 
    ON traces(project_id, idempotency_key) 
    WHERE idempotency_key IS NOT NULL;

COMMENT ON TABLE traces IS 'Root of each execution tree (request, agent run, workflow)';
COMMENT ON COLUMN traces.trace_id IS 'External trace identifier from SDK/OTel';
COMMENT ON COLUMN traces.server_received_at IS 'Server-trust timestamp for clock-skew protection';

-- ============================================================================
-- SPANS
-- Individual operations within a trace
-- ============================================================================

CREATE TABLE spans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    
    -- External identifiers
    span_id TEXT NOT NULL,
    parent_span_id TEXT,  -- NULL for root spans, TEXT not FK for out-of-order
    
    -- Classification
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'custom',  -- llm, tool, retrieval, agent, custom
    
    -- Status
    status TEXT DEFAULT 'running',  -- running, ok, error, cancelled
    status_message TEXT,
    level TEXT DEFAULT 'default',  -- debug, default, warning, error
    
    -- Timing (client-provided)
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    
    -- Server-trust timestamp
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Computed duration
    duration_ms BIGINT,
    
    -- === INPUT PAYLOAD ===
    input JSONB,  -- Always valid JSONB (invalid wrapped at ingestion)
    input_truncated BOOLEAN DEFAULT FALSE,
    input_original_size_bytes BIGINT,
    input_truncation_reason TEXT,
    input_blob_id UUID REFERENCES payload_blobs(id),  -- v1.5 blob offloading
    
    -- === OUTPUT PAYLOAD ===
    output JSONB,
    output_truncated BOOLEAN DEFAULT FALSE,
    output_original_size_bytes BIGINT,
    output_truncation_reason TEXT,
    output_blob_id UUID REFERENCES payload_blobs(id),
    
    -- === THINKING (denormalized snapshot from events) ===
    thinking TEXT,  -- Latest/final thinking content
    thinking_truncated BOOLEAN DEFAULT FALSE,
    thinking_original_size_bytes BIGINT,
    thinking_truncation_reason TEXT,
    thinking_blob_id UUID REFERENCES payload_blobs(id),
    
    -- === METADATA ===
    metadata JSONB DEFAULT '{}',
    
    -- === LLM-SPECIFIC FIELDS ===
    model TEXT,
    provider TEXT,
    model_parameters JSONB,
    
    -- Token usage
    usage_details JSONB DEFAULT '{}',  -- {prompt_tokens, completion_tokens, reasoning_tokens, ...}
    prompt_tokens BIGINT,
    completion_tokens BIGINT,
    reasoning_tokens BIGINT,
    total_tokens BIGINT,
    
    -- Cost
    cost_details JSONB DEFAULT '{}',  -- {prompt_cost, completion_cost, total_cost, currency}
    total_cost DECIMAL(20, 10) DEFAULT 0,
    
    -- === TOOL-SPECIFIC FIELDS ===
    tool_name TEXT,
    tool_arguments JSONB,
    tool_result JSONB,
    
    -- === ORDERING & VERSIONING ===
    sequence INT,  -- Order within parent
    depth INT DEFAULT 0,  -- Tree depth for UI
    
    -- Idempotency and versioning
    idempotency_key TEXT,
    version INT DEFAULT 1,
    
    -- Redaction tracking
    redaction_rule_version INT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE(trace_id, span_id)
);

CREATE INDEX idx_spans_trace ON spans(trace_id);
CREATE INDEX idx_spans_trace_parent ON spans(trace_id, parent_span_id);
CREATE INDEX idx_spans_trace_time ON spans(trace_id, start_time);
CREATE INDEX idx_spans_project_type ON spans(project_id, type);
CREATE INDEX idx_spans_project_model ON spans(project_id, model) WHERE model IS NOT NULL;
CREATE INDEX idx_spans_status ON spans(status);
CREATE INDEX idx_spans_start_time ON spans(start_time);
CREATE UNIQUE INDEX idx_spans_idempotency 
    ON spans(trace_id, idempotency_key) 
    WHERE idempotency_key IS NOT NULL;

COMMENT ON TABLE spans IS 'Individual operations within a trace';
COMMENT ON COLUMN spans.span_id IS 'External span identifier from SDK/OTel';
COMMENT ON COLUMN spans.parent_span_id IS 'Parent span_id (TEXT, not FK) for out-of-order ingestion tolerance';
COMMENT ON COLUMN spans.type IS 'Semantic classification: llm, tool, retrieval, agent, custom';
COMMENT ON COLUMN spans.thinking IS 'Denormalized snapshot of latest thinking (full history in span_events)';
COMMENT ON COLUMN spans.server_received_at IS 'Server-trust timestamp for clock-skew protection';

-- ============================================================================
-- SPAN EVENTS
-- Append-only event log for durability, replay, and streaming
-- ============================================================================

CREATE TABLE span_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Denormalized for efficient queries (no FK on span for out-of-order tolerance)
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    span_id TEXT NOT NULL,  -- TEXT, not FK - allows orphan events
    
    -- Event classification
    event_type TEXT NOT NULL,  -- Validated at app layer, not DB
    -- Examples: span.start, span.end, span.error, llm.request, llm.response, 
    --           llm.thinking, llm.thinking_chunk, tool.call, tool.result,
    --           state.change, state.checkpoint, retry, chunk
    
    level TEXT NOT NULL DEFAULT 'info'
        CHECK (level IN ('debug', 'info', 'warn', 'error')),
    
    -- Timing
    event_ts TIMESTAMPTZ,  -- Client-provided event timestamp
    server_ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),  -- Server-trust
    
    -- Ordering within span
    sequence INT,  -- Monotonic per span if SDK supplies it
    
    -- Content
    message TEXT,  -- Human-readable message
    payload JSONB DEFAULT '{}',  -- Event-specific data (wrapped if invalid)
    
    -- Truncation metadata
    truncated BOOLEAN DEFAULT FALSE,
    original_size_bytes BIGINT,
    truncation_reason TEXT,
    
    -- Blob reference (v1.5)
    payload_blob_id UUID REFERENCES payload_blobs(id),
    
    -- Idempotency (event-level)
    idempotency_key TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Soft integrity: index for joins, no FK to spans
CREATE INDEX idx_span_events_span ON span_events(project_id, trace_id, span_id);
CREATE INDEX idx_span_events_trace ON span_events(trace_id);
CREATE INDEX idx_span_events_trace_time ON span_events(trace_id, server_ingested_at);
CREATE INDEX idx_span_events_span_time ON span_events(span_id, server_ingested_at);
CREATE INDEX idx_span_events_type ON span_events(event_type);
CREATE INDEX idx_span_events_level ON span_events(level);

-- Sequence uniqueness per span (when provided)
CREATE UNIQUE INDEX idx_span_events_sequence 
    ON span_events(trace_id, span_id, sequence) 
    WHERE sequence IS NOT NULL;

-- Idempotency uniqueness per project (when provided)
CREATE UNIQUE INDEX idx_span_events_idempotency 
    ON span_events(project_id, idempotency_key) 
    WHERE idempotency_key IS NOT NULL;

COMMENT ON TABLE span_events IS 'Append-only event log for durability, replay, and streaming';
COMMENT ON COLUMN span_events.span_id IS 'TEXT reference to spans.span_id (no FK for out-of-order tolerance)';
COMMENT ON COLUMN span_events.event_type IS 'Event type (validated at app layer): span.*, llm.*, tool.*, state.*, etc.';
COMMENT ON COLUMN span_events.level IS 'Event severity for timeline filtering/coloring';
COMMENT ON COLUMN span_events.server_ingested_at IS 'Server-trust timestamp for reliable ordering';

-- ============================================================================
-- SCORES
-- Annotations and evaluations attached to traces/spans
-- ============================================================================

CREATE TABLE scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    span_id TEXT,  -- Nullable for trace-level scores
    session_id UUID REFERENCES sessions(id) ON DELETE SET NULL,
    
    -- Score definition
    name TEXT NOT NULL,
    data_type TEXT NOT NULL DEFAULT 'numeric'
        CHECK (data_type IN ('numeric', 'categorical', 'boolean')),
    
    -- Values (set based on data_type)
    value_numeric DECIMAL(20, 10),
    value_text TEXT,
    value_boolean BOOLEAN,
    
    -- Source and context
    source TEXT NOT NULL DEFAULT 'api',  -- api, sdk, annotation, evaluator, auto
    comment TEXT,
    metadata JSONB DEFAULT '{}',
    author_user_id TEXT,
    
    -- Idempotency
    idempotency_key TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_scores_project_trace ON scores(project_id, trace_id);
CREATE INDEX idx_scores_project_span ON scores(project_id, span_id) 
    WHERE span_id IS NOT NULL;
CREATE INDEX idx_scores_project_name ON scores(project_id, name);
CREATE INDEX idx_scores_project_source ON scores(project_id, source);
CREATE INDEX idx_scores_session ON scores(session_id) WHERE session_id IS NOT NULL;
CREATE UNIQUE INDEX idx_scores_idempotency 
    ON scores(project_id, idempotency_key) 
    WHERE idempotency_key IS NOT NULL;

COMMENT ON TABLE scores IS 'Annotations and evaluations for traces, spans, or sessions';
COMMENT ON COLUMN scores.source IS 'Origin: api, sdk, annotation (human), evaluator (automated), auto';

-- ============================================================================
-- EXTERNAL TRACE IDS
-- Maps external trace IDs (OTel hex) to internal UUIDs
-- ============================================================================

CREATE TABLE external_trace_ids (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    
    -- External system
    system TEXT NOT NULL DEFAULT 'otel',  -- otel, import, etc.
    
    -- External identifiers
    external_trace_id TEXT NOT NULL,
    external_span_id TEXT,  -- Nullable, for span-level mapping
    
    -- Internal references
    internal_trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    internal_span_id TEXT,  -- Nullable
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_external_ids_lookup 
    ON external_trace_ids(project_id, system, external_trace_id, COALESCE(external_span_id, ''));
CREATE INDEX idx_external_ids_internal ON external_trace_ids(internal_trace_id);

COMMENT ON TABLE external_trace_ids IS 'Maps external trace IDs (OTel hex strings) to internal UUIDs';

-- ============================================================================
-- INGEST BATCHES
-- Audit log for ingestion batches (debugging, replay, idempotency)
-- ============================================================================

CREATE TABLE ingest_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    
    -- Batch identification
    batch_key TEXT NOT NULL,  -- Client-provided batch idempotency key
    
    -- Timing
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_completed_at TIMESTAMPTZ,
    
    -- SDK metadata
    sdk_name TEXT,
    sdk_version TEXT,
    sdk_language TEXT,
    
    -- Batch stats
    payload_size_bytes BIGINT,
    trace_count INT DEFAULT 0,
    span_count INT DEFAULT 0,
    event_count INT DEFAULT 0,
    
    -- Result
    status TEXT NOT NULL DEFAULT 'accepted'
        CHECK (status IN ('accepted', 'partially_accepted', 'rejected')),
    accepted_count INT DEFAULT 0,
    rejected_count INT DEFAULT 0,
    error_summary JSONB,
    
    UNIQUE(project_id, batch_key)
);

CREATE INDEX idx_ingest_batches_project_time 
    ON ingest_batches(project_id, server_received_at DESC);

COMMENT ON TABLE ingest_batches IS 'Audit log for ingestion batches';

-- ============================================================================
-- UPDATED_AT TRIGGERS
-- Automatically update updated_at timestamp on modifications
-- ============================================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_projects_updated_at 
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_sessions_updated_at 
    BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_traces_updated_at 
    BEFORE UPDATE ON traces
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_spans_updated_at 
    BEFORE UPDATE ON spans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_scores_updated_at 
    BEFORE UPDATE ON scores
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_redaction_rules_updated_at 
    BEFORE UPDATE ON redaction_rules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Update trace rollups from spans
CREATE OR REPLACE FUNCTION update_trace_rollups(p_trace_id UUID)
RETURNS VOID AS $$
BEGIN
    UPDATE traces SET
        total_spans = (SELECT COUNT(*) FROM spans WHERE trace_id = p_trace_id),
        total_cost = (SELECT COALESCE(SUM(total_cost), 0) FROM spans WHERE trace_id = p_trace_id),
        total_tokens = (SELECT COALESCE(SUM(total_tokens), 0) FROM spans WHERE trace_id = p_trace_id),
        error_count = (SELECT COUNT(*) FROM spans WHERE trace_id = p_trace_id AND status = 'error'),
        status = CASE 
            WHEN EXISTS (SELECT 1 FROM spans WHERE trace_id = p_trace_id AND status = 'error') THEN 'error'
            WHEN EXISTS (SELECT 1 FROM spans WHERE trace_id = p_trace_id AND status = 'ok') THEN 'ok'
            WHEN EXISTS (SELECT 1 FROM spans WHERE trace_id = p_trace_id AND status = 'running') THEN 'running'
            ELSE 'ok'
        END,
        start_time = COALESCE(traces.start_time, (SELECT MIN(start_time) FROM spans WHERE trace_id = p_trace_id)),
        end_time = (SELECT MAX(end_time) FROM spans WHERE trace_id = p_trace_id),
        duration_ms = EXTRACT(EPOCH FROM (
            (SELECT MAX(end_time) FROM spans WHERE trace_id = p_trace_id) -
            COALESCE(traces.start_time, (SELECT MIN(start_time) FROM spans WHERE trace_id = p_trace_id))
        )) * 1000
    WHERE id = p_trace_id;
END;
$$ LANGUAGE plpgsql;

-- Update session rollups from traces
CREATE OR REPLACE FUNCTION update_session_rollups(p_session_id UUID)
RETURNS VOID AS $$
BEGIN
    UPDATE sessions SET
        trace_count = (SELECT COUNT(*) FROM traces WHERE session_id = p_session_id),
        total_cost = (SELECT COALESCE(SUM(total_cost), 0) FROM traces WHERE session_id = p_session_id),
        total_tokens = (SELECT COALESCE(SUM(total_tokens), 0) FROM traces WHERE session_id = p_session_id),
        start_time = (SELECT MIN(start_time) FROM traces WHERE session_id = p_session_id),
        end_time = (SELECT MAX(end_time) FROM traces WHERE session_id = p_session_id)
    WHERE id = p_session_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_trace_rollups IS 'Updates denormalized aggregates on trace from its spans';
COMMENT ON FUNCTION update_session_rollups IS 'Updates denormalized aggregates on session from its traces';

-- ============================================================================
-- END OF SCHEMA
-- ============================================================================
