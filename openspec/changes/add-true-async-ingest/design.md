## Context

Phase 4.5 converts the current fake-async ingest (`202` returned but processing inline) into real queued background processing. This spans the API layer, ingest service, River job system, database schema, and Python SDK.

### Current State

- `POST /v1/ingest` returns `202` but processes everything synchronously before responding
- `ingest_batches` stores legacy `processing` and `accepted` status values
- `ClaimBatch` uses `ON CONFLICT DO NOTHING`, so duplicates cannot return the existing `batch_id`
- `IngestResponse` has no `batch_id`
- River currently uses one `default` queue with shared workers
- Rollup jobs do not specify a queue

### Constraints

- No new Go dependencies
- Must not break the existing sync path
- Must not break clients that do not opt in during Stage A
- Contract-first: OpenAPI changes before implementation
- Generated code via `make generate`
- Migrations must stay backward compatible during rolling deploys

## Goals / Non-Goals

**Goals**
- True background processing for `POST /v1/ingest` behind staged rollout
- Durable acceptance: batch row, payload row, and River job committed before `202`
- Batch status polling via `GET /v1/ingest/batches/{id}`
- Correct failure handling with terminal vs retryable classification
- Preserve correctness for dependent multi-batch workflows despite removed FIFO assumptions
- Queue isolation for ingest, rollup, and maintenance work
- Python SDK async mode and batch polling helper

**Non-Goals**
- Public batch list/search API
- Global FIFO ordering between batches
- Chunked or partial batch processing
- WebSocket/SSE batch status notifications
- TypeScript SDK changes

## Decisions

### Decision 1: Staged Rollout

**What:** Three-stage rollout from opt-in to default.

**Stages**
- **A**: Default remains legacy. `X-Continua-Async-Version: 2` enables true async.
- **B**: SDK adds explicit ingest mode config (`sync`, `async_v2`, `server_default`) and `wait_for_batch()`.
- **C**: `INGEST_TRUE_ASYNC_DEFAULT` controls the server default for non-sync requests.

**Precedence:** `sync=true` > header opt-in > env default > legacy fake-async.

**Header handling:** If `X-Continua-Async-Version` is present with any value other than `2`, the API returns `400 unsupported_async_version`. Silent downgrade would hide rollout mistakes.

### Decision 2: Internal vs Public Status Vocabulary

**What:** DB writes `queued | processing | completed | failed`. Public API returns `accepted | processing | completed | failed`.

**Mapping**
- `POST` sync returns `ok | duplicate`
- `POST` async returns `accepted | duplicate`
- `GET` returns `accepted | processing | completed | failed`
- Compatibility reads map legacy `accepted → completed` during rollout

### Decision 3: Separate Payload Table

**What:** Store raw request bytes in `ingest_batch_payloads` as gzip-compressed `BYTEA`, not in `ingest_batches`.

**Why:** Keep status lookups lean, allow independent payload deletion, and avoid bloating batch metadata rows.

**Schema**
```sql
CREATE TABLE ingest_batch_payloads (
    batch_id UUID PRIMARY KEY REFERENCES ingest_batches(id) ON DELETE CASCADE,
    payload_bytes BYTEA NOT NULL,
    compression TEXT NOT NULL DEFAULT 'gzip',
    content_type TEXT NOT NULL DEFAULT 'application/json',
    byte_size INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Retention**
- On `completed`: delete payload in the same transaction as final domain writes
- On `failed`: keep payload for 7 days for debugging
- `ingest_batches` rows are retained indefinitely as durable idempotency records

### Decision 4: One River Client, Multiple Queues

**What:** Use one River client with separate named queues for ingest, rollup, and maintenance work.

**Defaults**
- `ingest=4`
- `rollup=10`
- `maintenance=1`

Worker counts are configurable so operators can rebalance queue capacity without code changes.

**Timeouts**
- Client: `JobTimeout=5m`, `RescueStuckJobsAfter=10m`
- `IngestBatchWorker.Timeout()=5m`
- `TraceRollupWorker.Timeout()=30s`
- `CleanupWorker.Timeout()=1m`

**Mixed-version rollout**
1. Deploy consumers that process both `default` and `rollup`
2. Switch producers so new rollup jobs target `rollup`
3. Remove `default` only after old producers are gone and backlog drains

`default` must retain non-zero workers during the compatibility window. `MaxWorkers: 0` would strand old jobs.

### Decision 5: Acceptance Transaction

**What:** Acceptance commits three things atomically: new batch row in `queued`, compressed payload row, and River ingest job via `InsertTx`.

**Why:** `202` is returned only after durable acceptance. If any of the three fails, the client sees an error and can retry with the same `batch_key`.

**Duplicate handling:** Use try-insert plus lookup on conflict so duplicate submissions return the existing `batch_id` and current status.

### Decision 6: Worker Execution Model

**What:** Split ingest execution into explicit acceptance, worker claim, data processing, and failure-status steps.

**Worker flow**
1. **Claim transaction**: load batch; if terminal, return success; otherwise transition `queued → processing`, increment `attempt_count`, set `processing_started_at`, commit
2. Load and decompress payload outside the data transaction
3. **Data transaction**: process traces/spans/events, enqueue rollups, mark `completed`, clear `last_error_*`, delete payload, commit atomically
4. On failure, **status transaction**: mark either `queued` or `failed` with error metadata

**Why:** This makes `processing` externally visible while still keeping final writes and completion state atomic with payload deletion.

**Failure handling**
- Terminal: `payload_decode_error`, acceptance-missed validation issues, `reference_timeout`
- Retryable: `db_error`, `tx_conflict`, `worker_timeout`, `internal_retryable`
- Dependency-not-ready: unresolved trace references retry until the dependency window expires, then become terminal `reference_timeout`

If the failure-status transaction also fails, the batch may remain `processing` until River rescues it. This is acceptable but must be visible in logs and metrics.

### Decision 7: Validation Split

**What:** Perform full structural decode and request-shape validation at acceptance time. Defer only DB-dependent validation to workers.

**Acceptance validates**
- auth
- body size
- valid JSON
- supported async-version header
- decode into `IngestRequest`
- required fields
- enum/value formats

**Worker validates**
- unknown trace references
- dependency resolution against committed DB state
- other DB-dependent invariants

**Why:** Client-visible mistakes should still fail fast with `400`. Only validation that depends on database state belongs in background processing.

### Decision 8: Dependency Retry Window

**What:** Unknown trace references do not fail immediately in true async. They retry for a bounded window.

**Defaults**
- Retry window: 15 minutes from `server_received_at`
- Retry schedule: River-managed backoff
- Terminal conversion after expiry: `last_error_code='reference_timeout'`

**Why:** True async removes incidental ordering guarantees. Bounded retries preserve valid multi-batch workflows without allowing infinite poison-pill retries.

The 15-minute window is an operational default, not a wire-level contract. The normative behavior is “retry until the configured dependency window expires.”

### Decision 9: Idempotency Retention

**What:** A durably accepted async batch permanently reserves `(project_id, batch_key)`.

**Implications**
- Acceptance that fails before commit is still retryable with the same `batch_key`
- Once `202` returns with a `batch_id`, resubmissions return `duplicate` with the same `batch_id`
- Cleanup never deletes the idempotency row

**Why:** Releasing the key after cleanup would break duplicate semantics and make batch status polling unreliable.

### Decision 10: Migration Compatibility

**What:** Use a compatibility-first rollout instead of a destructive one-step status rewrite.

**Phase 1: additive schema migration**
- add new columns
- add `ingest_batch_payloads`
- keep `status` permissive so old and new binaries can coexist
- do not rewrite existing rows in the schema migration

**Phase 2: application compatibility**
- new code writes only `queued | processing | completed | failed`
- reads tolerate legacy `accepted` and `processing`
- batch status API maps legacy `accepted → completed`

**Phase 3: post-cutover backfill**
- after all servers run new code, backfill `accepted → completed`
- convert stale legacy `processing` rows older than a grace window to `failed` with `last_error_code='legacy_interrupted'`
- remove temporary rollout compatibility only after queue drain and backfill complete

### Decision 11: Operational Visibility

**What:** Async ingest must ship with metrics as well as logs.

**Required metrics**
- `ingest_batch_accept_total{mode,status}`
- `ingest_batch_processing_total{status,error_code}`
- `ingest_batch_processing_duration_seconds`
- `ingest_batch_dependency_wait_total`
- `ingest_batch_oldest_queued_age_seconds`
- River queue depth by queue name

**Why:** The main failure modes are operational: dependency waits, stuck processing states, and queue imbalance.

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| Read-after-write breakage | SDKs/tests assuming data visible after `202` | Staged rollout; `sync=true`; SDK polling helper |
| Valid span/event batches arrive before their trace batch | False terminal failures | Bounded dependency retry window |
| Worker crash during processing | Batch stuck in `processing` | Committed processing claim + River rescue |
| Failure-status transaction fails | Batch remains `processing` longer than intended | Rescue, metrics, and logs |
| Queue routing change during deploy | Jobs stranded on old queue | Dual-consume `default` and `rollup` during rollout |
| Permanent idempotency rows grow over time | More metadata retained long-term | Keep rows small, prune payloads separately, revisit archival only with an explicit tombstone design |

## Open Questions

None. The remaining work is implementation detail and rollout execution, not design uncertainty.
