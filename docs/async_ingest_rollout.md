> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](./README.md) and [DEBUGGER_PLATFORM_BASELINE.md](./DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Async Ingest Rollout

This runbook covers the compatibility-first rollout for `add-true-async-ingest`.

## Phase 1: additive schema deploy

1. Apply migration `000010_add_true_async_ingest`.
2. Apply migration `000011_async_ingest_cleanup_index`.
3. Confirm old servers still read `ingest_batches` rows without requiring any status rewrite.

At this point:
- legacy rows with `status='accepted'` remain valid
- legacy rows with `status='processing'` remain readable
- payload cleanup is available, but no backfill has run yet

## Phase 2: mixed-version application deploy

1. Deploy the new server version with both the primary queues and temporary `default` queue consumers enabled.
2. Keep `INGEST_TRUE_ASYNC_DEFAULT=false` during the first mixed-version wave unless you are explicitly opting clients in with `X-Continua-Async-Version: 2`.
3. Verify both of these are true before cutover:
   - new async ingest jobs are draining from `ingest`
   - legacy jobs can still drain from `default`

## Phase 3: producer cutover

1. Enable async v2 traffic either by:
   - client header opt-in: `X-Continua-Async-Version: 2`
   - or setting `INGEST_TRUE_ASYNC_DEFAULT=true` after the fleet is on the new build
2. Monitor logs for:
   - `batch_accepted`
   - `batch_processing_started`
   - `batch_processing_completed`
   - `batch_processing_failed`
   - `batch_cleanup_completed`

## Phase 4: post-cutover backfill

Run this only after all servers are on the new code and the legacy compatibility window is over.

```sql
UPDATE ingest_batches
SET status = 'completed'
WHERE status = 'accepted';

UPDATE ingest_batches
SET status = 'failed',
    processing_completed_at = COALESCE(processing_completed_at, NOW()),
    last_error_code = 'legacy_interrupted',
    last_error_message = 'legacy processing row was interrupted before async cutover completed',
    last_error_at = COALESCE(last_error_at, NOW())
WHERE status = 'processing'
  AND processing_started_at IS NOT NULL
  AND processing_started_at < NOW() - INTERVAL '15 minutes';
```

If older legacy rows do not have `processing_started_at`, use the operator-selected grace condition that best matches your deploy window before converting them.

## Phase 5: compatibility removal

After queue drain and backfill are complete:

1. Remove temporary `default` queue workers.
2. Keep the batch-status API compatibility mapping for legacy reads only as long as needed.
3. Re-run smoke checks:
   - async accept returns `202` with `batch_id`
   - polling reaches `completed` or `failed`
   - failed payload cleanup retains `ingest_batches` rows
