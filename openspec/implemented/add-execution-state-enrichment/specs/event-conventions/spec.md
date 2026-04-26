## MODIFIED Requirements

### Requirement: Event Convention Documentation
A `docs/event-conventions.md` document SHALL provide usage-oriented guidance for all supported event types. The document SHALL include a reference table of all explicit event types (including `snapshot_marker`) with expected payload fields and default levels, but its primary focus SHALL be guiding developers on when to use each event type. SDK usage examples SHALL demonstrate real-world patterns. The document SHALL explicitly state: "These are debugger semantics, not replay primitives." The `snapshot_marker` entry SHALL document:
- `marker_kind`: conventionally required non-empty string with documented default vocabulary `milestone`
- `label`: conventionally required human-facing marker label
- That `snapshot_marker` is a debugger milestone event, not a checkpoint or resumability primitive

#### Scenario: Usage guidance for event type selection
- **WHEN** a developer reads `docs/event-conventions.md`
- **THEN** they understand when to use `state_change`, `decision`, `custom`, and `snapshot_marker`

#### Scenario: Snapshot marker documentation
- **WHEN** a developer reads the snapshot_marker section
- **THEN** they find documented payload conventions for `marker_kind` and `label`
- **AND** a clear statement that markers are debugger milestones, not checkpoint/resume primitives

#### Scenario: SDK usage examples
- **WHEN** a developer reads the snapshot_marker section
- **THEN** they find Python SDK examples showing `span.snapshot_marker()` in a realistic context

#### Scenario: Scope guard present
- **WHEN** a developer reads the document
- **THEN** they find a clear statement that these are debugger/observability semantics and not replay or state-machine primitives
