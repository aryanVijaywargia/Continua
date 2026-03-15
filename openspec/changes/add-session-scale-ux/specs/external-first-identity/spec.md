## ADDED Requirements

### Requirement: External-First Session Identity
Session identity SHALL be displayed external-first throughout the debugger. External ID SHALL be the primary clickable label and UUID SHALL be secondary muted text. This applies to the sessions list, session detail header, trace list session links, and trace detail session field.

#### Scenario: Sessions list shows external ID as primary label
- **WHEN** the sessions list renders a session row
- **THEN** the external ID is the primary clickable label linking to session detail
- **AND** the UUID is displayed as secondary muted text

#### Scenario: Session detail header shows external ID first
- **WHEN** the session detail page renders
- **THEN** the header displays external ID first and UUID second
- **AND** both are copyable

#### Scenario: Trace list shows session external ID
- **WHEN** the trace list renders a trace that belongs to a session
- **THEN** the session link shows the external ID as primary text
- **AND** the UUID is available as secondary support text

#### Scenario: Trace detail shows session external ID
- **WHEN** the trace detail page renders for a trace with a session
- **THEN** the session field shows external ID first
- **AND** the UUID is available for copy/debugging
