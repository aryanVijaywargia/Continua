# Continua Skills

These are the Claude-facing Continua skills. They should stay aligned with the repo-local Codex skills for the overlapping Continua domains.

## Current repo reality these skills assume
- The implemented platform is REST ingest + Postgres + River + React debugger UI.
- The strongest active areas are `internal/api`, `internal/ingest`, `internal/jobs`, `internal/store`, `web/src`, and `sdks/python`.
- The engine, proxy runtime, WebSocket runtime, replay system, and TypeScript SDK are mostly scaffolded.
- The shortest current repo-state handoff is `docs/DEBUGGER_PLATFORM_BASELINE.md`.

## Continua skills

### `continua-backend-dev`
- Use for current Go platform server work in `cmd/`, `internal/`, `db/`, and `contracts/`
- Covers OpenAPI-first backend changes, sqlc/store work, ingest/jobs boundaries, and API mapping rules

### `continua-debugger-ui`
- Use for trace detail workspace behavior, traces and sessions page UX, payload inspection, state diff, settings, command palette, and theming work in `web/src`
- Preserves the current URL-driven state and workspace-selection patterns

### `continua-observability`
- Use for trace/span/session/event semantics, async ingest lifecycle, rollups, timeline behavior, and debugger data surfaces
- Explicitly calls out that WebSocket runtime and replay are not implemented today

### `continua-integrations`
- Use for Python SDK work, contract-driven SDK generation, TypeScript SDK stub work, and planning new proxy/adapter features
- Treats proxy and framework adapters as future capability work, not active runtime surfaces

### `continua-testing`
- Use for suite selection, real-DB backend test patterns, web Vitest coverage, and SDK verification

### `skill-developer`
- Meta-skill for building or updating skills themselves

## Shared reference
- `references/decisions.md`: source-of-truth order, generated-file boundaries, runtime/scaffolded split, and current repo realities shared by the Continua skills
