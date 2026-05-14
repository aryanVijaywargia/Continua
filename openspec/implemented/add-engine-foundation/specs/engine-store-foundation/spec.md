# Capability: engine-store-foundation

Engine-local store package with connection pool, transaction support, sqlc-backed query wrappers, and sentinel errors.

Related capabilities: [engine-schema-foundation](../engine-schema-foundation/spec.md), [engine-cli-foundation](../engine-cli-foundation/spec.md)

## ADDED Requirements

### Requirement: Engine connection pool

The engine store MUST manage its own pgx connection pool, separate from the platform pool, with conservative defaults.

#### Scenario: Pool initialization
- **WHEN** the engine store is created with a resolved database URL (from `ENGINE_DATABASE_URL` or `DATABASE_URL` fallback)
- **THEN** a pgxpool is established with defaults: `MaxConns=10`, `MinConns=2`, `MaxConnLifetime=1h`, `MaxConnIdleTime=30m`, `HealthCheckPeriod=1m`

#### Scenario: Pool shutdown
- **WHEN** the engine store is closed
- **THEN** the underlying pgx pool is closed cleanly

---

### Requirement: Engine store transactions

The engine store MUST support explicit transaction management with `BeginTx`, commit, and rollback.

#### Scenario: Transaction commit
- **WHEN** a transaction is started, a row is written, and the transaction is committed
- **THEN** the row is visible in subsequent queries

#### Scenario: Transaction rollback
- **WHEN** a transaction is started, a row is written, and the transaction is rolled back
- **THEN** the row is not visible in subsequent queries

---

### Requirement: SQLC query files

SQLC query files MUST exist for all seven engine tables and MUST use fully-qualified table names (`engine.instances`, `engine.runs`, etc.).

#### Scenario: Instances queries
- **WHEN** engine sqlc generation runs
- **THEN** queries exist for: create instance, get instance by ID, get instance by project and key, list instances by project, update instance status

#### Scenario: Runs queries
- **WHEN** engine sqlc generation runs
- **THEN** queries exist for: create run, get run by ID, list runs by instance, update run status, claim next eligible run (dual-condition: fresh `queued` OR `running` with expired lease, SKIP LOCKED)

#### Scenario: History queries
- **WHEN** engine sqlc generation runs
- **THEN** queries exist for: append history event, get history by run (ordered by sequence_no), get history by instance (ordered by id)

#### Scenario: Inbox queries
- **WHEN** engine sqlc generation runs
- **THEN** queries exist for: create inbox item, claim next inbox item (dual-condition: fresh `pending` OR `claimed` with expired lease, SKIP LOCKED), mark inbox processed/discarded

#### Scenario: Activity tasks queries
- **WHEN** engine sqlc generation runs
- **THEN** queries exist for: create activity task, get by run and key, claim next task (dual-condition: fresh `queued` OR `claimed` with expired lease, SKIP LOCKED), complete task, fail task

#### Scenario: Request dedupe queries
- **WHEN** engine sqlc generation runs
- **THEN** queries exist for: create dedupe record, get by scope and key, finalize with response or error

#### Scenario: Projection checkpoint queries
- **WHEN** engine sqlc generation runs
- **THEN** queries exist for: get checkpoint, advance checkpoint (must not move backward)

---

### Requirement: Store wrappers

The engine store MUST expose thin wrapper methods over sqlc-generated queries, named after business operations.

#### Scenario: Store method naming
- **WHEN** a store method is added
- **THEN** it is named after the business operation (e.g., `CreateInstance`, `ClaimNextRun`, `AppendHistory`) not the SQL operation

#### Scenario: Store method signatures
- **WHEN** a store method is called
- **THEN** it accepts a `context.Context` and typed parameters, and returns typed results with `error`

---

### Requirement: Sentinel errors

The engine store MUST define sentinel errors for common failure modes.

#### Scenario: Not found error
- **WHEN** a query returns no rows
- **THEN** the store returns `ErrNotFound`

#### Scenario: Already exists error
- **WHEN** an insert violates a unique constraint
- **THEN** the store returns `ErrAlreadyExists`

#### Scenario: Stale checkpoint error
- **WHEN** a checkpoint advance is attempted with a `last_history_id` that would move the checkpoint backward
- **THEN** the store returns `ErrStaleCheckpoint`

---

### Requirement: Engine test harness

Engine tests MUST use an engine-local test helper that applies both platform and engine migrations.

#### Scenario: Test database setup
- **WHEN** an engine test starts
- **THEN** the test helper creates a fresh test database, applies all platform migrations, then applies all engine migrations

#### Scenario: No root testutil import
- **WHEN** engine test code is compiled
- **THEN** it does not import from the root module's `internal/testutil`
