# Test Commands

## Make Targets (Repo Root)
- `make test` (Go + JS)
- `make test-go` (Go + engine, `-race`)
- `make test-js` (workspace tests via pnpm)
- `make test-integration` (requires DB)
- `make lint`
- `make generate`

## SDKs
- Web UI: `pnpm --filter web test`
- Web UI smoke: `pnpm --filter web test:e2e`
- Web UI type-check: `pnpm --filter web type-check`
- TypeScript SDK: `pnpm --filter @continua/sdk test`
- Python: `cd sdks/python && uv run pytest`

## Common targeted Go suites
- `go test ./internal/api/...`
- `go test ./internal/ingest/...`
- `go test ./internal/jobs/...`
- `go test ./internal/store/...`
