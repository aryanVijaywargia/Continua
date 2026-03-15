# Repo-local Codex assets

This directory is the Codex-facing guidance layer for Continua. It should describe the repo as it exists today, not the repo as earlier plans imagined it.

## What lives here
- `skills/`: focused Continua skills for backend, observability, integrations, and testing
- `references/decisions.md`: current source-of-truth hierarchy and implementation boundaries
- `references/guardrails.md`: Codex-safe workflow and file-safety rules
- `references/commands.md`: useful workflow prompts translated from the older Claude setup
- `references/subagents.md`: reference-only subagent patterns

## Current repo reality these assets assume
- The active product is the Go platform server plus the React debugger UI.
- The strongest implemented areas are ingest, async jobs, store/query code, trace/session APIs, timeline debugging, and the Python SDK.
- The engine, proxy runtime, WebSocket runtime, replay system, and TypeScript SDK are mostly scaffolded.

## Maintenance rule
- Keep `.codex/` and the overlapping Continua skills under `.claude/skills/` aligned.
- When a repo archaeology pass proves a doc or skill wrong, update the guidance instead of preserving the older story.
