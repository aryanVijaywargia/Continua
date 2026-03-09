## ADDED Requirements

### Requirement: Multi-Queue River Configuration

The system SHALL use a single River client with multiple named queues to isolate work classes and prevent resource contention.

#### Scenario: Queue isolation
- **WHEN** the River client is configured
- **THEN** it has three primary queues with default worker counts `ingest=4`, `rollup=10`, and `maintenance=1`
- **AND** those worker counts are configurable

#### Scenario: Ingest jobs routed to ingest queue
- **WHEN** an async ingest job is enqueued
- **THEN** its `InsertOpts` specifies `Queue: "ingest"`

#### Scenario: Rollup jobs routed to rollup queue
- **WHEN** a trace rollup job is enqueued
- **THEN** its `InsertOpts` specifies `Queue: "rollup"`

#### Scenario: Cleanup jobs routed to maintenance queue
- **WHEN** the periodic cleanup job runs
- **THEN** it executes on the `maintenance` queue

#### Scenario: Mixed-version rollout drains legacy default queue
- **WHEN** the system deploys with the new queue topology while some producers still target `default`
- **THEN** the River client continues to consume both `default` and `rollup` with non-zero workers until old producers are gone
- **AND** the `default` queue is removed only after its backlog drains

### Requirement: Timeout and Rescue Configuration

The system SHALL configure per-worker timeouts and a rescue interval appropriate for async ingest workloads.

#### Scenario: Client-level timeout
- **WHEN** the River client is configured
- **THEN** `JobTimeout` is set to `5m` and `RescueStuckJobsAfter` is set to `10m`

#### Scenario: Worker-specific timeout overrides
- **WHEN** workers define their `Timeout()` method
- **THEN** `IngestBatchWorker` returns `5m`, `TraceRollupWorker` returns `30s`, and `CleanupWorker` returns `1m`

#### Scenario: Stuck job rescue
- **WHEN** a worker crashes after marking a batch `processing` without completing
- **THEN** River rescues the stuck job within about 10 minutes
- **AND** the worker re-enters safely via idempotent state checks
