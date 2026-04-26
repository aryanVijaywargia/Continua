## ADDED Requirements

### Requirement: Python SDK Snapshot Marker Helper
The Python SDK `Span` class SHALL provide a `snapshot_marker(label, *, marker_kind="milestone", payload=None, message=None)` method that records an event with `event_type: "snapshot_marker"`. The helper SHALL reject empty `label` (raising `ValueError`). The helper SHALL reject empty `marker_kind` (raising `ValueError`). When `message` is omitted or `None`, the helper SHALL default `message` to the `label` value. Helper-owned payload keys (`marker_kind`, `label`) SHALL be written last so they override any conflicting keys in the caller-provided `payload` dict.

#### Scenario: Record a snapshot marker via SDK
- **WHEN** a user calls `span.snapshot_marker("Data loaded")`
- **THEN** an event is recorded with `event_type: "snapshot_marker"`, `level: "info"`, and payload `{"marker_kind": "milestone", "label": "Data loaded"}`
- **AND** the event `message` is `"Data loaded"`

#### Scenario: Record a marker with custom kind
- **WHEN** a user calls `span.snapshot_marker("Phase 2 complete", marker_kind="phase")`
- **THEN** the payload includes `{"marker_kind": "phase", "label": "Phase 2 complete"}`

#### Scenario: Record a marker with explicit message
- **WHEN** a user calls `span.snapshot_marker("Validation passed", message="All input schemas validated successfully")`
- **THEN** the event message is `"All input schemas validated successfully"`, not `"Validation passed"`

#### Scenario: Record a marker with caller payload
- **WHEN** a user calls `span.snapshot_marker("Done", payload={"extra": "data", "marker_kind": "wrong"})`
- **THEN** the payload contains `{"extra": "data", "marker_kind": "milestone", "label": "Done"}`
- **AND** the helper-owned `marker_kind` overrides the caller's conflicting value

#### Scenario: Reject empty label
- **WHEN** a user calls `span.snapshot_marker("")`
- **THEN** a `ValueError` is raised

#### Scenario: Reject empty marker_kind
- **WHEN** a user calls `span.snapshot_marker("Test", marker_kind="")`
- **THEN** a `ValueError` is raised

### Requirement: Python SDK Snapshot Marker Docstring
The Python SDK `Span.snapshot_marker()` method SHALL include a docstring with parameter descriptions, return type, and a usage example.

#### Scenario: Docstring content
- **WHEN** a developer inspects `span.snapshot_marker` via `help()` or IDE
- **THEN** they see parameter descriptions for `label`, `marker_kind`, `payload`, and `message`
