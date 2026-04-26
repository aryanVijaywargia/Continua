# Continua

> **Status: Current**
> This README is the repo-verified entrypoint for the current checkout. For the short architecture handoff, use [docs/DEBUGGER_PLATFORM_BASELINE.md](./docs/DEBUGGER_PLATFORM_BASELINE.md). For the doc map and status convention, use [docs/README.md](./docs/README.md).

AI agent observability debugger and operator console.

## Hosted Demo Vs Local Self-Host

Continua now supports two distinct deployment modes:

- **Public portfolio demo**: public landing page plus a read-only seeded debugger demo
- **Private operator console**: authenticated debugger for real traces and sessions

The hosted portfolio site is intentionally sample-data only. For real usage with your own traces, run Continua locally or deploy a separate private environment.

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
- `/` landing page
- `/dashboard` overview built from existing trace and session endpoints
- `/traces` and `/traces/:id` for trace triage and investigation
- `/sessions`, `/sessions/:id`, and `/sessions/:id/compare` for workflow-level investigation
- `/settings` for operator session and theme controls

## Quick Start

```bash
# Setup
./scripts/setup.sh

# Start development
make dev           # Start Postgres via docker compose
make dev-server    # Start the Go server
make dev-web       # Start the Vite web app
make seed-demo     # Rebuild the seeded public demo project through ingest

# Read the local run guide
# docs/guides/run-locally.md
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
make seed-demo                 # Reset and repopulate the public demo dataset
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
