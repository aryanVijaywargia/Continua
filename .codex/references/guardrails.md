# Codex guardrails for Continua

These rules are adapted from the project's Claude hooks so Codex sessions keep the same safety and workflow expectations.

## File safety
- Do not edit generated outputs directly:
  - `contracts/generated/`
  - `internal/api/server_gen.go`
  - `db/platform/gen/`
  - `engine/db/gen/`
  - `web/dist/`
- Do not edit lockfiles or module manifests casually:
  - `go.mod`, `go.sum`
  - `pnpm-lock.yaml`, `package-lock.json`, `yarn.lock`
- Avoid reading or writing `.env*` files from Codex. Handle secrets manually outside the agent.

## Migration safety
- Existing SQL migrations under `db/platform/migrations/` and `engine/db/migrations/` are treated as immutable.
- Create a new migration with `make migrate-create name=<description>` instead of modifying an old one.

## Command safety
- Avoid destructive shell commands unless the user explicitly asks:
  - `rm -rf`, `git reset --hard`, `git clean -fd`, `docker system prune`
- Never use broad staging commands like `git add .`, `git add -A`, or wildcard `git add`.
- Treat force pushes and shell pipelines such as `curl ... | sh` as exceptional operations requiring confirmation.

## Workflow reminders
- If you change OpenAPI, WebSocket contracts, SQLC queries, SQLC config, or migrations, run `make generate`.
- If you edit Go source, run `gofmt` and `goimports` when available.
- If a task touches protected or generated areas, prefer changing the source inputs, not the derived files.
