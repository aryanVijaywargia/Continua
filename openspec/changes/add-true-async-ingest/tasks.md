## 1. Contract & Schema Foundation

- [x] 1.1 Update `contracts/openapi/openapi.yaml`: add `batch_id` to `IngestResponse`, add `GET /v1/ingest/batches/{id}`, add `BatchStatusResponse`, add `X-Continua-Async-Version`, and document `400 unsupported_async_version`
- [x] 1.2 Run `make generate` and verify generated server/types compile cleanly
- [x] 1.3 Create compatibility-first database migration: extend `ingest_batches` with `processing_started_at`, `attempt_count`, `last_error_code`, `last_error_message`, `last_error_at`; create `ingest_batch_payloads`; do not destructively rewrite legacy statuses in this migration
- [x] 1.4 Add SQLC queries: `ClaimBatchOrGetExisting`, `InsertBatchPayload`, `GetBatchPayload`, `DeleteBatchPayload`, `MarkBatchProcessingIfQueued`, `MarkBatchCompleted`, `MarkBatchFailed`, `MarkBatchQueued`, `GetBatchForProject`, `CleanupExpiredPayloads`
- [x] 1.5 Run `make generate` for SQLC; verify generated models include new columns
- [ ] 1.6 Run migration against a test DB; verify old rows remain readable and new code can map legacy statuses compatibly

**Validation:** `make generate` succeeds, migration applies cleanly, SQLC models compile

## 2. River Queue Topology

- [x] 2.1 Update `internal/jobs/module.go`: configure primary queues (`ingest`, `rollup`, `maintenance`) plus temporary `default` drain workers for mixed-version deploys; set `JobTimeout=5m`, `RescueStuckJobsAfter=10m`
- [x] 2.2 Update `TraceRollupArgs.InsertOpts()` to set `Queue: "rollup"`; add `Timeout()` returning `30s`
- [x] 2.3 Make queue worker counts configurable with proposal defaults
- [x] 2.4 Add tests: rollup jobs target `rollup`, legacy `default` jobs still execute during transition

**Validation:** `make test-go` passes; queue routing and mixed-version consumption work correctly

## 3. Ingest Service Refactor

- [x] 3.1 Extract shared batch processor from `internal/ingest/service.go` for traces, spans, events, and rollup enqueue logic
- [x] 3.2 Keep existing `Ingest()` as the sync wrapper calling the shared processor
- [x] 3.3 Add `AcceptAsync()` path: acceptance-time validation, gzip payload storage, atomic insert of batch + payload + job
- [x] 3.4 Add `BatchID` to `IngestResponse`
- [x] 3.5 Update store wrappers in `internal/store/batches.go`: claim-or-get, payload CRUD, state transitions, compatibility lookups
- [ ] 3.6 Keep body-size enforcement and unsupported-header handling in the API layer with focused API tests
- [x] 3.7 Add unit tests for acceptance path: new batch, duplicate, invalid enum/value, missing `batch_key`
- [ ] 3.8 Add unit tests for shared processor: terminal error, retryable error, dependency-not-ready classification, idempotent re-entry

**Validation:** `make test-go` passes; sync path behavior is unchanged

## 4. Async Ingest Worker

- [x] 4.1 Create `internal/jobs/ingest_batch.go`: `IngestBatchArgs`, `IngestBatchWorker`, and explicit claim/data/failure-status transaction flow
- [x] 4.2 Register `IngestBatchWorker` in `NewClient()` alongside `TraceRollupWorker`
- [x] 4.3 Implement failure handling: terminal errors → `failed`; retryable errors → `queued` + River retry; unresolved references retry until dependency window expires
- [x] 4.4 Add integration test: `queued → processing → completed` lifecycle with externally visible `processing`
- [x] 4.5 Add integration test: valid dependent span/event batch retries until its trace batch commits, then succeeds
- [x] 4.6 Add integration test: unresolved dependency after retry window expires → `failed` with `reference_timeout`
- [x] 4.7 Add integration test: idempotent retry after completion is a no-op
- [x] 4.8 Add integration test: `last_error_*` preserved during retries and cleared on success

**Validation:** `make test-integration` passes; worker lifecycle and dependency handling are correct

## 5. Cleanup Worker

- [x] 5.1 Create `internal/jobs/cleanup.go`: River periodic job on `maintenance` queue, `Timeout()=1m`
- [x] 5.2 Implement: delete failed payloads older than 7 days, skip active batches, retain `ingest_batches` rows for idempotency
- [x] 5.3 Add integration test: cleanup prunes payloads correctly and never deletes idempotency rows

**Validation:** `make test-integration` passes; cleanup logic is correct

## 6. API Layer

- [x] 6.1 Update `Ingest()` handler in `server.go`: header-based routing, env-based default check, unsupported-version rejection, and `batch_id` in responses
- [x] 6.2 Implement `GetBatchStatus()` handler: project scoping, internal→public status mapping, legacy-status compatibility mapping
- [x] 6.3 Add API test: header opt-in returns `202` without inline writes
- [x] 6.4 Add API test: legacy behavior is preserved without header
- [x] 6.5 Add API test: `sync=true` is always inline regardless of header/env
- [x] 6.6 Add API test: invalid async-version header returns `400`
- [x] 6.7 Add API test: batch status endpoint returns correct mapped status, including visible `processing`
- [x] 6.8 Add API test: batch status endpoint returns `404` for wrong project

**Validation:** `make test-go` passes; API behavior matches spec

## 7. Python SDK

- [x] 7.1 Add `ingest_mode` config to `Continua` client (`sync`, `async_v2`, `server_default`)
- [x] 7.2 Update `flush()` to set header/param based on mode
- [x] 7.3 Add `wait_for_batch(batch_id, timeout, poll_interval)` helper
- [x] 7.4 Add SDK tests: mode configuration, header/param behavior, wait helper success/timeout
- [x] 7.5 Update SDK examples and docs: read-after-write caveat, duplicate semantics after durable acceptance, failed batch polling

**Validation:** `cd sdks/python && uv run pytest -q` passes

## 8. Deployment Compatibility

- [x] 8.1 Add config wiring for `INGEST_TRUE_ASYNC_DEFAULT`, dependency retry window, and queue worker counts
- [x] 8.2 Write rollout notes/runbook: additive migration, dual-consume queue phase, producer cutover, post-cutover backfill, `default` queue removal
- [x] 8.3 Add a post-cutover backfill task or documented SQL for `accepted → completed` and stale legacy `processing → failed`
- [x] 8.4 Add mixed-version coverage showing legacy `default` jobs still drain during rollout

**Validation:** rollout sequence is documented and testable without an unsafe one-step deploy

## 9. Operational Visibility

- [x] 9.1 Add structured logging: `batch_accepted`, `batch_duplicate`, `batch_processing_started`, `batch_processing_retried`, `batch_processing_completed`, `batch_processing_failed`, `batch_cleanup_completed`
- [x] 9.2 Ensure consistent log fields: `batch_id`, `batch_key`, `project_id`, `attempt_count`, `duration_ms`, `error_code`
- [ ] 9.3 Add metrics for acceptance totals, processing totals, dependency waits, processing duration, oldest queued batch age, and River queue depth

**Validation:** logs and metrics are reviewable during integration and smoke runs

## 10. Final Validation

- [x] 10.1 `make generate` — no drift
- [ ] 10.2 `make test` — all tests pass
- [ ] 10.3 `make test-integration` — integration tests pass
- [ ] 10.4 `make lint` — no new warnings
- [ ] 10.5 Manual smoke test: submit async batch, poll `accepted → processing → completed`
- [ ] 10.6 Manual rollout smoke test: verify `default` queue drains during the mixed-version compatibility window
