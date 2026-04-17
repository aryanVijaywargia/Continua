## ADDED Requirements

### Requirement: Execution Target Activity Option
`workflow.ActivityOptions` MUST support an `ExecutionTarget` field with normalized values `local` and `remote`. An empty or unset value MUST default to `local`.

When a workflow schedules an activity with `ExecutionTarget: "remote"`, the created `engine.activity_tasks` row MUST have `execution_target = 'remote'`.

The `execution_target` column MUST have a CHECK constraint: `execution_target IN ('local', 'remote')`. The migration MUST add partial indexes that include `execution_target` to support efficient local and remote claim queries (e.g., `WHERE execution_target = 'local' AND status = 'queued'` and `WHERE execution_target = 'remote' AND status = 'queued' AND activity_type IN (...)`).

#### Scenario: Default execution target is local
- **WHEN** a workflow calls `Activity("key", "type", input, &out)` without options
- **THEN** the activity task is created with `execution_target = 'local'`

#### Scenario: Explicit remote execution target
- **WHEN** a workflow calls `ActivityWithOptions("key", "type", input, &out, ActivityOptions{ExecutionTarget: "remote"})`
- **THEN** the activity task is created with `execution_target = 'remote'`

#### Scenario: Invalid execution target rejected
- **WHEN** a workflow provides `ExecutionTarget: "gpu_cluster"`
- **THEN** the activity options normalization returns an error

### Requirement: Local And Remote Claim Isolation
In-process (local) activity worker claim queries MUST only claim tasks with `execution_target = 'local'`. Remote claim endpoint queries MUST only claim tasks with `execution_target = 'remote'`.

Existing in-process workers MUST continue handling default local tasks without any migration or configuration change.

#### Scenario: Local worker ignores remote tasks
- **WHEN** the in-process activity worker claims the next task
- **AND** there are pending tasks with `execution_target = 'remote'` but none with `local`
- **THEN** the claim returns no task

#### Scenario: Remote claim ignores local tasks
- **WHEN** a remote worker calls the claim endpoint
- **AND** there are pending tasks with `execution_target = 'local'` but none with `remote`
- **THEN** the claim returns an empty task list

#### Scenario: No migration break for existing workflows
- **WHEN** the execution_target column migration runs
- **THEN** all existing activity tasks default to `execution_target = 'local'`
- **AND** existing in-process workers continue claiming and completing them with no behavior change

### Requirement: No Task Queue In V1
The engine MUST NOT implement named task queues or queue-based routing in this version. Remote task routing MUST use only `execution_target` filtering combined with `activity_types` filtering on the claim request.

#### Scenario: Routing by execution target and activity type
- **WHEN** a remote worker claims with `activity_types: ["send_email"]`
- **THEN** only remote tasks with `activity_type = 'send_email'` are returned
- **AND** no queue name or queue routing is involved
