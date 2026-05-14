# Continua Documentation

> **Status: Current**
> This page is the documentation index for the current checkout. It also defines the status labels used across `docs/`.

## Status Convention

- **Current**: authoritative, repo-verified guidance for the current checkout
- **Historical**: preserved context, rollout notes, plans, or reports that do not define today's architecture
- **Active change**: proposal material under `openspec/changes/`; useful for intent/history, but not the current-state source of truth

## Current Docs

Use these first:
- [Setup Guide](./setup.md)
- [Debugger Platform Baseline](./DEBUGGER_PLATFORM_BASELINE.md)
- [Architecture Overview](./architecture/overview.md)
- [Architecture Rules](./architecture/RULES.md)
- [Data Model](./architecture/data-model.md)
- [Event Conventions](./event-conventions.md)
- [Repo Root README](../README.md)

## Historical Docs

These remain in place for provenance and archaeology:
- [Phase 5 Current State Report](./PHASE5_CURRENT_STATE_REPORT.md)
- `docs/phase2/`, `docs/phase3/`, `docs/phase4/`
- `docs/guides/`
- `docs/reviews/`
- `docs/plans/`
- `docs/*_v1.md`, rollout notes, and scratch docs such as `docs/temp.md`

Historical docs should not be treated as the current architecture contract unless a newer current doc points back to them for context.

## Active Change Docs

OpenSpec change proposals live under:
- `openspec/changes/`

Implemented OpenSpec history lives under:
- `openspec/implemented/`

Because `openspec/specs/` is still empty, OpenSpec is not a complete current-state spec set for this repo.

## Current Runtime Snapshot

Continua is currently an AI agent observability debugger with one implemented path:

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> embedded React debugger operator console
```

The current web surface includes `/` landing, `/dashboard` overview, traces, sessions, session compare, and settings.
