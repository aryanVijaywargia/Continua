ALTER TABLE engine.inbox ADD COLUMN ordinal BIGINT GENERATED ALWAYS AS IDENTITY;

CREATE INDEX idx_engine_inbox_open_order
    ON engine.inbox(available_at, ordinal)
    WHERE status IN ('pending', 'claimed');

CREATE INDEX idx_engine_inbox_run_pending_order
    ON engine.inbox(run_id, available_at, ordinal)
    WHERE status = 'pending';

CREATE INDEX idx_engine_inbox_run_kind_open_order
    ON engine.inbox(run_id, kind, available_at, ordinal)
    WHERE status IN ('pending', 'claimed');

CREATE INDEX idx_engine_inbox_discarded_timer_order
    ON engine.inbox(run_id, available_at, ordinal)
    WHERE status = 'discarded' AND kind = 'timer';
