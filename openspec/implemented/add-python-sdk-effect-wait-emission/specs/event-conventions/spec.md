## MODIFIED Requirements

### Requirement: Event Convention Documentation

A `docs/event-conventions.md` document SHALL provide usage-oriented guidance for all supported event types. The document SHALL include a reference table of all 10 explicit event types (`log`, `error`, `exception`, `message`, `metric`, `custom`, `state_change`, `decision`, `effect`, `wait`) with expected payload fields and default levels. The document SHALL guide developers on when to use each event type — including when to use `effect` (external side effects and model calls) and `wait` (human approval, external dependencies, timers). SDK usage examples SHALL demonstrate real-world patterns including implicit effect emission via `set_llm_response()` and `set_tool_call()`, and explicit wait enter/resolve patterns via `span.wait()`. The document SHALL state that `message` and `custom` remain supported event types without dedicated Python helpers in this phase. The document SHALL explicitly state: "These are debugger semantics, not replay primitives."

#### Scenario: Usage guidance for event type selection
- **WHEN** a developer reads `docs/event-conventions.md`
- **THEN** they understand when to use `state_change` (observable state transitions), `decision` (branch points with reasoning), `effect` (external side effects), `wait` (blocking on external resolution), and `custom` (everything else)

#### Scenario: SDK usage examples
- **WHEN** a developer reads the state_change section
- **THEN** they find Python SDK examples showing `span.state_change()` in a realistic context

#### Scenario: Scope guard present
- **WHEN** a developer reads the document
- **THEN** they find a clear statement that these are debugger/observability semantics and not replay or state-machine primitives

#### Scenario: All 10 event types documented
- **WHEN** a developer reads `docs/event-conventions.md`
- **THEN** they find a reference entry for each of the 10 explicit event types with expected payload fields

#### Scenario: Implicit effect examples present
- **WHEN** a developer reads the effect section
- **THEN** they find Python SDK examples showing implicit emission via `set_llm_response()` and `set_tool_call()`

#### Scenario: Wait examples present
- **WHEN** a developer reads the wait section
- **THEN** they find Python SDK examples showing `span.wait()` with enter/resolve patterns

#### Scenario: Undocumented helper types acknowledged
- **WHEN** a developer reads the document
- **THEN** they find a statement that `message` and `custom` event types are supported but have no dedicated Python SDK helper in this phase
