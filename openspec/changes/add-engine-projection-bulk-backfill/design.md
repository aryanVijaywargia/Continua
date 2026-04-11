## Context
Operators who have run retention cycles may have many `summary_only` traces with retained engine history that could be re-projected. The single-run `POST /v1/engine/runs/{run_id}/repair` endpoint works for individual runs but is impractical for bulk recovery. This change adds a bulk backfill endpoint that identifies eligible runs and triggers repairs in a single call.

## Goals / Non-Goals
- Goals:
  - Single API call to identify and repair eligible `summary_only` runs
  - Dry-run mode for safe previews without mutation
  - Reuse existing repair service (no new projection writer, no new maintenance worker)
  - Per-run result transparency with action and reason
  - Python SDK convenience methods with bounded paging
- Non-Goals:
  - No `cmd/continua backfill` CLI command in this phase
  - No traces-page bulk backfill UI
  - No automatic/scheduled backfill worker
  - No admin/operations UI

## Decisions
- **Reuse repair service**: The backfill endpoint iterates eligible runs and calls the existing `RepairRun()` for each. This avoids a second projection writer and preserves the single-writer invariant that the projector owns.
- **Limit cap at 100**: Prevents unbounded DB work per request. The Python SDK provides a `backfill_projections_all()` paging helper with a configurable `max_total` (default 1000) for larger batches.
- **Default to summary_only candidates**: If `engine_projection_state` is omitted, only `summary_only` runs are eligible. Explicitly requesting `up_to_date`, `catching_up`, or `journal_expired` returns zero eligible rows because those states are not valid backfill targets.
- **Eligibility condition**: A run is eligible when it is `summary_only`, has a retained engine run/history shell, and the latest retained engine history ID is greater than `engine_last_projected_history_id`. This matches the existing repair-service checkpoint condition.
- **Preview gate**: The endpoint requires `X-Continua-Engine-Preview: 1` header, matching existing engine mutating routes.
- **Per-run result actions**: `would_repair` (dry-run eligible, no mutation), `repair_requested` (repair accepted in apply mode), `skipped` (race-time no-op in apply mode, with reason from repair service).

## Risks / Trade-offs
- **Lock contention**: Multiple concurrent backfill calls hitting the repair service could create CAS contention. Mitigation: the limit cap keeps batch sizes small, and the repair CAS prevents double-transitions safely.
- **Stale eligibility**: A run may become ineligible between the eligibility query and the per-run repair call (e.g., another repair request). Mitigation: the per-run result reports `skipped` with the repair reason, and repeated calls converge to zero eligible.

## Open Questions
- None remaining; all resolved in review.
