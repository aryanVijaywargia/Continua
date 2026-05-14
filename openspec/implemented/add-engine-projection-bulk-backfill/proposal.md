# Change: Add Engine Projection Bulk Backfill

## Why
When many engine traces have `summary_only` projection state after retention purges, operators need a way to request bulk repair without calling the single-run repair endpoint repeatedly. A bulk backfill API provides a one-call path to identify eligible runs and trigger repairs, with a dry-run mode for safe previews.

## What Changes
- Add preview-gated `POST /v1/engine/projections/backfill` endpoint
- Support dry-run mode to preview eligible runs without mutating state
- Support filtering by `older_than`, `engine_instance_key`, `engine_definition_name`, `engine_run_status`, and `engine_projection_state`
- Default to `summary_only` candidates when `engine_projection_state` is omitted; return zero eligible rows for incompatible states (`up_to_date`, `catching_up`, `journal_expired`)
- Enforce limit cap at 100 (default 50) with 400 for values above 100
- Reuse existing repair service for apply-mode per-run processing; no new projection writer or maintenance worker
- Return structured per-run results with `action` (would_repair, repair_requested, skipped) and optional `reason`
- Add Python SDK `backfill_projections()` single-call method and `backfill_projections_all()` bounded paging helper
- Defer: CLI command, traces-page bulk backfill UI, admin/operations UI

## Impact
- Affected specs: engine-projection-backfill-api, engine-python-backfill-client
- Affected code: `contracts/openapi/openapi.yaml`, `internal/api/engine_handlers.go`, `internal/enginecontrol/service.go`, `internal/store/`, `sdks/python/src/continua/engine_control.py`, `sdks/python/src/continua/types.py`
