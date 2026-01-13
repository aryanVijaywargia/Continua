<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Continua is an AI Agent Observability Platform for debugging AI agents by capturing and replaying execution traces. It's a Go/React monorepo with TypeScript and Python SDKs.

## Essential Commands

```bash
# Development
make dev              # Start PostgreSQL (Docker)
make dev-server       # Start Go backend with hot reload
make dev-web          # Start Vite React dev server

# Code Generation (CRITICAL - run after contract/schema changes)
make generate         # Regenerate ALL code (contracts, sqlc, SDK types)
                      # CI fails if generated code is out of sync

# Testing
make test             # Run all tests (Go + JS)
make test-go          # Go tests with race detector
make test-integration # Integration tests (requires running DB)

# Linting
make lint             # All linters (Go + JS/TS)
make lint-fix         # Auto-fix issues
make type-check       # TypeScript type checking

# Building
make build            # Build server + web UI
make build-server     # Go binary → bin/continua

# Database
make migrate                      # Run migrations
make migrate-create name=add_foo  # Create new migration

# Pre-commit validation
make ci               # Full CI pipeline locally
```

## Architecture

### Module Boundaries

```
cmd/continua/        → Server CLI entrypoint (Cobra)
contracts/           → API contracts (SOURCE OF TRUTH)
├── openapi/         → REST API spec (openapi.yaml)
└── websocket/       → WebSocket events (events.ts with Zod)
internal/            → Server internals (not importable externally)
├── api/             → REST handlers (uses generated code)
├── store/           → Database access (uses sqlc generated code)
└── ws/              → WebSocket handling
pkg/                 → Shared public packages
engine/              → ISOLATED Go module (cannot import internal/)
db/platform/         → Migrations + sqlc queries
web/                 → Vite React SPA (built to internal/web/static/)
sdks/typescript/     → TS SDK (peer deps: openai, anthropic)
sdks/python/         → Python SDK (httpx, pydantic)
```

### 10 Architecture Rules

1. **`make generate`** is the ONE command for all code generation
2. **Contracts are source of truth** — never hand-edit generated files
3. **Generated Go files use `_gen.go` suffix** (except sqlc output)
4. **Track generated code in git**, not build artifacts
5. **Module boundaries enforced by Go** — engine/ cannot import internal/
6. **Platform and Engine have separate schemas**
7. **Web UI is static-only** — no SSR, embedded in Go binary
8. **One lockfile at root** (pnpm-workspace)
9. **CI drift check is gatekeeper** — fails if generated code differs
10. **Domain types never leak into API responses** — use mappers

### Code Generation Flow

```
contracts/openapi/openapi.yaml  →  internal/api/server_gen.go
contracts/websocket/events.ts   →  contracts/websocket/events.schema.json
db/platform/queries/*.sql       →  db/gen/go/platform/*.go
contracts/openapi/              →  sdks/python/src/continua/types.py
```

### Database Schema

Core tables: `sessions`, `traces`, `spans`, `payloads`
- Traces belong to sessions
- Spans belong to traces (self-referential for parent/child)
- Payloads store request/response bodies for spans

### Key Patterns

- **Chi router** for HTTP
- **Uber Fx** for dependency injection
- **sqlc** for type-safe SQL (not an ORM)
- **golang-migrate** for migrations
- **TanStack Query** in React frontend

## Files to Never Edit Manually

- `*_gen.go` files (regenerate with `make generate`)
- `db/gen/` directory (sqlc output)
- `contracts/generated/` directory
- `contracts/websocket/*.schema.json`

## Workflow After Contract/Schema Changes

1. Edit `contracts/openapi/openapi.yaml` or `contracts/websocket/events.ts`
2. Or edit `db/platform/queries/*.sql`
3. Run `make generate`
4. Commit the regenerated files

## Tech Stack

- **Backend**: Go 1.22+, Chi, Fx, pgx/v5, sqlc
- **Frontend**: Vite, React 18, TypeScript 5.6, TanStack Query, Tailwind
- **Database**: PostgreSQL (primary), SQLite (local dev option)
- **SDKs**: TypeScript (tsup, vitest), Python (uv, pytest, pydantic)

## Git Commit Rules

- **Never add Co-Authored-By signatures** to commit messages
- Use conventional commit format: `type: description`
- Types: feat, fix, chore, docs, refactor, test, perf, ci

## Test and CI Failure Policy

- **Never bypass, skip, or remove failing tests** as a solution
- **Never disable CI checks** to make builds pass
- Always perform root cause analysis (RCA) first
- Fix the actual issue, not the symptom
- If a test is genuinely wrong, fix the test logic - don't delete it

## Go Performance Patterns

- Use pointer parameters for structs > 200 bytes (spans, traces, events)
- Use index-based range loops: `for i := range slice { item := &slice[i] }`
- Dereference at sqlc call boundary, not before

## Review Feedback Guidelines

When receiving PR/code review feedback:

**Accept and fix:**
- Redundant code (unused handlers, variables)
- Type safety issues (overly restrictive TypeScript types)
- Missing error handling
- Bug fixes (atexit accumulation, status mismatches)

**Push back on:**
- Suggestions to modify generated code (use `make generate`)
- Database-specific suggestions for wrong database (SQLite for PostgreSQL-only)
- Cosmetic changes (enum ordering, minor formatting)
- Documentation changes in implementation reviews

## Development Workflow

- Create feature branches for all changes (never commit directly to main)
- Use OpenSpec for proposals involving architecture changes
- Run `make ci` before committing
- Use `/smart-commit` for commits with changelog updates
