# Testing Strategy

## Scope
- Unit tests for logic changes.
- Integration tests for DB or API behavior (use `-tags=integration`).
- SDK tests for contract or client changes.

## Placement
- Go tests: `*_test.go` next to the package under `Continua/` and `Continua/engine/`.
- SDK tests:
  - TypeScript: `Continua/sdks/typescript/tests`
  - Python: `Continua/sdks/python/tests`

## Coverage Expectations
- Happy path, edge cases, and error handling.
- For contract changes, add at least one test that exercises the new field or endpoint.
