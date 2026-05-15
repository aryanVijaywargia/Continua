# Contributing to Continua

## Project orientation

Before making changes, skim:

- [`docs-site/concepts/overview.mdx`](./docs-site/concepts/overview.mdx) — current architecture and runtime components
- [`docs/architecture/overview.md`](./docs/architecture/overview.md) — package-level map
- [`AGENTS.md`](./AGENTS.md) — repo conventions, generated-file boundaries, testing expectations

## Development workflow

1. Run `make generate` after changing OpenAPI, sqlc queries, WebSocket schemas, or migrations
2. Run `make lint` before committing
3. Run `make test` before pushing
4. For docs changes: `make docs-dev` to preview locally (requires `npm i -g mint`)

## PR checklist

- [ ] `make generate` produces no diff
- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] Documentation updated if needed (`docs-site/` and/or `docs/architecture/`)
- [ ] Never edit generated files directly (see [`AGENTS.md`](./AGENTS.md#contracts-generation-and-generated-files))
