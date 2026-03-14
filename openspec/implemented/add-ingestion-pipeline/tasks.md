# Tasks: Add Ingestion Pipeline

## Phase 1: Schema Foundation

- [ ] Replace `db/platform/migrations/postgres/0001_initial_schema.up.sql`
  - Add `projects` table (id, name, api_key_hash)
  - Add `ingest_batches` table with UNIQUE(project_id, batch_key)
  - Modify `traces`: add trace_id TEXT, project_id FK, input/output JSONB
  - Modify `spans`: add span_id TEXT, project_id FK, input/output JSONB
  - Create `span_events` table (no FK to spans)
  - Add all required indexes
- [ ] Update `db/platform/migrations/postgres/0001_initial_schema.down.sql`
- [ ] Run `make generate` to regenerate sqlc models
- [ ] Verify generated models in `db/gen/go/platform/`

## Phase 2A: Store Layer

- [ ] Create `internal/store/store.go`
  - Define `Store` struct with pgxpool.Pool
  - Define error types: `ErrNotFound`, `ErrDuplicateBatch`
  - Define `DBTX` interface for pool/transaction abstraction
- [ ] Create `internal/store/tx.go`
  - Define `Tx` wrapper struct
  - Implement `BeginTx()`, `Commit()`, `Rollback()`
- [ ] Create `db/platform/queries/batches.sql`
  - `ClaimBatch` query with ON CONFLICT DO NOTHING
  - `UpdateBatchStatus` query
- [ ] Create `internal/store/batches.go`
  - Implement `ClaimBatch(ctx, tx, projectID, batchKey) (uuid, error)`
  - Implement `UpdateBatchStatus(ctx, tx, batchID, status, counts) error`
- [ ] Create `db/platform/queries/traces.sql` (update existing)
  - Add upsert query with COALESCE for patch semantics
  - Add `GetTraceUUID` query
- [ ] Create `internal/store/traces.go`
  - Implement `UpsertTrace(ctx, tx, trace) (uuid, error)`
  - Implement `GetTraceUUID(ctx, tx, projectID, externalTraceID) (uuid, error)`
- [ ] Create `db/platform/queries/spans.sql` (update existing)
  - Add upsert query with COALESCE
- [ ] Create `internal/store/spans.go`
  - Implement `UpsertSpan(ctx, tx, traceUUID, span) error`
- [ ] Create `db/platform/queries/events.sql`
  - Add batch insert with ON CONFLICT DO NOTHING
- [ ] Create `internal/store/span_events.go`
  - Implement `InsertSpanEvents(ctx, tx, events) (inserted int, error)`
- [ ] Run `make generate`

## Phase 2B: Domain Types (parallel with 2A)

- [ ] Create `internal/domain/trace.go`
  - Define `Trace` struct with all fields
- [ ] Create `internal/domain/span.go`
  - Define `Span` struct with all fields
  - Define `SpanSummary` struct (no payloads)
- [ ] Create `internal/domain/event.go`
  - Define `SpanEvent` struct
- [ ] Create `internal/domain/batch.go`
  - Define `IngestBatch` struct

## Phase 2C: Payload Utils (parallel with 2A)

- [ ] Create `pkg/jsonutil/wrapper.go`
  - Implement `WrapPayload(data []byte) ([]byte, bool, error)`
  - Implement `IsWrapped(data map[string]any) bool`
  - Implement `UnwrapPayload(data map[string]any) (string, string, bool)`
- [ ] Create `pkg/jsonutil/wrapper_test.go`
  - Test valid JSON passthrough
  - Test invalid JSON wrapping
  - Test edge cases (empty, binary)
- [ ] Create `pkg/jsonutil/truncate.go`
  - Implement `TruncatePayload(data []byte, maxBytes int) TruncationResult`
- [ ] Create `pkg/jsonutil/truncate_test.go`
  - Test under limit (no truncation)
  - Test over limit (truncation + metadata)
  - Test various JSON types (string, array, object)

## Phase 3: Service Orchestration

- [ ] Create `internal/ingest/dto.go`
  - Define `IngestRequest` struct
  - Define `TraceInput`, `SpanInput`, `EventInput` structs
  - Define `IngestResult` struct
- [ ] Create `internal/ingest/processor.go`
  - Implement `processTrace(projectID, input) (*domain.Trace, error)`
  - Implement `processSpan(projectID, traceUUID, input) (*domain.Span, error)`
  - Implement `processEvent(projectID, traceUUID, input) (*domain.SpanEvent, error)`
- [ ] Create `internal/ingest/service.go`
  - Implement `Service` struct with store dependency
  - Implement `Process(ctx, projectID, req) (*IngestResult, error)`
  - Implement transaction flow: claim → upsert → update → commit
- [ ] Create `internal/ingest/service_test.go`
  - Test happy path
  - Test duplicate batch returns success
  - Test invalid JSON wrapped (not error)
  - Test unknown trace reference (error)

## Phase 4: API Layer

- [ ] Update `contracts/openapi/openapi.yaml`
  - Add `POST /v1/ingest` endpoint
  - Add `IngestRequest` schema
  - Add `IngestResponse` schema
  - Add query param `sync` boolean
- [ ] Run `make generate`
- [ ] Create `internal/api/ingest.go`
  - Implement `IngestHandler` struct
  - Implement `Ingest(w, r)` handler
  - Add 5MB size limit with `http.MaxBytesReader`
  - Add manual validation (no go-playground/validator)
- [ ] Create `internal/api/ingest_test.go`
  - Test success (200)
  - Test duplicate (200 with status: duplicate)
  - Test validation error (400)
  - Test too large (413)
- [ ] Wire handler in `cmd/continua/main.go`
  - Add Fx providers for store, service, handler
  - Register route

## Phase 5: Integration Tests

- [ ] Create `internal/ingest/integration_test.go`
  - Setup helper using docker-compose.test.yml
  - Test full ingestion flow
  - Test batch idempotency
  - Test concurrent duplicate handling
- [ ] Update `Makefile` if needed for integration test target
- [ ] Verify `make test-integration` passes

## Phase 6: Validation

- [ ] Run `make generate` - no drift
- [ ] Run `make lint` - no errors
- [ ] Run `make test` - all pass
- [ ] Run `openspec validate add-ingestion-pipeline --strict`
- [ ] Manual smoke test with curl

## Dependencies

```
Phase 1 → Phase 2A, 2B, 2C (parallel)
Phase 2A → Phase 3
Phase 3 → Phase 4
Phase 4 → Phase 5
Phase 5 → Phase 6
```

## Notes

- Do NOT add go-playground/validator dependency
- Do NOT implement async/River in v1 (defer to v1.1)
- v1 validation: reject entire batch on error
- Stop after each phase, run tests, verify before continuing
