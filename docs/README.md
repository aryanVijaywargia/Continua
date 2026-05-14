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
- [Architecture Overview](./architecture/overview.md)
- [Architecture Rules](./architecture/RULES.md)
- [Data Model](./architecture/data-model.md)
- [Event Conventions](./event-conventions.md)
- [Run Locally Guide](./guides/run-locally.md)
- [Repo Root README](../README.md)

## Private And Historical Docs

Historical phase reports, review notes, old implementation plans, scratch docs, and planning artifacts are intentionally not kept in this public documentation tree. Treat the checked-in code, contracts, migrations, and current docs above as the source of truth for this checkout.

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
