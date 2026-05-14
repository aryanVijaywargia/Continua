## ADDED Requirements

### Requirement: Projected Trace Lineage Columns
The platform traces table MUST include `engine_parent_run_id UUID`, `engine_root_run_id UUID`, `engine_child_key TEXT`, and `engine_child_depth INTEGER` columns.

For non-engine traces, all four columns MUST be NULL. For root-level engine traces (no parent), `engine_parent_run_id` and `engine_child_key` MUST be NULL, `engine_root_run_id` MUST equal the run's own ID, and `engine_child_depth` MUST be 0. For child engine traces, all four columns MUST be populated.

The projector MUST populate these columns when creating projected traces, reading lineage values from `engine.child_workflows` and `engine.runs`.

#### Scenario: Child trace has lineage columns populated
- **WHEN** a child workflow run is projected to a trace
- **THEN** `engine_parent_run_id` equals the parent's run ID
- **AND** `engine_root_run_id` equals the root ancestor's run ID
- **AND** `engine_child_key` equals the child key used in the parent
- **AND** `engine_child_depth` equals the child's nesting depth

#### Scenario: Root-level engine trace has partial lineage
- **WHEN** a root-level engine run (no parent) is projected to a trace
- **THEN** `engine_parent_run_id` is NULL
- **AND** `engine_root_run_id` equals the run's own ID
- **AND** `engine_child_key` is NULL
- **AND** `engine_child_depth` is 0

### Requirement: Lineage Trace Filters
The traces API MUST support the following new filter parameters in addition to existing filters (`engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`):
- `engine_run_id`: exact match on engine run UUID
- `engine_definition_version`: exact match on definition version
- `engine_parent_run_id`: exact match on parent run UUID
- `engine_root_run_id`: exact match on root run UUID
- `engine_child_key`: exact match on child key
- `engine_child_depth`: exact match on nesting depth (integer)

All filters MUST be project-scoped and composable with existing filters.

#### Scenario: Filter by parent run ID
- **WHEN** a client queries `GET /api/traces?engine_parent_run_id={parentRunId}`
- **THEN** only traces whose `engine_parent_run_id` matches are returned
- **AND** results are scoped to the authenticated project

#### Scenario: Filter by engine run ID
- **WHEN** a client queries `GET /api/traces?engine_run_id={runId}`
- **THEN** only traces whose `engine_run_id` matches are returned
- **AND** results are scoped to the authenticated project

#### Scenario: Filter by root run ID
- **WHEN** a client queries `GET /api/traces?engine_root_run_id={rootRunId}`
- **THEN** all traces in the lineage tree under that root are returned

#### Scenario: Filter by definition version
- **WHEN** a client queries `GET /api/traces?engine_definition_version=v2`
- **THEN** only traces whose engine definition version matches `v2` are returned

#### Scenario: Compose lineage filter with existing filters
- **WHEN** a client queries with both `engine_root_run_id` and `engine_run_status=failed`
- **THEN** only failed traces within that lineage tree are returned

### Requirement: Lineage Projection Repair And Backfill
The existing projection repair and backfill mechanisms MUST be extended to populate lineage columns from `engine.child_workflows` and `engine.runs` state.

When a trace is repaired and its engine run has lineage data, the repair MUST set all four lineage columns. Existing root-level engine traces MUST be repaired/backfilled with `engine_root_run_id` equal to their own engine run ID and `engine_child_depth = 0`; root traces MUST NOT retain NULL root/depth lineage after repair.

#### Scenario: Repair populates lineage for child trace
- **WHEN** a child trace in `summary_only` state is repaired
- **THEN** the repaired trace has correct `engine_parent_run_id`, `engine_root_run_id`, `engine_child_key`, and `engine_child_depth` values

#### Scenario: Repair populates root lineage for root trace
- **WHEN** a root-level engine trace in `summary_only` state is repaired
- **THEN** `engine_root_run_id` is set to the trace's own engine run ID
- **AND** `engine_child_depth` is set to 0
- **AND** `engine_parent_run_id` and `engine_child_key` remain NULL

#### Scenario: Backfill populates lineage for existing child traces
- **WHEN** bulk backfill runs after the lineage migration
- **THEN** all child traces without lineage columns populated have their lineage columns filled from engine state

#### Scenario: Backfill populates root lineage for existing root traces
- **WHEN** bulk backfill runs after the lineage migration
- **AND** an existing root-level engine trace has NULL lineage columns
- **THEN** `engine_root_run_id` is set to the trace's own engine run ID
- **AND** `engine_child_depth` is set to 0
- **AND** `engine_parent_run_id` and `engine_child_key` remain NULL
