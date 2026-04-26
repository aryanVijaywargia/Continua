# Run Continua Locally

This guide covers the two local workflows that matter now:

1. a **public-demo-style** local run with seeded sample data
2. a **private self-hosted** run for your own traces

The hosted portfolio site is intentionally a read-only sample environment. Real usage stays local or in a separate private deployment.

## 1. Prerequisites

```bash
./scripts/setup.sh
make dev
```

That installs Go, Node, and Python SDK dependencies, then starts the local Postgres instance through Docker Compose.

## 2. Start a local public demo

Use this when you want the same read-only sample experience that the hosted portfolio demo exposes.

```bash
export DATABASE_URL="postgres://continua:continua@localhost:5432/continua?sslmode=disable"
export PUBLIC_DEMO_ENABLED=1
export PUBLIC_DEMO_PROJECT_ID="11111111-1111-4111-8111-111111111111"
export PUBLIC_DEMO_LABEL="Sample data"

make migrate
make dev-server
```

In another terminal:

```bash
make dev-web
make seed-demo
```

Then open:

- `http://localhost:3000/` for the landing page
- `http://localhost:3000/dashboard` for the seeded debugger demo

The demo seed is rerunnable. `make seed-demo` recreates the dedicated demo project and repopulates it through the ingest API.

## 3. Start a private local console for your own traces

Use this when you want to inspect real local data instead of the public demo dataset.

1. Unset the public demo variables:

```bash
unset PUBLIC_DEMO_ENABLED
unset PUBLIC_DEMO_PROJECT_ID
unset PUBLIC_DEMO_LABEL
```

2. Start the backend and web app:

```bash
export DATABASE_URL="postgres://continua:continua@localhost:5432/continua?sslmode=disable"
make migrate
make dev-server
make dev-web
```

3. Open `http://localhost:3000/dashboard` and enter a project API key when prompted.

For a fresh local database, the default seed project API key is `default`. The key is stored only in your browser's local storage and is sent as a bearer token to the local server. Public demo mode never uses or exposes this browser key.

Auth0 is optional for a separate private/team deployment. Configure it only when you intentionally want hosted operator sign-in:

```bash
export AUTH0_DOMAIN="your-tenant.us.auth0.com"
export AUTH0_CLIENT_ID="your_spa_client_id"
export AUTH0_AUDIENCE="https://continua/api"
export AUTH0_ALLOWED_EMAILS="you@example.com"
```

## 4. Ingest sample data locally

The Python SDK example remains the quickest way to push traces through the real ingest path:

```bash
cd sdks/python
CONTINUA_API_URL="http://localhost:8080" \
CONTINUA_API_KEY="default" \
uv run python examples/e2e_demo.py
```

That example emits:

- a healthy research-style trace
- a nested multi-step review trace
- a failing trace with timeline error detail
- multi-trace session data for compare flows

## 5. Useful commands

```bash
make generate
pnpm --filter web test
pnpm --filter web test:e2e
go test ./internal/api/...
go test ./internal/store/...
```

## 6. Deployment split

- **Hosted portfolio site:** public landing page + read-only seeded demo
- **Private deployment:** authenticated operator console for real traces
- **Local self-host:** the path for your own data during development
