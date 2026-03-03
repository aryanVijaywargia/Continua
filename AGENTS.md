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

## Skills
Repo-local skills for this repository live under `.claude/skills/`.

### Available skills
- `continua-backend-dev`: Backend development patterns for Continua's Go monorepo. (file: `/Users/aryanvijaywargia/Projects/Continua/.claude/skills/continua-backend-dev/SKILL.md`)
- `continua-observability`: Domain-specific guide for trace/span model, WebSocket events, and replay. (file: `/Users/aryanvijaywargia/Projects/Continua/.claude/skills/continua-observability/SKILL.md`)
- `continua-integrations`: SDK and proxy development patterns. (file: `/Users/aryanvijaywargia/Projects/Continua/.claude/skills/continua-integrations/SKILL.md`)
- `continua-testing`: Testing strategy and commands for this repo. (file: `/Users/aryanvijaywargia/Projects/Continua/.claude/skills/continua-testing/SKILL.md`)
- `skill-developer`: Guide for creating/updating Claude-style skills in this repo. (file: `/Users/aryanvijaywargia/Projects/Continua/.claude/skills/skill-developer/SKILL.md`)

### How to use skills
- Trigger rules: If the user names a skill (with `$SkillName` or plain text) OR the task clearly matches a listed skill description, use that skill for the turn.
- Discovery: Open the corresponding `SKILL.md` and read only what is needed to execute the request.
- Progressive disclosure: Only open referenced `resources/` or `references/` files when needed.
- Safety: Do not edit files inside `.claude/` unless the user explicitly asks.
