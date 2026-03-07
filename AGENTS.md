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

# Repository Guidelines

## Project Structure & Module Organization
- `cmd/continua` holds the server entrypoint; core Go logic is in `internal/` and shared libs in `pkg/`.
- `engine/` is a separate Go module wired via `go.work`.
- API contracts live in `contracts/`; generated Go types are copied into `internal/api`.
- Database schema and migrations are in `db/platform/` (migrations under `db/platform/migrations/postgres`).
- Frontend lives in `web/`; SDKs are under `sdks/` (TypeScript in `sdks/typescript`, Python in `sdks/python`).
- Deploy manifests are in `deploy/`; documentation in `docs/`; scripts in `scripts/`.

## Build, Test, and Development Commands
- `./scripts/setup.sh` (or `make setup`): install Go, Node, and Python SDK deps.
- `make dev`: start the dev database (Docker); `make dev-server`: run Go backend; `make dev-web`: run Vite UI.
- `make generate`: regenerate contracts, sqlc code, and SDK types after contract/db changes.
- `make build`: build server + web; `make lint` and `make test`: run linters/tests; `make ci`: full local CI.
- `make migrate-create name=add_users`: create a new migration.

## Coding Style & Naming Conventions
- Go code is formatted by `gofmt`/`goimports` and linted via `golangci-lint` (`make lint-go`).
- JS/TS code uses ESLint (`make lint-js` or `pnpm lint`); `pnpm type-check` runs TypeScript checks.
- Keep Go package names lowercase and follow existing file naming patterns per module.
- Avoid editing generated files (`*_gen.go`, `db/.../gen`, `contracts/generated`); regenerate instead.

## Testing Guidelines
- Go tests live alongside packages as `*_test.go`; run with `make test` or `make test-go`.
- Integration tests use the `integration` build tag: `make test-integration` (requires DB).
- TypeScript SDK tests run with Vitest: `pnpm --filter @continua/sdk test`.
- No explicit coverage threshold is documented; add tests for new behavior.

## Commit & Pull Request Guidelines
- Recent history uses conventional prefixes like `feat:`; keep commit subjects short and imperative.
- PRs should include a clear description, linked issues, and updates to docs if behavior changes.
- Before submitting, ensure: `make generate` has no diff, `make lint` passes, `make test` passes.

## Architecture & Configuration Notes
- Follow the 10 architecture rules in `docs/architecture/RULES.md`.
- Use `config.example.yaml` as a template; keep secrets out of the repo.


## Project-local Codex Context
- Repo-local Codex assets live in `.codex/`. Prefer these over `.claude/` when working from Codex so the guidance stays versioned with the repo.

### Repo-local skills
- `continua-backend-dev`: Backend changes in `cmd/`, `internal/`, `db/`, and `contracts/`.
- `continua-observability`: Trace/span/session data model, replay, token rollups, and WebSocket event flows.
- `continua-integrations`: SDKs, proxy capture, and framework adapters.
- `continua-testing`: Test planning, suite selection, and coverage expectations.

### How to use repo-local skills
- If a task clearly matches one of the skills above, open the corresponding `.codex/skills/<skill>/SKILL.md` and follow it.
- Load linked `resources/` or `references/` files only when they are relevant to the current task.
- Start with `.codex/references/decisions.md` when you need shared project rules, generated-file boundaries, or source-of-truth locations.

### Codex guardrails
- Do not edit generated files directly; change the source inputs and run `make generate`.
- Treat existing migrations under `db/platform/migrations/` and `engine/db/migrations/` as immutable; create a new migration instead.
- Avoid direct `.env*` reads or writes, and avoid broad `git add .`, `git add -A`, or wildcard staging; use specific file paths.
- If you touch OpenAPI, WebSocket contracts, SQLC inputs, or migrations, run `make generate` before finishing.
- Format edited Go files with `gofmt` and `goimports` when available.
- See `.codex/references/guardrails.md`, `.codex/references/commands.md`, and `.codex/references/subagents.md` for Claude-derived workflow notes that were ported into Codex form.
