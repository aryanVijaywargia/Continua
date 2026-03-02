## ADDED Requirements

### Requirement: Async Rollup Job Queue

The system SHALL process trace rollups asynchronously using River job queue to avoid blocking the ingest critical path.

#### Scenario: Rollup job enqueued after span ingest
- **WHEN** spans are ingested for a trace
- **THEN** a rollup job is enqueued for that trace
- **AND** the ingest response returns without waiting for rollup completion

#### Scenario: Rollup job deduplication
- **WHEN** multiple span ingests occur for the same trace in quick succession
- **THEN** only one pending rollup job exists for that trace
- **AND** completed jobs allow re-enqueue when new spans arrive

#### Scenario: Rollup job execution
- **WHEN** the River worker processes a rollup job
- **THEN** the worker computes trace aggregates (total_tokens, total_cost, span_count, error_count)
- **AND** updates the trace record with computed values

#### Scenario: Rollup job failure handling
- **WHEN** a rollup job fails
- **THEN** the job is retried with exponential backoff
- **AND** the ingest operation is not affected

### Requirement: River Tables Migration

The system SHALL include River queue tables in standard database migrations.

#### Scenario: River tables created on migrate
- **WHEN** `continua migrate up` is run
- **THEN** River tables (river_job, river_leader, etc.) are created
- **AND** no separate River CLI is required

#### Scenario: River tables removed on rollback
- **WHEN** `continua migrate down` is run
- **THEN** River tables are dropped
- **AND** any pending jobs are lost

### Requirement: Jobs Fx Module

The system SHALL provide a Jobs Fx module for dependency injection.

#### Scenario: Jobs module wired into app
- **WHEN** the server starts
- **THEN** the River client is initialized
- **AND** the rollup worker is registered and starts processing jobs

### Requirement: Rollup Enqueue Transaction Boundary

The system SHALL enqueue rollup jobs in the same database transaction that writes spans/traces, and the job SHALL only become visible after the ingest transaction commits.

#### Scenario: Ingest rollback does not enqueue rollup
- **WHEN** the ingest transaction rolls back
- **THEN** no rollup job exists for that trace

#### Scenario: Ingest commit enqueues rollup
- **WHEN** the ingest transaction commits
- **THEN** a rollup job exists for each affected trace

### Requirement: Rollup Job Idempotency

The rollup worker SHALL recompute aggregates from spans on every run and MUST NOT apply incremental deltas.

#### Scenario: Retry does not double-count
- **WHEN** a rollup job is retried
- **THEN** the resulting totals match a single execution on the same spans

### Requirement: Rollup Coalescing Without Lost Updates

The system SHALL coalesce rollup jobs per trace without dropping updates that arrive while a job is running.

#### Scenario: New spans during running job
- **WHEN** new spans are ingested for a trace while a rollup job is running
- **THEN** enqueueing creates (or preserves) one queued follow-up job for that trace
- **AND** additional ingests coalesce onto the queued job while the first runs
- **AND** the final rollup reflects all spans committed before the last job finishes

### Requirement: Job Retention

The system SHALL configure River to retain completed/failed jobs for a bounded period to prevent unbounded growth of river_job.

#### Scenario: Completed jobs are pruned
- **WHEN** a rollup job completes
- **THEN** the job is eligible for cleanup after the configured retention window (default 7 days)
