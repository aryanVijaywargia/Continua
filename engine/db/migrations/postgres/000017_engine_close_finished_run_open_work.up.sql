-- Since #359, a run that completes or fails cancels its open activity tasks and
-- discards its open inbox items in the same transaction. Runs that finished before
-- that change can still hold open work: a worker can claim such a task again, and a
-- retry report appends history to the finished run. Close that work once.
--
-- Completed and failed runs never change status again. Each statement locks only
-- task or inbox rows, but another multi-row statement on the same table (the
-- retention reap) can visit them in a different order, so the rows are taken with
-- SKIP LOCKED: the migration never waits on a row lock, and a failed migration
-- cannot leave the version dirty. A skipped row is one another transaction is
-- changing at that moment; retention removes it with its run.
WITH open_tasks AS (
    SELECT t.id
    FROM engine.activity_tasks AS t
    JOIN engine.runs AS r ON r.id = t.run_id
    WHERE r.status IN ('completed', 'failed')
      AND t.status IN ('queued', 'claimed')
    FOR UPDATE OF t SKIP LOCKED
)
UPDATE engine.activity_tasks AS t
SET status = 'cancelled',
    completed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
FROM open_tasks
WHERE t.id = open_tasks.id;

WITH open_items AS (
    SELECT i.id
    FROM engine.inbox AS i
    JOIN engine.runs AS r ON r.id = i.run_id
    WHERE r.status IN ('completed', 'failed')
      AND i.status IN ('pending', 'claimed')
    FOR UPDATE OF i SKIP LOCKED
)
UPDATE engine.inbox AS i
SET status = 'discarded',
    resolved_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
FROM open_items
WHERE i.id = open_items.id;
