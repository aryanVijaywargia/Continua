# Change: Add Engine Dark-Launch Core (Phase 11)

## Why

The engine foundation (Phase 10.1) established schema, store, CLI, and generation but contains no runtime execution. Before exposing any public surface — OpenAPI routes, debugger projection, SDK control, or `cmd/continua` integration — the engine must prove one complete internal workflow end-to-end: start, activity, timer, signal, restart recovery, and replay-correct completion. This dark-launch phase builds that proof entirely inside `continua-engine`.

## What Changes

### Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| **engine-schema-runtime-delta** | NEW | Migration 000002 adding `waiting` to `run_lifecycle_status`, plus `result`, `custom_status`, `waiting_for`, `completed_at` columns on `engine.runs` |
| **engine-runtime-config** | NEW | Env-only config vars for runtime poll intervals, lease TTLs, and request-dedupe TTL |
| **engine-workflow-authoring** | NEW | Minimal public Go API in `engine/pkg/workflow` for defining workflow `Definition` and using `Context` primitives |
| **engine-history-events** | NEW | Canonical event types, payload structs, and JSON encoding in `engine/internal/history` |
| **engine-runtime-execution** | NEW | Three worker loops (workflow, activity, maintenance), activation transaction, replay-from-history, guarded CAS transitions, activity at-least-once execution |
| **engine-cli-runtime** | NEW | `serve`, `start`, `signal`, `cancel`, and `inspect` commands with JSON output for `continua-engine` |

### Key Design Decisions

1. **Internal-only surface**: No OpenAPI routes, no debugger projection, no `cmd/continua` wiring, no SDK control surface. All interaction is via `continua-engine` CLI commands.
2. **Single-primitive blocking**: Phase 11 blocks on one primitive at a time (activity, timer, or signal). Multi-primitive waits are deferred.
3. **History is authoritative**: `runs.result`, `runs.custom_status`, and `runs.waiting_for` are materialized caches rewritten by each activation. History events are the only source of truth, including the initial `workflow.started` event that durably carries optional workflow input.
4. **CAS-based guarded transitions**: Workflow-owned transitions CAS on `(status, claimed_by)`. External wakeups CAS on `status` only. `ErrStaleClaim` is reserved for ownership-based CAS paths; status-only wakeups report applied vs no-op.
5. **Activation transaction**: One DB transaction per workflow activation loads state, replays history, processes side-table completions and inbox, appends the full event stream at explicit schedule/consume/terminal points, writes caches, and transitions the run.
6. **At-least-once activities**: Activity execution is explicitly at-least-once. Stale claim rejection prevents double-completion but does not prevent duplicate execution.
7. **Exact version match**: `start` validates the requested `definition_name` + `definition_version` against the compiled registry before writing, and replay requires the same exact match. This is a Phase 11 simplification.
8. **Observational cancellation**: Cancel inserts an inbox row; workflow code checks `CancellationRequested()`. No forced preemption.

### Breaking Changes

None. The live product path (`POST /v1/ingest`, `/api/traces*`, `/api/sessions*`, debugger UI, `cmd/continua` runtime) is completely untouched. All changes are isolated to the `engine/` module.

## Impact

### Affected Specs

- `engine-schema-runtime-delta` (new — extends `engine-schema-foundation`)
- `engine-runtime-config` (new — extends `engine-cli-foundation`)
- `engine-workflow-authoring` (new)
- `engine-history-events` (new)
- `engine-runtime-execution` (new)
- `engine-cli-runtime` (new — extends `engine-cli-foundation`)

### Affected Code

| Path | Change |
|------|--------|
| `engine/db/migrations/postgres/000002_runtime_columns.{up,down}.sql` | ADD: enum extension, four new columns on `engine.runs` |
| `engine/db/queries/runs.sql` | MODIFY: add guarded transition queries |
| `engine/db/queries/activity_tasks.sql` | MODIFY: CAS `claimed_by` on complete/fail, add `ListActivityTasksByRun` |
| `engine/db/queries/inbox.sql` | MODIFY: add run-scoped listing/consumption queries |
| `engine/db/queries/request_dedupe.sql` | MODIFY: add atomic start-claim/takeover helpers plus expiry query |
| `engine/internal/config/config.go` | MODIFY: add runtime config vars |
| `engine/internal/store/*.go` | MODIFY: add new store wrappers including atomic start-dedupe claim, `ErrStaleClaim` sentinel |
| `engine/pkg/workflow/` | ADD: `Definition`, `Context`, primitive methods including workflow input access |
| `engine/internal/history/` | ADD: event type registry, payload structs, canonical encoding |
| `engine/internal/workflow/` | ADD: replay engine, activation transaction |
| `engine/internal/activity/` | ADD: activity worker loop |
| `engine/internal/worker/` | ADD: worker loop infrastructure, identity generation |
| `engine/cmd/continua-engine/main.go` | MODIFY: add `serve`, `start`, `signal`, `cancel`, `inspect` commands |
| `engine/cmd/continua-engine/internal/darklaunch/` | ADD: demo workflow definition and activity handlers |

### Not Affected

- `cmd/continua` — no changes to the main server binary
- `internal/api`, `internal/ingest`, `internal/jobs`, `internal/store` — untouched
- `contracts/openapi/openapi.yaml` — no API surface changes
- `web/src` — no frontend changes
- `sdks/python`, `sdks/typescript` — no SDK changes
- `db/platform` — no platform schema or query changes

## Assumptions

- Activity execution is explicitly at-least-once in Phase 11.
- Engine tables are the only execution truth.
- Restart recovery reclaims the same run; it does not create a new `run_number`.
- `projection_checkpoints`, debugger projection, ContinueAsNew, subworkflows, sticky cache, remote activity workers, and public lifecycle APIs remain deferred.
- `ClaimNextInboxItem` is explicitly unused in Phase 11; inbox is consumed inside the activation transaction.
- `instance_key` is the consistent external identifier in Phase 11. Public naming can change later.
