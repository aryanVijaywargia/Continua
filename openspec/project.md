# Project Context

## Purpose

Continua is an **AI Agent Observability Platform** for debugging AI agents by capturing and replaying execution traces. It provides:

- **Trace Capture**: Record agent execution flows, tool calls, and LLM interactions
- **Trace Replay**: Debug and analyze agent behavior by replaying captured sessions
- **Real-time Monitoring**: WebSocket-based live updates for running agent sessions
- **SDK Integration**: Drop-in SDKs for TypeScript and Python to instrument AI agents

## Tech Stack

### Backend
- **Go 1.22+** - Primary server language
- **Chi** - HTTP router
- **Uber Fx** - Dependency injection
- **pgx/v5** - PostgreSQL driver
- **sqlc** - Type-safe SQL code generation
- **golang-migrate** - Database migrations
- **Cobra** - CLI framework

### Frontend
- **Vite** - Build tool
- **React 18** - UI framework
- **TypeScript 5.6** - Type safety
- **TanStack Query** - Data fetching/caching
- **Tailwind CSS** - Styling

### Database
- **PostgreSQL** - Primary database
- **SQLite** - Local development option

### SDKs
- **TypeScript SDK** - tsup (bundler), vitest (testing)
- **Python SDK** - uv (package manager), pytest, pydantic

## Project Conventions

### Code Style

**Go:**
- Standard `gofmt` formatting
- Generated files use `_gen.go` suffix (except sqlc output)
- Package names match directory names
- Domain types never leak into API responses - use mappers

**TypeScript/React:**
- ESLint + Prettier for formatting
- Functional components with hooks
- TanStack Query for server state

**General:**
- Prefer explicit over implicit
- Keep functions focused and small
- Document non-obvious behavior

### Architecture Patterns

**Contract-First Development:**
1. OpenAPI spec (`contracts/openapi/openapi.yaml`) is source of truth for REST API
2. Zod schemas (`contracts/websocket/events.ts`) define WebSocket events
3. `make generate` regenerates all derived code
4. CI fails if generated code is out of sync

**Module Boundaries:**
```
cmd/continua/        → Server CLI entrypoint (Cobra)
contracts/           → API contracts (SOURCE OF TRUTH)
internal/            → Server internals (not importable externally)
pkg/                 → Shared public packages
engine/              → ISOLATED Go module (cannot import internal/)
db/platform/         → Migrations + sqlc queries
web/                 → Vite React SPA (embedded in Go binary)
sdks/                → TypeScript and Python SDKs
```

**10 Architecture Rules:**
1. `make generate` is the ONE command for all code generation
2. Contracts are source of truth — never hand-edit generated files
3. Generated Go files use `_gen.go` suffix (except sqlc output)
4. Track generated code in git, not build artifacts
5. Module boundaries enforced by Go — engine/ cannot import internal/
6. Platform and Engine have separate schemas
7. Web UI is static-only — no SSR, embedded in Go binary
8. One lockfile at root (pnpm-workspace)
9. CI drift check is gatekeeper — fails if generated code differs
10. Domain types never leak into API responses — use mappers

### Testing Strategy

- **Go tests**: Run with race detector (`make test-go`)
- **Integration tests**: Require running database (`make test-integration`)
- **TypeScript tests**: Vitest for SDK and frontend tests
- **Python tests**: pytest for SDK tests
- **Pre-commit validation**: `make ci` runs full pipeline locally

### Git Workflow

- Feature branches off `main`
- Run `make ci` before pushing
- Commit messages should be descriptive without Claude signature
- Generated files should be committed (not gitignored)

## Domain Context

**Core Concepts:**
- **Session**: A logical grouping of traces (e.g., a user conversation)
- **Trace**: A single execution flow within a session
- **Span**: A unit of work within a trace (LLM call, tool invocation, etc.)
- **Payload**: Request/response bodies stored for spans

**Span Hierarchy:**
- Spans form a tree structure (parent/child relationships)
- Root spans have no parent
- Child spans represent nested operations

**Real-time Updates:**
- WebSocket connections push live span data
- Clients can subscribe to specific sessions/traces

## Important Constraints

**Technical:**
- Generated code must never be manually edited
- Engine module cannot import from internal/ (Go enforced boundary)
- Web UI must be static (no SSR) for Go binary embedding
- All contract changes require `make generate` before commit

**Operational:**
- CI drift check gates all merges
- Database migrations must be backward compatible
- SDK changes require version bumps

## External Dependencies

**Infrastructure:**
- PostgreSQL database (primary store)
- Docker for local development

**AI Provider Integrations (via SDKs):**
- OpenAI API (peer dependency in TS SDK)
- Anthropic API (peer dependency in TS SDK)
