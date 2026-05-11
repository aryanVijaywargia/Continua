# README Improvement Changelog

Status: Current

## Summary

Iterative refinement of the root `README.md`. Each entry documents what changed, which skills were used or skipped, and what assets were produced.

---

## 2026-05-12 — Premium visual pass

### Goal

Take the (already accurate) refreshed README and make it visibly stunning, drawing design cues from Langfuse, Trigger.dev, and Temporal — without introducing any new claims about replay, live WebSocket runtime, proxy capture, TypeScript SDK parity, or durable engine execution.

### Skills used

- `readme-generator-glincker` — repo-aware structure, prerequisites, commands, project tree, license accuracy.
- `readme-generator-visual` — full-bleed banner, 16:9 feature/architecture/ingest strips, badge stack, GitHub description and topic suggestions.
- `mermaid-tools` — Mermaid sources for runtime architecture, ingest flow, data model, and event types, alongside static SVG exports for GitHub rendering and offline viewers.

### Skills skipped

- `cli-demo-generator` — skipped because generating a terminal GIF requires VHS or similar global tooling not already required by the repo (the brief disallows global installs and root-level dependency churn). The README continues to rely on copy-paste-clean `make demo` commands.
- `walkthrough` — skipped for the README refresh because the root README already links to [`docs/setup.md`](./setup.md), [`docs/DEBUGGER_PLATFORM_BASELINE.md`](./DEBUGGER_PLATFORM_BASELINE.md), and [`docs/architecture/overview.md`](./architecture/overview.md). A separate interactive walkthrough page would duplicate those without new insight.

### README changes

- **New full-bleed banner** at the top of the README (`assets/banner.svg`, 2400×600), Langfuse/Trigger.dev style, replacing the smaller hero treatment.
- **Two-row centered nav** under the title: primary CTAs (Quickstart · Why · Features · Architecture · Python SDK · REST API · Docs) plus a secondary doc-row (Setup guide · Baseline · Architecture overview · Event conventions · OpenSpec · Issues).
- **Tighter badge stack** with logo-style shields for License, Go 1.24+, Node 20+, pnpm 9+, Python 3.10+, Postgres 16+, OpenAPI 3, Docker demo, and status: alpha — plus GitHub-state shields (stars, last commit, open issues). No badges that imply community surfaces we do not have (no Discord, Twitter, npm/PyPI downloads, or Docker Hub pulls).
- **"At a glance" status matrix** placed near the top to make the implemented-vs-scaffolded boundary visible above the fold.
- **Wide 16:9 feature strip** under `Features` (`assets/diagrams/feature-strip.svg`) illustrating the Ingest · Persist · Inspect · Compare surfaces as abstract tiles. Explicitly not labeled as screenshots.
- **Refreshed 16:9 architecture image** (`assets/diagrams/runtime-architecture.svg`) and **16:9 ingest flow image** (`assets/diagrams/ingest-flow.svg`) — both paired with Mermaid blocks for GitHub-native rendering.
- **New data model section** with `assets/diagrams/data-model.svg`, an `erDiagram` Mermaid block, and `docs/diagrams/data-model.mmd` as the reviewable source.
- **New event semantics section** with `assets/diagrams/event-types.svg`, `docs/diagrams/event-types.mmd`, and a one-paragraph pointer to `docs/event-conventions.md`. Lists the eleven implemented event types only.
- **REST API surface table** sourced from `contracts/openapi/openapi.yaml` (`POST /v1/ingest`, `GET /v1/ingest/batches/{id}`, `GET /api/traces`, `GET /api/traces/{id}`, `GET /api/traces/{id}/spans`, `GET /api/traces/{id}/events`, `GET /api/sessions`, `GET /api/sessions/{id}`, `GET /api/sessions/{id}/compare`, plus the directly routed `GET /api/health`).
- **Explicit "Engine foundation" section** clarifying that only schema/store/CLI exist today; workflow execution, history replay, activity workers, public exec APIs, and the engine debugger UI are not implemented.
- **GitHub callouts** used consistently (`> [!NOTE]`, `> [!IMPORTANT]`, `> [!WARNING]`).
- **Contributors mosaic** via `contrib.rocks` and a single-line "Made with…" stack credit at the bottom.

### Assets added or refreshed

- `assets/banner.svg` — **new** full-bleed banner.
- `assets/diagrams/feature-strip.svg` — **new** four-tile 16:9 feature image.
- `assets/diagrams/runtime-architecture.svg` — refreshed to 16:9, dark theme.
- `assets/diagrams/ingest-flow.svg` — refreshed to 16:9, dark theme.
- `assets/diagrams/data-model.svg` — **new** entity-relationship illustration.
- `assets/diagrams/event-types.svg` — **new** 11-cell event type grid.
- `docs/diagrams/data-model.mmd` — **new** Mermaid source.
- `docs/diagrams/event-types.mmd` — **new** Mermaid source.

`assets/readme-hero.svg` is retained as a smaller alternate, but is no longer referenced from the root README.

### Accuracy guardrails

- Only endpoints currently in `contracts/openapi/openapi.yaml` are listed.
- Engine section is explicitly labeled "foundation only".
- Trace detail timeline described as **polling** based on `GET /api/traces/{id}/events`, never WebSocket.
- TypeScript SDK described as a stub package.
- `config.example.yaml` continues to be flagged as not the runtime contract.
- No screenshots of the live UI. The illustrations are abstract operator-console motifs.
- No fake metrics, testimonials, or community badges.

### Suggested GitHub metadata

For repository `description`:

> Self-hosted debugging for AI agent runs — authenticated REST ingest, Postgres persistence, React debugger, Python SDK.

For repository `topics`:

`ai-agents`, `agent-observability`, `agent-debugging`, `llm-tracing`, `observability`, `tracing`, `self-hosted`, `go`, `postgres`, `react`, `python-sdk`, `openapi`, `river-jobs`, `sqlc`, `tanstack-query`, `chi-router`.

### Accuracy sources

- `README.old.md`
- `docs/setup.md`
- `docs/DEBUGGER_PLATFORM_BASELINE.md`
- `docs/architecture/overview.md`
- `docs/event-conventions.md`
- `Makefile`
- `package.json`
- `go.mod`
- `web/src/App.tsx`
- `contracts/openapi/openapi.yaml`
- `sdks/python/README.md`
- `engine/README.md`

---

## 2026-05-11 — Initial refresh

### Summary

Updated the root `README.md` into a more polished GitHub landing page while keeping claims grounded in the current repository implementation.

### Skills used

- `readme-generator-glincker`: repo-aware README structure, prerequisites, commands, testing, project structure, and license coverage.
- `readme-generator-visual`: visual hierarchy and static README asset direction. Playwright was not installed because SVG assets and GitHub-native Mermaid blocks were sufficient.
- `mermaid-tools`: architecture and ingest/data-flow Mermaid source structure.

### Skills skipped

- `cli-demo-generator`: skipped because generating a terminal GIF would require VHS or similar local recording dependencies not already required by the repo.
- `walkthrough`: skipped for the README refresh because the root README benefits more from concise diagrams and links to current docs than a separate interactive walkthrough page.

### README changes

- Added a centered hero section with badges and a static SVG visual.
- Reframed the product description around the implemented self-hosted AI agent debugger path.
- Added clearer Quickstart, Why Continua, Features, Demo, Python SDK, Architecture, How it Works, Project Structure, Configuration, Development, Testing, Roadmap Signals, Documentation, Contributing, and License sections.
- Added Mermaid diagrams for runtime architecture and ingest/investigation flow.
- Preserved an explicit implemented-vs-scaffolded boundary for WebSockets, proxy capture, replay, TypeScript SDK parity, and durable engine runtime.

### Assets added

- `assets/readme-hero.svg`
- `assets/diagrams/runtime-architecture.svg`
- `assets/diagrams/ingest-flow.svg`
- `docs/diagrams/runtime-architecture.mmd`
- `docs/diagrams/ingest-flow.mmd`

### Accuracy sources

- `README.old.md`
- `docs/setup.md`
- `docs/DEBUGGER_PLATFORM_BASELINE.md`
- `docs/architecture/overview.md`
- `docs/event-conventions.md`
- `Makefile`
- `package.json`
- `go.mod`
- `web/src/App.tsx`
- `contracts/openapi/openapi.yaml`
- `sdks/python/README.md`
- `engine/README.md`
