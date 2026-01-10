<!--
  Source: codex_claude_setup
  Original Path: commands/migrate/COMMAND.md
  Adaptations: Minor formatting for consistency
-->
---
description: Create or run database migrations
argument-hint: [up|down|create <name>]
---

# Database Migrations

Manage platform DB migrations stored under `db/platform/migrations/postgres/`.

## Context
- Migration tool: `migrate` CLI via Makefile targets
- SQLC schema is sourced from migrations; run `/project:generate` after schema changes
- Dev DB is started via `make dev`

## Command Behavior
- **Inputs:**
  - `up` (default): apply pending migrations
  - `down`: rollback last migration
  - `create <name>`: create new migration files

## Process

Parse $ARGUMENTS and execute:

1. **Apply migrations** (empty or `up`):
   ```bash
   make migrate
   ```

2. **Rollback last migration** (`down`):
   ```bash
   make migrate-down
   ```

3. **Create new migration** (`create <name>`):
   ```bash
   make migrate-create name=<name>
   ```

After `up` or `down`, recommend running `/project:generate` to refresh SQLC outputs.

## Examples
- `/project:migrate` - Apply pending migrations
- `/project:migrate up` - Apply pending migrations
- `/project:migrate down` - Rollback last migration
- `/project:migrate create add_traces_table` - Create new migration

## Anti-patterns
- Editing old migration files after they have been applied
- Running migrations without the dev DB running
- Skipping `/project:generate` after schema changes
- Creating migrations without descriptive names
