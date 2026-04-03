# Capability: engine-schema-foundation

Dedicated Postgres schema, enums, tables, indexes, and reversible migrations for the engine module.

Related capabilities: [engine-store-foundation](../engine-store-foundation/spec.md), [engine-cli-foundation](../engine-cli-foundation/spec.md)

## ADDED Requirements

### Requirement: Engine schema namespace

The engine module MUST use a dedicated `engine` schema in Postgres. All DDL MUST use fully-qualified names (`engine.instances`, `engine.runs`, etc.).

#### Scenario: Engine schema creation
- **WHEN** the engine up migration runs
- **THEN** `CREATE SCHEMA IF NOT EXISTS engine` executes before any table or enum creation

#### Scenario: Fully-qualified DDL
- **WHEN** any engine table or enum is created
- **THEN** the DDL uses `engine.*` qualification (e.g., `CREATE TABLE engine.instances`, `CREATE TYPE engine.instance_lifecycle_status`)

---

### Requirement: Engine enums

The engine schema MUST define five Postgres enums to represent lifecycle and status states.

#### Scenario: instance_lifecycle_status enum
- **WHEN** the engine schema is created
- **THEN** `engine.instance_lifecycle_status` exists with values: `active`, `completed`, `failed`, `cancelled`

#### Scenario: run_lifecycle_status enum
- **WHEN** the engine schema is created
- **THEN** `engine.run_lifecycle_status` exists with values: `queued`, `running`, `completed`, `failed`, `cancelled`

#### Scenario: activity_task_status enum
- **WHEN** the engine schema is created
- **THEN** `engine.activity_task_status` exists with values: `queued`, `claimed`, `completed`, `failed`, `cancelled`

#### Scenario: inbox_status enum
- **WHEN** the engine schema is created
- **THEN** `engine.inbox_status` exists with values: `pending`, `claimed`, `processed`, `discarded`

#### Scenario: request_dedupe_status enum
- **WHEN** the engine schema is created
- **THEN** `engine.request_dedupe_status` exists with values: `in_progress`, `completed`, `failed`, `expired`

---

### Requirement: Instances table

The system MUST provide `engine.instances` to store workflow instance lifecycle state.

#### Scenario: Instances table structure
- **WHEN** the engine schema is created
- **THEN** `engine.instances` has columns: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `project_id UUID NOT NULL`, `instance_key TEXT NOT NULL`, `definition_name TEXT NOT NULL`, `status engine.instance_lifecycle_status NOT NULL DEFAULT 'active'`, `metadata JSONB`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

#### Scenario: Instance key uniqueness
- **WHEN** two instances are inserted with the same `project_id` and `instance_key`
- **THEN** the second insert fails with a unique constraint violation

#### Scenario: Instances indexes
- **WHEN** the engine schema is created
- **THEN** indexes exist on `(project_id, definition_name)` and `(project_id, status, updated_at DESC)`

---

### Requirement: Runs table

The system MUST provide `engine.runs` to store execution attempts per instance with lease-based claiming.

#### Scenario: Runs table structure
- **WHEN** the engine schema is created
- **THEN** `engine.runs` has columns: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `project_id UUID NOT NULL`, `instance_id UUID NOT NULL REFERENCES engine.instances(id)`, `run_number INT NOT NULL`, `definition_version TEXT NOT NULL`, `status engine.run_lifecycle_status NOT NULL DEFAULT 'queued'`, `ready_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `attempt_count INT NOT NULL DEFAULT 0`, `last_error_code TEXT`, `last_error_message TEXT`, `claimed_by TEXT`, `claimed_at TIMESTAMPTZ`, `lease_expires_at TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

#### Scenario: Run number uniqueness
- **WHEN** two runs are inserted with the same `instance_id` and `run_number`
- **THEN** the second insert fails with a unique constraint violation

#### Scenario: Runs FK to instances
- **WHEN** a run references a non-existent `instance_id`
- **THEN** the insert fails with a foreign key violation

#### Scenario: Runs indexes
- **WHEN** the engine schema is created
- **THEN** indexes exist on `(instance_id, created_at DESC)` and a partial claim index on `(status, ready_at, lease_expires_at)` covering both fresh (`queued`) and expired-lease (`running`) rows

---

### Requirement: History table

The system MUST provide `engine.history` as an append-only event log with dual ordering: global allocation-ordered `id` and per-run `sequence_no`.

#### Scenario: History table structure
- **WHEN** the engine schema is created
- **THEN** `engine.history` has columns: `id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY`, `project_id UUID NOT NULL`, `instance_id UUID NOT NULL REFERENCES engine.instances(id)`, `run_id UUID NOT NULL REFERENCES engine.runs(id)`, `sequence_no INT NOT NULL`, `event_type TEXT NOT NULL`, `payload JSONB`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

#### Scenario: History sequence uniqueness
- **WHEN** two history rows are inserted with the same `run_id` and `sequence_no`
- **THEN** the second insert fails with a unique constraint violation

#### Scenario: History allocation ordering
- **WHEN** multiple history rows are inserted sequentially (single-writer) across different runs
- **THEN** the `id` column values are monotonically increasing in allocation order. Note: under concurrent writers, allocation order may differ from commit order — runtime phases must define a visibility strategy for projection consumers

#### Scenario: History indexes
- **WHEN** the engine schema is created
- **THEN** indexes exist on `(instance_id, id)` and `(project_id, id)`

---

### Requirement: Inbox table

The system MUST provide `engine.inbox` to store queued signals and timers with lease-based claiming and optional deduplication.

#### Scenario: Inbox table structure
- **WHEN** the engine schema is created
- **THEN** `engine.inbox` has columns: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `project_id UUID NOT NULL`, `instance_id UUID NOT NULL REFERENCES engine.instances(id)`, `run_id UUID REFERENCES engine.runs(id)`, `history_id BIGINT REFERENCES engine.history(id)`, `kind TEXT NOT NULL`, `payload JSONB`, `status engine.inbox_status NOT NULL DEFAULT 'pending'`, `available_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `claimed_by TEXT`, `claimed_at TIMESTAMPTZ`, `lease_expires_at TIMESTAMPTZ`, `dedupe_key TEXT`, `resolved_at TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

#### Scenario: Inbox dedupe uniqueness
- **WHEN** two inbox rows are inserted with the same `project_id` and non-null `dedupe_key`
- **THEN** the second insert fails with a unique constraint violation

#### Scenario: Inbox indexes
- **WHEN** the engine schema is created
- **THEN** a partial claim index exists on `(status, available_at, lease_expires_at)` covering both fresh (`pending`) and expired-lease (`claimed`) rows

---

### Requirement: Activity tasks table

The system MUST provide `engine.activity_tasks` to store external work items with lease-based claiming.

#### Scenario: Activity tasks table structure
- **WHEN** the engine schema is created
- **THEN** `engine.activity_tasks` has columns: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `project_id UUID NOT NULL`, `instance_id UUID NOT NULL REFERENCES engine.instances(id)`, `run_id UUID NOT NULL REFERENCES engine.runs(id)`, `history_id BIGINT REFERENCES engine.history(id)`, `activity_key TEXT NOT NULL`, `activity_type TEXT NOT NULL`, `input JSONB`, `output JSONB`, `status engine.activity_task_status NOT NULL DEFAULT 'queued'`, `available_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `attempt_count INT NOT NULL DEFAULT 0`, `claimed_by TEXT`, `claimed_at TIMESTAMPTZ`, `lease_expires_at TIMESTAMPTZ`, `last_error_code TEXT`, `last_error_message TEXT`, `completed_at TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

#### Scenario: Activity key uniqueness
- **WHEN** two activity tasks are inserted with the same `run_id` and `activity_key`
- **THEN** the second insert fails with a unique constraint violation

#### Scenario: Activity tasks indexes
- **WHEN** the engine schema is created
- **THEN** a partial claim index exists on `(status, available_at, lease_expires_at)` covering both fresh (`queued`) and expired-lease (`claimed`) rows

---

### Requirement: Request dedupe table

The system MUST provide `engine.request_dedupe` to enforce idempotency for external requests scoped to a project.

#### Scenario: Request dedupe table structure
- **WHEN** the engine schema is created
- **THEN** `engine.request_dedupe` has columns: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `project_id UUID NOT NULL`, `request_scope TEXT NOT NULL`, `request_key TEXT NOT NULL`, `instance_id UUID REFERENCES engine.instances(id)`, `run_id UUID REFERENCES engine.runs(id)`, `status engine.request_dedupe_status NOT NULL DEFAULT 'in_progress'`, `response_payload JSONB`, `error_code TEXT`, `error_message TEXT`, `expires_at TIMESTAMPTZ NOT NULL`, `finalized_at TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

#### Scenario: Request dedupe uniqueness
- **WHEN** two request dedupe rows are inserted with the same `project_id`, `request_scope`, and `request_key`
- **THEN** the second insert fails with a unique constraint violation

---

### Requirement: Projection checkpoints table

The system MUST provide `engine.projection_checkpoints` to track consumer positions in the history stream.

#### Scenario: Projection checkpoints table structure
- **WHEN** the engine schema is created
- **THEN** `engine.projection_checkpoints` has columns: `projection_name TEXT NOT NULL`, `scope_key TEXT NOT NULL`, `last_history_id BIGINT NOT NULL`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()` with `PRIMARY KEY (projection_name, scope_key)`

#### Scenario: Checkpoint uniqueness
- **WHEN** two checkpoint rows are inserted with the same `projection_name` and `scope_key`
- **THEN** the second insert fails with a unique constraint violation

---

### Requirement: Engine-internal foreign keys

Engine tables MUST have foreign keys within the engine schema only. The schema MUST NOT add cross-schema FKs to `public.projects` or use `ON DELETE CASCADE`.

#### Scenario: FK dependency chain
- **WHEN** the engine schema is fully created
- **THEN** FKs exist: `runs.instance_id -> instances.id`, `history.instance_id -> instances.id`, `history.run_id -> runs.id`, `inbox.instance_id -> instances.id`, `inbox.run_id -> runs.id`, `inbox.history_id -> history.id`, `activity_tasks.instance_id -> instances.id`, `activity_tasks.run_id -> runs.id`, `activity_tasks.history_id -> history.id`, `request_dedupe.instance_id -> instances.id`, `request_dedupe.run_id -> runs.id`

#### Scenario: No cascade behavior
- **WHEN** an instance with dependent runs is deleted
- **THEN** the delete fails with a FK violation (RESTRICT/NO ACTION default)

---

### Requirement: Reversible down migration

The down migration MUST drop objects in reverse foreign-key dependency order without using `DROP SCHEMA engine CASCADE`.

#### Scenario: Down migration ordering
- **WHEN** `continua-engine migrate down` runs the foundation down migration
- **THEN** objects are dropped in order: `projection_checkpoints`, `request_dedupe`, `activity_tasks`, `inbox`, `history`, `runs`, `instances`, then enums, then the `engine` schema

#### Scenario: Down migration completeness
- **WHEN** the down migration completes
- **THEN** the `engine` schema no longer exists and no engine objects remain in the database
