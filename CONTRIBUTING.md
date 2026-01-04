# Contributing to Continua

## Architecture Rules

See `docs/architecture/RULES.md` for the 10 rules that prevent drift.

## Development Workflow

1. Run `make generate` after changing contracts or queries
2. Run `make lint` before committing
3. Run `make test` before pushing

## PR Checklist

- [ ] `make generate` produces no diff
- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] Documentation updated if needed
