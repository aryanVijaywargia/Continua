## ADDED Requirements

### Requirement: Unresolved Wait Rows in Running State Panel
When a running trace has one or more unresolved (open) waits, the `RunningStatePanel` SHALL display individual unresolved-wait rows beneath the existing summary content. The existing summary behavior (classification label, basis, explanatory copy, timing fields, current-wait field, and decisive jump action) SHALL remain unchanged. Unresolved waits SHALL be derived once via `useMemo` from the existing `events` array using `computeOpenWaits()` in the trace-detail page layer, then passed into `RunningStatePanel`.

#### Scenario: Single unresolved wait displayed
- **WHEN** a running trace has one open wait event with `wait_kind: "human_approval"`
- **THEN** the `RunningStatePanel` shows the existing summary content
- **AND** an unresolved-wait row appears beneath with title "Approval gate"

#### Scenario: No unresolved waits
- **WHEN** a running trace has no open wait events
- **THEN** the `RunningStatePanel` shows only the existing summary content without any wait rows

#### Scenario: Non-running trace
- **WHEN** a trace has terminal status (COMPLETED, FAILED)
- **THEN** no `RunningStatePanel` or unresolved-wait rows are displayed

### Requirement: Unresolved Wait Row Content
Each unresolved-wait row SHALL display: a title ("Approval gate" when `wait_kind === "human_approval"`, otherwise "Wait gate"), the `wait_kind` value, the entered timestamp, the open duration, an originating span jump action when span context is available, the `wait_id` when present, and the original event message when present.

#### Scenario: Human approval wait row content
- **WHEN** an unresolved wait has `wait_kind: "human_approval"`, `wait_id: "approval-123"`, and a message "Awaiting manager sign-off"
- **THEN** the row shows title "Approval gate", kind "human_approval", the entered timestamp, open duration, wait_id "approval-123", and message "Awaiting manager sign-off"

#### Scenario: Generic wait row content
- **WHEN** an unresolved wait has `wait_kind: "external_api"` and no `wait_id`
- **THEN** the row shows title "Wait gate", kind "external_api", entered timestamp, and open duration
- **AND** no wait_id is displayed

#### Scenario: Wait row with span context
- **WHEN** an unresolved wait has an associated span via `span_id` in the event
- **THEN** the row includes a jump action to navigate to the originating span

### Requirement: Unresolved Wait Ordering and Lifecycle
Unresolved-wait rows SHALL be rendered newest-first so that the first row represents the current gate. When a matching `resolved` wait event arrives on the next poll cycle, the corresponding unresolved-wait row SHALL disappear. Anonymous waits without `wait_id` SHALL remain visible as standalone open-wait rows and SHALL NOT be auto-paired with resolved events. Open durations SHALL refresh only on the existing running-trace poll cadence; no separate interval timer SHALL be introduced.

#### Scenario: Multiple unresolved waits ordered newest-first
- **WHEN** a running trace has three unresolved waits entered at t=1, t=5, and t=10
- **THEN** the wait rows appear in order: t=10 (first/top), t=5, t=1 (last/bottom)

#### Scenario: Resolved wait disappears on poll
- **WHEN** a wait with `wait_id: "w1"` is currently shown as unresolved
- **AND** a new poll returns a `resolved` event matching `wait_id: "w1"`
- **THEN** the "w1" unresolved-wait row is no longer displayed

#### Scenario: Anonymous wait remains standalone
- **WHEN** a wait event has `phase: "entered"` but no `wait_id`
- **THEN** the wait appears as a standalone unresolved-wait row
- **AND** it is never auto-paired with any resolved event

#### Scenario: Open duration refreshes on poll only
- **WHEN** the running-trace poll cadence fires and re-renders the panel
- **THEN** open durations update to reflect current time
- **AND** no separate `setInterval` or `requestAnimationFrame` timer is used for duration updates
