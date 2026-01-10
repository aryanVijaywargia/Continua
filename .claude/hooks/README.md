# Continua Claude Code Hooks

Lean, focused hooks for the Continua project - tailored specifically for Go + PostgreSQL + OpenAPI development.

## Hooks (6 total)

| Hook | Event | Purpose |
|------|-------|---------|
| `bash-command-safety.py` | PreToolUse/Bash | Block dangerous commands (rm -rf, force push, .env access) |
| `protected-files-blocker.py` | PreToolUse/Edit | Block edits to generated code, lockfiles, .env files |
| `migration-immutability-guard.py` | PreToolUse/Edit | Block edits to existing migration files |
| `go-format-on-edit.sh` | PostToolUse/Edit | Auto-run gofmt/goimports on Go files |
| `codegen-drift-guard.sh` | PostToolUse/Edit | Remind to run `make generate` when codegen inputs change |
| `context-monitor.py` | PostToolUse | Warn when context usage is high (80%/90%) |

## What Each Hook Does

### bash-command-safety.py (PreToolUse)
Blocks dangerous bash commands:
- `rm -rf /`, `dd if=`, `mkfs` (destructive)
- `git push --force` to main/master
- Direct .env file access (`cat .env`, `grep .env`)
- `git add .` / `git add -A` (require specific files)

### protected-files-blocker.py (PreToolUse)
Blocks editing protected files with Continua-specific paths:
- `.env*` files (secrets)
- `go.mod`, `go.sum` (use `go get`)
- `pnpm-lock.yaml`, `package-lock.json` (lockfiles)
- `contracts/generated/`, `internal/api/server_gen.go` (generated)
- `db/platform/gen/`, `engine/db/gen/` (sqlc generated)
- `web/dist/` (build output)

Asks for confirmation on:
- `db/platform/sqlc.yaml`, `engine/db/sqlc.yaml` (codegen config)
- `contracts/openapi/openapi.yaml` (API contract)

### migration-immutability-guard.py (PreToolUse)
Blocks editing existing migration files in:
- `db/platform/migrations/`
- `engine/db/migrations/`

Instead prompts: `make migrate-create name=<description>`

### go-format-on-edit.sh (PostToolUse)
Auto-formats Go files after editing:
- Runs `gofmt -w` and `goimports -w`
- Skips generated files (`_gen.go`, `.pb.go`)
- Skips vendor directory

### codegen-drift-guard.sh (PostToolUse)
Reminds to regenerate code when inputs change:
- `contracts/openapi/*`, `contracts/websocket/*`
- `db/platform/queries/*`, `db/platform/migrations/*`, `db/platform/sqlc.yaml`
- `engine/db/queries/*`, `engine/db/migrations/*`, `engine/db/sqlc.yaml`

Shows reminder:
```
[codegen] Detected change in: contracts/openapi/traces.yaml
[codegen] Run: make generate
[codegen] Verify: ./scripts/check-generated.sh
```

### context-monitor.py (PostToolUse)
Monitors context usage:
- 80%: Warning to finish current task
- 90%: Critical - save state soon

## Installation

1. Make shell scripts executable:
```bash
chmod +x claude_setup/hooks/*.sh
```

2. Copy hooks section from `settings.json` to your `.claude/settings.json`

## Dependencies

- Python 3.9+
- jq (for shell scripts)
- gofmt, goimports (Go formatting)

```bash
# macOS
brew install jq
go install golang.org/x/tools/cmd/goimports@latest
```

## Speed Bump Pattern

The blocker hooks use a "speed bump" pattern:
1. First attempt: Block with warning
2. Retry within 60 seconds: Allow (user acknowledged)

This catches mistakes while allowing intentional overrides.

## Source

All hooks adapted from `codex_claude_setup/hooks/` which were specifically designed for the Continua codebase.
