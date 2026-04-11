## ADDED Requirements

### Requirement: Engine control actions in trace detail
The trace detail page SHALL display engine control action buttons for signal, cancel, suspend, resume, terminate, purge, and repair when the trace has engine metadata (`trace.engine` is present). Control actions SHALL NOT be rendered for non-engine traces.

#### Scenario: Control actions appear for engine-backed traces
- **WHEN** a trace detail loads with `trace.engine` present
- **THEN** engine control action buttons are rendered

#### Scenario: Control actions are absent for non-engine traces
- **WHEN** a trace detail loads without `trace.engine`
- **THEN** no engine control action buttons are rendered

### Requirement: Engine control mutation preview header
All engine control mutation API wrappers (signal, cancel, suspend, resume, terminate, purge, repair) SHALL send the `X-Continua-Engine-Preview: 1` header. The pending-work read endpoint does not require this header.

#### Scenario: Signal mutation sends preview header
- **WHEN** the signal API wrapper is called
- **THEN** the request includes the `X-Continua-Engine-Preview: 1` header

#### Scenario: Pending-work read does not send preview header
- **WHEN** the pending-work fetch is called
- **THEN** the request does not include the preview header

### Requirement: Engine control action state gating
Each control action SHALL be enabled or disabled based on the current engine run status:
- `signal` and `cancel`: enabled while the run is non-terminal, including `SUSPENDED`
- `suspend`: enabled only for `QUEUED`, `RUNNING`, or `WAITING`
- `resume`: enabled only for `SUSPENDED`
- `terminate`: enabled for any non-terminal run including `SUSPENDED`
- `purge`: enabled only for terminal runs (`COMPLETED`, `FAILED`, `CANCELLED`, `TERMINATED`, `CONTINUED_AS_NEW`)
- `repair`: enabled for any engine-backed trace regardless of status

#### Scenario: Suspend is disabled for a COMPLETED run
- **WHEN** the engine run status is `COMPLETED`
- **THEN** the suspend button is disabled

#### Scenario: Resume is enabled only for SUSPENDED
- **WHEN** the engine run status is `SUSPENDED`
- **THEN** the resume button is enabled

#### Scenario: Purge is disabled for non-terminal runs
- **WHEN** the engine run status is `RUNNING`
- **THEN** the purge button is disabled

#### Scenario: Purge is enabled for CONTINUED_AS_NEW
- **WHEN** the engine run status is `CONTINUED_AS_NEW`
- **THEN** the purge button is enabled

#### Scenario: Signal is enabled for SUSPENDED runs
- **WHEN** the engine run status is `SUSPENDED`
- **THEN** the signal button is enabled

### Requirement: Signal modal with validation
The signal action SHALL open a modal with a required `signal_name` text input that MUST be non-empty after trimming whitespace, and an optional `payload` text area. If a payload is provided, it MUST be valid JSON; invalid JSON SHALL prevent submission.

#### Scenario: Signal modal validates JSON payload
- **WHEN** a user enters `signal_name=approve` and payload `{invalid`
- **THEN** the submit button is disabled and a validation error is shown

#### Scenario: Signal modal allows empty payload
- **WHEN** a user enters `signal_name=approve` with no payload
- **THEN** the submit button is enabled

#### Scenario: Signal modal rejects whitespace-only signal name
- **WHEN** a user enters `signal_name` as `   ` (whitespace only)
- **THEN** the submit button is disabled and a validation error is shown

### Requirement: Confirmation for destructive control actions
`cancel`, `terminate`, and `purge` SHALL require user confirmation before execution. `repair` SHALL NOT require confirmation; it sends the request immediately and displays informational feedback from the response.

#### Scenario: Cancel requires confirmation
- **WHEN** a user clicks the cancel button
- **THEN** a confirmation dialog appears before the cancel request is sent

#### Scenario: Repair does not require confirmation
- **WHEN** a user clicks the repair button
- **THEN** the repair request is sent immediately without a confirmation dialog

### Requirement: Purge mode selection
The purge confirmation dialog SHALL include a mode selection between `projection_only` (default) and `full`. The `full` mode SHALL be presented with a destructive warning because it purges retained engine history that cannot be recovered. The selected mode SHALL be sent as the `mode` field in the purge request body.

#### Scenario: Purge defaults to projection_only mode
- **WHEN** the purge confirmation dialog opens
- **THEN** the `projection_only` mode is selected by default

#### Scenario: Full purge mode shows destructive warning
- **WHEN** a user selects `full` purge mode
- **THEN** a destructive warning is displayed indicating retained engine history will be permanently deleted

### Requirement: Purge feedback
After a purge mutation, the UI SHALL display inline feedback in the control area using existing alert/status styling: `deleted=true` indicates the purge was applied; `deleted=false` indicates the target was already in the requested state. No global toast or notification system SHALL be introduced.

#### Scenario: Purge shows applied feedback
- **WHEN** the purge response returns `deleted=true`
- **THEN** an inline success message in the control area indicates the purge was applied

#### Scenario: Purge shows already-satisfied feedback
- **WHEN** the purge response returns `deleted=false`
- **THEN** an inline informational message in the control area indicates the purge was already satisfied

### Requirement: Repair reason feedback
After a repair mutation, the UI SHALL display inline feedback in the control area using existing alert/status styling based on the returned reason:
- `repair_requested`: success message
- `already_catching_up`: informational message
- `already_up_to_date`: informational message
- `no_events_to_project`: informational message
- `history_expired`: warning message

No global toast or notification system SHALL be introduced.

#### Scenario: Repair shows success feedback
- **WHEN** the repair response returns reason `repair_requested`
- **THEN** an inline success message in the control area indicates repair was requested

#### Scenario: Repair shows history_expired warning
- **WHEN** the repair response returns reason `history_expired`
- **THEN** an inline warning message in the control area indicates the event journal has expired and repair is not possible

### Requirement: In-flight mutation pending state
While a control mutation request is in flight, the active action button (and modal submit button, if applicable) SHALL be disabled to prevent double-submission. The control area SHALL show a submitting indicator. This is especially important for `signal`, where duplicate sends are semantically meaningful.

#### Scenario: Signal button disabled during in-flight request
- **WHEN** a signal mutation is in flight
- **THEN** the signal modal submit button is disabled and a submitting indicator is shown

#### Scenario: Purge button disabled during in-flight request
- **WHEN** a purge mutation is in flight
- **THEN** the purge button is disabled until the response arrives

### Requirement: Single-slot inline feedback model
The control area SHALL maintain a single feedback slot. Every mutation result -- whether success, informational, warning, or error -- SHALL replace whatever message currently occupies the slot. There is no stacking, accumulation, or auto-dismiss timer. The slot is cleared only by a new mutation result or by the user navigating away.

#### Scenario: Success replaces previous error
- **WHEN** a signal mutation failed (showing an error) and a subsequent retry succeeds
- **THEN** the success message replaces the previous error message

#### Scenario: Error replaces previous success
- **WHEN** a repair succeeded (showing success feedback) and a subsequent signal mutation fails
- **THEN** the signal error message replaces the previous repair success message

#### Scenario: Informational replaces previous warning
- **WHEN** a repair returned `history_expired` (showing a warning) and a subsequent repair returns `already_up_to_date`
- **THEN** the informational message replaces the previous warning

### Requirement: Mutation failure handling
When a control mutation fails, the control area SHALL display an inline error message in the single feedback slot. The backend returns typed `409` conflict errors for normal race conditions (`run_terminal`, `run_not_terminal`, `run_not_suspendable`). On a `409` conflict, the debugger SHALL refetch the trace detail and pending-work queries so the control bar reflects the current run state.

#### Scenario: 409 conflict shows inline error and refetches state
- **WHEN** a suspend mutation returns `409` with code `run_not_suspendable`
- **THEN** an inline error message is displayed in the control area and the trace detail and pending-work queries are refetched

#### Scenario: Network error shows inline error
- **WHEN** a terminate mutation fails due to a network error
- **THEN** an inline error message is displayed in the control area

### Requirement: Post-mutation query invalidation
After any successful engine control mutation (signal, cancel, suspend, resume, terminate, purge, repair), the debugger SHALL invalidate and refetch the trace detail, timeline events, spans, pending-work, and trace-list TanStack Query caches.

#### Scenario: Signal success triggers cache invalidation
- **WHEN** a signal mutation succeeds
- **THEN** TanStack Query invalidates and refetches trace detail, timeline, spans, pending-work, and trace-list queries
