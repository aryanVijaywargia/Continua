## ADDED Requirements

### Requirement: Run Suspension
The engine SHALL support a `SUSPENDED` run lifecycle status that pauses execution without terminating the workflow.

Suspend SHALL be allowed from `queued` or `waiting` status only. Suspend from `running` SHALL be rejected with a typed error. Suspend on a terminal run SHALL be rejected.

Suspend SHALL use CAS on the current `status` to ensure atomicity with the claim operation.

Suspend SHALL clear `claimed_by`, `claimed_at`, and `lease_expires_at` and set `updated_at = NOW()`.

Suspend on an already-suspended run SHALL be idempotent (no-op, success response).

#### Scenario: Suspend from queued
- **WHEN** an operator calls suspend on a run with status `queued`
- **THEN** the run transitions to `suspended` via CAS
- **AND** a `workflow.suspended` history event is appended

#### Scenario: Suspend from waiting
- **WHEN** an operator calls suspend on a run with status `waiting`
- **THEN** the run transitions to `suspended` via CAS
- **AND** a `workflow.suspended` history event is appended

#### Scenario: Suspend from running rejected
- **WHEN** an operator calls suspend on a run with status `running`
- **THEN** the operation returns a typed error indicating the run is mid-activation

#### Scenario: Suspend on terminal run rejected
- **WHEN** an operator calls suspend on a run with a terminal status
- **THEN** the operation returns a typed error indicating the run is terminal

#### Scenario: Suspend idempotency
- **WHEN** an operator calls suspend on a run with status `suspended`
- **THEN** the operation returns success with the current run state

### Requirement: Run Resume
The engine SHALL support resuming a suspended run by transitioning it from `suspended` to `queued`.

Resume SHALL clear `waiting_for`, `claimed_by`, `claimed_at`, `lease_expires_at` and set `ready_at = NOW()`, `updated_at = NOW()`.

Resume on a non-suspended active run SHALL be idempotent (no-op, success response). Resume on a terminal run SHALL be rejected.

#### Scenario: Resume from suspended
- **WHEN** an operator calls resume on a run with status `suspended`
- **THEN** the run transitions to `queued` with `ready_at = NOW()`
- **AND** `waiting_for` is cleared
- **AND** a `workflow.resumed` history event is appended

#### Scenario: Resume on active non-suspended run
- **WHEN** an operator calls resume on a run with status `queued`, `running`, or `waiting`
- **THEN** the operation returns success with the current run state

#### Scenario: Resume on terminal run rejected
- **WHEN** an operator calls resume on a run with a terminal status
- **THEN** the operation returns a typed error indicating the run is terminal

### Requirement: Suspend Accumulation
While a run is suspended, the engine SHALL allow timers, signals, activity completions, and cancel requests to accumulate without waking the run.

Maintenance timer wakeups SHALL skip suspended runs because the `ListDueTimerRunIDs` query filters for `waiting` status.

Signal delivery SHALL create inbox items regardless of run status.

Activity completion SHALL record the terminal task status but the `WakeWaitingRun` CAS SHALL fail for suspended runs (status is not `waiting`).

Cancel requests SHALL create `cancel` inbox items regardless of run status.

#### Scenario: Timer fires during suspension
- **WHEN** a timer becomes due while the run is suspended
- **THEN** the timer inbox item stays `pending` with `available_at` in the past
- **AND** the maintenance worker does not wake the run

#### Scenario: Signal delivered during suspension
- **WHEN** a signal is delivered while the run is suspended
- **THEN** a signal inbox item is created

#### Scenario: Activity completes during suspension
- **WHEN** an activity task completes while the run is suspended
- **THEN** the task's output is recorded
- **AND** the run is NOT waked (CAS on `waiting` status fails)

#### Scenario: Cancel requested during suspension
- **WHEN** a cancel request is delivered while the run is suspended
- **THEN** a `cancel` inbox item is created

### Requirement: Resume Reconciliation
On resume, the first activation's inbox frontier SHALL contain all items accumulated during suspension. The existing frontier processing logic SHALL reconcile accumulated state.

#### Scenario: Accumulated items visible after resume
- **WHEN** a suspended run is resumed and the workflow worker claims it
- **THEN** the activation loads all pending inbox items including those accumulated during suspension
- **AND** timers, signals, activity outcomes, and cancel requests are all visible in the frontier

### Requirement: Cancel During Suspension
A cancel request delivered during suspension SHALL be folded into the first resumed activation frontier before any wait resumes.

The existing `consumeFrontierCancels` logic SHALL process cancel inbox items before timer/signal/activity items in the frontier.

#### Scenario: Cancel during suspension followed by resume
- **WHEN** a cancel request is delivered while the run is suspended
- **AND** the run is subsequently resumed
- **THEN** the first activation sees `CancellationRequested() == true` before any timer/signal/activity resolution

### Requirement: Suspend/Resume History Events
The engine SHALL append `workflow.suspended` and `workflow.resumed` history events when suspend and resume operations take effect.

These events SHALL be appended in the same transaction as the status CAS, outside of an activation transaction.

Their `sequence_no` SHALL be allocated using the shared non-activation history sequencing rule so suspend/resume events cannot collide with `activity.retry_scheduled` or other control-path history appends for the same run.

#### Scenario: Suspend appends history event
- **WHEN** a suspend operation succeeds
- **THEN** a `workflow.suspended` event is appended to the run's history

#### Scenario: Resume appends history event
- **WHEN** a resume operation succeeds
- **THEN** a `workflow.resumed` event is appended to the run's history

### Requirement: Suspended Run Not Claimable
The workflow worker's `ClaimNextRun` query SHALL NOT match suspended runs.

#### Scenario: Claim skips suspended runs
- **WHEN** the workflow worker polls for claimable runs
- **THEN** runs with status `suspended` are not returned

### Requirement: Terminate Widened to Include Suspended
The `TransitionRunToTerminated` CAS predicate SHALL be widened from `status IN ('queued', 'running', 'waiting')` to `status IN ('queued', 'running', 'waiting', 'suspended')`.

Operators SHALL be able to terminate a suspended run without first resuming it.

#### Scenario: Terminate suspended run
- **WHEN** an operator calls terminate on a `suspended` run
- **THEN** the run transitions to `terminated` with `completed_at = NOW()`
- **AND** a `workflow.terminated` history event is appended

### Requirement: Suspend/Resume Summary Sync
The suspend and resume handlers SHALL call `syncProjectedTraceSummary` after the status CAS to immediately write `engine_run_status`, `engine_wait_state`, and pending-work counts to `public.traces`.

This ensures the debugger and trace-search filters reflect the current status without waiting for the async projector.

#### Scenario: Suspend syncs summary immediately
- **WHEN** a suspend operation succeeds
- **THEN** `engine_run_status` on `public.traces` is `suspended` within the same transaction
- **AND** the change is visible to trace-search filters immediately
