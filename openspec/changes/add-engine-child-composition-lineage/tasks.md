## 0. Prerequisite Gate
- [ ] 0.1 Close prior-phase validation debt: retries, suspend/resume, ContinueAsNew, generated drift, and debugger tests must be green before starting Milestone 1A

## 1. Milestone 1A: Engine Composition Gate

### 1.1 Schema And History Foundation
- [x] 1.1.1 Add child workflow history event types and payload structs to `engine/pkg/history/history.go` with the exact payload fields specified in `engine-child-workflow-primitives` (`child_workflow.scheduled`, `child_workflow.started`, `child_workflow.completed`, `child_workflow.failed`, `child_workflow.cancelled`, `child_workflow.terminated`, `child_workflow.wait_failed`)
- [x] 1.1.2 Register new event types in `DecodePayload`, `EventKey`, and `payloadTarget`
- [x] 1.1.3 Add `WaitKindChildWorkflow = "child_workflow"` constant and `ChildWorkflowWait` struct to `engine/pkg/history/`
- [x] 1.1.4 Create engine migration: `engine.child_workflows` table with columns (id, project_id, parent_instance_id, parent_run_id, child_key, requested_definition_name, requested_definition_version, child_instance_id, child_instance_key, current_child_run_id, terminal_child_run_id, root_run_id, child_depth, continuation_count, status, parent_wait_failed_at, parent_wait_error_code, parent_wait_error_message, created_at, updated_at), a uniqueness constraint on `(project_id, parent_run_id, child_key)`, and a uniqueness constraint on `(project_id, child_instance_id)` for unambiguous child-instance ownership
- [x] 1.1.5 Create engine migration: add `parent_run_id`, `root_run_id`, `child_key`, `child_depth` to `engine.runs`; backfill existing rows with `root_run_id = id`, `child_depth = 0`
- [x] 1.1.6 Add sqlc queries for `engine.child_workflows` CRUD (create, get by parent run + child key, get by child instance, list by parent run, update status/terminal run, set parent wait failed marker/error fields)
- [x] 1.1.7 Add sqlc queries for lineage-aware run creation (root creation sets parent_run_id NULL, root_run_id = id, child_key NULL, child_depth 0; child creation accepts parent/root/child_key/depth params)
- [x] 1.1.8 Run `make generate` and verify generated code compiles

### 1.2 Store Layer
- [x] 1.2.1 Add store wrappers for child workflow queries in `engine/internal/store/`
- [x] 1.2.2 Add store/query support for atomic child creation and keep the transaction orchestration in `engine/internal/workflow/activation.go` (instance get-or-create + run create + child_workflows insert/update + history append in one transaction)
- [x] 1.2.3 Add store/query support for child terminal transition and keep the transaction orchestration in `engine/internal/workflow/activation.go` (update child_workflows status + terminal_child_run_id; call WakeWaitingRun only when no parent wait failed marker is set and the parent is still waiting on the matching child workflow wait identity)
- [x] 1.2.4 Add store/query support for child ContinueAsNew and keep the transaction orchestration in `engine/internal/workflow/activation.go` (create next child run + update current_child_run_id and continuation_count atomically)

### 1.3 Workflow Context API
- [x] 1.3.1 Add `ChildWorkflowOptions` struct to `engine/pkg/workflow/` with `InstanceKey` field
- [x] 1.3.2 Add `ChildWorkflow(childKey, definitionName, definitionVersion string, input any, out any) error` to `workflow.Context` interface
- [x] 1.3.3 Add `ChildWorkflowWithOptions(childKey, definitionName, definitionVersion string, input any, out any, opts ChildWorkflowOptions) error` to `workflow.Context` interface
- [x] 1.3.4 Implement default child instance key derivation: `child:v1:<hex_sha256(project_id.String() + "\x00" + parent_run_id.String() + "\x00" + child_key)>` using canonical UUID strings and lowercase hex digest encoding
- [x] 1.3.5 Add exported `workflow.ChildWorkflowError` with `Code()`, `Message()`, and `TerminalState()` accessors; child terminal errors and parent-side child wait failures must support `errors.As`

### 1.4 Replay Logic
- [x] 1.4.1 Add child workflow replay path to `workflowRunner` in `engine/internal/workflow/replay.go`: consume `child_workflow.scheduled` + `child_workflow.started` on replay, check for terminal outcome events and durable `child_workflow.wait_failed` events
- [x] 1.4.2 Add pending child outcome loading (from `engine.child_workflows` table) analogous to `pendingActivities`; when `continuation_count >= 32` is present before a recorded parent wait failure, prioritize appending `child_workflow.wait_failed` over any later terminal child outcome
- [x] 1.4.3 Add `blockOnWait` call with `ChildWorkflowWait` when child is not yet terminal
- [x] 1.4.4 Add `newChildWorkflow` field to `activationDecision` for the activation transaction to consume

### 1.5 Activation Transaction
- [x] 1.5.1 Extend `Activate` in `engine/internal/workflow/activation.go` to handle `newChildWorkflow` decisions: create child instance/run, insert child_workflows, append scheduled+started events in one transaction
- [x] 1.5.2 Add child terminal wake path: on child run terminal transition, update child_workflows and call WakeWaitingRun only when no parent wait failed marker is set and the parent is still waiting on the matching child workflow wait identity
- [x] 1.5.3 Add child ContinueAsNew handling in activation: create next child run + update current_child_run_id and continuation_count
- [x] 1.5.4 Add depth enforcement: fail deterministically if `child_depth >= 32`
- [x] 1.5.5 Add continuation follow depth enforcement as a parent-side replay guard: when a child continuation increments continuation_count to 32, wake the parent; on replay append `child_workflow.wait_failed`, set the durable parent wait failed marker/error fields on `engine.child_workflows`, and fail the child wait without setting terminal_child_run_id or changing child status

### 1.6 Cancel And Terminate Cascade
- [x] 1.6.1 Extend cooperative cancel: when parent returns `ErrCancelled`, cancel all active children by inserting cancel inbox items with dedupe key `cancel:<current_child_run_id>` and treating existing dedupe rows as idempotent success
- [x] 1.6.2 Extend force terminate: in parent termination transaction, recursively terminate all active descendants in the lineage tree (transition runs + append history + update child_workflows at every depth)

### 1.7 Milestone 1A Tests
- [x] 1.7.1 Test: child workflow replay determinism (schedule, start, complete round-trip)
- [x] 1.7.2 Test: child key and definition binding idempotency (same child_key/definition version in same parent run returns same child; definition-version mismatch with reused custom instance key fails deterministically; one child instance cannot have multiple parent relationship rows)
- [x] 1.7.3 Test: observable atomicity of scheduled/started state (both events and child_workflows row appear together)
- [x] 1.7.4 Test: lineage table and run-column sync (root runs get root lineage defaults; child_workflows matches child run denormalized columns)
- [x] 1.7.5 Test: depth limit enforcement (child at depth 32 fails deterministically)
- [x] 1.7.6 Test: continuation follow depth enforcement (below-limit continuations do not wake parent; 32nd continuation wakes parent, appends `child_workflow.wait_failed`, sets the parent wait failed marker, and fails the wait while child remains active; if the child reaches terminal state before parent replay, wait_failed still wins; late child completion after wait_failed does not wake the parent out of an unrelated later wait)
- [x] 1.7.7 Test: child ContinueAsNew tracking (parent blocks until terminal child run)
- [x] 1.7.8 Test: terminal child update ordering, wait-identity wake guard, and public `ChildWorkflowError` inspection (completed, failed, cancelled, terminated outcomes)
- [x] 1.7.9 Test: cooperative cancel cascade timing and idempotency (children cancelled after parent returns ErrCancelled; replay/retry does not duplicate child cancel inbox rows)
- [x] 1.7.10 Test: force terminate cascade is recursive (immediate termination of children, grandchildren, and full lineage tree)
- [x] 1.7.11 Test: child and parent terminal event history correctness

## 2. Milestone 1B: Projection, Search, And Debugger Gate

### 2.1 Platform Schema
- [x] 2.1.1 Create platform migration: add `engine_parent_run_id UUID`, `engine_root_run_id UUID`, `engine_child_key TEXT`, `engine_child_depth INTEGER` to `traces` table
- [x] 2.1.2 Add `engine_definition_version` index (if not already indexed)
- [x] 2.1.3 Add indexes for lineage columns on traces
- [x] 2.1.4 Run `make generate`

### 2.2 Projection And Repair
- [x] 2.2.1 Extend projector to populate lineage columns when creating projected traces for child runs
- [x] 2.2.2 Extend projection repair/backfill to populate lineage columns from `engine.child_workflows` and `engine.runs`
- [x] 2.2.3 Wire lineage columns into the existing handwritten dynamic trace search path in `internal/store/search.go` (do not add a parallel sqlc search path)

### 2.3 API Layer
- [x] 2.3.1 Add trace filter parameters to OpenAPI: `engine_run_id`, `engine_definition_version`, `engine_parent_run_id`, `engine_root_run_id`, `engine_child_key`, `engine_child_depth`
- [x] 2.3.2 Run `make generate`
- [x] 2.3.3 Wire new filters in `internal/store/search.go` and `internal/api/traces_handlers.go`
- [x] 2.3.4 Map lineage columns under the existing nested `engine` response object as `parent_run_id`, `root_run_id`, `child_key`, and `child_depth` for both trace list rows and trace detail: update projected trace mapping in `internal/api/mapper.go`/`internal/api/engine_mapper.go`, and extend the live `engineControl.ReadRunSummary` path (`engineRunSummary`, `buildRunSummary`, `terminalRunSummaryFromRun`, `engineRunSummaryToAPI`, and OpenAPI `EngineRunSummary`) so live summary replacement cannot drop lineage fields

### 2.4 Debugger UX
- [x] 2.4.1 Write `design.md` UX section: text wireframe and acceptance criteria for breadcrumb, child rows, navigation, mobile placement
- [x] 2.4.2 Desktop: add lineage breadcrumb under trace header (parent > current)
- [x] 2.4.3 Desktop: add direct children in existing trace context drawer (child key, definition name/version, run status, trace link)
- [x] 2.4.4 Child click navigates to `/traces/{childTraceId}`
- [x] 2.4.5 Mobile: place child summary in existing Summary tab

### 2.5 Milestone 1B Tests
- [x] 2.5.1 Test: lineage projection populates correct columns for child traces
- [x] 2.5.2 Test: repair/backfill fills lineage columns from engine state for child traces and existing root traces (`engine_root_run_id = own run ID`, `engine_child_depth = 0`)
- [x] 2.5.3 Test: project-scoped filters (existing filters still work)
- [x] 2.5.4 Test: run-id, definition-version, parent, root, child-key, depth filters return correct results
- [x] 2.5.5 Test: API mapping includes nested `engine.parent_run_id`, `engine.root_run_id`, `engine.child_key`, and `engine.child_depth` fields in trace list rows, projected trace detail, and trace detail when live `engineControl.ReadRunSummary` replaces the projected engine summary
- [x] 2.5.6 Test: desktop breadcrumb renders and navigates correctly
- [x] 2.5.7 Test: child navigation opens correct trace detail
- [x] 2.5.8 Test: mobile Summary tab shows child summary
