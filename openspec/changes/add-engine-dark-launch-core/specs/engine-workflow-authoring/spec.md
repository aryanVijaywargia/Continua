# Capability: engine-workflow-authoring

Minimal public Go API for defining durable workflows. Lives in `engine/pkg/workflow` and is the only engine package importable by external consumers.

Related capabilities: [engine-history-events](../engine-history-events/spec.md), [engine-runtime-execution](../engine-runtime-execution/spec.md)

## ADDED Requirements

### Requirement: Workflow definition type

The authoring API MUST provide a `Definition` struct for registering workflow implementations.

#### Scenario: Definition structure
- **WHEN** a workflow author creates a `Definition`
- **THEN** the struct requires `Name string`, `Version string`, and `Run func(Context) error`

#### Scenario: Definition registration
- **WHEN** a `Definition` is passed to the `serve` command's workflow registry
- **THEN** the runtime can look up the definition by `(Name, Version)` for execution and replay

#### Scenario: Start requires a registered definition
- **WHEN** `continua-engine start` is invoked with a `(definition, version)` pair that is not present in the compiled registry
- **THEN** the command rejects the request before writing any `instances`, `runs`, or `request_dedupe` rows

---

### Requirement: Workflow input primitive

The authoring API MUST provide `Context.Input(out)` for reading the workflow input captured by the initial `workflow.started` event.

#### Scenario: Input on first activation
- **WHEN** `start` creates a run with `--input` and workflow code calls `ctx.Input(&value)`
- **THEN** the value is deserialized from the `workflow.started.input` payload for that run

#### Scenario: Input on replay
- **WHEN** workflow code calls `ctx.Input(&value)` during replay
- **THEN** the same value is deserialized from the recorded `workflow.started` event without relying on process-local CLI state

#### Scenario: No input supplied
- **WHEN** workflow code calls `ctx.Input(&value)` and `start` omitted `--input`
- **THEN** the call succeeds and leaves the target at its zero value

---

### Requirement: Activity primitive

The authoring API MUST provide `Context.Activity(key, activityType, input, out)` for scheduling durable activities.

#### Scenario: Activity scheduling
- **WHEN** a workflow calls `ctx.Activity("fetch-user", "http.get", input, &result)`
- **THEN** during first execution, the runtime appends `activity.scheduled` and schedules an activity task with the given key and type
- **THEN** the workflow blocks until the activity completes

#### Scenario: Activity replay
- **WHEN** a workflow calls `ctx.Activity(...)` during replay and matching `activity.scheduled` then `activity.completed` events exist in history
- **THEN** the result is deserialized from the recorded output without re-executing the activity

#### Scenario: Activity replay failure
- **WHEN** a workflow calls `ctx.Activity(...)` during replay and matching `activity.scheduled` then `activity.failed` events exist in history
- **THEN** the call returns the recorded failure without re-executing the activity

#### Scenario: Stable key required
- **WHEN** a workflow calls `ctx.Activity("")` with an empty key
- **THEN** the call returns an error

---

### Requirement: Timer primitive

The authoring API MUST provide `Context.SleepUntil(key, at)` for durable timer scheduling.

#### Scenario: Timer scheduling
- **WHEN** a workflow calls `ctx.SleepUntil("approval-deadline", futureTime)`
- **THEN** during first execution, the runtime records a `timer.scheduled` event, creates a run-scoped inbox row with `kind = 'timer'` and `available_at = due_at`, and the run transitions to `waiting`
- **THEN** the workflow resumes after the maintenance worker detects the timer inbox row is due and wakes the run

#### Scenario: Timer replay
- **WHEN** a workflow calls `ctx.SleepUntil(...)` during replay and a matching `timer.fired` event exists in history
- **THEN** the call returns immediately without re-scheduling

#### Scenario: Stable key required
- **WHEN** a workflow calls `ctx.SleepUntil("")` with an empty key
- **THEN** the call returns an error

---

### Requirement: Signal primitive

The authoring API MUST provide `Context.ReceiveSignal(name, out)` for receiving durable signals.

#### Scenario: Signal reception
- **WHEN** a workflow calls `ctx.ReceiveSignal("approval", &decision)`
- **THEN** during first execution, the run transitions to `waiting` for the named signal with `waiting_for = {"kind":"signal","signal_name":"approval"}`
- **THEN** the workflow resumes when a signal with the matching name arrives via the `signal` command and activation appends `signal.received`

#### Scenario: Signal replay
- **WHEN** a workflow calls `ctx.ReceiveSignal(...)` during replay and a matching `signal.received` event exists in history
- **THEN** the payload is deserialized from the recorded event without re-waiting

---

### Requirement: Cancellation observation

The authoring API MUST provide `Context.CancellationRequested()` for checking if cancellation has been requested.

#### Scenario: Cancellation check
- **WHEN** a workflow calls `ctx.CancellationRequested()`
- **THEN** returns `true` if a `cancel.requested` event exists in the current activation's inbox/history
- **THEN** returns `false` otherwise

#### Scenario: Cancellation delivery
- **WHEN** activation consumes a cancel inbox row for the active run
- **THEN** it appends `cancel.requested` before `ctx.CancellationRequested()` can observe the cancellation

#### Scenario: Cancellation is observational
- **WHEN** cancellation is requested but the workflow does not check `CancellationRequested()`
- **THEN** the workflow continues executing normally; cancellation is not forced

---

### Requirement: Custom status primitive

The authoring API MUST provide `Context.SetCustomStatus(value)` for setting workflow-visible status.

#### Scenario: Custom status update
- **WHEN** a workflow calls `ctx.SetCustomStatus(map[string]string{"step": "processing"})`
- **THEN** a `custom_status.updated` event is appended to history
- **THEN** `runs.custom_status` cache is updated during activation commit

---

### Requirement: Non-determinism prohibition

Workflow definitions MUST NOT perform non-deterministic operations.

#### Scenario: Prohibited operations
- **WHEN** a workflow definition is authored
- **THEN** it MUST NOT read wall clocks, generate random values, perform network I/O, or spawn goroutines
- **THEN** all external interaction MUST go through `Context` primitives

---

### Requirement: Dark-launch demo definitions

Test and demo workflow definitions MUST live under `engine/cmd/continua-engine/internal/darklaunch`, not in `engine/pkg/workflow` or `engine/internal/workflow`.

#### Scenario: Demo location
- **WHEN** demo workflow definitions and activity handlers are created for testing the dark-launch
- **THEN** they are placed in `engine/cmd/continua-engine/internal/darklaunch`
- **THEN** they are not placed in the public API package or runtime machinery packages
