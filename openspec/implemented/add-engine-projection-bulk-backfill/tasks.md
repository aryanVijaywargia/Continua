## 1. API Contract
- [x] 1.1 Add `POST /v1/engine/projections/backfill` path to `contracts/openapi/openapi.yaml` with preview-gate header requirement
- [x] 1.2 Add `EngineProjectionBackfillRequest` schema with `dry_run`, `limit`, `older_than`, `engine_instance_key`, `engine_definition_name`, `engine_run_status`, `engine_projection_state` fields
- [x] 1.3 Add `EngineProjectionBackfillResponse` schema with `dry_run`, `limit`, `eligible_count`, `repair_requested_count`, `skipped_count`, `results` fields
- [x] 1.4 Add `EngineProjectionBackfillRunResult` schema with `run_id`, `trace_id`, `projection_state`, `action`, `reason` fields
- [x] 1.5 Add `EngineProjectionBackfillAction` enum with `would_repair`, `repair_requested`, `skipped` values
- [x] 1.6 Run `make generate` and verify generated code compiles

## 2. Backend Implementation
- [x] 2.1 Add backfill eligibility query to store: select `summary_only` traces with retained history shell where latest retained engine history ID > `engine_last_projected_history_id`, supporting `older_than`, `engine_instance_key`, `engine_definition_name`, `engine_run_status` filters and `limit`
- [x] 2.2 Add backfill handler in `internal/api/engine_handlers.go` with preview-gate validation, limit validation (400 for > 100, default 50), and dry-run/apply branching
- [x] 2.3 Wire apply-mode per-run processing to the existing repair service `RepairRun()` method
- [x] 2.4 Return incompatible `engine_projection_state` filters (`up_to_date`, `catching_up`, `journal_expired`) as zero eligible rows
- [x] 2.5 Write Go integration tests: dry-run returns `would_repair` without mutation, apply returns `repair_requested` and flips state, limit cap returns 400, `older_than` filters correctly, convergent repeated calls reach zero eligible, incompatible projection-state filters return empty results

## 3. Python SDK
- [x] 3.1 Add `EngineProjectionBackfillResponse`, `EngineProjectionBackfillRunResult`, `EngineProjectionBackfillAction` types to `sdks/python/src/continua/types.py`
- [x] 3.2 Add `backfill_projections(*, dry_run, limit, older_than, engine_instance_key, engine_definition_name, engine_run_status, engine_projection_state)` method to `EngineControlClient`
- [x] 3.3 Add `backfill_projections_all(*, max_total=1000, **kwargs)` apply-mode paging helper that calls `backfill_projections()` repeatedly until no eligible runs remain or `max_total` is reached, and rejects `dry_run=True` because dry-run pagination cannot advance without an API cursor
- [x] 3.4 Write pytest tests: single-call response decoding, paging helper stops at `max_total`, paging helper stops when eligible runs exhausted, dry-run paging rejection
