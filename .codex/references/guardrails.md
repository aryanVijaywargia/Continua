# Codex guardrails for Continua

These rules are adapted from the project's Claude hooks so Codex sessions keep the same safety and workflow expectations.

## File safety
- Do not edit generated outputs directly:
  - `contracts/generated/`
  - `internal/api/server_gen.go`
  - `db/gen/go/platform/`
  - `engine/db/gen/`
  - `web/dist/`
  - `internal/web/static/`
- Do not edit lockfiles or module manifests casually:
  - `go.mod`, `go.sum`
  - `pnpm-lock.yaml`, `package-lock.json`, `yarn.lock`
- Avoid reading or writing `.env*` files from Codex. Handle secrets manually outside the agent.
- Do not treat `config.example.yaml` as the live config contract; runtime config is env-only in `internal/config/config.go`.

## Migration safety
- This repo is still pre-production. Until the first production release, SQL migrations under `db/platform/migrations/` and `engine/db/migrations/` may be rewritten, renumbered, or squashed when that keeps the pre-release schema history cleaner.
- After the first production release, treat existing SQL migrations under `db/platform/migrations/` and `engine/db/migrations/` as immutable.
- If a pre-production migration is rewritten, also update any dependent down migrations, migration smoke tests, generated code, and docs that reference the old numbering or behavior.
- When a new step is clearer than rewriting history, still prefer `make migrate-create name=<description>`.

## Command safety
- Avoid destructive shell commands unless the user explicitly asks:
  - `rm -rf`, `git reset --hard`, `git clean -fd`, `docker system prune`
- Never use broad staging commands like `git add .`, `git add -A`, or wildcard `git add`.
- Treat force pushes and shell pipelines such as `curl ... | sh` as exceptional operations requiring confirmation.

## Workflow reminders
- If you change OpenAPI, WebSocket contracts, SQLC queries, SQLC config, or migrations, run `make generate`.
- If you edit Go source, run `gofmt` and `goimports` when available.
- If a task touches protected or generated areas, prefer changing the source inputs, not the derived files.
- Treat placeholder directories such as `internal/proxy`, `internal/ws`, `internal/replay`, and `engine/` as future work unless the task is explicitly expanding them.
