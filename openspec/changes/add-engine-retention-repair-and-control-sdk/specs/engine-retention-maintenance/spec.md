# Capability: engine-retention-maintenance

Env-driven platform-side retention maintenance that automates `projection_only` and `full` purges on terminal engine runs. It is implemented as one River-backed maintenance path with two stages, is idempotent across crash/restart, and keys off existing engine projection states.

Related capabilities: [engine-trace-projection](../engine-trace-projection/spec.md), [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-schema-runtime-delta](../engine-schema-runtime-delta/spec.md)

## ADDED Requirements

### Requirement: Retention env configuration

The platform server MUST configure retention via two environment variables and MUST make the feature opt-in.

#### Scenario: Default disabled
- **WHEN** neither `ENGINE_PROJECTION_RETENTION_AFTER` nor `ENGINE_HISTORY_RETENTION_AFTER` is set
- **THEN** no retention job is registered or scheduled
- **THEN** the platform behaves identically to a deployment without retention

#### Scenario: Projection-only retention enabled
- **WHEN** `ENGINE_PROJECTION_RETENTION_AFTER` is set to a valid duration and `ENGINE_HISTORY_RETENTION_AFTER` is empty or zero
- **THEN** the retention worker runs stage 1 (`projection_only` purge) only
- **THEN** stage 2 (`full` purge) is skipped

#### Scenario: Both stages enabled
- **WHEN** both `ENGINE_PROJECTION_RETENTION_AFTER` and `ENGINE_HISTORY_RETENTION_AFTER` are set to valid durations and history > projection
- **THEN** the retention worker runs stage 1 then stage 2 on each iteration
- **THEN** stage 2 targets only runs whose completion age exceeds `ENGINE_HISTORY_RETENTION_AFTER`

#### Scenario: History without projection is invalid
- **WHEN** `ENGINE_HISTORY_RETENTION_AFTER` is set but `ENGINE_PROJECTION_RETENTION_AFTER` is unset or zero
- **THEN** startup fails fast with a typed configuration error
- **THEN** the process does not silently disable retention

#### Scenario: Invalid ordering is invalid
- **WHEN** both are set and `ENGINE_HISTORY_RETENTION_AFTER <= ENGINE_PROJECTION_RETENTION_AFTER`
- **THEN** startup fails fast with a typed configuration error
- **THEN** the process does not silently disable retention

#### Scenario: Unparseable duration is invalid
- **WHEN** either env var is present but cannot be parsed as a duration
- **THEN** startup fails fast with a typed configuration error naming the offending variable
- **THEN** the process does not silently disable retention

---

### Requirement: Single platform-side retention job with two stages

The platform server MUST add exactly one retention maintenance path, implemented as one periodic River job and its worker, that processes stage 1 and stage 2 in order during each run.

#### Scenario: One maintenance path only
- **WHEN** retention is enabled
- **THEN** exactly one platform-side periodic job / worker pair implements the retention loop
- **THEN** two separate workers are NOT introduced for the two stages
- **THEN** no engine-side maintenance subroutine is introduced in `continua-engine`

#### Scenario: Shared service dependency
- **WHEN** the retention worker is constructed
- **THEN** it depends on the same shared Fx-provided purge service used by the public API
- **THEN** it does not depend on a service that is privately constructed inside the API server

#### Scenario: Stage 1 runs before stage 2
- **WHEN** the retention worker executes an iteration
- **THEN** stage 1 (`projection_only` purge on runs past `ENGINE_PROJECTION_RETENTION_AFTER`) runs first
- **THEN** stage 2 (`full` purge on runs past `ENGINE_HISTORY_RETENTION_AFTER`) runs after stage 1 completes

#### Scenario: Stage gating honors current projection state
- **WHEN** stage 1 selects candidates
- **THEN** only traces whose current `engine_projection_state` is `up_to_date` or `catching_up` are transitioned to `summary_only`
- **WHEN** stage 2 selects candidates
- **THEN** traces whose current `engine_projection_state` is `summary_only`, `up_to_date`, or `catching_up` are transitioned to `journal_expired`
- **THEN** traces already at `journal_expired` are skipped by both stages

#### Scenario: Stage 2 skips stage 1 when run is past history window
- **WHEN** a terminal run's completion age already exceeds `ENGINE_HISTORY_RETENTION_AFTER`
- **THEN** stage 2 applies the `full` purge directly
- **THEN** stage 1 does NOT need to have run on that specific run first

---

### Requirement: Retention transitions are idempotent across crash/restart

The retention worker MUST produce the same end state if it is interrupted mid-iteration and restarted.

#### Scenario: Crash before purge service commit
- **WHEN** retention is interrupted before a candidate purge commits
- **THEN** the trace remains in its original projection state or the transaction rolls back cleanly
- **THEN** the next iteration reselects the candidate and retries

#### Scenario: Crash after commit
- **WHEN** retention committed a barrier flip and its row deletions
- **THEN** the next iteration sees the trace in the new state
- **THEN** the next iteration does not re-delete or re-flip

#### Scenario: Repeated iterations converge
- **WHEN** retention runs N times with no new terminal runs
- **THEN** the final state after each iteration is identical
- **THEN** stable traces are skipped by candidate selection

---

### Requirement: Retention applies only to terminal runs

The retention worker MUST NOT purge a run whose status is non-terminal.

#### Scenario: Candidate filter
- **WHEN** retention selects candidates at either stage
- **THEN** only runs whose authoritative `engine.runs.status IN ('completed','failed','cancelled','terminated')` are considered
- **THEN** runs in `queued`, `running`, or `waiting` are never purged by retention

#### Scenario: Status check re-verified at purge time
- **WHEN** retention hands a candidate to the shared purge service
- **THEN** the purge service re-verifies the terminal-only gate inside its own transaction
- **THEN** a race where a run becomes non-terminal after selection is safely rejected

---

### Requirement: Purge execution uses the shared root-side service

The retention worker MUST NOT directly access `public.traces`, `public.spans`, or `public.span_events`. It MUST execute purges through the same shared root-side purge service used by the public API.

#### Scenario: Shared service invocation
- **WHEN** the retention worker needs to purge a candidate run
- **THEN** it calls the shared in-process purge service directly
- **THEN** it does NOT self-call the public HTTP endpoint
- **THEN** it does NOT duplicate purge SQL in a separate retention-only code path

#### Scenario: No cross-process callback bridge
- **WHEN** retention is wired up
- **THEN** no injected callback is passed from `cmd/continua` into the separate `continua-engine` binary
- **THEN** retention remains fully root-side and directly implementable against the current runtime split

#### Scenario: Shared service respects purge contract
- **WHEN** the shared service is invoked by retention
- **THEN** it executes the full purge contract (terminal-only gate, CAS barrier, row lock, detail deletion, optional history deletion)
- **THEN** the structured purge result still includes the post-purge projection state and a `deleted` flag for idempotency

---

### Requirement: Retention scheduling

The retention job MUST have a deterministic, non-interactive schedule.

#### Scenario: Fixed daily cadence with no boot scan
- **WHEN** the platform server starts
- **THEN** retention does NOT run immediately on boot
- **THEN** retention is triggered by a periodic River schedule once every 24 hours in this phase
- **THEN** `RunOnStart` is disabled

#### Scenario: Active-state uniqueness on periodic enqueue
- **WHEN** multiple River clients register the same periodic retention job
- **THEN** the job args use active-state uniqueness so only one active retention job is retained for a given cadence tick
- **THEN** duplicate enqueues are treated as best-effort collapsible duplicates rather than separate intended work items

#### Scenario: Single-instance execution
- **WHEN** multiple platform instances run in parallel
- **THEN** retention is serialized so only one instance performs a given iteration at a time (via an advisory lock or equivalent)
- **THEN** iterations do not overlap to the point of double-purging

#### Scenario: Bounded batch size per iteration
- **WHEN** retention runs an iteration
- **THEN** each stage processes a bounded batch of candidates
- **THEN** over-large backlogs drain across multiple iterations without blocking the platform
