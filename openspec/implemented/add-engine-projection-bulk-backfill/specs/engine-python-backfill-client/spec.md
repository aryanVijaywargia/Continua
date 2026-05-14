## ADDED Requirements

### Requirement: Single-call backfill method
The Python `EngineControlClient` SHALL provide a `backfill_projections()` method accepting keyword arguments: `dry_run`, `limit`, `older_than`, `engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state`. The method SHALL call `POST /v1/engine/projections/backfill` and return a typed `EngineProjectionBackfillResponse`.

#### Scenario: Single backfill call returns typed response
- **WHEN** `backfill_projections(dry_run=True, limit=10)` is called
- **THEN** the method returns an `EngineProjectionBackfillResponse` with `eligible_count`, `results`, and other top-level fields

#### Scenario: Backfill method sends preview header
- **WHEN** `backfill_projections()` is called
- **THEN** the request includes the `X-Continua-Engine-Preview: 1` header

### Requirement: Bounded paging backfill helper
The Python `EngineControlClient` SHALL provide a `backfill_projections_all()` method accepting `max_total` (default 1000) and the `backfill_projections()` filter keyword arguments. The method SHALL call `backfill_projections()` repeatedly in apply mode until no more eligible runs remain or `max_total` cumulative results are reached. Each page uses the same filter parameters.

Because the backfill API has no cursor and dry-run calls do not mutate candidate eligibility, `backfill_projections_all(dry_run=True, ...)` SHALL reject with a clear `ValueError` instead of silently returning a truncated preview. Callers that need previews SHALL use `backfill_projections(dry_run=True, limit=...)`.

#### Scenario: Paging helper stops at max_total
- **WHEN** `backfill_projections_all(max_total=100, limit=50)` is called and more than 100 eligible runs exist
- **THEN** the method makes 2 calls (50 + 50 = 100 cumulative results) and stops at `max_total`

#### Scenario: Paging helper stops when no eligible runs remain
- **WHEN** `backfill_projections_all(max_total=1000, limit=50)` is called and 30 eligible runs exist
- **THEN** the method makes 1 call returning 30 results and stops because `eligible_count < limit` (no more eligible runs)

#### Scenario: Paging helper returns aggregated results
- **WHEN** `backfill_projections_all()` pages through 3 calls
- **THEN** the returned response aggregates results from all pages

#### Scenario: Paging helper rejects dry-run previews
- **WHEN** `backfill_projections_all(dry_run=True, max_total=100, limit=50)` is called
- **THEN** the method raises a `ValueError`
- **AND** it does not call the backfill API

### Requirement: Backfill response types
The Python SDK SHALL define `EngineProjectionBackfillResponse`, `EngineProjectionBackfillRunResult`, and `EngineProjectionBackfillAction` types matching the API response shape.

#### Scenario: Response type includes all fields
- **WHEN** a backfill response is decoded
- **THEN** the typed response has `dry_run`, `limit`, `eligible_count`, `repair_requested_count`, `skipped_count`, and `results` fields

#### Scenario: Run result type includes action and reason
- **WHEN** a run result is decoded
- **THEN** it has `run_id`, `trace_id`, `projection_state`, `action`, and optional `reason` fields
