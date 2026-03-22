---
name: continua-testing
description: Testing strategy for Continua (Go, engine, web, SDKs). Use when adding features, fixing bugs, or changing API/DB behavior; triggers on go test, make test, integration tests, pnpm test, or SDK test updates.
---

# Continua Testing

## Read first
- [../references/decisions.md](../references/decisions.md)
- [references/strategy.md](references/strategy.md)
- [references/commands.md](references/commands.md)

## Use this skill when
- adding or fixing behavior in Go, web, or SDK code
- changing contracts, migrations, store logic, ingest behavior, or UI state handling
- deciding which suites to run before stopping

## Current testing conventions
- Keep tests beside the code they verify.
- Many backend tests are real-DB tests using `internal/testutil`.
- In `internal/api`, test files mirror the feature split:
  - `ingest_handlers_test.go`
  - `traces_handlers_test.go`
  - `sessions_handlers_test.go`
  - `server_helpers_test.go`
  - `timeline_unit_test.go`
- Web UI uses Vitest and Testing Library under `web/src`.
- Python SDK uses pytest under `sdks/python/tests`.
- The TypeScript SDK currently has only minimal stub coverage.

## Default test selection
- contract or mapper change -> `go test ./internal/api/...`
- ingest or job change -> `go test ./internal/ingest/... ./internal/jobs/...`
- SQL/store change -> `go test ./internal/store/...`
- web UI change -> `pnpm --filter web test`
- Python SDK change -> `cd sdks/python && uv run pytest`
- cross-layer change -> `make test`
