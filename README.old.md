<div align="center">

<img src="web/public/logo.svg" alt="Continua" width="96" height="96" />

# Continua

**Self-hosted debugging for AI agent runs.**

[Quick Start](#quick-start) | [What You Get](#what-you-get) | [Python SDK](#python-sdk) | [Architecture](#architecture) | [Setup Guide](./docs/setup.md)

</div>

Continua gives agent builders a local operator console for the moments when a run fails, stalls, retries, or behaves differently than expected. Send traces from your agent, keep them durably in Postgres, and inspect the execution with a React debugger served by the Go backend.

It is meant to be easy to clone and run. The recommended first path is Docker Compose: one command starts Postgres, applies migrations, boots the embedded web UI, and seeds a safe demo workspace.

> [!NOTE]
> The public/demo mode uses seeded sample traces only. Use the private local console path when you want to inspect your own traces.

## Quick Start

```bash
git clone https://github.com/continua-ai/continua.git
cd continua
make demo
```

Open <http://localhost:8080>.

`make demo` does the full first-run path:

- builds the Continua Docker image with the embedded React UI
- starts Postgres
- runs database migrations
- starts the Go server
- seeds deterministic sample traces and sessions

Useful follow-up commands:

```bash
curl http://localhost:8080/api/health
make docker-logs
make reset-demo
make docker-down
```

For a deterministic agent-readable runbook, point your coding agent at [docs/setup.md](./docs/setup.md).

## What You Get

- **Failure-first trace debugger**: trace tree, execution waterfall, inspector tabs, payload inspection, state diff, and breadcrumbs.
- **Session investigation**: list sessions, open session detail, and compare two traces from the same workflow.
- **Durable ingest**: authenticated REST ingest, idempotent batches, async processing, and batch polling.
- **Local operator console**: project-scoped API key mode for private local usage.
- **Single server artifact**: the Vite React app is embedded into the Go binary for production-style runs.
- **Python SDK**: helpers for traces, spans, sessions, events, batching, retries, and async ingest polling.

The main app routes are:

| Route | Purpose |
| --- | --- |
| `/` | Landing page |
| `/dashboard` | Overview built from trace and session APIs |
| `/traces`, `/traces/:id` | Trace list and failure investigation |
| `/sessions`, `/sessions/:id` | Session list and workflow detail |
| `/sessions/:id/compare` | Baseline vs. candidate trace comparison |
| `/settings` | Local API key, operator session, and theme controls |

## Python SDK

```bash
pip install continua
```

```python
from continua import Continua

client = Continua(
    api_key="default",
    endpoint="http://localhost:8080",
    ingest_mode="server_default",  # or "sync", "async_v2"
)

with client.trace(name="agent-run") as trace:
    with trace.span(name="plan") as span:
        span.set_input({"goal": "summarize doc"})
        span.set_output({"plan": ["read", "summarize"]})
```

> [!IMPORTANT]
> True async ingest is not read-after-write. If your code reads ingested data immediately after writing it, use `ingest_mode="sync"` or call `client.wait_for_batch(batch_id)` before reading.

See [sdks/python/README.md](./sdks/python/README.md) for full SDK usage.

## Native Development

Use the native path when you are changing Go, React, contracts, or the Python SDK.

```bash
./scripts/setup.sh
make dev

export DATABASE_URL="postgres://continua:continua@localhost:5432/continua?sslmode=disable"
make migrate
make dev-server
```

In another terminal:

```bash
make dev-web
```

Open <http://localhost:3000>. The backend stays on <http://localhost:8080>.

`./scripts/setup.sh` verifies that Go, Node.js, pnpm, Docker, Python, and uv are already installed, then installs repo dependencies and local Go developer tools.

## Architecture

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> embedded React debugger operator console
```

| Layer | Stack |
| --- | --- |
| Backend | Go 1.24+, Chi router, Fx wiring, River workers |
| Database | PostgreSQL, sqlc-backed store |
| Contracts | OpenAPI 3 plus generated Go, TypeScript, and Python types |
| Frontend | Vite, React, TypeScript, TanStack Query |
| SDKs | Python active; TypeScript currently stubbed |

Source-of-truth files:

- REST contract: `contracts/openapi/openapi.yaml`
- WebSocket schema contract: `contracts/websocket/events.ts`
- Platform schema and queries: `db/platform/migrations/postgres/`, `db/platform/queries/`
- Current runtime behavior: `cmd/continua`, `internal/`, `web/`, `sdks/python/`

Run `make generate` after changing contracts, sqlc queries, or migrations that affect generated types.

> [!WARNING]
> WebSocket runtime, proxy capture, replay runtime, durable engine execution, and TypeScript SDK parity are scaffolded or future-facing in this checkout. Treat the React debugger and Python SDK as the active product surface.

## Common Commands

```bash
make demo                      # Docker-first demo: build, migrate, serve, seed
make reset-demo                # Remove Docker demo volume and reseed
make docker-logs               # Follow Docker service logs
make docker-down               # Stop Docker services
make setup                     # Install native repo dependencies
make generate                  # Regenerate contracts, sqlc, server types
make build                     # Build Go server and embedded web UI
make test                      # Run Go and JS tests
make lint                      # Run Go and JS linters
pnpm --filter web test         # Frontend Vitest suites
cd sdks/python && uv run pytest
```

## Documentation

- [docs/setup.md](./docs/setup.md): canonical setup guide for humans and agents
- [docs/README.md](./docs/README.md): documentation map and status convention
- [docs/architecture/overview.md](./docs/architecture/overview.md): runtime architecture overview
- [docs/architecture/RULES.md](./docs/architecture/RULES.md): anti-drift architecture rules
- [openspec/](./openspec): active and implemented change proposals
