# Backend testing guide

## Current test style
- Many backend tests use a real Postgres database via `internal/testutil`.
- Do not assume packages are unit-test-only just because the filenames are `*_test.go`.
- The most valuable coverage is around project scoping, ingest idempotency, async batch handling, rollups, search filters, and API mapping.

## Useful commands
- `go test ./internal/api/...`
- `go test ./internal/ingest/...`
- `go test ./internal/store/...`
- `go test ./internal/jobs/...`
- `make test-go`
- `make test`

## Shared helpers
- `internal/testutil/testutil.go`
  - `TestDB`
  - `CreateTestProject`
  - `CreateTestTrace`
  - pointer helpers
  - pgtype helpers

## Good backend test targets

### API handlers
- auth behavior
- project-scoped `404` behavior
- mapper-visible fields from OpenAPI changes
- pagination and cursor behavior for timeline endpoints
- session compare validation, scoping, and too-large responses

### Ingest
- batch validation
- sync vs async acceptance
- duplicate batch behavior
- dependency-not-ready behavior
- payload truncation metadata

### Jobs
- batch state transitions
- rollup re-run behavior when trace versions change
- cleanup worker retention behavior

### Store
- sqlc wrapper semantics
- search filter behavior and ranking
- rollup computations
- session external-id behavior

## Test selection rule
- if you touched `contracts/` or API mappers, run `go test ./internal/api/...`
- if you touched ingest logic, run `go test ./internal/ingest/... ./internal/jobs/...`
- if you touched SQL or store wrappers, run `go test ./internal/store/...`
- if the change crosses multiple layers, run `make test`

Notable current backend coverage includes:
- `internal/api/session_compare_integration_test.go`
- timeline unit coverage in `internal/api/timeline_unit_test.go`
