# Testing Strategy

## Scope
- Prefer the smallest relevant suite first, but cover the real behavior you changed.
- Use DB-backed tests for store, auth, ingest, and API behavior when the logic depends on Postgres semantics.
- Use web Vitest coverage for URL state, payload inspector behavior, failure analysis, and component rendering logic.
- Use SDK tests when changing contract handling, batching, retry behavior, or helper APIs.

## Placement
- Go tests: `*_test.go` next to the package under `Continua/` and `Continua/engine/`.
- Web tests: `web/src/**/*.test.ts?(x)`
- SDK tests:
  - TypeScript: `sdks/typescript/tests`
  - Python: `sdks/python/tests`

## Coverage Expectations
- Happy path, edge cases, and error handling.
- For contract changes, add at least one test that exercises the new field or endpoint.
- For project-scoped endpoints, cover both authorized success and cross-project/not-found behavior.
- For async ingest work, cover accepted, duplicate, processing, completed, and failure paths where relevant.
