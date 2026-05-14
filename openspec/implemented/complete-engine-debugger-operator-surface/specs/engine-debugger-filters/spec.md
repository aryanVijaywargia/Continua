## ADDED Requirements

### Requirement: URL-driven engine trace filters
The traces page SHALL support four URL-driven engine filter parameters: `engine_instance_key` (text input), `engine_definition_name` (text input), `engine_run_status` (select with human-readable labels), and `engine_projection_state` (select from EngineProjectionState values). These filters SHALL be sent as query parameters to `GET /api/traces` and additive with existing filters.

The allowed `engine_run_status` query values and `engine_projection_state` query values are owned by the backend `engine-trace-search` capability and the OpenAPI contract (`contracts/openapi/openapi.yaml`). This spec does not redefine them. The frontend SHALL derive its select options from the current OpenAPI enum values (at time of writing: `queued`, `running`, `waiting`, `suspended`, `completed`, `failed`, `cancelled`, `terminated`, `continued_as_new` for run status; `up_to_date`, `catching_up`, `summary_only`, `journal_expired` for projection state). If the backend adds or removes values, the frontend select options update accordingly.

#### Scenario: Engine filters are applied from URL on page load
- **WHEN** the traces page loads with `?engine_run_status=running&engine_definition_name=my-workflow` in the URL
- **THEN** the engine filter inputs display the corresponding human-readable labels and the trace list fetches with those lowercase query parameters

#### Scenario: Engine filter changes update the URL
- **WHEN** a user sets `engine_instance_key` to `order-123`
- **THEN** the URL is updated to include `engine_instance_key=order-123` and the trace list refetches

### Requirement: Engine filters in collapsible section
Engine filters SHALL be rendered in a collapsible "Engine filters" section beneath the existing filter grid. The section SHALL be collapsed by default and auto-expand when any engine filter parameter is present in the URL.

#### Scenario: Engine filters section auto-expands with active filters
- **WHEN** the page loads with `?engine_projection_state=summary_only`
- **THEN** the Engine filters section is expanded and the `engine_projection_state` select shows "summary_only"

#### Scenario: Engine filters section is collapsed by default
- **WHEN** the traces page loads with no engine filter parameters in the URL
- **THEN** the Engine filters section is collapsed

### Requirement: Engine projection state helper text
The `engine_projection_state` filter control SHALL display helper text identifying it as an operator-oriented/advanced filter useful for inspecting projection health across engine traces.

#### Scenario: Projection state filter shows helper text
- **WHEN** the Engine filters section is expanded
- **THEN** the `engine_projection_state` select displays helper text explaining its operator-focused purpose

### Requirement: Engine filter chip behavior
Active engine filters SHALL produce removable chips in the existing filter chip row. Removing a chip SHALL clear the corresponding URL parameter and refetch the trace list.

#### Scenario: Engine filter chip is displayed and removable
- **WHEN** `engine_run_status=waiting` is active
- **THEN** a chip showing the human-readable label (e.g., "Engine status: Waiting") appears in the filter chip row
- **AND** clicking the chip's remove button clears `engine_run_status` from the URL and refetches

### Requirement: Engine filters preserve existing filter behavior
Engine filter URL serialization, hydration, canonical query-string construction, additive updates, and browser back/forward support SHALL follow the same patterns as existing trace filters (`status`, `session_id`, `user_id`, etc.).

#### Scenario: Back/forward navigation restores engine filters
- **WHEN** a user sets `engine_definition_name=payments`, then navigates to another page, then presses browser back
- **THEN** the traces page restores `engine_definition_name=payments` and displays the correct filter state
