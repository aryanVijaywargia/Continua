# Architecture Overview

## Monorepo Structure

Continua uses a **Go workspace** (`go.work`) with multiple modules:

```
continua/                 # Root
├── go.work               # Workspace definition
├── cmd/continua/         # Main server binary
├── internal/             # Private packages
├── pkg/                  # Public shared libraries
├── engine/               # Separate module (replay engine)
├── contracts/            # API contracts (OpenAPI, Zod)
├── db/                   # Database schemas + queries
├── web/                  # React frontend
└── sdks/                 # Client SDKs
```

## Request Flow

```
HTTP Request
    ↓
Chi Router (contracts/generated/go/server_gen.go)
    ↓
Handler (internal/api/handlers.go)
    ↓
SQLC Queries (db/gen/go/platform/)
    ↓
PostgreSQL
    ↓
Map to API types (internal/api/mapper.go)
    ↓
JSON Response
```

## Module Boundaries

### `internal/` - Private to this module
- `internal/api/` - HTTP handlers
- `internal/web/` - Static file embedding

### `pkg/` - Shared across modules
- `pkg/infra/` - Database connections, config
- `pkg/idempotency/` - Request deduplication
- `pkg/redaction/` - PII masking

### `engine/` - Separate Go module
- Has own `go.mod`
- Replay and debugging logic
- Separate database schema

## Generated Code Locations

| Source | Generated | Command |
|--------|-----------|---------|
| `contracts/openapi/openapi.yaml` | `contracts/generated/go/server_gen.go` | `make generate` |
| `db/platform/queries/*.sql` | `db/gen/go/platform/*.go` | `make generate` |
| `contracts/openapi/openapi.yaml` | `contracts/generated/typescript/api.ts` | `make generate` |

## Configuration

Config is loaded via `config.yaml` (see `config.example.yaml`):

```yaml
server:
  addr: ":8080"
database:
  url: "postgres://localhost:5432/continua"
```

Access in code via config structs, never `os.Getenv()` directly.

## Frontend Embedding

Web UI is built with Vite, output to `web/dist/`, then:
1. `make build-web` copies to `internal/web/static/`
2. `internal/web/embed.go` embeds via `//go:embed`
3. Single binary serves both API and UI
