# Capability: engine-public-api

Public REST API surface under `/v1/engine/*` hosted by `cmd/continua` for starting, inspecting, signaling, and canceling engine workflow runs.

Related capabilities: [engine-trace-projection](../engine-trace-projection/spec.md), [engine-projector-runtime](../engine-projector-runtime/spec.md)

## ADDED Requirements

### Requirement: Engine API rollout gating

The engine API surface MUST be gated behind an environment variable and a preview header for mutating routes.

#### Scenario: API disabled by default
- **WHEN** `ENGINE_PUBLIC_API_ENABLED` is not set or is `false`
- **THEN** all `/v1/engine/*` routes return 404

#### Scenario: API enabled by environment variable
- **WHEN** `ENGINE_PUBLIC_API_ENABLED=true`
- **THEN** all `/v1/engine/*` routes are registered and accessible

#### Scenario: Mutating routes require preview header
- **WHEN** `ENGINE_PUBLIC_API_ENABLED=true`
- **WHEN** a request to `POST /v1/engine/runs`, `POST /v1/engine/runs/{run_id}/signal`, or `POST /v1/engine/runs/{run_id}/cancel` lacks the `X-Continua-Engine-Preview: 1` header
- **THEN** the server returns 400 with a message indicating the preview header is required

#### Scenario: Read routes do not require preview header
- **WHEN** `ENGINE_PUBLIC_API_ENABLED=true`
- **WHEN** a GET request is made to any `/v1/engine/*` route without the preview header
- **THEN** the request is processed normally

---

### Requirement: Engine API authentication

All `/v1/engine/*` routes MUST reuse existing API-key authentication and `project_id` request-context scoping.

#### Scenario: Authenticated request
- **WHEN** a valid API key is provided on an engine route
- **THEN** the `project_id` is extracted from the API key and injected into request context
- **THEN** all engine operations are scoped to that project

#### Scenario: Unauthenticated request
- **WHEN** no valid API key is provided on an engine route
- **THEN** the server returns 401

---

### Requirement: Start engine run

`POST /v1/engine/runs` MUST create an engine run and its projected trace shell atomically.

#### Scenario: Start run request body
- **WHEN** a start request is received
- **THEN** the request body MUST include: `instance_key`, `definition_name`, `definition_version`, `request_key`
- **THEN** the request body MAY include: `input`, `session` (with `key`, `name`, `metadata`), `trace` (with `name`, `user_id`, `tags`, `environment`, `release`, `metadata`)

#### Scenario: Atomic start transaction
- **WHEN** a valid start request is processed
- **THEN** within a single shared-Postgres transaction:
  - validate the definition against `engine.definition_catalog` rows published by `continua-engine`
  - claim/replay start dedupe in `engine.request_dedupe`
  - create `engine.instances` (reuse of existing instance_key is an `instance_conflict`)
  - create `engine.runs`
  - append initial `engine.history` with `workflow.started` using public engine history event constants and payload DTOs
  - create or upsert projected `public.sessions`
  - create projected `public.traces` with engine linkage columns
  - create projected root span shell in `public.spans`
- **THEN** the response includes the `run_id`, `instance_key`, and `trace_id`

#### Scenario: Session upsert merge rule
- **WHEN** the start request includes `session.key` that matches an existing session's `external_id`
- **THEN** the session's `name` is updated only if the request provides a non-empty `session.name`
- **THEN** the session's `metadata` is shallow-merged with the request's `session.metadata` (new keys added, existing keys overwritten, missing keys retained)
- **THEN** the session's `updated_at` is refreshed

#### Scenario: Idempotent start replay
- **WHEN** a start request is received with a `request_key` that was already claimed and finalized
- **THEN** the server returns the original response without creating duplicate rows

#### Scenario: Start with missing definition
- **WHEN** a start request references a `definition_name` and `definition_version` that are not registered in the engine runtime
- **THEN** the server rejects the request before creating any run, instance, or history rows
- **THEN** the server returns 400 with error code `definition_not_registered`

#### Scenario: Instance conflict on start
- **WHEN** a start request uses an `instance_key` that already exists for the authenticated project
- **WHEN** the `request_key` is different from the original start request for that instance
- **THEN** the server returns 409 with error code `instance_conflict`
- **THEN** no new run or history rows are created

#### Scenario: Start with missing required fields
- **WHEN** a start request omits `instance_key`, `definition_name`, `definition_version`, or `request_key`
- **THEN** the server returns 400

#### Scenario: Runtime-published catalog is the validation boundary
- **WHEN** `cmd/continua` validates a requested definition before starting a run
- **THEN** it reads `engine.definition_catalog` through the generated engine query package inside the start transaction
- **THEN** it does not import or interrogate `engine/internal/workflow.Registry` directly

#### Scenario: Root-side start path uses only public engine DTOs
- **WHEN** the start handler constructs or parses engine history payloads
- **THEN** it uses a public engine package (for example `engine/pkg/history`)
- **THEN** root-side code does not import `engine/internal/history`

---

### Requirement: Get engine instance

`GET /v1/engine/instances/{instance_key}` MUST return the project-scoped instance and its latest/current run summary.

#### Scenario: Instance found
- **WHEN** a valid `instance_key` is provided for the authenticated project
- **THEN** the response includes the instance metadata and the latest or current run summary

#### Scenario: Instance not found
- **WHEN** the `instance_key` does not exist for the authenticated project
- **THEN** the server returns 404

#### Scenario: Cross-project isolation
- **WHEN** an `instance_key` exists for a different project
- **THEN** the server returns 404 for the requesting project

---

### Requirement: Get engine run

`GET /v1/engine/runs/{run_id}` MUST return the project-scoped run detail.

#### Scenario: Run found
- **WHEN** a valid `run_id` is provided for the authenticated project
- **THEN** the response includes run status, definition info, timestamps, and custom status

#### Scenario: Run not found or cross-project
- **WHEN** the `run_id` does not exist or belongs to a different project
- **THEN** the server returns 404

---

### Requirement: Get engine run result

`GET /v1/engine/runs/{run_id}/result` MUST return the terminal result of a completed run.

#### Scenario: Completed run result
- **WHEN** a run has status `completed`
- **THEN** the response includes the run result payload

#### Scenario: Non-terminal run
- **WHEN** a run has not yet completed (status is `queued`, `running`, or `waiting`)
- **THEN** the server returns 409 indicating the run is not yet terminal

#### Scenario: Failed or cancelled run
- **WHEN** a run has status `failed` or `cancelled`
- **THEN** the response includes error details (error code, error message) rather than a result payload

---

### Requirement: Get engine run history

`GET /v1/engine/runs/{run_id}/history` MUST return the ordered history events for a run.

#### Scenario: History returned with pagination
- **WHEN** a valid `run_id` is provided
- **THEN** the response includes history events ordered by `sequence_no` ascending
- **THEN** each event includes `id`, `sequence_no`, `event_type`, `payload`, `created_at`
- **THEN** the endpoint accepts optional `after` (cursor, `sequence_no` to start after) and `limit` (default 100, max 1000) query parameters
- **THEN** the response includes `has_more` indicating whether additional events exist beyond the returned page

#### Scenario: History pagination cursor
- **WHEN** a request includes `after=N`
- **THEN** only events with `sequence_no > N` are returned, ordered ascending

#### Scenario: Run not found or cross-project
- **WHEN** the `run_id` does not exist or belongs to a different project
- **THEN** the server returns 404

---

### Requirement: Signal engine run

`POST /v1/engine/runs/{run_id}/signal` MUST deliver a signal to a run.

#### Scenario: Signal delivered
- **WHEN** a signal request is received with `signal_name` and optional `payload`
- **THEN** the handler creates an inbox signal item for the target run
- **THEN** if the run is in `waiting` status, it is woken (transitioned to `queued`)

#### Scenario: Signal to non-waiting active run
- **WHEN** a signal is sent to a run with status `queued` or `running`
- **THEN** the inbox item is created and will be consumed on the next activation
- **THEN** the response indicates success (signal is durable)

#### Scenario: Signal to terminal run
- **WHEN** a signal is sent to a run with status `completed`, `failed`, or `cancelled`
- **THEN** the server returns 409 with error code `run_terminal`

#### Scenario: Run not found or cross-project
- **WHEN** the `run_id` does not exist or belongs to a different project
- **THEN** the server returns 404

---

### Requirement: Cancel engine run

`POST /v1/engine/runs/{run_id}/cancel` MUST request cancellation of a run.

#### Scenario: Cancel requested
- **WHEN** a cancel request is received for an active run (status `queued`, `running`, or `waiting`)
- **THEN** the handler creates an inbox cancel item with dedupe key `"cancel:" + run_id`
- **THEN** if the run is in `waiting` status, it is woken

#### Scenario: Duplicate cancel on active run
- **WHEN** a cancel request is received but a cancel inbox item already exists for the run
- **WHEN** the run is still active
- **THEN** the request is idempotent and returns success without creating a duplicate

#### Scenario: Cancel on terminal run
- **WHEN** a cancel request is received for a run that is already `completed`, `failed`, or `cancelled`
- **THEN** the server returns 409 with error code `run_terminal`

#### Scenario: Run not found or cross-project
- **WHEN** the `run_id` does not exist or belongs to a different project
- **THEN** the server returns 404

---

### Requirement: Root-side engine control via generated queries

The platform server (`cmd/continua`) MUST access engine tables through the public generated engine query package (`engine/db/gen/go`), not through hand-copied SQL or engine internal packages.

#### Scenario: Engine query import boundary
- **WHEN** platform-side engine handlers need to read or write engine tables
- **THEN** they use `enginedb.Queries` (from `engine/db/gen/go`) wrapped with project-scoped validation
- **THEN** no `engine/internal/*` packages are imported

#### Scenario: Extending engine queries for root-side use
- **WHEN** the public API requires a new engine-table query
- **THEN** the query is added to `engine/db/queries/*.sql` and regenerated
- **THEN** the root-side control layer consumes the newly generated method
