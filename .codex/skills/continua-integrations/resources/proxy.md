# Proxy status

`internal/proxy/` is currently a placeholder directory only.

There is no implemented:
- proxy HTTP handler
- provider forwarding layer
- streaming capture runtime
- proxy route mounted from `cmd/continua`

## How to treat proxy work
- Proxy work is new capability work, not routine extension of existing code.
- Start by checking OpenSpec and the current platform baseline in `docs/DEBUGGER_PLATFORM_BASELINE.md`.
- Expect proxy work to touch contracts, auth, ingest semantics, payload capture rules, and likely product scope.

## Do not assume
- a provider abstraction already exists
- request/response payload capture routes are wired
- streaming behavior is implemented
- proxy runtime metrics or replay hooks exist

If a task is explicitly to build proxy capture, create or update the proposal first unless the user is clearly asking for a small bug fix in code that already exists.
