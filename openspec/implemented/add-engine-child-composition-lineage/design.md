## Context

The engine currently supports activities, timers, and signals as blocking primitives. Workflows cannot spawn other workflows. This proposal adds child workflow composition as a transactional, replay-safe primitive with full lineage tracking from engine state through to the debugger UI.

### Stakeholders
- Engine runtime: new wait kind, replay logic, activation transaction changes
- Engine store: new table, denormalized columns, migration
- Projector: lineage columns projected from engine state to platform traces
- Platform API: new trace filters
- Debugger UI: breadcrumb navigation and child listing

## Goals / Non-Goals

### Goals
- Replay-deterministic child workflow scheduling and completion
- Transactional atomicity: child creation, continuation, and terminal transitions are atomic with history appends
- Lineage tracking from engine tables through projected traces to debugger UI
- Cancel and terminate cascade semantics
- Depth limits to prevent runaway nesting

### Non-Goals
- Fire-and-forget (detached) child workflows
- Multi-child fan-out (parallel children from a single workflow step)
- Child-completion inbox kind (child completion mirrors activity: authoritative state + wake, replay reads from `engine.child_workflows`)
- Session-compare child grouping
- Historical trace migration into engine history

## Decisions

### Decision: No child-completion inbox kind
Child completion follows the activity-task outcome pattern. On child terminal transition, update `engine.child_workflows` status and `terminal_child_run_id`, then call `WakeWaitingRun` only when the parent has not already consumed a durable parent-side wait failure and is still waiting on the matching child workflow wait identity. Replay reads the child outcome from `engine.child_workflows` directly.

**Why:** Adding a new inbox kind would duplicate authoritative state and complicate replay without benefit. Activities already prove this pattern works.

**Alternatives considered:** Inbox-based child completion notification. Rejected because it introduces dual-write complexity and the inbox would only ever contain one item per child key.

### Decision: Default child instance key is deterministic
Derived as `child:v1:<hex_sha256(project_id.String() + "\x00" + parent_run_id.String() + "\x00" + child_key)>` using canonical UUID string inputs and a lowercase hexadecimal SHA-256 digest. This ensures replay determinism without requiring the user to provide an instance key.

**Why:** Instance keys must be deterministic for replay. Deriving from parent run ID and child key makes them unique per parent run without user burden.

**Alternatives considered:** Requiring explicit instance keys. Rejected because it adds friction for the common case and risks non-determinism if users generate keys incorrectly.

### Decision: Denormalized lineage columns on engine.runs
`parent_run_id`, `root_run_id`, `child_key`, and `child_depth` are denormalized onto `engine.runs` for query performance. `engine.child_workflows` remains authoritative; repair resolves disagreements in its favor.

**Why:** Run queries for lineage filtering should not require joins to `engine.child_workflows`. Pre-production row counts make the backfill migration safe as an in-migration UPDATE.

### Decision: Depth limits
`max_child_depth = 32` and `max_continuation_follow_depth = 32`. Exceeding either is a deterministic workflow failure (not a runtime error that might differ across replays). Child workflow continuation count is tracked on `engine.child_workflows`; ordinary child ContinueAsNew updates below the limit do not wake the parent, but the 32nd continuation wakes the parent so replay can fail the wait deterministically while leaving the child active.

**Why:** Prevents unbounded nesting that would exhaust resources. 32 levels is generous for any reasonable workflow tree.

### Decision: Cancel cascade is cooperative; terminate cascade is immediate and recursive
Cooperative cancel only cascades after the parent workflow returns `workflow.ErrCancelled`. Force terminate cascades immediately and recursively in the parent termination transaction, transitioning all active descendants (children, grandchildren, etc.) to terminated.

**Why:** Cooperative cancel respects workflow autonomy (children get a chance to clean up). Force terminate is an operator emergency action that must propagate immediately. Recursive termination prevents orphaned grandchildren; direct-only would leave active descendants with no parent to observe their outcomes.

### Decision: Child ContinueAsNew tracking
On child ContinueAsNew, create the next child run and update `current_child_run_id` plus `continuation_count` in the same transaction. The parent wait blocks until a terminal child run exists (`terminal_child_run_id` is set), except for the continuation follow-depth guard described above.

If the child reaches a terminal state after the 32nd continuation but before the parent records the wait failure, the parent-side wait guard wins and replay returns `max_continuation_follow_depth_exceeded`.

**Why:** ContinueAsNew is transparent to the parent. The parent asked for a child workflow result; it should not wake on intermediate continuations.

## Risks / Trade-offs

- **Migration risk:** Backfilling `engine.runs` with `root_run_id = id` and `child_depth = 0` assumes small pre-production row counts. If row counts grow before this lands, consider batched backfill instead.
- **Transaction size:** First child creation does scheduled event + instance/run creation + child_workflows insert + started event in one transaction. This is acceptable for single-child-per-step but would need review for fan-out.
- **Replay complexity:** Child workflow wait adds a fourth code path to replay alongside activity, timer, and signal. Each must be independently tested for determinism.

## Debugger UX

### Text Wireframe

Desktop trace header:

```text
Trace name
Root trace > Parent trace > Current trace
definition@version  status  trace id
```

Desktop trace context drawer:

```text
Trace Context
  existing trace/session fields...

  Child Workflows
    child_key
    definition@version
    status
    Open trace
```

Mobile Summary tab:

```text
Child Workflows
  parent trace link (when present)
  direct child rows
  existing summary content...
```

### Acceptance Criteria

- Child traces render a root-to-current breadcrumb under the desktop trace header with each ancestor trace clickable and the current trace as the trailing item.
- Breadcrumb navigation preserves the existing `returnTo` state so operators can move back to traces or sessions without losing context.
- The existing desktop trace context drawer shows direct child workflows for the current run using the existing `/api/traces` surface filtered by `engine_parent_run_id`.
- Each child row shows child key, definition name/version, run status summary, and a trace link to `/traces/{childTraceId}`.
- Mobile keeps lineage in the existing Summary tab and does not duplicate lineage in the header or trace context sheet; no new top-level mobile tab is introduced.
- Missing projected lineage data degrades quietly: omit the breadcrumb when no parent trace is found, and show an empty-state message when a run has no direct children.

## Open Questions

None. All clarifications have been resolved in the proposal phase.
