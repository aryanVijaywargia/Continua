> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Spec 1: Async Rollups via River

## Summary

Moved trace rollup computation off the ingest critical path using River, a PostgreSQL-native job queue.

## Changes Made

### 1. Added River Dependencies

**go.mod** now includes:
- `github.com/riverqueue/river v0.30.0`
- `github.com/riverqueue/river/riverdriver/riverpgxv5 v0.30.0`
- `github.com/riverqueue/river/rivertype v0.30.0`

### 2. Created River Migration

**File**: `db/platform/migrations/postgres/000003_add_river_tables.up.sql`

Vendored River's required tables (consolidated from migrations 001-006):
- `river_migration` - Migration tracking
- `river_job` - Main job queue table
- `river_leader` - Leader election for workers
- `river_queue` - Queue configuration
- `river_client` / `river_client_queue` - Client tracking

Key features:
- Job state enum with states: available, cancelled, completed, discarded, pending, retryable, running, scheduled
- GIN indexes on args and metadata for efficient querying
- Unique index for job deduplication
- Bitmask function for unique state checking

### 3. Created Jobs Module

**File**: `internal/jobs/module.go`
- Fx module for dependency injection
- `NewClient(pool, store)` creates River client with workers
- Worker lifecycle management via Fx hooks

**File**: `internal/jobs/trace_rollup.go`
- `TraceRollupArgs` - Job arguments with trace_id
- `TraceRollupWorker` - Worker that computes rollups
- `EnqueueRollup` / `EnqueueRollupInTx` - Job enqueue functions

### 4. Modified Ingest Service

**File**: `internal/ingest/service.go`

Before:
```go
// Compute rollups inline
for _, traceUUID := range traceMap {
    s.store.ComputeAndUpdateTraceRollupsTx(ctx, tx, traceUUID)
}
```

After:
```go
// Enqueue rollup jobs (async via River)
for _, traceUUID := range traceMap {
    jobs.EnqueueRollupInTx(ctx, s.riverClient, tx.Tx(), traceUUID)
}
```

### 5. Wired into Fx App

**File**: `cmd/continua/main.go`
```go
app := fx.New(
    config.Module,
    store.Module,
    jobs.Module,  // NEW - River job processing
    ingest.Module,
    api.Module,
    ...
)
```

## Job Deduplication

Jobs are deduplicated using River's `UniqueOpts`:
```go
UniqueOpts: river.UniqueOpts{
    ByArgs: true,
    ByState: []rivertype.JobState{
        rivertype.JobStateAvailable,
        rivertype.JobStateRetryable,
    },
}
```

This means:
- Only one pending job per trace_id at a time
- Running jobs are excluded - a follow-up can queue while one runs
- Completed jobs can be re-enqueued when new spans arrive

## Transaction Boundaries

Jobs are enqueued within the ingest transaction:
- If the transaction commits, the job becomes visible
- If the transaction rolls back, no job is enqueued
- This ensures data consistency

## Verification

```bash
# Build
go build ./cmd/continua/...

# Run migrations (after starting PostgreSQL)
make migrate

# Start server (River worker starts automatically)
make dev-server

# Check river_job table after ingesting data
psql -c "SELECT id, kind, state, args FROM river_job LIMIT 10"
```

## Test Coverage

Tests exist in `internal/jobs/rollup_test.go`:
- `TestRollupJob_EnqueuedAfterSpanIngest`
- `TestRollupJob_Deduplication`
- `TestRollupJob_Execution`
- `TestRollupJob_RetryDoesNotDoubleCount`
- `TestRollupJob_TransactionBoundary_CommitEnqueuesRollup`
- `TestRollupJob_TransactionBoundary_RollbackNoJob`
- `TestRollupJob_CoalescingWithRunningJob`

---

=== SPEC 1 COMPLETE: READY FOR REVIEW ===
