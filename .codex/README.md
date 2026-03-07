# Repo-local Codex assets

This directory mirrors the reusable parts of the project's `.claude/` setup in a Codex-friendly form.

## Contents
- `skills/`: Continua domain skills copied from `.claude/skills/`
- `references/decisions.md`: shared project rules and source-of-truth locations
- `references/guardrails.md`: Claude hook policies rewritten as Codex guidance
- `references/commands.md`: Claude slash-command inventory translated into direct Codex workflows
- `references/subagents.md`: Claude subagent patterns summarized for Codex use

## Compatibility notes
- Claude hooks are not executable in Codex. Their behavior is documented in `references/guardrails.md` and reinforced in the root `AGENTS.md`.
- Claude slash commands and subagents are references only. Use them as prompt patterns and workflow checklists, not as executable features.
- OpenSpec remains under `openspec/`; follow the root `AGENTS.md` instructions for proposal and implementation work.
