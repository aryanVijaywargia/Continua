# Capability: engine-python-control-client

Separate Python control module that wraps the full frozen engine control surface (Proposal 1 + this proposal). Ships as its own class/module alongside the existing ingest batching client, with a polling `wait_for_terminal()` helper.

Related capabilities: [engine-public-api](../engine-public-api/spec.md)

## ADDED Requirements

### Requirement: Separate control module

The Python SDK MUST expose engine control as a standalone module/class that does not modify the existing ingest batching client.

#### Scenario: Module lives under the Python SDK package
- **WHEN** a user imports the control client
- **THEN** it is available from the existing `continua` package (e.g. `from continua import EngineControlClient` or equivalent)
- **THEN** it lives in its own file under `sdks/python/src/continua/` (e.g. `engine_control.py`)
- **THEN** it is NOT an extension of the existing ingest `Client` class

#### Scenario: Constructor arguments
- **WHEN** a user instantiates the control client
- **THEN** required arguments are `endpoint` and `api_key` (consistent with the existing ingest `Client` which uses `endpoint`, not `base_url`)
- **THEN** optional arguments include `timeout` and any HTTP session/transport injection consistent with the existing SDK's conventions
- **THEN** the control client does NOT require a `Client` instance to be passed in

#### Scenario: No shared batching state
- **WHEN** the control client is instantiated or used
- **THEN** it does not mutate or depend on the ingest batching client's in-flight batches, pollers, or background threads
- **THEN** disabling the ingest client does not affect the control client

#### Scenario: Auth via API key
- **WHEN** the control client issues any request
- **THEN** it sends the API key via the `X-API-Key` header (matching the existing OpenAPI `apiKeyAuth` security scheme and the existing ingest `Client` at `client.py:116`)
- **THEN** `project_id` is derived server-side from the API key; the client does not pass a project id explicitly

---

### Requirement: Control surface methods

The control client MUST expose the full frozen control surface from Proposal 1 and this proposal.

#### Scenario: Method names and endpoints
- **WHEN** the control client is used
- **THEN** it exposes methods matching the following endpoint set (exact Python method names may follow SDK conventions, e.g. `snake_case`):
  - `start` → `POST /v1/engine/runs` (start a new run)
  - `signal` → `POST /v1/engine/runs/{run_id}/signal`
  - `cancel` → `POST /v1/engine/runs/{run_id}/cancel`
  - `terminate` → `POST /v1/engine/runs/{run_id}/terminate`
  - `get_instance` → `GET /v1/engine/instances/{instance_key}` (matching the existing OpenAPI path parameter)
  - `get_run` → `GET /v1/engine/runs/{run_id}`
  - `get_result` → `GET /v1/engine/runs/{run_id}/result`
  - `get_history` → `GET /v1/engine/runs/{run_id}/history`
  - `get_pending_work` → `GET /v1/engine/runs/{run_id}/pending-work`
  - `purge` → `POST /v1/engine/runs/{run_id}/purge`
  - `repair` → `POST /v1/engine/runs/{run_id}/repair`
  - `wait_for_terminal` → polling helper (see separate requirement)

#### Scenario: Request / response typing
- **WHEN** a control method is called
- **THEN** request bodies and response objects use typed Python objects (dataclasses / pydantic / typed dicts) generated or derived from the OpenAPI schema
- **THEN** response objects are returned to callers as typed values (not raw dicts)

#### Scenario: Error mapping
- **WHEN** a control method receives a non-2xx response
- **THEN** the client raises a typed exception (e.g. `EngineRunNotTerminalError` for 409 on purge, `EngineRunNotFoundError` for 404)
- **THEN** error types are documented and importable from the same module
- **THEN** `EngineHistoryExpiredError` is NOT used — `get_history()` returns a typed response object with an `expired: bool` field (the server returns HTTP 200 for purged-empty history, not an error code). Callers check `response.expired` rather than catching an exception

#### Scenario: Idempotent methods are safe to retry
- **WHEN** `terminate`, `purge`, or `repair` is retried after the same operation already completed
- **THEN** the second call returns the structured result from the server reflecting the current state without raising
- **THEN** the client does not attempt its own write-side retry that could double-execute non-idempotent endpoints

---

### Requirement: wait_for_terminal polling helper

The control client MUST provide a `wait_for_terminal()` polling helper.

#### Scenario: Default poll interval
- **WHEN** `wait_for_terminal(run_id)` is called without an explicit poll interval
- **THEN** the client polls at a default interval of `1s`
- **THEN** the poll interval is configurable via a keyword argument

#### Scenario: Optional timeout
- **WHEN** a timeout argument is provided
- **THEN** the helper polls until terminal OR until the timeout is exceeded
- **THEN** on timeout, the helper raises a typed timeout error (e.g. `EngineRunWaitTimeoutError`)
- **THEN** omitting the timeout means no deadline is enforced

#### Scenario: Terminal return value
- **WHEN** the helper observes the run in a terminal state (`COMPLETED`, `FAILED`, `CANCELLED`, `TERMINATED`)
- **THEN** it returns the final run summary / detail object (same shape as `get_result()` returns for terminal runs)
- **THEN** the helper does not return for non-terminal observations

#### Scenario: Uses existing control methods
- **WHEN** the helper polls
- **THEN** it calls `get_result()` (or `get_run()`) internally
- **THEN** it does not introduce a dedicated waiting endpoint or separate HTTP path

#### Scenario: Handles non-terminal 409 during polling
- **WHEN** `get_result()` returns a 409 `run_not_terminal` response during polling
- **THEN** the helper treats this as "not yet terminal" and continues polling
- **THEN** the 409 is NOT raised as an exception during the polling loop — it is the expected non-terminal signal

#### Scenario: Respects API key and scoping
- **WHEN** the helper runs
- **THEN** it uses the same `api_key` and `endpoint` as the control client instance
- **THEN** cross-project runs appear as 404 (same as any other control method)

---

### Requirement: Contract-driven code generation alignment

The control client SHOULD reuse types generated from the OpenAPI schema via the existing SDK generation flow where applicable, and MUST NOT duplicate request / response shapes by hand.

#### Scenario: Types derived from OpenAPI
- **WHEN** a request body or response type is defined in OpenAPI (e.g. `EngineRunResultResponse`, `EnginePendingWorkResponse`, purge/repair request+response bodies)
- **THEN** the control client uses the same shape (possibly via generated code or a thin hand-maintained mirror that is kept in sync via `make generate`)
- **THEN** hand-authored drift between the SDK shape and the OpenAPI schema is not introduced

#### Scenario: Tests enforce contract shape
- **WHEN** SDK tests run against a running platform (integration tests) or recorded fixtures (unit tests)
- **THEN** they assert that responses decode into the typed shapes without discarding fields
- **THEN** adding a new field server-side does NOT silently break the SDK
