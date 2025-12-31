-- Executions table
CREATE TABLE IF NOT EXISTS executions (
    tenant_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    agent_type TEXT NOT NULL,
    agent_version TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'RUNNING',
    input BYTEA,
    output BYTEA,
    config JSONB,
    parent_execution_id TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    next_event_id BIGINT NOT NULL DEFAULT 1,
    db_record_version BIGINT NOT NULL DEFAULT 1,
    PRIMARY KEY (tenant_id, execution_id)
);

-- Events table
CREATE TABLE IF NOT EXISTS events (
    tenant_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    event_id BIGINT NOT NULL,
    event_type INTEGER NOT NULL,
    event_time TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    version BIGINT NOT NULL DEFAULT 1,
    data BYTEA NOT NULL,
    PRIMARY KEY (tenant_id, execution_id, event_id)
);

-- Tasks table
CREATE TABLE IF NOT EXISTS tasks (
    tenant_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    task_queue TEXT NOT NULL,
    task_type INTEGER NOT NULL,
    execution_id TEXT NOT NULL,
    data BYTEA,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    visible_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, task_id)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_executions_tenant_status ON executions(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_executions_tenant_type ON executions(tenant_id, agent_type);
CREATE INDEX IF NOT EXISTS idx_events_execution ON events(tenant_id, execution_id, event_id);
CREATE INDEX IF NOT EXISTS idx_tasks_queue ON tasks(tenant_id, task_queue, visible_at);
