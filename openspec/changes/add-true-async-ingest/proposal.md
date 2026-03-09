# Change: Add True Async Ingest (Phase 4.5)

## Why

The current `POST /v1/ingest` endpoint returns `202` but processes everything inline before responding. That is fake-async. It blocks the HTTP response on full batch processing, creates latency spikes, and couples acceptance availability to processing capacity. True async ingest decouples acceptance from processing, improves response times, and enables independent scaling.

## What Changes

### Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| **async-ingest** | NEW | Real queued background processing for `POST /v1/ingest` with durable acceptance, separate payload storage, staged rollout, and worker-based batch processing |
| **batch-status-api** | NEW | `GET /v1/ingest/batches/{id}` endpoint for polling batch processing status |
| **river-queue-topology** | NEW | Multi-queue River configuration isolating ingest, rollup, and maintenance work classes |
| **ingest-sdk-async** | NEW | Python SDK async mode configuration, `wait_for_batch()` helper, and migration documentation |
| **ingestion** | NEW | Establishes ingest endpoint behavior for staged true async acceptance, sync responses, and unsupported async-version handling |
| **idempotency** | NEW | Establishes durable batch-key semantics for accepted async batches |

### Key Design Decisions

1. **Staged rollout**: opt-in via `X-Continua-Async-Version: 2` first, then SDK migration, then env-controlled default flip.
2. **Compatibility-first migration**: additive schema first, mixed-version queue consumption during deploy, then post-cutover status backfill and cleanup.
3. **Acceptance-time validation**: reject invalid JSON, required-field violations, unsupported async-version headers, and enum/value mistakes before durable acceptance.
4. **Dependency-aware retries**: no global FIFO guarantee, but unresolved trace references retry for a bounded window before terminal failure.
5. **Explicit worker transaction boundaries**: commit `processing` state before heavy work; keep final domain writes, `completed`, and payload deletion atomic with each other.
6. **Durable idempotency retention**: retain `ingest_batches` rows as permanent idempotency records; cleanup prunes payload/debug artifacts only.

### Breaking Changes

- When true async is active, HTTP `202` becomes actually asynchronous. Callers relying on read-after-write after `202` must use `sync=true` or poll batch status.
- In Stage A this is opt-in only. It becomes broadly breaking only if Stage C flips the server default.

## Impact

### Affected Specs

- `async-ingest`
- `batch-status-api`
- `river-queue-topology`
- `ingest-sdk-async`
- `ingestion`
- `idempotency`

### Affected Code

| Path | Change |
|------|--------|
| `contracts/openapi/openapi.yaml` | MODIFY: add `batch_id` to `IngestResponse`, add `GET /v1/ingest/batches/{id}`, add `BatchStatusResponse`, add `X-Continua-Async-Version` behavior |
| `db/platform/migrations/postgres/` | ADD: compatibility-first migration extending `ingest_batches` and creating `ingest_batch_payloads` |
| `db/platform/queries/batches.sql` | MODIFY: add claim-or-get, state transitions, payload CRUD, and cleanup queries |
| `internal/ingest/service.go` | MODIFY: split into acceptance path, shared processor, sync wrapper, and async worker wrapper |
| `internal/ingest/dto.go` | MODIFY: add `BatchID` to `IngestResponse` |
| `internal/jobs/module.go` | MODIFY: multi-queue config, mixed-version rollout support, adjusted timeouts |
| `internal/jobs/trace_rollup.go` | MODIFY: add `Queue: "rollup"` to `InsertOpts()` |
| `internal/jobs/ingest_batch.go` | NEW: async ingest worker |
| `internal/jobs/cleanup.go` | NEW: periodic payload cleanup worker |
| `internal/api/server.go` | MODIFY: header-based async routing, unsupported-version rejection, `batch_id` in responses |
| `internal/store/batches.go` | MODIFY: add async batch store methods |
| `internal/config/` | MODIFY: env/config surface for async default, dependency retry window, and queue settings |
| `sdks/python/src/continua/client.py` | MODIFY: async mode config and `wait_for_batch()` |

### Database Changes

- Extend `ingest_batches` with `processing_started_at`, `attempt_count`, `last_error_code`, `last_error_message`, and `last_error_at`
- Add `ingest_batch_payloads` with gzip-compressed request bytes
- Keep legacy status values readable during rollout; perform status backfill only after all servers are upgraded

### OpenAPI Changes

- `POST /v1/ingest`: add `batch_id` to responses and define async-version header behavior
- `GET /v1/ingest/batches/{id}`: add batch status lookup

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking read-after-write assumptions | Staged rollout, `sync=true`, SDK polling helper |
| Valid dependent batches run out of order | Bounded dependency retry window before terminal `reference_timeout` |
| Worker crash leaves batch in `processing` | Committed processing claim + River rescue after 10 minutes |
| Failure-path status update fails | River rescue handles stuck jobs; metrics/logs surface it |
| Mixed-version deploy strands rollup jobs | Consume both `default` and `rollup` until old producers are gone |
| Idempotency lost after cleanup | Retain `ingest_batches` rows indefinitely; prune payload/debug data only |

## Dependencies

No new Go dependencies. River is already used. Python SDK uses existing `httpx`.

## Related Documents

- Design: [design.md](./design.md)
- Tasks: [tasks.md](./tasks.md)
- Spec Deltas: [specs/](./specs/)
