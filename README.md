<p align="center">
  <img src="./assets/banner.svg" alt="Continua — self-hosted debugging for AI agent runs" />
</p>

<div align="center">

  # Continua

  **Self-hosted debugging for AI agent runs.**

  <p>
    <a href="#quickstart"><b>Quickstart</b></a> ·
    <a href="#why-continua"><b>Why</b></a> ·
    <a href="#features"><b>Features</b></a> ·
    <a href="#architecture"><b>Architecture</b></a> ·
    <a href="#python-sdk"><b>Python SDK</b></a> ·
    <a href="#rest-api-surface"><b>REST API</b></a> ·
    <a href="./docs/setup.md"><b>Docs</b></a>
  </p>

  <p>
    <a href="./docs/setup.md">Setup guide</a> ·
    <a href="./docs/DEBUGGER_PLATFORM_BASELINE.md">Baseline</a> ·
    <a href="./docs/architecture/overview.md">Architecture overview</a> ·
    <a href="./docs/event-conventions.md">Event conventions</a> ·
    <a href="./openspec">OpenSpec</a> ·
    <a href="https://github.com/continua-ai/continua/issues">Issues</a>
  </p>

  <p>
    <a href="./LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-0EA5E9?style=flat-square"></a>
    <img alt="Go 1.24+" src="https://img.shields.io/badge/go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white">
    <img alt="Node 20+" src="https://img.shields.io/badge/node-20%2B-339933?style=flat-square&logo=node.js&logoColor=white">
    <img alt="pnpm 9+" src="https://img.shields.io/badge/pnpm-9%2B-F69220?style=flat-square&logo=pnpm&logoColor=white">
    <img alt="Python 3.10+" src="https://img.shields.io/badge/python-3.10%2B-3776AB?style=flat-square&logo=python&logoColor=white">
    <img alt="Postgres 16+" src="https://img.shields.io/badge/postgres-16%2B-4169E1?style=flat-square&logo=postgresql&logoColor=white">
    <img alt="OpenAPI 3" src="https://img.shields.io/badge/openapi-3-6BA539?style=flat-square&logo=openapiinitiative&logoColor=white">
    <img alt="Docker demo" src="https://img.shields.io/badge/demo-Docker%20Compose-2496ED?style=flat-square&logo=docker&logoColor=white">
    <img alt="Status: alpha" src="https://img.shields.io/badge/status-alpha-F59E0B?style=flat-square">
  </p>

  <p>
    <a href="https://github.com/continua-ai/continua/stargazers"><img alt="GitHub stars" src="https://img.shields.io/github/stars/continua-ai/continua?style=flat-square&color=2DD4BF"></a>
    <a href="https://github.com/continua-ai/continua/commits/main"><img alt="Last commit" src="https://img.shields.io/github/last-commit/continua-ai/continua?style=flat-square&color=8B5CF6"></a>
    <a href="https://github.com/continua-ai/continua/issues"><img alt="Open issues" src="https://img.shields.io/github/issues/continua-ai/continua?style=flat-square&color=F43F5E"></a>
  </p>

</div>

---

Continua is a **local operator console for debugging AI agent executions**. It accepts traces over an authenticated REST ingest API, persists them in Postgres, processes async work with River, and serves a React debugger from the Go backend — in a single binary, on your own infrastructure.

The current product is intentionally concrete: trace runs, inspect spans and payloads, compare session attempts, and keep enough durable state to understand why an agent failed, stalled, retried, or behaved differently than expected.

> [!NOTE]
> Public demo mode uses seeded sample traces only. Use the private local console path when you want to ingest and inspect your own traces.

## At a glance

| Shipped today | Scaffolded / future-facing |
| --- | --- |
| Authenticated REST ingest (`POST /v1/ingest`) | Live WebSocket runtime (`internal/ws`) |
| Idempotent batches, sync + durable async paths | Proxy capture (`internal/proxy`) |
| Postgres persistence (sqlc-backed store) | Replay execution runtime (`internal/replay`) |
| River workers: ingest, rollups, payload cleanup | Feature-complete TypeScript SDK (`sdks/typescript`) |
| Trace, session, timeline, and compare read APIs | Durable engine workflow execution (`engine/`) |
| Embedded React debugger (failure-first) | Score/eval APIs, alerts, telemetry |
| Python SDK (traces, spans, sessions, batching, async polling) | |
| Engine schema/store/CLI **foundation only** | |

Source of truth for this boundary: [`docs/DEBUGGER_PLATFORM_BASELINE.md`](./docs/DEBUGGER_PLATFORM_BASELINE.md).

## Quickstart

The recommended first run is Docker Compose. It does not require local Go, Node, pnpm, Python, or uv.

```bash
git clone https://github.com/continua-ai/continua.git
cd continua
make demo
```

Open:

```text
http://localhost:8080
```

Check the server:

```bash
curl http://localhost:8080/api/health
```

Useful demo commands:

```bash
make docker-logs      # follow service logs
make reset-demo       # reset demo data and reseed
make docker-down      # stop the Docker stack
```

`make demo` builds the Continua image, starts Postgres, runs migrations, starts the Go server with the embedded web UI, and seeds deterministic sample traces and sessions.

## Why Continua

Agent failures are rarely a single log line. The useful context is usually spread across model calls, tool calls, retries, state changes, branch decisions, and session-level attempts. Continua gives that context a durable home and a debugger built for investigation, not just dashboards.

Use it when you need to answer:

| Question | Where Continua helps |
| --- | --- |
| What failed first? | Failure-first trace detail, span tree, waterfall, and timeline |
| What changed during the run? | State diff and semantic event surfaces |
| Why did this branch happen? | `decision` event inspection with reasoning |
| How did two attempts differ? | Session detail and trace comparison (`/sessions/:id/compare`) |
| Was ingest processed yet? | Sync ingest, async batch status, and polling helpers |

## Features

![Continua features at a glance](./assets/diagrams/feature-strip.svg)

- **Trace debugger** — span tree, execution waterfall, selected spans, payload inspection, breadcrumbs, truncation banners, and merged timeline events.
- **Session workflows** — browse sessions, open session detail, and compare baseline vs. candidate traces from the same workflow.
- **Durable ingest path** — project-scoped API key auth, idempotent batches via `batch_key`, sync ingest, opt-in async ingest (`X-Continua-Async-Version: 2`), and batch polling.
- **Background processing** — River workers handle async ingest, trace rollups, and payload cleanup.
- **Embedded operator console** — the Vite React app is built into `internal/web/static/` and served by the Go binary.
- **Python SDK** — `trace`, `span`, `session`, `event`, batching, retries, async polling, and engine-control helpers live under `sdks/python`.

Current debugger routes (sourced from [`web/src/App.tsx`](./web/src/App.tsx)):

| Route | Purpose |
| --- | --- |
| `/` | Landing page |
| `/dashboard` | Overview built from trace and session APIs |
| `/traces` | Trace search, filters, sorting, and pagination |
| `/traces/:id` | Failure-first trace investigation |
| `/sessions` | Session list |
| `/sessions/:id` | Session detail and trace drill-down |
| `/sessions/:id/compare` | Baseline vs. candidate trace comparison |
| `/settings` | API key, operator session, and theme controls |

## Architecture

![Continua runtime architecture](./assets/diagrams/runtime-architecture.svg)

```mermaid
flowchart LR
  client["Python SDK or custom client"]
  ingest["Authenticated REST ingest"]
  postgres[("Postgres")]
  jobs["River background jobs"]
  readapi["REST read APIs"]
  ui["Embedded React debugger"]

  client --> ingest --> postgres
  postgres --> jobs --> postgres
  postgres --> readapi --> ui
```

| Layer | Current implementation |
| --- | --- |
| Backend | Go 1.24+, Chi router, Fx wiring, OpenAPI-backed handlers |
| Database | PostgreSQL 16+, sqlc-backed store wrappers |
| Background jobs | River workers for async ingest, rollups, and retention cleanup |
| Contracts | OpenAPI 3, plus generated Go, TypeScript, and Python types |
| Frontend | Vite, React, TypeScript, TanStack Query |
| SDKs | Python active; TypeScript stub only |

Source-of-truth files:

- REST contract: [`contracts/openapi/openapi.yaml`](./contracts/openapi/openapi.yaml)
- WebSocket schema contract: [`contracts/websocket/events.ts`](./contracts/websocket/events.ts)
- Platform schema and queries: [`db/platform/migrations/postgres`](./db/platform/migrations/postgres/) and [`db/platform/queries`](./db/platform/queries/)
- Runtime behavior: [`cmd/continua`](./cmd/continua/), [`internal`](./internal/), [`web`](./web/), and [`sdks/python`](./sdks/python/)

## How it works

![Continua ingest flow](./assets/diagrams/ingest-flow.svg)

```mermaid
flowchart LR
  client["SDK / REST client"]
  auth["API key auth"]
  validate["Validate payload"]
  batch["Claim idempotent batch"]
  sync["Sync write path"]
  async["Async accepted batch"]
  jobs["River ingest worker"]
  store[("Postgres traces, spans, events")]
  poll["Batch polling"]
  debugger["Debugger read APIs"]

  client --> auth --> validate --> batch
  batch --> sync --> store
  batch --> async --> jobs --> store
  async --> poll
  store --> debugger
```

1. A Python SDK or custom client posts trace data to `POST /v1/ingest`.
2. API key auth resolves the project scope before any protected write.
3. Validation and idempotent batch handling choose the sync path or async acceptance path.
4. Postgres stores projects, batches, sessions, traces, spans, and span events.
5. River workers process async batches, compute trace rollups, and run cleanup jobs.
6. The debugger reads traces, sessions, spans, timelines, and comparisons through REST APIs.

Trace detail uses REST polling against `GET /api/traces/{id}/events` today. There is no live WebSocket runtime in the current checkout.

## Data model

![Continua data model](./assets/diagrams/data-model.svg)

```mermaid
erDiagram
  projects ||--o{ ingest_batches : owns
  projects ||--o{ sessions : owns
  projects ||--o{ traces : owns
  sessions ||--o{ traces : groups
  traces   ||--o{ spans : contains
  spans    ||--o{ span_events : emits
```

External identifiers (`external_id`, `trace_id`, `span_id`, `parent_span_id`) are the IDs SDKs send and the debugger surfaces in URLs. Timeline responses merge stored explicit events with synthetic lifecycle markers derived from spans. See [`docs/architecture/overview.md`](./docs/architecture/overview.md) for the full storage model.

## Event semantics

![Implemented event types](./assets/diagrams/event-types.svg)

Continua events are **debugger-facing observability signals**, not replay primitives. The debugger renders eleven implemented event types — `log`, `error`, `exception`, `message`, `metric`, `custom`, `state_change`, `decision`, `effect`, `wait`, and `snapshot_marker` — each with a documented payload shape. See [`docs/event-conventions.md`](./docs/event-conventions.md) for when to use each type and the expected payload fields.

## Python SDK

Install:

```bash
pip install continua
```

Create a trace:

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

See [`sdks/python/README.md`](./sdks/python/README.md) for SDK-specific usage and development commands.

## REST API surface

The full REST contract lives in [`contracts/openapi/openapi.yaml`](./contracts/openapi/openapi.yaml). Current operations:

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/v1/ingest` | Batch ingest traces, spans, and events (sync or async) |
| `GET` | `/v1/ingest/batches/{id}` | Poll the processing status of an ingest batch |
| `GET` | `/api/traces` | List traces with filters, sort, and pagination |
| `GET` | `/api/traces/{id}` | Get a single trace |
| `GET` | `/api/traces/{id}/spans` | List spans for a trace |
| `GET` | `/api/traces/{id}/events` | Get the merged timeline for a trace |
| `GET` | `/api/sessions` | List sessions |
| `GET` | `/api/sessions/{id}` | Get a single session |
| `GET` | `/api/sessions/{id}/compare` | Compare baseline vs. candidate traces in a session |
| `GET` | `/api/health` | Health check (routed directly, not in OpenAPI) |

All protected routes require a project-scoped API key. The 5 MB request-body cap applies to `/v1/ingest`.

## Engine foundation

[`engine/`](./engine/) is an isolated Go module shipping the durable-execution **foundation only**:

- a dedicated Postgres `engine` schema with reversible migrations
- sqlc-backed query generation
- an engine-local store with its own pgx pool
- a `continua-engine` CLI with `version`, `migrate up`, and `migrate down <steps>`

What this does **not** include yet: workflow execution, history replay, activity workers, public execution APIs, or a debugger UI for engine state. Treat the `engine/` schema as plumbing, not product surface. See [`engine/README.md`](./engine/README.md) for details.

## Project structure

```text
cmd/continua                     Cobra entrypoint for serve, migrate, and version
internal/api                     OpenAPI-backed handlers, router, auth, and mappers
internal/ingest                  Ingest DTOs, validation, orchestration, and write path
internal/jobs                    River workers for async ingest, rollups, and cleanup
internal/store                   sqlc-backed store wrappers and focused handwritten SQL
internal/config                  Env-only runtime configuration
internal/web                     Embedded React app handler and static assets
contracts                        OpenAPI, WebSocket schema, and generated contract types
db/platform                      Postgres migrations, sqlc queries, and generated DB code
web                              Vite React debugger UI
sdks/python                      Active Python SDK
sdks/typescript                  Early TypeScript SDK stub
engine                           Isolated engine schema/store/CLI foundation
docs                             Current docs, historical context, diagrams, and setup guides
openspec                         Active and implemented change proposals
```

## Configuration

The platform server is configured with environment variables in [`internal/config/config.go`](./internal/config/config.go).

Required:

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | Postgres connection string |

Common optional variables:

| Variable | Purpose |
| --- | --- |
| `HOST`, `PORT` | Server bind address, defaulting to `0.0.0.0:8080` |
| `PUBLIC_DEMO_ENABLED` | Enables the read-only seeded demo workspace |
| `PUBLIC_DEMO_PROJECT_ID` | Project shown in public demo mode |
| `PUBLIC_DEMO_LABEL` | Demo workspace label in the UI |
| `INGEST_TRUE_ASYNC_DEFAULT` | Server default async ingest behavior |
| `INGEST_DEPENDENCY_RETRY_WINDOW` | Retry window for dependency-sensitive ingest |
| `INGEST_FAILED_PAYLOAD_RETENTION` | Retention for failed ingest payloads |
| `RIVER_QUEUE_*` | River worker pool sizing |

> [!WARNING]
> `config.example.yaml` is not the runtime contract for the current server. The live config source is environment variables read by `internal/config/config.go`.

## Native development

Use the native path when changing Go, React, contracts, database queries, or SDK code.

Prerequisites:

- Go 1.24+
- Node.js 20+
- pnpm 9+
- Docker with Docker Compose v2
- Python 3.10+
- uv

Start the native stack:

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

Open:

```text
http://localhost:3000/dashboard
```

For a fresh local database, use API key `default` when the UI asks for a project API key.

Regenerate code after changing contracts, sqlc queries, or migrations that affect generated types:

```bash
make generate
```

## Testing

Targeted checks:

```bash
go test ./internal/api/...
go test ./internal/ingest/...
go test ./internal/store/...
go test ./internal/jobs/...
pnpm --filter web test
pnpm --filter web test:e2e
cd sdks/python && uv run pytest
```

Full local validation:

```bash
make generate
make lint
make test
```

## Current boundary and roadmap signals

Implemented and active:

- Authenticated REST ingest
- Postgres persistence
- River-backed background jobs
- Trace, session, timeline, and compare read APIs
- Embedded React debugger operator console
- Python SDK
- Engine schema/store/CLI foundation

Scaffolded or future-facing in this checkout:

- Live WebSocket runtime
- Proxy capture
- Replay execution runtime
- Feature-complete TypeScript SDK
- Durable engine workflow execution and public debugger runtime

These roadmap signals come from the current baseline docs, source tree, and TODOs — they are not described here as shipped product behavior.

## Documentation

- [`docs/setup.md`](./docs/setup.md) — canonical setup guide for humans and agents
- [`docs/DEBUGGER_PLATFORM_BASELINE.md`](./docs/DEBUGGER_PLATFORM_BASELINE.md) — repo-verified current platform baseline
- [`docs/architecture/overview.md`](./docs/architecture/overview.md) — runtime architecture overview
- [`docs/architecture/RULES.md`](./docs/architecture/RULES.md) — anti-drift architecture rules
- [`docs/event-conventions.md`](./docs/event-conventions.md) — debugger-facing event semantics
- [`docs/README.md`](./docs/README.md) — documentation map and status convention
- [`openspec/`](./openspec/) — active and implemented change proposals

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md). The short version:

```bash
make generate
make lint
make test
```

Documentation follows the status convention in [`docs/README.md`](./docs/README.md): current docs are authoritative for the checkout, while historical phase docs are context only.

<a href="https://github.com/continua-ai/continua/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=continua-ai/continua" alt="Continua contributors" />
</a>

## License

Continua is released under the [MIT License](./LICENSE).

---

<sub>Made with Go · Chi · Fx · Postgres · River · sqlc · React · Vite · TanStack Query · OpenAPI.</sub>
