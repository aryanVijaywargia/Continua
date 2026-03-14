# Change: Add Batch Ingestion Pipeline with Idempotency

## Summary

Implement a batch ingestion pipeline for Continua that accepts traces, spans, and events from SDKs with idempotent processing and multi-tenant support.

## Why

Continua needs to accept trace/span/event data from TypeScript and Python SDKs reliably. Without batch ingestion:
- SDKs must make N API calls per trace (slow, unreliable)
- Network failures cause data loss with no retry mechanism
- No deduplication mechanism for retries
- No multi-tenant isolation

## What Changes

### NEW Capabilities
- `POST /v1/ingest` endpoint for batch ingestion (up to 5MB)
- Store layer with transaction support and upsert semantics
- Batch idempotency with upfront claiming pattern
- Multi-tenancy support via `projects` table
- Span events table for fine-grained event tracking

### MODIFIED Components
- Schema extended with external IDs (`trace_id` TEXT, `span_id` TEXT)
- Schema extended with `project_id` FK on all data tables
- OpenAPI contract updated with ingest endpoint

## Impact

### Affected Specs
| Spec | Type | Description |
|------|------|-------------|
| ingestion | NEW | Batch ingestion capability |
| idempotency | NEW | Batch deduplication |
| data-model | MODIFIED | Extended schema for multi-tenancy |

### Affected Code
| Path | Change |
|------|--------|
| `db/platform/migrations/postgres/0001_initial_schema.up.sql` | REPLACE |
| `db/platform/queries/*.sql` | NEW queries |
| `internal/store/` | NEW store layer |
| `internal/domain/` | NEW domain types |
| `internal/ingest/` | NEW service |
| `internal/api/ingest.go` | NEW handler |
| `contracts/openapi/openapi.yaml` | MODIFIED |
| `pkg/jsonutil/` | NEW utilities |

### Breaking Changes
None - this adds new functionality to an empty/scaffold codebase.

## Dependencies

- River queue library (for async mode in v1.1)
- No other new dependencies (per project constraints)

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Schema changes break existing code | Replace 0001 entirely (no production yet) |
| Transaction deadlocks | Always claim batch FIRST, consistent upsert order |
| Validation complexity | v1 rejects entire batch (no partial success) |

## Success Criteria

- POST /v1/ingest accepts batches up to 5MB
- Duplicate batch_key returns success (not error)
- Upsert semantics update existing traces/spans
- Integration tests pass
- `make ci` green

## Related Documents

- Design: [design.md](./design.md)
- Tasks: [tasks.md](./tasks.md)
- Spec Deltas: [specs/](./specs/)
