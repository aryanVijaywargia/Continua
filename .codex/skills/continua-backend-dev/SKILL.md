---
name: continua-backend-dev
description: Backend development guide for Continua's Go monorepo. Use when creating handlers, services, database queries, migrations, or working with the API layer. Covers contract-first development, SQLC queries, Chi routing, Cobra CLI, and Continua's 10 architecture rules.
---

# Continua Backend Development

## Purpose

Establish consistency for backend development in Continua - the AI agent observability platform built with Go, PostgreSQL, and contract-first API design.

## Shared reference

- Read [../../references/decisions.md](../../references/decisions.md) for source-of-truth files, generated-file boundaries, and architecture references before cross-cutting changes.

## When to Use This Skill

Activates when working on:
- Go handlers or services in `internal/` or `cmd/`
- Database migrations or SQLC queries in `db/`
- OpenAPI contracts in `contracts/`
- CLI commands with Cobra

---

## Quick Start Checklist

### New API Endpoint
- [ ] Add to `contracts/openapi/openapi.yaml`
- [ ] Run `make generate`
- [ ] Implement handler in `internal/api/`
- [ ] Add SQLC query if needed
- [ ] Map domain types to API types (never expose DB types)

### New Database Query
- [ ] Add to `db/platform/queries/*.sql` with SQLC annotations
- [ ] Run `make generate`
- [ ] Use generated `*platform.Queries` in handlers

### New Migration
- [ ] `make migrate-create name=your_migration`
- [ ] Edit `db/platform/migrations/postgres/*.sql`
- [ ] Run `make migrate`

---

## Project Structure

```
cmd/continua/           # Server entrypoint (Cobra CLI)
internal/
├── api/                # HTTP handlers + generated server
│   ├── server_gen.go   # GENERATED - DO NOT EDIT
│   └── mapper.go       # Domain → API type mapping
├── web/                # Static file serving
contracts/
├── openapi/            # OpenAPI spec (source of truth)
├── websocket/          # WebSocket event schemas (Zod)
├── generated/          # Generated Go + TS types
db/
├── platform/
│   ├── migrations/     # SQL migrations (postgres/sqlite)
│   └── queries/        # SQLC query definitions
├── gen/go/platform/    # GENERATED SQLC types
engine/                 # Separate Go module (replay engine)
pkg/                    # Shared libraries
web/                    # React frontend (Vite)
sdks/                   # Python + TypeScript SDKs
```

---

## The 10 Architecture Rules

1. **ONE generation command**: `make generate` - CI fails on drift
2. **Contracts are source of truth**: OpenAPI + Zod schemas
3. **Generated files use `_gen.go` suffix** (except SQLC)
4. **Track generated code**, not build artifacts
5. **Module boundaries enforced by Go**
6. **Platform and Engine have separate schemas**
7. **Web UI is static-only** (embedded in Go binary)
8. **One lockfile at root**
9. **CI drift check is the gatekeeper**
10. **Domain types never leak into API responses**

---

## Core Patterns

### Contract-First Development

```yaml
# contracts/openapi/openapi.yaml - ALWAYS edit here first
paths:
  /api/traces/{id}:
    get:
      operationId: getTrace  # This becomes Go method name
```

Then: `make generate` → implements `ServerInterface` in `internal/api/`

### SQLC Queries

```sql
-- db/platform/queries/traces.sql
-- name: GetTrace :one
SELECT * FROM traces WHERE id = $1;

-- name: ListTraces :many
SELECT * FROM traces ORDER BY started_at DESC LIMIT $1 OFFSET $2;
```

### Handler Implementation

```go
// internal/api/handlers.go - implement generated interface
func (s *Server) GetTrace(w http.ResponseWriter, r *http.Request, id string) {
    trace, err := s.queries.GetTrace(r.Context(), uuid.MustParse(id))
    if err != nil {
        // Handle error
        return
    }
    // ALWAYS map to API types
    writeJSON(w, http.StatusOK, mapTraceToAPI(trace))
}
```

### Type Mapping (Rule #10)

```go
// internal/api/mapper.go - NEVER expose DB types directly
func mapTraceToAPI(t platform.Trace) Trace {
    return Trace{
        Id:        t.ID.String(),
        Name:      t.Name,
        Status:    TraceStatus(t.Status),
        StartedAt: t.StartedAt.Time,
    }
}
```

---

## Common Commands

| Task | Command |
|------|---------|
| Generate all code | `make generate` |
| Run dev server | `make dev-server` |
| Run tests | `make test` |
| Lint | `make lint` |
| New migration | `make migrate-create name=xyz` |
| Full CI locally | `make ci` |

---

## Anti-Patterns

| Don't | Do |
|-------|-----|
| Edit `*_gen.go` files | Edit source + `make generate` |
| Return `platform.Trace` from handlers | Map to API types |
| Add queries outside SQLC | Use `db/platform/queries/` |
| Use `process.env` style config | Use config structs |
| Panic on errors | Return structured errors |

---

## Navigation

| Need to... | Read this |
|------------|-----------|
| Understand project layout | [architecture.md](resources/architecture.md) |
| Add API endpoints | [api-patterns.md](resources/api-patterns.md) |
| Work with database | [database.md](resources/database.md) |
| Write tests | [testing.md](resources/testing.md) |

---

**Skill Status**: COMPLETE
**Line Count**: ~150
