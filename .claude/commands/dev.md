<!--
  Source: codex_claude_setup
  Original Path: commands/dev/COMMAND.md
  Adaptations: Minor formatting for consistency
-->
---
description: Start or stop development services
argument-hint: [db|server|web|stop]
---

# Development Services

Start or stop Continua development services from the repo root.

## Context
- **DB**: docker compose via `deploy/docker-compose/docker-compose.dev.yml`
- **Server**: hot-reload via `air` (`make dev-server`)
- **Web**: Vite dev server (`make dev-web`)
- Server and web are long-running; run in separate terminals

## Process

Parse $ARGUMENTS and execute:

1. **Start database** (empty or `db`):
   ```bash
   make dev
   ```

2. **Start server with hot-reload** (`server`):
   ```bash
   make dev-server
   ```
   Note: Long-running process

3. **Start web dev server** (`web`):
   ```bash
   make dev-web
   ```
   Note: Long-running process

4. **Stop all services** (`stop`):
   ```bash
   make dev-stop
   ```

## Service URLs
- **Web UI**: http://localhost:3000
- **Database**: localhost:5432

## Examples
- `/project:dev` - Start database
- `/project:dev db` - Start database
- `/project:dev server` - Start server (hot-reload)
- `/project:dev web` - Start web UI
- `/project:dev stop` - Stop all services

## Anti-patterns
- Starting server and web in the same shell without warning
- Running docker compose outside the designated compose file
- Forgetting to start DB before server
