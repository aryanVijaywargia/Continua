## ADDED Requirements

### Requirement: Bulk backfill endpoint
The system SHALL provide `POST /v1/engine/projections/backfill` gated by the `X-Continua-Engine-Preview: 1` header. Requests without the preview header SHALL receive a 400 response.

#### Scenario: Request without preview header is rejected
- **WHEN** a POST is sent to `/v1/engine/projections/backfill` without the `X-Continua-Engine-Preview: 1` header
- **THEN** the response status is 400

#### Scenario: Request with preview header is accepted
- **WHEN** a POST is sent with a valid body and the `X-Continua-Engine-Preview: 1` header
- **THEN** the endpoint processes the request and returns a backfill response

### Requirement: Backfill request parameters
The request body SHALL accept: `dry_run` (boolean, defaults to false), `limit` (integer, defaults to 50, maximum 100), `older_than` (ISO 8601 timestamp), `engine_instance_key` (string), `engine_definition_name` (string), `engine_run_status` (EngineRunStatus), and `engine_projection_state` (EngineProjectionState). All fields are optional. Values of `limit` above 100 SHALL return a 400 response.

#### Scenario: Limit above 100 returns 400
- **WHEN** a backfill request has `limit=150`
- **THEN** the response status is 400 with an error indicating the maximum limit is 100

#### Scenario: Default limit is 50
- **WHEN** a backfill request omits `limit`
- **THEN** up to 50 eligible runs are processed

#### Scenario: Default dry_run is false
- **WHEN** a backfill request omits `dry_run`
- **THEN** the endpoint runs in apply mode

### Requirement: Default eligibility selection
If `engine_projection_state` is omitted from the request, the endpoint SHALL select only `summary_only` candidates whose trace has a retained engine run/history shell and whose latest retained engine history ID is greater than `engine_last_projected_history_id`.

#### Scenario: Omitted projection state selects summary_only candidates
- **WHEN** a backfill request omits `engine_projection_state` and 5 `summary_only` traces have retained history ahead of their projection checkpoint
- **THEN** those 5 traces are eligible

#### Scenario: Non-engine traces are excluded
- **WHEN** a backfill request omits `engine_projection_state` and no engine traces exist
- **THEN** the response has `eligible_count=0` and empty results

### Requirement: Incompatible projection state filters
If `engine_projection_state` is explicitly set to `up_to_date`, `catching_up`, or `journal_expired`, the endpoint SHALL return zero eligible rows. This behavior SHALL be documented in the API description.

#### Scenario: Explicit up_to_date filter returns zero results
- **WHEN** a backfill request has `engine_projection_state=up_to_date`
- **THEN** the response has `eligible_count=0` and empty `results`

#### Scenario: Explicit journal_expired filter returns zero results
- **WHEN** a backfill request has `engine_projection_state=journal_expired`
- **THEN** the response has `eligible_count=0` and empty `results`

### Requirement: Backfill response shape
The response SHALL include: `dry_run` (boolean), `limit` (integer), `eligible_count` (integer), `repair_requested_count` (integer), `skipped_count` (integer), and `results` (array). Each result SHALL include `run_id` (UUID), `trace_id` (string), `projection_state` (EngineProjectionState), `action` (EngineProjectionBackfillAction), and optional `reason` (string).

#### Scenario: Apply response includes counts and per-run results
- **WHEN** a backfill request with `dry_run=false` processes 3 eligible runs where 2 accept repair and 1 is skipped
- **THEN** the response has `eligible_count=3`, `repair_requested_count=2`, `skipped_count=1`, and 3 entries in `results`

### Requirement: Backfill action values
The `action` field SHALL use the enum `EngineProjectionBackfillAction` with values: `would_repair` (dry-run eligible run, no mutation), `repair_requested` (repair accepted in apply mode), `skipped` (race-time no-op in apply mode).

#### Scenario: Dry-run returns would_repair actions
- **WHEN** a backfill request with `dry_run=true` finds 2 eligible runs
- **THEN** each result has `action=would_repair` and no state is mutated

#### Scenario: Apply returns repair_requested for accepted repairs
- **WHEN** a backfill request with `dry_run=false` processes a run whose repair is accepted
- **THEN** the result has `action=repair_requested`

#### Scenario: Apply returns skipped for race-time no-ops
- **WHEN** a run becomes ineligible between the eligibility query and per-run repair (e.g., already transitioned to `catching_up`)
- **THEN** the result has `action=skipped` with the repair service reason

### Requirement: Backfill reason values
The `reason` field, when present, SHALL use existing repair-service reason values: `already_up_to_date`, `history_expired`, `no_events_to_project`, `repair_requested`, `already_catching_up`.

#### Scenario: Skipped run includes reason
- **WHEN** a run is skipped because it is already catching up
- **THEN** the result has `action=skipped` and `reason=already_catching_up`

### Requirement: Backfill reuses repair service
The apply path SHALL call the existing repair service (`RepairRun()`) for each eligible run. No direct projection writes and no new maintenance worker SHALL be introduced. This preserves the single-writer invariant.

#### Scenario: Repeated backfill calls converge
- **WHEN** a backfill request is repeated after all eligible runs have been repaired
- **THEN** subsequent calls return `eligible_count=0` or all results as `skipped`

### Requirement: older_than filtering
The `older_than` parameter SHALL filter candidates by `engine_projection_updated_at`, selecting only runs whose projection was last updated before the given timestamp.

#### Scenario: older_than filters by projection update time
- **WHEN** a backfill request has `older_than=2026-01-01T00:00:00Z` and 3 runs have `engine_projection_updated_at` before that date while 2 have it after
- **THEN** only the 3 older runs are eligible candidates
