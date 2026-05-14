# Capability: engine-cli-runtime

CLI commands for engine runtime: `serve`, `start`, `signal`, `cancel`, and `inspect`. Extends [engine-cli-foundation](../../../../changes/add-engine-foundation/specs/engine-cli-foundation/spec.md).

Related capabilities: [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-runtime-config](../engine-runtime-config/spec.md)

## ADDED Requirements

### Requirement: serve command

The `continua-engine serve` command MUST start the engine runtime with three worker loops.

#### Scenario: Serve startup
- **WHEN** `continua-engine serve` is invoked
- **THEN** it starts workflow, activity, and maintenance worker loops
- **THEN** it uses normal process logging (not JSON command output)

#### Scenario: Serve shutdown
- **WHEN** the process receives a termination signal
- **THEN** worker loops drain gracefully and the process exits

#### Scenario: Serve fatal error
- **WHEN** a fatal startup or runtime error occurs
- **THEN** the process exits with nonzero exit code

#### Scenario: Serve is not a JSON command
- **WHEN** `serve` runs
- **THEN** it does NOT emit structured JSON to stdout for success/failure; it uses standard logging

---

### Requirement: start command

The `continua-engine start` command MUST create a new workflow instance with durable request deduplication.

#### Scenario: Start success
- **WHEN** `start` is invoked with a unique `instance_key` and `request_key`
- **THEN** it creates one instance and one run with `run_number = 1`
- **THEN** it outputs JSON to stdout: `{"instance_id":"...","instance_key":"...","run_id":"...","run_number":1,"status":"queued"}`
- **THEN** exit code is `0`

#### Scenario: Start transaction ordering
- **WHEN** `start` executes
- **THEN** within a single DB transaction: (1) run the atomic start-dedupe claim primitive, (2) if it returns a newly claimed or expired-takeover `in_progress` row, create the instance and run, (3) append `workflow.started` as the first history row for the run, carrying `definition_name`, `definition_version`, `instance_key`, and optional `input`, (4) finalize that dedupe row with the success response, (5) commit
- **THEN** if the transaction fails at any step, it rolls back atomically — no stranded `in_progress` dedupe rows

#### Scenario: Stranded in-progress dedupe row
- **WHEN** `start` encounters an existing dedupe row with `status = 'in_progress'` and `expires_at < NOW()`
- **THEN** it treats the row as expired (the original `start` crashed before finalizing) and atomically takes over that same claim with a refreshed `expires_at`
- **WHEN** `start` encounters an existing dedupe row with `status = 'in_progress'` and `expires_at >= NOW()`
- **THEN** it returns `request_in_progress` (another `start` is actively executing)

#### Scenario: Retry after maintenance expiry
- **WHEN** `start` encounters an existing dedupe row with `status = 'expired'`
- **THEN** it atomically reclaims that same row as a new `in_progress` claim with a refreshed `expires_at`

#### Scenario: Start input persistence
- **WHEN** `start` is invoked with `--input`
- **THEN** that value is durably persisted in the `workflow.started` history payload for the new run
- **THEN** workflow code can read it through `Context.Input(...)` on first activation and replay

#### Scenario: Unknown definition or version
- **WHEN** `start` is invoked with a `(definition, version)` pair that is not registered in the compiled runtime
- **THEN** it returns `{"error":{"code":"definition_not_registered","message":"..."}}`
- **THEN** it does not write any `instances`, `runs`, or `request_dedupe` rows
- **THEN** exit code is `1`

#### Scenario: Duplicate request_key (finalized)
- **WHEN** `start` is invoked with a `request_key` that was already finalized with `status = 'completed'`
- **THEN** it returns the recorded response from the original request
- **THEN** exit code is `0`

#### Scenario: Duplicate request_key (failed)
- **WHEN** `start` is invoked with a `request_key` that was already finalized with `status = 'failed'`
- **THEN** it returns the recorded error from the original request
- **THEN** exit code is `1`

#### Scenario: Instance key conflict
- **WHEN** `start` is invoked with an `instance_key` that already exists but a different request
- **THEN** it returns `{"error":{"code":"instance_conflict","message":"..."}}`
- **THEN** exit code is `1`

#### Scenario: Request dedupe scope
- **WHEN** `start` creates a request dedupe entry
- **THEN** the `request_scope` is `engine.start`

---

### Requirement: signal command

The `continua-engine signal` command MUST deliver a durable signal to the active non-terminal run of a workflow instance.

#### Scenario: Signal success
- **WHEN** `signal` is invoked with an `instance_key`, `signal_name`, and optional payload
- **THEN** it inserts an inbox row with `kind = 'signal'` and `available_at = NOW()`
- **THEN** it attempts guarded `waiting -> queued` on the active run
- **THEN** it outputs JSON: `{"instance_id":"...","run_id":"...","accepted":true,"wake_applied":true|false}`
- **THEN** exit code is `0`

#### Scenario: Signal with dedupe key
- **WHEN** `signal` is invoked with a caller-supplied dedupe key
- **THEN** the inbox row uses that dedupe key for idempotent delivery

#### Scenario: Signal to non-waiting run
- **WHEN** `signal` is invoked and the run is not in `waiting` status
- **THEN** the inbox row is still inserted (signal is queued for next activation)
- **THEN** `wake_applied` is `false`

#### Scenario: Signal to terminal run
- **WHEN** `signal` is invoked and the active run is in `completed`, `failed`, or `cancelled` status
- **THEN** it returns `{"error":{"code":"run_terminal","message":"..."}}`
- **THEN** it does not insert an inbox row
- **THEN** exit code is `1`

---

### Requirement: cancel command

The `continua-engine cancel` command MUST deliver a durable cancellation request to the active non-terminal run of a workflow instance.

#### Scenario: Cancel success
- **WHEN** `cancel` is invoked with an `instance_key`
- **THEN** it inserts an inbox row with `kind = 'cancel'` using a fixed per-run dedupe key
- **THEN** it attempts guarded `waiting -> queued` on the active run
- **THEN** it outputs JSON: `{"instance_id":"...","run_id":"...","accepted":true,"wake_applied":true|false}`
- **THEN** exit code is `0`

#### Scenario: Repeated cancel coalescing
- **WHEN** `cancel` is invoked multiple times for the same run
- **THEN** the fixed dedupe key ensures only one cancel inbox row exists

#### Scenario: Cancellation is observational
- **WHEN** `cancel` succeeds
- **THEN** the workflow is NOT forcibly terminated; it must check `CancellationRequested()` to observe the cancellation

#### Scenario: Cancel on terminal run
- **WHEN** `cancel` is invoked and the active run is in `completed`, `failed`, or `cancelled` status
- **THEN** it returns `{"error":{"code":"run_terminal","message":"..."}}`
- **THEN** it does not insert an inbox row
- **THEN** exit code is `1`

---

### Requirement: inspect command

The `continua-engine inspect` command MUST return the current state of a workflow instance.

#### Scenario: Inspect success
- **WHEN** `inspect` is invoked with an `instance_key`
- **THEN** it outputs JSON to stdout containing: `instance_id`, `instance_key`, `definition_name`, `definition_version`, `run_id`, `run_number`, `status`, `result`, `custom_status`, `waiting_for`, `history`
- **THEN** exit code is `0`

#### Scenario: Inspect not found
- **WHEN** `inspect` is invoked with a non-existent `instance_key`
- **THEN** it returns `{"error":{"code":"not_found","message":"..."}}`
- **THEN** exit code is `1`

---

### Requirement: JSON output conventions

All command output (except `serve`) MUST follow consistent JSON conventions.

#### Scenario: Success output
- **WHEN** a command succeeds
- **THEN** it outputs a JSON object to stdout with command-specific fields
- **THEN** exit code is `0`

#### Scenario: Error output
- **WHEN** a command fails
- **THEN** it outputs `{"error":{"code":"...","message":"..."}}` to stdout
- **THEN** exit code is `1`

#### Scenario: Error codes
- **WHEN** an error occurs
- **THEN** the `code` field uses one of: `definition_not_registered`, `instance_conflict`, `request_in_progress`, `run_terminal`, `not_found`, `internal_error`
