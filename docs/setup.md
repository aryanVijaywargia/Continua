# Continua Setup Guide

> **Status: Current**
> This is the canonical setup guide for humans and coding agents. Prefer this over older phase docs when bringing up the repo.

## Recommended Path: Docker Demo

Use this path for first contact, demos, and agent-driven setup. It does not require a local Go, Node, pnpm, Python, or uv installation.

Prerequisites:

- Git
- Docker with Docker Compose v2
- A free local port `8080`

Run:

```bash
git clone https://github.com/aryanVijaywargia/Continua.git
cd continua
make demo
```

Expected result:

- Postgres is running in Docker.
- Continua has applied platform migrations.
- The Go server is running on `http://localhost:8080`.
- The embedded React UI is served by the Go server.
- Demo traces and sessions are visible in the public demo workspace.

Open:

```text
http://localhost:8080
```

Health check:

```bash
curl http://localhost:8080/api/health
```

Useful Docker commands:

```bash
make docker-logs      # follow service logs
make docker-down      # stop the demo stack
make reset-demo       # delete Docker data volume, rebuild, migrate, and reseed
```

## What `make demo` Does

`make demo` runs `scripts/demo.sh`, which performs the complete first-run flow:

1. Builds the Docker image from `deploy/docker/Dockerfile`.
2. Starts `postgres` and `continua` from `deploy/docker-compose/docker-compose.yml`.
3. Runs `continua migrate up` before the server starts.
4. Starts `continua serve` with public demo mode enabled.
5. Runs the `demo-seed` service, which resets a deterministic demo project and emits sample traces through the ingest API.

Demo project defaults:

| Setting | Value |
| --- | --- |
| Project ID | `11111111-1111-4111-8111-111111111111` |
| Demo API key used by seeding | `continua-demo-local` |
| App URL | `http://localhost:8080` |
| Postgres URL inside Docker | `postgres://continua:continua@postgres:5432/continua?sslmode=disable` |

The public demo UI is read-only and uses seeded sample traces. It is safe to explore without entering an API key.

## Private Local Console

Use this path when you want to ingest and inspect your own local traces. This path uses the native dev server and Vite for fast frontend iteration.

Prerequisites:

- Go 1.24+
- Node.js 20+
- pnpm 9+
- Docker
- Python 3.10+
- uv

Run:

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

Emit sample private traces:

```bash
cd sdks/python
CONTINUA_API_URL="http://localhost:8080" \
CONTINUA_API_KEY="default" \
uv run python examples/e2e_demo.py
```

## Ports

| Port | Used by | Path |
| --- | --- | --- |
| `8080` | Go server and embedded UI in Docker demo | `make demo` |
| `5432` | Postgres | Docker demo and native `make dev` |
| `3000` | Vite dev server | native `make dev-web` |

## Runtime Configuration

The server uses environment variables through `internal/config/config.go`.

Required:

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | Postgres connection string |

Common optional variables:

| Variable | Purpose |
| --- | --- |
| `HOST`, `PORT` | Server bind address, defaults to `0.0.0.0:8080` |
| `PUBLIC_DEMO_ENABLED` | Enables read-only seeded demo mode |
| `PUBLIC_DEMO_PROJECT_ID` | Project shown in public demo mode |
| `PUBLIC_DEMO_LABEL` | Demo workspace label in the UI |
| `INGEST_TRUE_ASYNC_DEFAULT` | Server default async ingest behavior |
| `RIVER_QUEUE_*` | River worker pool sizes |

Do not use `config.example.yaml` as the runtime contract. It is forward-looking and not the active config source.

## Troubleshooting

Port `8080` is already in use:

```bash
lsof -nP -iTCP:8080
make docker-down
```

Docker data looks stale:

```bash
make reset-demo
```

The app is up but no traces appear:

```bash
make docker-logs
docker compose -f deploy/docker-compose/docker-compose.yml --profile seed run --rm demo-seed
```

Native server fails with `DATABASE_URL environment variable is required`:

```bash
export DATABASE_URL="postgres://continua:continua@localhost:5432/continua?sslmode=disable"
make migrate
make dev-server
```

Native UI cannot load data:

- Confirm the backend is running on `http://localhost:8080`.
- Confirm Vite is running on `http://localhost:3000`.
- Confirm the UI has API key `default` in `/settings`.

## Current Product Boundary

Implemented setup path:

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> embedded React debugger operator console
```

The TypeScript SDK, live WebSocket runtime, proxy capture, replay runtime, and durable engine execution are not the main local setup path today.
