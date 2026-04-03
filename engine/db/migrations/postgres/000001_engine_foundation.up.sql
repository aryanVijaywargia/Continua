CREATE SCHEMA IF NOT EXISTS engine;

CREATE TYPE engine.instance_lifecycle_status AS ENUM (
    'active',
    'completed',
    'failed',
    'cancelled'
);

CREATE TYPE engine.run_lifecycle_status AS ENUM (
    'queued',
    'running',
    'completed',
    'failed',
    'cancelled'
);

CREATE TYPE engine.activity_task_status AS ENUM (
    'queued',
    'claimed',
    'completed',
    'failed',
    'cancelled'
);

CREATE TYPE engine.inbox_status AS ENUM (
    'pending',
    'claimed',
    'processed',
    'discarded'
);

CREATE TYPE engine.request_dedupe_status AS ENUM (
    'in_progress',
    'completed',
    'failed',
    'expired'
);

CREATE TABLE engine.instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    instance_key TEXT NOT NULL,
    definition_name TEXT NOT NULL,
    status engine.instance_lifecycle_status NOT NULL DEFAULT 'active',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, instance_key)
);

CREATE TABLE engine.runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    instance_id UUID NOT NULL REFERENCES engine.instances(id),
    run_number INT NOT NULL,
    definition_version TEXT NOT NULL,
    status engine.run_lifecycle_status NOT NULL DEFAULT 'queued',
    ready_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempt_count INT NOT NULL DEFAULT 0,
    last_error_code TEXT,
    last_error_message TEXT,
    claimed_by TEXT,
    claimed_at TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (instance_id, run_number)
);

CREATE TABLE engine.history (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id UUID NOT NULL,
    instance_id UUID NOT NULL REFERENCES engine.instances(id),
    run_id UUID NOT NULL REFERENCES engine.runs(id),
    sequence_no INT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, sequence_no)
);

CREATE TABLE engine.inbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    instance_id UUID NOT NULL REFERENCES engine.instances(id),
    run_id UUID REFERENCES engine.runs(id),
    history_id BIGINT REFERENCES engine.history(id),
    kind TEXT NOT NULL,
    payload JSONB,
    status engine.inbox_status NOT NULL DEFAULT 'pending',
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_by TEXT,
    claimed_at TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    dedupe_key TEXT,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE engine.activity_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    instance_id UUID NOT NULL REFERENCES engine.instances(id),
    run_id UUID NOT NULL REFERENCES engine.runs(id),
    history_id BIGINT REFERENCES engine.history(id),
    activity_key TEXT NOT NULL,
    activity_type TEXT NOT NULL,
    input JSONB,
    output JSONB,
    status engine.activity_task_status NOT NULL DEFAULT 'queued',
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempt_count INT NOT NULL DEFAULT 0,
    claimed_by TEXT,
    claimed_at TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    last_error_code TEXT,
    last_error_message TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, activity_key)
);

CREATE TABLE engine.request_dedupe (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    request_scope TEXT NOT NULL,
    request_key TEXT NOT NULL,
    instance_id UUID REFERENCES engine.instances(id),
    run_id UUID REFERENCES engine.runs(id),
    status engine.request_dedupe_status NOT NULL DEFAULT 'in_progress',
    response_payload JSONB,
    error_code TEXT,
    error_message TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    finalized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, request_scope, request_key)
);

CREATE TABLE engine.projection_checkpoints (
    projection_name TEXT NOT NULL,
    scope_key TEXT NOT NULL,
    last_history_id BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (projection_name, scope_key)
);

CREATE INDEX idx_engine_instances_definition_name
    ON engine.instances(project_id, definition_name);

CREATE INDEX idx_engine_instances_status_updated
    ON engine.instances(project_id, status, updated_at DESC);

CREATE INDEX idx_engine_runs_instance_created
    ON engine.runs(instance_id, created_at DESC);

CREATE INDEX idx_engine_runs_claim
    ON engine.runs(status, ready_at, lease_expires_at)
    WHERE status IN ('queued', 'running');

CREATE INDEX idx_engine_history_instance_id
    ON engine.history(instance_id, id);

CREATE INDEX idx_engine_history_project_id
    ON engine.history(project_id, id);

CREATE UNIQUE INDEX idx_engine_inbox_dedupe
    ON engine.inbox(project_id, dedupe_key)
    WHERE dedupe_key IS NOT NULL;

CREATE INDEX idx_engine_inbox_claim
    ON engine.inbox(status, available_at, lease_expires_at)
    WHERE status IN ('pending', 'claimed');

CREATE INDEX idx_engine_activity_tasks_claim
    ON engine.activity_tasks(status, available_at, lease_expires_at)
    WHERE status IN ('queued', 'claimed');
