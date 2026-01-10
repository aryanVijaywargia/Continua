<!--
  Source: codex_claude_setup
  Original Path: commands/build/COMMAND.md
  Adaptations: Minor formatting for consistency
-->
---
description: Build Continua components
argument-hint: [all|server|web|engine|sdk|contracts]
---

# Build Components

Build Continua artifacts using the Makefile targets.

## Context
- **Server binary**: `bin/continua`
- **Engine binary**: `bin/continua-engine`
- **Web build**: `web/dist/` → `internal/web/static/`
- SDK/contracts use pnpm workspace filters

## Process

Parse $ARGUMENTS and execute:

1. **Build all** (empty or `all`):
   ```bash
   make build
   ```

2. **Build server**:
   ```bash
   make build-server
   ```

3. **Build engine**:
   ```bash
   make build-engine
   ```

4. **Build web**:
   ```bash
   make build-web
   ```

5. **Build SDK**:
   ```bash
   make build-sdk
   ```

6. **Build contracts**:
   ```bash
   make build-contracts
   ```

Report generated artifacts and locations.

## Examples
- `/project:build` - Build all components
- `/project:build server` - Build server binary
- `/project:build web` - Build web assets
- `/project:build engine` - Build engine binary

## Anti-patterns
- Building without running `/project:generate` after contract/SQL changes
- Forgetting web assets are copied to `internal/web/static/`
- Building before running migrations
