# Change: Add Engine Child Workflow Composition And Lineage

## Why

Workflows need to orchestrate sub-workflows as first-class durable primitives. Without child workflow support, users must flatten all logic into a single workflow definition or manually coordinate separate instances, losing transactional guarantees and making replay correctness impossible. Child composition also requires lineage tracking so the debugger can navigate parent-child trees.

## What Changes

### Milestone 1A: Engine Composition Gate
- Add `ChildWorkflow(...)` and `ChildWorkflowWithOptions(...)` blocking primitives to `workflow.Context`
- Add public `workflow.ChildWorkflowError` inspection for failed/cancelled/terminated child outcomes
- Add child workflow history events (`child_workflow.scheduled`, `child_workflow.started`, `child_workflow.completed`, `child_workflow.failed`, `child_workflow.cancelled`, `child_workflow.terminated`, `child_workflow.wait_failed`)
- Add `WaitKindChildWorkflow` wait kind and replay handling equivalent to activity/timer waits
- Add `engine.child_workflows` authoritative relationship table
- Add denormalized lineage columns to `engine.runs` (`parent_run_id`, `root_run_id`, `child_key`, `child_depth`)
- Enforce `max_child_depth = 32` and `max_continuation_follow_depth = 32`
- Implement child ContinueAsNew continuation-count tracking and terminal state wake
- Implement cooperative cancel cascade and immediate force-terminate cascade

### Milestone 1B: Projection, Search, And Debugger Gate (after 1A green)
- Add projected trace lineage columns (`engine_parent_run_id`, `engine_root_run_id`, `engine_child_key`, `engine_child_depth`)
- Add lineage and definition-version trace filters
- Extend projection repair/backfill for lineage columns
- Add debugger UX: desktop breadcrumb, child rows in context drawer, child navigation

## Impact
- Affected specs: engine-child-workflow-primitives (new), engine-child-workflow-state (new), engine-child-lineage-projection (new), debugger-child-lineage-ux (new)
- Affected code: `engine/pkg/workflow/`, `engine/pkg/history/`, `engine/internal/workflow/`, `engine/internal/store/`, `engine/internal/projector/`, `engine/db/`, `db/platform/`, `internal/store/`, `internal/api/`, `web/src/`
