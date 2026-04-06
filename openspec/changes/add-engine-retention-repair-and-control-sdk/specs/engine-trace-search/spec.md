# Capability: engine-trace-search

Additive engine filters on the existing trace list endpoint: `engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`. Wired into the existing handwritten dynamic SQL builder in `internal/store/search.go`.

Related capabilities: [engine-public-api](../engine-public-api/spec.md), [engine-trace-projection](../engine-trace-projection/spec.md)

## ADDED Requirements

### Requirement: Additive engine filters on trace list endpoint

The existing trace list endpoint (`GET /api/traces`) MUST accept four additive engine filters without changing any existing query-parameter behavior.

#### Scenario: Filter names and columns
- **WHEN** a trace list request includes engine filters
- **THEN** the supported filter names are `engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`
- **THEN** each filter is an equality predicate against the corresponding `public.traces` column
- **THEN** no new OR / IN / range semantics are introduced in this phase

#### Scenario: Filters are optional
- **WHEN** a trace list request does NOT include any engine filter
- **THEN** the endpoint returns the same result set it would have returned before this change
- **THEN** no engine predicate is appended to the query

#### Scenario: engine_run_status filter values
- **WHEN** `engine_run_status` is provided
- **THEN** allowed values match the existing `engine_run_status` CHECK constraint (`queued`, `running`, `waiting`, `completed`, `failed`, `cancelled`, `terminated`)
- **THEN** invalid values return 400 with a typed validation error

#### Scenario: engine_projection_state filter values
- **WHEN** `engine_projection_state` is provided
- **THEN** allowed values are `up_to_date`, `catching_up`, `summary_only`, `journal_expired`
- **THEN** invalid values return 400 with a typed validation error

#### Scenario: Out-of-scope filters are silently ignored
- **WHEN** a request includes `engine_definition_version`, `engine_wait_kind`, or an error-code filter
- **THEN** the server silently ignores the unrecognized parameters (standard REST behavior)
- **THEN** no 400 is returned for unknown query parameter names
- **THEN** the result set is based only on the four supported engine filters

---

### Requirement: Engine-filtered queries exclude non-engine traces naturally

Engine-filtered trace queries MUST rely on the fact that the filter columns are `NULL` for non-engine traces, so non-engine traces are naturally excluded by equality predicates.

#### Scenario: Equality predicate excludes NULLs
- **WHEN** a trace list request includes any engine filter
- **THEN** the generated SQL uses `column = $N` (not `IS NOT DISTINCT FROM`)
- **THEN** rows where the filter column is `NULL` are not matched
- **THEN** non-engine traces (with NULL engine columns) are excluded from the result set

#### Scenario: Mixed engine + non-engine result set when no filter
- **WHEN** a trace list request includes no engine filter
- **THEN** the result set contains both engine-linked and non-engine traces as it did before this change
- **THEN** no engine predicate is appended

#### Scenario: Combined filters AND together
- **WHEN** a request includes multiple engine filters
- **THEN** all provided filters are combined with AND
- **THEN** the result set is further narrowed by each additional predicate

---

### Requirement: Filter wiring preserves existing pagination and ordering

Engine filters MUST NOT alter the existing trace list pagination, ordering, or other query parameters.

#### Scenario: Pagination unaffected
- **WHEN** an engine filter is combined with `limit` / `cursor` / offset parameters
- **THEN** pagination behaves identically to the no-engine-filter case
- **THEN** cursor encoding / decoding is unchanged

#### Scenario: Ordering unaffected
- **WHEN** an engine filter is combined with existing sort parameters
- **THEN** the ordering is the same as it would be without the engine filter
- **THEN** the ORDER BY clause is unchanged by engine filters

#### Scenario: No new index required in this phase, but EXPLAIN verification is required
- **WHEN** the filters are applied
- **THEN** existing indexes on `public.traces` (including those added by engine trace-linkage migrations) are expected to be sufficient for the equality filters
- **THEN** if query plans regress, indexes MAY be added in a follow-up but are not required by this capability
- **THEN** EXPLAIN-based tests MUST verify that the four engine filter queries use index scans (or at worst index-condition pushdown) rather than sequential scans against the current index set. If EXPLAIN reveals sequential scans for any filter combination, the implementation SHOULD add a targeted index in this phase rather than deferring

---

### Requirement: Handwritten dynamic builder owns the new filters

The filter wiring MUST live in the existing handwritten SQL builder (`internal/store/search.go`) and MUST NOT introduce a new store method or endpoint.

#### Scenario: Single wiring point
- **WHEN** filter code is added
- **THEN** it is added to the existing dynamic builder's optional-predicate section
- **THEN** no new exported store method is added
- **THEN** no new `/api/*` endpoint is added for engine-filtered trace search

#### Scenario: Parameter binding
- **WHEN** a filter value is applied
- **THEN** it is passed as a bound parameter (positional placeholder)
- **THEN** values are never concatenated into SQL strings
- **THEN** enum values (`engine_run_status`, `engine_projection_state`) are validated against the allowed set before binding
