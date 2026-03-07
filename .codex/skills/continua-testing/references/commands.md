# Test Commands

## Make Targets (Repo Root)
- `make test` (Go + JS)
- `make test-go` (Go + engine, `-race`)
- `make test-js` (workspace tests via pnpm)
- `make test-integration` (requires DB)

## SDKs
- TypeScript: `pnpm -C sdks/typescript test`
- Python: `cd sdks/python && uv run pytest`
