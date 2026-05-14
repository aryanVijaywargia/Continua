## ADDED Requirements

### Requirement: Child Workflow Context API
The engine MUST provide `ChildWorkflow(childKey, definitionName, definitionVersion string, input any, out any) error` and `ChildWorkflowWithOptions(childKey, definitionName, definitionVersion string, input any, out any, opts ChildWorkflowOptions) error` as blocking primitives on `workflow.Context`.

`ChildWorkflowOptions` MUST support an optional `InstanceKey` field. When omitted, the system MUST derive a deterministic default using canonical UUID string inputs and a lowercase hexadecimal SHA-256 digest: `child:v1:<hex_sha256(project_id.String() + "\x00" + parent_run_id.String() + "\x00" + child_key)>`.

Both methods MUST block until the child workflow reaches a terminal state (completed, failed, cancelled, or terminated), or until a durable parent-side child wait failure guard fires. On child completion, the output MUST be unmarshaled into `out`. On child failure, the method MUST return an error containing the child's error code and message. On child cancellation or termination, the method MUST return a coded error that preserves the child terminal state so callers can distinguish cooperative cancellation from force termination. A terminated child error MUST preserve the child termination code and message when available. On parent-side wait failure, the method MUST return a `workflow.ChildWorkflowError` with terminal state `wait_failed` and the code/message recorded in the durable `child_workflow.wait_failed` event.

The public `workflow` package MUST expose an inspectable child workflow error contract. Returned child terminal errors and parent-side child wait failures MUST satisfy `errors.As` for an exported `*workflow.ChildWorkflowError` type with accessors for `Code() string`, `Message() string`, and `TerminalState() string`. `TerminalState()` MUST return one of `failed`, `cancelled`, or `terminated` for terminal child outcomes, and `wait_failed` for non-terminal parent wait failures.

#### Scenario: Child workflow completes successfully
- **WHEN** a parent workflow calls `ChildWorkflow("process-order", "OrderProcessor", "v1", orderInput, &orderResult)`
- **THEN** a child workflow instance is created and started
- **AND** the parent blocks until the child completes
- **AND** the child's result is unmarshaled into `orderResult`

#### Scenario: Child workflow fails
- **WHEN** a parent workflow calls `ChildWorkflow(...)` and the child workflow fails
- **THEN** the parent receives an error with the child's error code and message
- **AND** `errors.As(err, &childErr)` succeeds for `*workflow.ChildWorkflowError`
- **AND** `childErr.TerminalState()` returns `failed`

#### Scenario: Child workflow is cancelled
- **WHEN** a parent workflow calls `ChildWorkflow(...)` and the child workflow is cancelled
- **THEN** the parent receives a coded error whose terminal state is `cancelled`
- **AND** `errors.As(err, &childErr)` succeeds for `*workflow.ChildWorkflowError`

#### Scenario: Child workflow is terminated
- **WHEN** a parent workflow calls `ChildWorkflow(...)` and the child workflow is force-terminated
- **THEN** the parent receives a coded error whose terminal state is `terminated`
- **AND** the error preserves the child termination code and message when available
- **AND** `errors.As(err, &childErr)` succeeds for `*workflow.ChildWorkflowError`

#### Scenario: Default instance key derivation
- **WHEN** a parent workflow calls `ChildWorkflow(...)` without explicit `InstanceKey`
- **THEN** the child instance key is `child:v1:<hex_sha256(project_id.String() + "\x00" + parent_run_id.String() + "\x00" + child_key)>` using a lowercase hexadecimal digest
- **AND** the same parent run calling with the same child_key produces the same instance key (idempotent)

#### Scenario: Custom instance key
- **WHEN** a parent workflow calls `ChildWorkflowWithOptions(...)` with `ChildWorkflowOptions{InstanceKey: "custom-key"}`
- **THEN** the child instance uses `"custom-key"` as its instance key

#### Scenario: Custom instance key already exists with matching binding
- **WHEN** a parent workflow provides an `InstanceKey` that already exists as an instance
- **AND** the existing instance has the same `definition_name`
- **AND** the `engine.child_workflows` entry links to that instance and matches the same parent run, child key, requested definition name, and requested definition version
- **THEN** the child attaches to the existing instance (idempotent replay case)

#### Scenario: Custom instance key conflicts with unrelated instance
- **WHEN** a parent workflow provides an `InstanceKey` that already exists as an instance
- **AND** the existing instance belongs to a different parent, has a different `definition_name`, lacks a matching `engine.child_workflows` entry, the matching entry points to a different instance, or the matching entry has a different requested definition version
- **THEN** the child creation fails deterministically with error code `instance_conflict`

### Requirement: Child Workflow History Events
The engine MUST record the following history events on the parent run for child workflow lifecycle:
- `child_workflow.scheduled`: recorded when the child is first requested (during parent activation)
- `child_workflow.started`: recorded when the child's first run is created (during parent activation)
- `child_workflow.completed`: recorded when the parent replays and observes the child's completed outcome
- `child_workflow.failed`: recorded when the parent replays and observes the child's failed outcome
- `child_workflow.cancelled`: recorded when the parent replays and observes the child's cancelled outcome
- `child_workflow.terminated`: recorded when the parent replays and observes the child's terminated outcome
- `child_workflow.wait_failed`: recorded when the parent wait fails for a non-terminal parent-side guard such as `max_continuation_follow_depth_exceeded`

The event payloads MUST be public structs in `engine/pkg/history` with the following JSON fields:
- `child_workflow.scheduled`: `child_key`, `definition_name`, `definition_version`, `input`, `child_instance_key`
- `child_workflow.started`: `child_key`, `child_instance_id`, `child_instance_key`, `child_run_id`, `root_run_id`, `child_depth`
- `child_workflow.completed`: `child_key`, `child_instance_id`, `terminal_child_run_id`, `result`
- `child_workflow.failed`: `child_key`, `child_instance_id`, `terminal_child_run_id`, `error_code`, `error_message`
- `child_workflow.cancelled`: `child_key`, `child_instance_id`, `terminal_child_run_id`, `error_code`, `error_message`
- `child_workflow.terminated`: `child_key`, `child_instance_id`, `terminal_child_run_id`, `error_code`, `error_message`
- `child_workflow.wait_failed`: `child_key`, `child_instance_id`, `current_child_run_id`, `error_code`, `error_message`

The `child_workflow.completed.result` value MUST be the same JSON result recorded by the terminal child `workflow.completed` event. The failed, cancelled, and terminated child workflow events MUST provide the values used to construct `workflow.ChildWorkflowError`; cancelled children SHOULD use error code `cancelled` and error message `workflow cancelled` unless a more specific cancellation message is available.

`child_workflow.wait_failed` MUST NOT require `terminal_child_run_id` because it represents a parent-side wait failure while the child workflow remains active. Replay MUST consume `child_workflow.wait_failed` as a durable parent wait outcome and return a `workflow.ChildWorkflowError` with code `max_continuation_follow_depth_exceeded` and terminal state `wait_failed`.

The child terminal transaction updates `engine.child_workflows` and calls `WakeWaitingRun` for the parent only when the state rules allow it: no durable parent wait failed marker is set, and the parent run is still waiting on the matching child workflow wait identity. The parent's `child_workflow.completed|failed|cancelled|terminated` history event is then appended during the parent's next activation/replay, matching the activity-task outcome pattern where the outcome event is recorded by the consumer, not the producer.

The child's own run MUST also record its standard terminal history event (`workflow.completed`, `workflow.failed`, `workflow.cancelled`, or `workflow.terminated`) in the child's terminal transaction.

`child_workflow.scheduled` and `child_workflow.started` MUST be recorded in the same transaction as child instance/run creation.

#### Scenario: Parent history records child lifecycle
- **WHEN** a child workflow is scheduled, starts, and completes
- **THEN** the parent run's history contains `child_workflow.scheduled`, `child_workflow.started`, and `child_workflow.completed` events
- **AND** the child run's history contains `workflow.started` and `workflow.completed` events

#### Scenario: Atomic scheduled and started recording
- **WHEN** a child workflow is first created
- **THEN** `child_workflow.scheduled` and `child_workflow.started` appear in the same transaction
- **AND** the `engine.child_workflows` row is also created in that transaction

### Requirement: Child Workflow Wait Kind
The engine MUST support `WaitKindChildWorkflow = "child_workflow"` as a wait kind. Replay MUST handle child workflow waits equivalently to activity and timer waits: consuming recorded scheduled/started events, checking for terminal outcome events or `child_workflow.wait_failed` events in history, and blocking if no terminal child outcome or durable parent-side wait failure is available.

#### Scenario: Replay determinism for child workflows
- **WHEN** a workflow that previously scheduled a child is replayed
- **THEN** replay consumes the recorded `child_workflow.scheduled` and `child_workflow.started` events
- **AND** if a terminal outcome event or `child_workflow.wait_failed` event exists in history, replay consumes it and returns the result/error
- **AND** if no terminal outcome or wait failure exists, replay checks `engine.child_workflows` for a pending outcome or parent-side wait guard
- **AND** if no outcome or wait failure is available, the workflow blocks on a `child_workflow` wait

### Requirement: Child Workflow Depth Limits
The engine MUST enforce `max_child_depth = 32`. A workflow attempting to create a child at depth 32 or greater MUST fail deterministically with error code `max_child_depth_exceeded`.

The engine MUST enforce `max_continuation_follow_depth = 32` as a parent-side wait guard. Child ContinueAsNew transitions below this limit MUST NOT wake the waiting parent. When a child ContinueAsNew transition updates the child workflow entry to continuation count 32 before terminal completion, that transaction MUST wake the parent so the parent's next replay appends `child_workflow.wait_failed` and treats the child wait as failed with error code `max_continuation_follow_depth_exceeded`. The guard MUST NOT set `terminal_child_run_id` or change the child workflow status; the child remains active and may still complete independently.

If the child reaches a terminal state after the 32nd continuation but before the parent replays and records `child_workflow.wait_failed`, the parent-side wait guard MUST take precedence. Replay MUST append and return `child_workflow.wait_failed` before considering the later terminal child outcome, so timing cannot decide whether the parent observes a terminal child result or `max_continuation_follow_depth_exceeded`.

#### Scenario: Depth limit exceeded
- **WHEN** a workflow at child_depth 31 attempts to create a child workflow
- **THEN** the child creation succeeds (depth becomes 32 for the grandchild slot)
- **WHEN** a workflow at child_depth 32 attempts to create a child workflow
- **THEN** the workflow fails deterministically with error code `max_child_depth_exceeded`

#### Scenario: Continuation follow depth exceeded
- **WHEN** a child workflow continues-as-new for the 32nd time without terminal completion
- **THEN** the child continuation transaction updates `current_child_run_id` and continuation count
- **AND** wakes the waiting parent
- **WHEN** the parent replays
- **THEN** the parent appends `child_workflow.wait_failed` with `current_child_run_id` and error code `max_continuation_follow_depth_exceeded`
- **AND** the parent's child workflow wait fails with error code `max_continuation_follow_depth_exceeded`
- **AND** `errors.As(err, &childErr)` succeeds for `*workflow.ChildWorkflowError`
- **AND** `childErr.TerminalState()` returns `wait_failed`
- **AND** the child's `engine.child_workflows.status` remains `active`
- **AND** `terminal_child_run_id` remains unset
- **AND** the child remains active and may still complete independently

#### Scenario: Terminal race after continuation follow depth exceeded
- **WHEN** a child workflow continues-as-new for the 32nd time and wakes the parent
- **AND** the child reaches a terminal state before the parent replays
- **WHEN** the parent replays
- **THEN** the parent appends `child_workflow.wait_failed`
- **AND** the parent receives `max_continuation_follow_depth_exceeded`
- **AND** the later terminal child outcome is not returned to the parent for that wait
