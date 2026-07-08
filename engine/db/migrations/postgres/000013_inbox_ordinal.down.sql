DROP INDEX engine.idx_engine_inbox_discarded_timer_order;
DROP INDEX engine.idx_engine_inbox_run_kind_open_order;
DROP INDEX engine.idx_engine_inbox_run_pending_order;
DROP INDEX engine.idx_engine_inbox_open_order;

ALTER TABLE engine.inbox DROP COLUMN ordinal;
