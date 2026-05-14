# Capability: engine-public-api

Retention/repair/control-SDK layer on top of the Proposal 1 engine public API: introduces a manual purge endpoint with two modes, a per-run repair endpoint, an `engine_history_expired` condition on the existing history endpoint, terminal result responses for `summary_only` / `journal_expired` runs, and explicit surfacing of `definition_version_mismatch` failures via existing fields.

Related capabilities: [engine-trace-projection](../engine-trace-projection/spec.md), [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-retention-maintenance](../engine-retention-maintenance/spec.md)

## ADDED Requirements

### Requirement: Purge engine run endpoint

`POST /v1/engine/runs/{run_id}/purge` MUST let operators delete projection detail and optionally engine history for a terminal run while preserving the operator shell.

#### Scenario: Purge route registration and gating
- **WHEN** `ENGINE_PUBLIC_API_ENABLED=true`
- **THEN** `POST /v1/engine/runs/{run_id}/purge` is registered
- **WHEN** `ENGINE_PUBLIC_API_ENABLED` is not set or is `false`
- **THEN** the route returns 404

#### Scenario: Purge authentication and project scoping
- **WHEN** a purge request arrives with a valid API key
- **THEN** `project_id` is taken from the API key and all engine operations are scoped to that project
- **WHEN** no valid API key is provided
- **THEN** the server returns 401

#### Scenario: Purge request body shape
- **WHEN** a purge request body is parsed
- **THEN** the body is `{"mode": "projection_only" | "full"}`
- **WHEN** the body is missing, malformed, or `mode` is any other value
- **THEN** the server returns 400 with a typed validation error

#### Scenario: Purge rejects non-terminal runs
- **WHEN** a purge request targets a run whose `engine_run_status` is `queued`, `running`, or `waiting`
- **THEN** the server returns 409 Conflict with the existing typed engine error `run_not_terminal`
- **THEN** no projection rows, history rows, or state changes occur

#### Scenario: Purge projection_only response
- **WHEN** a purge request with `mode=projection_only` succeeds against a terminal run
- **THEN** the server returns HTTP 200 with a body that includes at least `run_id`, `mode: "projection_only"`, and the new `projection_state: "summary_only"`
- **THEN** the response indicates that `public.span_events` and non-root `public.spans` were deleted and the trace row + root span were preserved

#### Scenario: Purge full response
- **WHEN** a purge request with `mode=full` succeeds against a terminal run
- **THEN** the server returns HTTP 200 with a body that includes at least `run_id`, `mode: "full"`, and the new `projection_state: "journal_expired"`
- **THEN** the response indicates that projection detail was deleted and engine history rows for the run were deleted

#### Scenario: Purge is allowed during catching_up
- **WHEN** a purge request targets a terminal run whose trace is still `engine_projection_state=catching_up`
- **THEN** the server proceeds, flipping the projection state to `summary_only` or `journal_expired` under row lock
- **THEN** the projector's next write for that run hits the barrier and does not recreate detail rows

#### Scenario: Purge on missing or cross-project run
- **WHEN** the `run_id` does not exist or its trace belongs to a different project
- **THEN** the server returns 404

#### Scenario: Purge idempotency on already-purged runs
- **WHEN** a purge request targets a run whose `engine_projection_state` already matches the requested or stronger barrier
- **THEN** the server returns HTTP 200 with the current projection state
- **THEN** no additional rows are deleted and the response indicates a no-op
- **THEN** `full` on an already-`journal_expired` run is a no-op; `projection_only` on an already-`summary_only` or `journal_expired` run is a no-op

#### Scenario: Purge never deletes the operator shell
- **WHEN** any purge mode succeeds
- **THEN** the `public.traces` row remains, including `engine_run_id`, `engine_instance_key`, `engine_definition_name`, `engine_definition_version`, `engine_run_status`, `engine_projection_state`, `engine_projection_updated_at`, and the terminal summary
- **THEN** the root span carrying the terminal summary/result/failure payload remains

---

### Requirement: Repair engine run endpoint

`POST /v1/engine/runs/{run_id}/repair` MUST request projector resume for a single run when retained engine history still exists, returning a structured result immediately.

#### Scenario: Repair route registration and gating
- **WHEN** `ENGINE_PUBLIC_API_ENABLED=true`
- **THEN** `POST /v1/engine/runs/{run_id}/repair` is registered
- **WHEN** `ENGINE_PUBLIC_API_ENABLED` is not set or is `false`
- **THEN** the route returns 404

#### Scenario: Repair authentication and project scoping
- **WHEN** a repair request arrives with a valid API key
- **THEN** `project_id` is taken from the API key and all engine operations are scoped to that project
- **WHEN** no valid API key is provided
- **THEN** the server returns 401

#### Scenario: Repair is single-run only
- **WHEN** a repair request is parsed
- **THEN** the only targeted run is identified by `run_id` in the path
- **THEN** no bulk or multi-run repair request shape is accepted at this public route

#### Scenario: Repair response shape
- **WHEN** repair returns HTTP 200
- **THEN** the body includes at least `run_id`, a boolean `accepted`, a string `reason`, and the current `projection_state`
- **THEN** allowed `reason` values include `already_up_to_date`, `history_expired`, `no_events_to_project`, `repair_requested`, and `already_catching_up`

#### Scenario: Repair on up_to_date run
- **WHEN** repair runs against a trace with `engine_projection_state=up_to_date`
- **THEN** the response is `{accepted: false, reason: "already_up_to_date", projection_state: "up_to_date"}`
- **THEN** no projector work is performed

#### Scenario: Repair on summary_only run with retained history beyond checkpoint (catching_up recovery)
- **WHEN** repair runs against a trace with `engine_projection_state=summary_only` and retained engine history beyond `engine_last_projected_history_id` (i.e. the trace was still `catching_up` before purge)
- **THEN** the server flips the trace back to `projection_state: "catching_up"` and returns `{accepted: true, reason: "repair_requested", projection_state: "catching_up"}`
- **THEN** the separate `continua-engine` projector later rebuilds detail from the checkpoint forward
- **THEN** repair does not reproject from zero; it only resumes from the existing checkpoint

#### Scenario: Repair on summary_only run where checkpoint equals latest (fully projected before purge)
- **WHEN** repair runs against a trace with `engine_projection_state=summary_only` and `engine_last_projected_history_id == engine_latest_history_id`
- **THEN** the response is `{accepted: false, reason: "no_events_to_project", projection_state: "summary_only"}`
- **THEN** the `summary_only` barrier is preserved — repair never "undoes" an operator's deliberate purge of a fully-projected trace
- **THEN** full checkpoint rewind is explicitly out of scope for this change

#### Scenario: Repair on journal_expired run
- **WHEN** repair runs against a trace with `engine_projection_state=journal_expired`
- **THEN** the response is `{accepted: false, reason: "history_expired", projection_state: "journal_expired"}`
- **THEN** no detail is rebuilt and the projection state is not advanced

#### Scenario: Repair on catching_up run
- **WHEN** repair runs against a trace with `engine_projection_state=catching_up`
- **THEN** the response is `{accepted: true, reason: "already_catching_up", projection_state: "catching_up"}`
- **THEN** the server does not enqueue duplicate work or synchronously wait for completion

#### Scenario: Repair on missing or cross-project run
- **WHEN** the `run_id` does not exist or its trace belongs to a different project
- **THEN** the server returns 404

#### Scenario: Repair requests converge without duplicate writes
- **WHEN** repair runs multiple times against the same run
- **THEN** repeated calls do not duplicate projection detail writes
- **THEN** once the run is already `catching_up` or `up_to_date`, later responses reflect that current state instead of forcing another rebuild path

---

### Requirement: History endpoint distinguishes purged-empty from never-had-events

`GET /v1/engine/runs/{run_id}/history` MUST add a typed `expired` marker for runs whose engine history has been purged, so clients can distinguish purged-empty from a run that simply has no events yet. The endpoint already returns HTTP 200 with empty events today; this change adds the marker, not a new status code.

#### Scenario: Journal expired returns typed condition
- **WHEN** a history request targets a run whose trace has `engine_projection_state=journal_expired`
- **THEN** the server returns HTTP 200 with a body that includes an explicit expired marker (e.g. `expired: true`) and an empty `events` array
- **THEN** the response is NOT 404

#### Scenario: Non-expired history unchanged
- **WHEN** a history request targets a run whose trace is `up_to_date`, `catching_up`, or `summary_only`
- **THEN** the response shape is unchanged from the Proposal 1 baseline
- **THEN** `expired` is not set or is `false`

#### Scenario: Missing run still returns 404
- **WHEN** the `run_id` does not exist or belongs to a different project
- **THEN** the server returns 404
- **THEN** the `engine_history_expired` marker is never used as a stand-in for a missing run

---

### Requirement: Result endpoint returns retained terminal shell after purge

`GET /v1/engine/runs/{run_id}/result` MUST continue to return the retained terminal summary shell for `summary_only` and `journal_expired` runs.

#### Scenario: summary_only terminal result
- **WHEN** a result request targets a terminal run with `engine_projection_state=summary_only`
- **THEN** the server returns HTTP 200 with the existing `EngineRunResultResponse` shape
- **THEN** the response includes `run_id`, `status`, `result`, and `failure`

#### Scenario: journal_expired terminal result
- **WHEN** a result request targets a terminal run with `engine_projection_state=journal_expired`
- **THEN** the server returns HTTP 200 with the existing `EngineRunResultResponse` shape
- **THEN** the response includes `run_id`, `status`, `result`, and `failure`

#### Scenario: Purged runs are not 404
- **WHEN** a result request targets a `summary_only` or `journal_expired` terminal run
- **THEN** the server does NOT return 404
- **THEN** the retained summary shell is authoritative for the terminal outcome

---

### Requirement: Engine run detail surfaces definition_version_mismatch via existing fields

The existing engine run detail response MUST expose the requested `definition_name`, `definition_version`, and the stored failure code so clients can identify `definition_version_mismatch` without a new endpoint.

#### Scenario: Definition fields on run detail
- **WHEN** the engine run detail endpoint returns a response for any engine run
- **THEN** the response includes `definition_name` and `definition_version` fields mirroring the requested definition
- **THEN** these fields are populated for every engine run, including failures

#### Scenario: definition_version_mismatch failure code is surfaced
- **WHEN** a run failed with `error_code=definition_version_mismatch`
- **THEN** the response `failure` object includes the error code and message produced by the engine
- **THEN** clients can identify the mismatch by comparing `definition_name`/`definition_version` against the failure code in the same response

#### Scenario: No definition catalog endpoint
- **WHEN** a client needs to display available definition versions for a failed run
- **THEN** no dedicated mismatch or definition-catalog endpoint is provided by this change
- **THEN** clients MUST rely on the fields already returned by engine run detail responses

---

### Requirement: Purge and repair endpoints require preview header

`POST /v1/engine/runs/{run_id}/purge` and `POST /v1/engine/runs/{run_id}/repair` MUST require the `X-Continua-Engine-Preview: 1` header, consistent with all other mutating engine routes (`start`, `signal`, `cancel`, `terminate`).

#### Scenario: Preview header is required
- **WHEN** a purge or repair request lacks the `X-Continua-Engine-Preview: 1` header
- **THEN** the server returns 400 with a message indicating the preview header is required
- **THEN** these routes follow the same preview-gating pattern as `POST /v1/engine/runs/{run_id}/terminate` from Proposal 1

#### Scenario: Disabled gate returns 404
- **WHEN** `ENGINE_PUBLIC_API_ENABLED` is not set or is `false`
- **THEN** both routes return 404
