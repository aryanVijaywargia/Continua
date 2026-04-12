-- name: CreateChildWorkflow :one
INSERT INTO engine.child_workflows (
    project_id,
    parent_instance_id,
    parent_run_id,
    child_key,
    requested_definition_name,
    requested_definition_version,
    child_instance_id,
    child_instance_key,
    current_child_run_id,
    root_run_id,
    child_depth
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetChildWorkflowByParentRunAndKey :one
SELECT *
FROM engine.child_workflows
WHERE project_id = $1
  AND parent_run_id = $2
  AND child_key = $3;

-- name: GetChildWorkflowByParentRunAndKeyForUpdate :one
SELECT *
FROM engine.child_workflows
WHERE project_id = $1
  AND parent_run_id = $2
  AND child_key = $3
FOR UPDATE;

-- name: GetChildWorkflowByChildInstanceForUpdate :one
SELECT *
FROM engine.child_workflows
WHERE project_id = $1
  AND child_instance_id = $2
FOR UPDATE;

-- name: GetChildWorkflowByCurrentChildRunForUpdate :one
SELECT *
FROM engine.child_workflows
WHERE project_id = $1
  AND current_child_run_id = $2
FOR UPDATE;

-- name: GetChildWorkflowOutcomeByParentRunAndKey :one
SELECT cw.*,
       terminal.result AS terminal_result,
       terminal.last_error_code AS terminal_last_error_code,
       terminal.last_error_message AS terminal_last_error_message,
       terminal.status AS terminal_run_status
FROM engine.child_workflows AS cw
LEFT JOIN engine.runs AS terminal
    ON terminal.id = cw.terminal_child_run_id
WHERE cw.project_id = $1
  AND cw.parent_run_id = $2
  AND cw.child_key = $3;

-- name: ListChildWorkflowOutcomesByParentRun :many
SELECT cw.*,
       terminal.result AS terminal_result,
       terminal.last_error_code AS terminal_last_error_code,
       terminal.last_error_message AS terminal_last_error_message,
       terminal.status AS terminal_run_status
FROM engine.child_workflows AS cw
LEFT JOIN engine.runs AS terminal
    ON terminal.id = cw.terminal_child_run_id
WHERE cw.project_id = $1
  AND cw.parent_run_id = $2
ORDER BY cw.child_key ASC, cw.created_at ASC;

-- name: ListChildWorkflowsByParentRun :many
SELECT *
FROM engine.child_workflows
WHERE project_id = $1
  AND parent_run_id = $2
ORDER BY child_key ASC, created_at ASC;

-- name: ListActiveChildWorkflowsByParentRun :many
SELECT *
FROM engine.child_workflows
WHERE project_id = $1
  AND parent_run_id = $2
  AND status = 'active'
ORDER BY child_key ASC, created_at ASC;

-- name: ListActiveChildWorkflowDescendants :many
WITH RECURSIVE descendants AS (
    SELECT cw.*
    FROM engine.child_workflows AS cw
    WHERE cw.project_id = $1
      AND cw.parent_run_id = $2
      AND cw.status = 'active'

    UNION ALL

    SELECT child.*
    FROM engine.child_workflows AS child
    INNER JOIN descendants AS parent
        ON parent.project_id = child.project_id
       AND parent.current_child_run_id = child.parent_run_id
    WHERE child.status = 'active'
)
SELECT *
FROM descendants
ORDER BY child_depth DESC, created_at DESC;

-- name: UpdateChildWorkflowTerminal :one
UPDATE engine.child_workflows
SET terminal_child_run_id = $3,
    status = $4,
    updated_at = NOW()
WHERE project_id = $1
  AND current_child_run_id = $2
  AND status = 'active'
RETURNING *;

-- name: MarkChildWorkflowParentWaitFailed :one
UPDATE engine.child_workflows
SET parent_wait_failed_at = COALESCE(parent_wait_failed_at, NOW()),
    parent_wait_error_code = $4,
    parent_wait_error_message = $5,
    updated_at = NOW()
WHERE project_id = $1
  AND parent_run_id = $2
  AND child_key = $3
RETURNING *;

-- name: UpdateChildWorkflowContinuation :one
UPDATE engine.child_workflows
SET current_child_run_id = sqlc.arg(next_child_run_id),
    continuation_count = continuation_count + 1,
    updated_at = NOW()
WHERE project_id = sqlc.arg(project_id)
  AND current_child_run_id = sqlc.arg(previous_child_run_id)
  AND status = 'active'
RETURNING *;
