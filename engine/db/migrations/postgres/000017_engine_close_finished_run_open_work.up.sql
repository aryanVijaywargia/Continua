-- Since #359, a run that completes or fails cancels its open activity tasks and
-- discards its open inbox items in the same transaction. Runs that finished before
-- that change can still hold open work: a worker can claim such a task again, and a
-- retry report appends history to the finished run. Close that work once.
--
-- Completed and failed runs never change status again. These statements lock only
-- task and inbox rows, never run or trace rows, so they add no lock-order cycle.
UPDATE engine.activity_tasks AS t
SET status = 'cancelled',
    completed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
FROM engine.runs AS r
WHERE t.run_id = r.id
  AND r.status IN ('completed', 'failed')
  AND t.status IN ('queued', 'claimed');

UPDATE engine.inbox AS i
SET status = 'discarded',
    resolved_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
FROM engine.runs AS r
WHERE i.run_id = r.id
  AND r.status IN ('completed', 'failed')
  AND i.status IN ('pending', 'claimed');
