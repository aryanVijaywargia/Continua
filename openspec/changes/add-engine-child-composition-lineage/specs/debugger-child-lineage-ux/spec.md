## ADDED Requirements

### Requirement: Lineage Data Fetching Contract
The platform database columns and trace list query parameters MUST keep the existing `engine_` prefix (`engine_parent_run_id`, `engine_root_run_id`, `engine_child_key`, `engine_child_depth`). JSON API responses MUST expose lineage inside the existing nested `engine` object instead: `engine.parent_run_id`, `engine.root_run_id`, `engine.child_key`, and `engine.child_depth`.

Both trace list rows (`Trace.engine`) and trace detail responses (`TraceDetail.engine`) MUST include these nested lineage fields for engine traces. Root-level engine traces MUST use `null` for `engine.parent_run_id` and `engine.child_key`, set `engine.root_run_id` to their own run ID, and set `engine.child_depth` to `0`.

The UI MUST fetch direct children using `GET /api/traces?engine_parent_run_id={currentRunId}` (project-scoped). The UI MUST fetch ancestor breadcrumb data either by following `engine.parent_run_id` links from the current trace up to the root with `GET /api/traces?engine_run_id={parentRunId}`, or by fetching all traces under `GET /api/traces?engine_root_run_id={rootRunId}` and building the ancestor chain client-side. If the root-subtree strategy is used, the UI MUST page through the full result set before building the breadcrumb chain. The implementation MAY choose either strategy; the lineage filter API only requires exact-match filters and does not require a depth range expression.

No dedicated lineage endpoint is required. The existing trace detail and trace list filter APIs provide all necessary data.

#### Scenario: Trace detail includes lineage fields
- **WHEN** the UI fetches `GET /api/traces/{id}` for a child engine trace
- **THEN** the response includes `engine.parent_run_id`, `engine.root_run_id`, `engine.child_key`, and `engine.child_depth`

#### Scenario: UI fetches direct children via filter
- **WHEN** the UI needs to display children for trace with run ID R
- **THEN** it queries `GET /api/traces?engine_parent_run_id={R}`
- **AND** receives only direct children of that run

#### Scenario: UI fetches breadcrumb ancestor via parent run ID
- **WHEN** the UI needs a breadcrumb ancestor whose engine run ID is P
- **THEN** it queries `GET /api/traces?engine_run_id={P}`
- **AND** uses the returned trace ID to link to the ancestor trace detail page

### Requirement: Desktop Lineage Breadcrumb
The trace detail page MUST display a lineage breadcrumb under the trace header for engine traces that have a parent. The breadcrumb MUST show the ancestry chain from root to current trace. Each ancestor in the breadcrumb MUST be a clickable link navigating to that ancestor's trace detail page.

Root-level engine traces (no parent) MUST NOT display a lineage breadcrumb.

#### Scenario: Child trace shows breadcrumb
- **WHEN** a user views a child trace at depth 2
- **THEN** a breadcrumb appears under the trace header showing: root trace name > parent trace name > current trace name
- **AND** clicking the root or parent name navigates to `/traces/{ancestorTraceId}`

#### Scenario: Root trace has no breadcrumb
- **WHEN** a user views a root-level engine trace
- **THEN** no lineage breadcrumb is displayed

### Requirement: Direct Children In Context Drawer
The existing trace context drawer MUST display a list of direct child workflows for engine traces that have children. Each child row MUST show: child key, definition name, definition version, run status, and a trace link.

Clicking a child row MUST navigate to `/traces/{childTraceId}`.

If the trace has no children, the children section MUST NOT appear.

#### Scenario: Parent trace lists children
- **WHEN** a user views a parent trace that has spawned two child workflows
- **THEN** the context drawer shows two child rows
- **AND** each row displays child key, definition name, definition version, and run status
- **AND** clicking a child row navigates to the child's trace detail

#### Scenario: Trace with no children
- **WHEN** a user views an engine trace with no children
- **THEN** no children section appears in the context drawer

### Requirement: Mobile Lineage Summary
On mobile viewports, the lineage summary (breadcrumb and children list) MUST be placed in the existing Summary tab rather than in a separate drawer or header section.

The summary MUST include the same information as the desktop breadcrumb and children list but adapted for the mobile layout.

#### Scenario: Mobile child navigation
- **WHEN** a user views a child trace on a mobile viewport
- **THEN** the Summary tab contains the lineage breadcrumb and any direct children
- **AND** tapping a breadcrumb ancestor or child row navigates to the corresponding trace

### Requirement: UX Design Gate
Before any frontend code is written for Milestone 1B, a `design.md` UX section MUST be added with:
- A text wireframe showing breadcrumb placement on desktop
- A text wireframe showing children rows in the context drawer
- Acceptance criteria for navigation targets
- Mobile placement specification

#### Scenario: Design document exists before implementation
- **WHEN** a developer begins frontend work for lineage UX
- **THEN** a `design.md` UX section exists with text wireframes and acceptance criteria
