# Continua Engine

The durable execution engine for AI agent workflows.

**Status:** Pre-scaffolded for future development.

## Module Isolation

This is a fully isolated Go module with its own database.

| Can Do | Cannot Do |
|--------|-----------|
| ✅ Import own `internal/*` | ❌ Import root's `internal/*` |
| ✅ Import own `db/gen/go/*` | ❌ Import root's `db/gen/go/*` |

## Building

```bash
make build-engine
```
