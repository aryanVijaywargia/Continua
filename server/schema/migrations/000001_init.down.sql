DROP INDEX IF EXISTS idx_tasks_queue;
DROP INDEX IF EXISTS idx_events_execution;
DROP INDEX IF EXISTS idx_executions_tenant_type;
DROP INDEX IF EXISTS idx_executions_tenant_status;

DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS executions;
