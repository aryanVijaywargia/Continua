## ADDED Requirements

### Requirement: Event Convention Documentation
A `docs/event-conventions.md` document SHALL provide usage-oriented guidance for all supported event types. The document SHALL include a reference table of all 8 explicit event types with expected payload fields and default levels, but its primary focus SHALL be guiding developers on when to use each event type — especially when to emit `state_change` vs `decision` vs `custom`. SDK usage examples SHALL demonstrate real-world patterns. The document SHALL explicitly state: "These are debugger semantics, not replay primitives."

#### Scenario: Usage guidance for event type selection
- **WHEN** a developer reads `docs/event-conventions.md`
- **THEN** they understand when to use `state_change` (observable state transitions), `decision` (branch points with reasoning), and `custom` (everything else)

#### Scenario: SDK usage examples
- **WHEN** a developer reads the state_change section
- **THEN** they find Python SDK examples showing `span.state_change()` in a realistic context

#### Scenario: Scope guard present
- **WHEN** a developer reads the document
- **THEN** they find a clear statement that these are debugger/observability semantics and not replay or state-machine primitives

### Requirement: Python SDK Docstring Enhancement
The Python SDK `Span` class methods for `state_change()` and `decision()` SHALL include docstrings with parameter descriptions, return type, and usage examples.

#### Scenario: state_change docstring
- **WHEN** a developer inspects `span.state_change` via help() or IDE
- **THEN** they see parameter descriptions for `key`, `old_value`, `new_value`, `namespace`, and `message`

#### Scenario: decision docstring
- **WHEN** a developer inspects `span.decision` via help() or IDE
- **THEN** they see parameter descriptions for `question`, `chosen`, `alternatives`, `reasoning`, and `message`
