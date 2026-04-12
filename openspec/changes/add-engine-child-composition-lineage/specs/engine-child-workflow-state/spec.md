## ADDED Requirements

### Requirement: Child Workflows Authoritative Table
The engine MUST maintain `engine.child_workflows` as the authoritative parent-child relationship table. It MUST be project-scoped with `project_id` and MUST contain: parent instance ID, parent run ID, child key, requested definition name, requested definition version, child instance ID, child instance key, current child run ID, terminal child run ID, root run ID, child depth, continuation count, status, parent wait failed marker/error fields, and timestamps.

The table MUST enforce uniqueness on `(project_id, parent_run_id, child_key)` so repeated scheduling of the same child key by the same parent run is idempotent. It MUST also enforce uniqueness on `(project_id, child_instance_id)` so `get by child instance` is unambiguous and a child instance cannot be attached to multiple parent relationship rows in v1. The stored requested definition name and requested definition version are the authoritative binding used by replay and custom instance-key attach checks.

When an existing row is found for `(project_id, parent_run_id, child_key)`, the new request MUST match the stored requested definition name and requested definition version. A mismatched binding MUST fail deterministically instead of attaching to the existing child.

The status field MUST track the child workflow entry lifecycle: `active`, `completed`, `failed`, `cancelled`, `terminated`.

Replay MUST read child workflow outcomes from this table, matching the activity-task outcome pattern. The engine MUST NOT insert parent inbox items for child completion.

#### Scenario: Child workflows table created on first child
- **WHEN** a parent workflow creates its first child
- **THEN** a row is inserted into `engine.child_workflows` with status `active`, the correct project, parent/child IDs, requested definition name/version, root run, and depth

#### Scenario: Idempotent child creation
- **WHEN** a parent run calls `ChildWorkflow` with the same `child_key` twice (e.g., during replay)
- **THEN** the second call returns the existing child workflow entry
- **AND** no duplicate row is created
- **AND** the uniqueness constraint on `(project_id, parent_run_id, child_key)` remains satisfied

#### Scenario: Child key reused with different definition version
- **WHEN** a parent run calls `ChildWorkflow` with a `child_key` that already exists for that parent run
- **AND** the requested definition version differs from the stored requested definition version
- **THEN** the child creation fails deterministically without changing the existing child workflow entry

#### Scenario: Child instance cannot have multiple parent rows
- **WHEN** a child workflow relationship already exists for child instance C
- **AND** another parent run or child key attempts to create a second `engine.child_workflows` row for child instance C
- **THEN** the insert fails or is rejected deterministically
- **AND** lookup by child instance C returns at most one relationship row

### Requirement: Denormalized Lineage Columns On Runs
The engine MUST maintain `parent_run_id`, `root_run_id`, `child_key`, and `child_depth` as denormalized columns on `engine.runs`. Child run lineage columns MUST be set in the same transaction as `engine.child_workflows` inserts/updates.

New root run creation MUST set `parent_run_id = NULL`, `root_run_id = id`, `child_key = NULL`, and `child_depth = 0` in the same transaction as the root `engine.runs` insert.

Existing runs MUST be backfilled in migration with `root_run_id = id` and `child_depth = 0`.

If `engine.child_workflows` and `engine.runs` disagree on lineage values, repair MUST resolve in favor of `engine.child_workflows`.

#### Scenario: Denormalized columns set on child run creation
- **WHEN** a child run is created at depth 2 with parent run P and root run R
- **THEN** the child's `engine.runs` row has `parent_run_id = P`, `root_run_id = R`, `child_key = <key>`, `child_depth = 2`

#### Scenario: Root run lineage defaults
- **WHEN** a new root workflow run is created
- **THEN** the root `engine.runs` row has `parent_run_id = NULL`, `root_run_id = id`, `child_key = NULL`, and `child_depth = 0`

#### Scenario: Existing runs backfilled
- **WHEN** the migration runs on a database with existing engine runs
- **THEN** all existing runs have `root_run_id = id` and `child_depth = 0`

### Requirement: Child Terminal State Transition
On final child terminal transition (completed, failed, cancelled, or terminated), the engine MUST update `terminal_child_run_id` and child workflow status in the same transaction as the child's terminal history append. It MUST call `WakeWaitingRun` for the parent run only if the child workflow entry has no durable parent wait failed marker and the parent run is still waiting on the matching child workflow wait identity (for example child workflow row ID or `(parent_run_id, child_key)`). If the parent has already consumed a `child_workflow.wait_failed` outcome, late child terminal transition MUST NOT wake the parent.

The parent replay transaction that appends `child_workflow.wait_failed` MUST also set the durable parent wait failed marker and error code/message on `engine.child_workflows` without changing the child workflow `status` or `terminal_child_run_id`.

If the child workflow entry has reached the continuation follow-depth guard and the child reaches a terminal state before the parent replay records the wait failure, the parent replay MUST set the parent wait failed marker and return the wait failure before consuming the terminal outcome.

The engine MUST NOT wake the parent on intermediate child ContinueAsNew transitions below `max_continuation_follow_depth`. The parent wait blocks until a terminal child run exists, except when the continuation follow-depth guard wakes the parent with a deterministic wait failure.

#### Scenario: Child completion wakes parent
- **WHEN** a child workflow completes
- **AND** the parent has not consumed `child_workflow.wait_failed`
- **AND** the parent run is still waiting on the matching child workflow wait identity
- **THEN** `engine.child_workflows.terminal_child_run_id` is set to the completing run's ID
- **AND** `engine.child_workflows.status` is set to `completed`
- **AND** `WakeWaitingRun` is called for the parent run
- **AND** all three updates happen in the same transaction as the child's `workflow.completed` history event

#### Scenario: Late child completion after parent wait failure does not wake parent
- **WHEN** the parent has already consumed `child_workflow.wait_failed` for a child workflow entry
- **AND** the child workflow later completes
- **THEN** the child terminal transition updates `terminal_child_run_id` and status
- **AND** does not call `WakeWaitingRun` for the parent
- **AND** does not wake the parent out of any unrelated later wait

#### Scenario: Child ContinueAsNew does not wake parent
- **WHEN** a child run returns ContinueAsNew
- **AND** the child workflow continuation count remains below 32
- **THEN** a new child run is created and `current_child_run_id` is updated atomically
- **AND** the parent run is NOT woken
- **AND** the parent wait continues blocking

### Requirement: Cancel Cascade Semantics
Cooperative cancel MUST cascade only after the parent workflow returns `workflow.ErrCancelled`. At that point, the engine MUST insert cancel inbox items for all active children.

Each cascaded child cancel inbox item MUST use a per-child-run dedupe key in the existing top-level cancel form: `cancel:<current_child_run_id>`. If a cascaded cancel item already exists for that child run, replay or retry MUST treat the insert as idempotent success and MUST NOT create a duplicate cancel inbox item.

Force terminate MUST cascade recursively and immediately in the parent termination transaction. All active child runs at every depth in the bounded lineage tree MUST be transitioned to terminated, their history appended with `workflow.terminated`, and their `engine.child_workflows` entries updated to `terminated`. Direct-only termination is not sufficient because it would orphan active grandchildren.

#### Scenario: Cooperative cancel cascade
- **WHEN** a parent workflow receives a cancel request and returns `workflow.ErrCancelled`
- **THEN** all active children of that parent receive cancel inbox items
- **AND** children observe the cancel on their next activation

#### Scenario: Duplicate cooperative cancel cascade
- **WHEN** parent cancellation is replayed or retried after a child cancel inbox item was already inserted
- **THEN** the engine reuses the `cancel:<current_child_run_id>` dedupe key
- **AND** no duplicate child cancel inbox item is created

#### Scenario: Force terminate cascade is recursive
- **WHEN** an operator force-terminates a parent workflow that has children, and one child has its own grandchildren
- **THEN** all active children and grandchildren (the full lineage tree) are immediately terminated in the same transaction
- **AND** each terminated descendant has a `workflow.terminated` history event
- **AND** each descendant's `engine.child_workflows` entry shows status `terminated`

### Requirement: Child ContinueAsNew Tracking
When a child run returns ContinueAsNew, the engine MUST create the next child run before updating `current_child_run_id`, and MUST update `current_child_run_id` and `continuation_count` in the same transaction.

The child ContinueAsNew MUST NOT create a new `engine.child_workflows` entry. It MUST update the existing entry's `current_child_run_id`.

#### Scenario: Atomic child continuation
- **WHEN** a child run at run_number 1 returns ContinueAsNew
- **THEN** child run_number 2 is created
- **AND** `engine.child_workflows.current_child_run_id` is updated to run 2's ID
- **AND** `engine.child_workflows.continuation_count` is incremented
- **AND** all updates happen in the same transaction
