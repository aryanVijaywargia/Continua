## ADDED Requirements

### Requirement: ContinueAsNew Run Status
The engine SHALL support a `CONTINUED_AS_NEW` terminal run lifecycle status that indicates the current run has been atomically replaced by a new continuation run.

`CONTINUED_AS_NEW` is terminal for the current run: it SHALL NOT be claimed, activated, or further modified.

`CONTINUED_AS_NEW` is terminal for purge and retention purposes.

`continued_as_new` SHALL be included anywhere the runtime currently treats run statuses as terminal.

The instance SHALL remain `active` during continuation (a new active run exists).

#### Scenario: Run transitions to continued_as_new
- **WHEN** a workflow returns the `ContinueAsNew` sentinel
- **THEN** the run status transitions to `continued_as_new` with `completed_at = NOW()`

#### Scenario: Instance stays active
- **WHEN** a run transitions to `continued_as_new`
- **THEN** the instance status remains `active`

#### Scenario: Continued run is eligible for terminal handling
- **WHEN** a run transitions to `continued_as_new`
- **THEN** terminal read paths, purge checks, and retention candidate selection all treat it as terminal

### Requirement: Continuation Creates Run N+1
When a workflow returns the `ContinueAsNew` sentinel, the engine SHALL atomically create run N+1 within the same activation transaction.

Run N+1 SHALL have `run_number = previous + 1`, the same `instance_id`, the same `definition_version`, and the continuation input.

Run N+1 SHALL start with a single `workflow.started` history event and status `queued`.

#### Scenario: Continuation run creation
- **WHEN** run 1 returns `ContinueAsNew(newInput)`
- **THEN** run 2 is created with `run_number = 2`, same instance, status `queued`
- **AND** run 2's history starts with `workflow.started` containing `newInput`

### Requirement: Run Chain Linkage
The engine SHALL maintain bidirectional run chain linkage via `continued_from_run_id` and `continued_to_run_id` on `engine.runs`.

`continued_to_run_id` SHALL be set on run N within the continuation transaction. `continued_from_run_id` SHALL be set on run N+1 at creation.

#### Scenario: Chain linkage on continuation
- **WHEN** run 1 continues as run 2
- **THEN** run 1 has `continued_to_run_id = run2.id`
- **AND** run 2 has `continued_from_run_id = run1.id`

### Requirement: Latest Run Uses Highest Run Number
Any per-instance "latest/current run" lookup SHALL order runs by `run_number` descending, using `id` as the tiebreaker.

#### Scenario: Latest run resolves to continuation target
- **WHEN** run 1 continues as run 2
- **AND** both runs belong to the same instance
- **THEN** the instance's latest/current run resolves to run 2 because it has the higher `run_number`

### Requirement: Old Run Cleanup on Continuation
When a run transitions to `CONTINUED_AS_NEW`, the engine SHALL cancel open activity tasks and discard open inbox items on the old run, using the same cleanup operations as cancellation.

#### Scenario: Old run cleanup
- **WHEN** run 1 transitions to `continued_as_new`
- **THEN** run 1's open activity tasks are cancelled
- **AND** run 1's open inbox items are discarded

### Requirement: ContinueAsNew History Event
The engine SHALL append a `workflow.continued_as_new` history event as the terminal event on the old run.

The event payload SHALL contain the continuation input.

#### Scenario: Terminal history event
- **WHEN** a workflow returns `ContinueAsNew(newInput)`
- **THEN** a `workflow.continued_as_new` event is appended to the old run's history with the continuation input
