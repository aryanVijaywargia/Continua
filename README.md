# Continua

> **Status: Current**
> This README is the repo-verified entrypoint for the current checkout. For the short architecture handoff, use [docs/DEBUGGER_PLATFORM_BASELINE.md](./docs/DEBUGGER_PLATFORM_BASELINE.md). For the doc map and status convention, use [docs/README.md](./docs/README.md).

AI agent observability debugger and operator console.

## Current Product Surface

Continua's implemented product path today is:

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> embedded React debugger operator console
```

The current web surface includes:
- `/` overview built from existing trace and session endpoints
- `/traces` and `/traces/:id` for trace triage and investigation
- `/sessions`, `/sessions/:id`, and `/sessions/:id/compare` for workflow-level investigation
- `/settings` for local API-key and theme controls

## Quick Start

```bash
# Setup
./scripts/setup.sh

# Start development
make dev           # Start Postgres via docker compose
make dev-server    # Start the Go server
make dev-web       # Start the Vite web app

# Open the operator console
# http://localhost:3000
```

## Architecture

- **Backend**: Go 1.22+, Chi router, Fx wiring
- **Frontend**: Vite + React + TypeScript SPA embedded into the Go binary for production
- **Database**: PostgreSQL is the real runtime database
- **Queueing**: River workers run inside the platform server
- **SDKs**: Python is real; TypeScript is still a stub package

Important runtime boundaries:
- `db/platform/migrations/sqlite/` is bootstrap-only scaffolding, not a peer runtime target
- runtime config is env-only via `internal/config/config.go`
- WebSocket runtime, proxy capture, replay runtime, and durable engine execution are not implemented product features today

## Commands

```bash
make generate                  # Contracts, sqlc, copied server types, Python SDK types
make build                     # Go server + web UI
make test                      # Go + JS tests
make lint                      # Go + JS lint
pnpm --filter web test         # Frontend Vitest suites
pnpm --filter web test:e2e     # Playwright UI smoke coverage
make help                      # Show all make targets
```

## Project Structure

```text
continua/
├── cmd/continua/           # Go server binary and CLI
├── contracts/              # OpenAPI and WebSocket contract sources
├── db/platform/            # Postgres migrations and sqlc inputs
├── docs/                   # Current and historical documentation
├── engine/                 # Future durable execution module
├── internal/               # Active platform runtime
├── sdks/python/            # Primary usable SDK
├── sdks/typescript/        # Stub SDK package
├── web/                    # Vite React debugger app
├── Makefile                # Build/test/dev entrypoints
└── pnpm-workspace.yaml     # JS workspace
```

## Architecture Rules

See [docs/architecture/RULES.md](./docs/architecture/RULES.md) for the current anti-drift rules.

## License

MIT
