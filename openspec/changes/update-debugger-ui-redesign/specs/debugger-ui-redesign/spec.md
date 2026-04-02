## ADDED Requirements

### Requirement: Shared Operator App Shell

The web debugger SHALL provide a shared application shell with stable navigation and route-aware utilities for the main product routes.

#### Scenario: Desktop shell
- **WHEN** the app renders on desktop-sized viewports
- **THEN** a left navigation rail is visible with entries for `Overview`, `Traces`, `Sessions`, and `Settings`
- **AND** a compact top utility bar exposes the command palette, theme control, and API-key state

#### Scenario: Mobile shell
- **WHEN** the app renders on narrow viewports
- **THEN** navigation is accessible via a compact top bar and mobile drawer
- **AND** the same route set remains reachable

### Requirement: Overview Route

The `/` route SHALL render a real overview screen using existing trace and session endpoints only.

#### Scenario: Overview shows operator snapshot data
- **WHEN** the overview route loads successfully
- **THEN** it shows snapshot metrics and direct investigation entry points for recent failed traces, running traces, recent traces, and recent sessions

#### Scenario: Overview remains frontend-only
- **WHEN** the overview route is implemented
- **THEN** it does not require new REST endpoints, database queries, or backend analytics APIs

### Requirement: Traces and Sessions Triage Surfaces

The traces and sessions routes SHALL support denser operator triage without regressing existing URL-backed list behavior.

#### Scenario: Traces page preserves URL-driven filters
- **WHEN** a user loads or updates `/traces` with existing supported search params
- **THEN** the redesigned page rehydrates those filters and issues the same API requests as before
- **AND** the visual layout emphasizes trace name and status before secondary metrics

#### Scenario: Sessions page preserves URL-driven search, sort, and pagination
- **WHEN** a user loads or updates `/sessions` with existing supported search params
- **THEN** the redesigned page rehydrates those params and preserves existing search/sort/pagination semantics
- **AND** the visual layout emphasizes external session identity before secondary metadata

### Requirement: Session Investigation Workspace

The session detail and compare routes SHALL present a clearer investigation workspace without changing compare URL behavior.

#### Scenario: Session detail keeps compare selection state
- **WHEN** a user selects baseline and candidate traces on session detail
- **THEN** the redesigned session detail page preserves the existing compare URL params and action flow

#### Scenario: Session compare improves top-level scanability
- **WHEN** a user opens `/sessions/{id}/compare`
- **THEN** the page presents persistent baseline/candidate context and clear diff grouping before row-level inspection

### Requirement: Trace Investigation Workspace

The trace detail route SHALL preserve the current polling and selection model while improving workspace ergonomics.

#### Scenario: Desktop trace context uses a drawer
- **WHEN** a user opens trace detail on desktop
- **THEN** trace context is available via a toggleable side drawer rather than occupying prime workspace space by default

#### Scenario: Mobile trace detail uses four top-level tabs
- **WHEN** a user opens trace detail on a narrow viewport
- **THEN** the top-level tabs are `Summary`, `Execution`, `Timeline`, and `State`
- **AND** the execution tab provides access to both tree and waterfall views

#### Scenario: Tree rail quick filters are local-only
- **WHEN** a user uses trace detail tree-rail quick filters
- **THEN** those filters operate only on already loaded span data
- **AND** they do not add new public URL params or require new server requests
