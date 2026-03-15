# Replay status

Replay is not an implemented product capability today.

## Current repo state
- `internal/replay/` is a placeholder
- `engine/` is pre-scaffolded and `continua-engine` only prints a placeholder message
- there is no replay API in `contracts/openapi/openapi.yaml`
- there is no runtime replay orchestration in the server

## Existing ingredients a future replay design could use
- trace input/output stored on traces
- span input/output plus truncation metadata stored on spans
- explicit span events
- timeline ordering logic in the debugger

## Do not assume
- deterministic request/response capture is complete enough for replay
- provider matching exists
- timing preservation exists
- replay routes or UI state machines already exist

If a user asks for replay work, treat it as a new cross-cutting capability that likely needs OpenSpec, contract design, storage rules, and engine/runtime decisions.
