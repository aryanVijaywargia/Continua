---
name: continua-testing
description: Testing strategy for Continua (Go, engine, web, SDKs). Use when adding features, fixing bugs, or changing API/DB behavior; triggers on go test, make test, integration tests, pnpm test, or SDK test updates.
---

# Continua Testing

## Context
Use requirement-driven tests and run the smallest relevant suite before expanding coverage.

## Shared reference
- Read [../../references/decisions.md](../../references/decisions.md) if the change affects contracts, generated outputs, or cross-module architecture rules.

## References
- `references/strategy.md` - what to test and how to scope coverage.
- `references/commands.md` - make targets and SDK test commands.

## Current Conventions

- Keep Go tests colocated with the code they verify; do not move them into a repo-level `tests/` bucket.
- In `internal/api`, mirror the handler split in test filenames:
  - `ingest_handlers_test.go`
  - `traces_handlers_test.go`
  - `sessions_handlers_test.go`
  - `server_helpers_test.go`
- When reorganizing files without changing behavior, prefer renaming or regrouping existing tests over rewriting them.
