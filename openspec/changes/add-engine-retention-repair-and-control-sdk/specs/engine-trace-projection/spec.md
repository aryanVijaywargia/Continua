# Capability: engine-trace-projection

Purge / repair / retention additions to the engine trace projection: defines `projection_only` and `full` purge mappings, the retained operator shell, hard write barriers when projection state is `summary_only` or `journal_expired`, race resolution with in-progress `catching_up`, repair behavior from the projector checkpoint, and retention-driven state transitions.

Related capabilities: [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-public-api](../engine-public-api/spec.md), [engine-retention-maintenance](../engine-retention-maintenance/spec.md)

## ADDED Requirements

### Requirement: projection_only purge mapping

A `projection_only` purge MUST delete all `public.span_events` for the trace and all non-root `public.spans` for the trace, preserve the `public.traces` row and the root span, and set `engine_projection_state = 'summary_only'`.

#### Scenario: Span events deleted
- **WHEN** `projection_only` purge runs for a terminal trace
- **THEN** every row in `public.span_events` tied to that trace is deleted

#### Scenario: Non-root spans deleted
- **WHEN** `projection_only` purge runs for a terminal trace
- **THEN** every row in `public.spans` tied to that trace where `parent_span_id IS NOT NULL` (or otherwise classified as non-root) is deleted

#### Scenario: Trace row preserved
- **WHEN** `projection_only` purge runs
- **THEN** the `public.traces` row for the trace remains
- **THEN** all engine shell columns (`engine_run_id`, `engine_instance_key`, `engine_definition_name`, `engine_definition_version`, `engine_run_status`, `engine_projection_state`, `engine_projection_updated_at`, `engine_latest_history_id`, `engine_last_projected_history_id`) remain populated

#### Scenario: Root span preserved
- **WHEN** `projection_only` purge runs
- **THEN** the root span carrying the terminal summary/result/failure payload remains in `public.spans`
- **THEN** the root span's terminal status, timestamps, and payload are unchanged

#### Scenario: Projection state flips to summary_only
- **WHEN** `projection_only` purge commits
- **THEN** `public.traces.engine_projection_state = 'summary_only'`
- **THEN** `public.traces.engine_projection_updated_at` is advanced to the purge timestamp

---

### Requirement: full purge mapping

A `full` purge MUST perform the same projection deletions as `projection_only`, then delete engine history rows for the run, then set `engine_projection_state = 'journal_expired'`.

#### Scenario: Projection deletions first
- **WHEN** `full` purge runs for a terminal run
- **THEN** the projection deletions (span events + non-root spans) from `projection_only` run before any engine history deletion
- **THEN** the trace row and root span remain

#### Scenario: Engine history rows deleted
- **WHEN** `full` purge runs
- **THEN** every row in `engine.history` with `run_id = <run>` is deleted
- **THEN** any engine journal/inbox tail rows the implementation considers part of the history partition are deleted per the apply-stage plan

#### Scenario: Projection state flips to journal_expired
- **WHEN** `full` purge commits
- **THEN** `public.traces.engine_projection_state = 'journal_expired'`
- **THEN** `public.traces.engine_projection_updated_at` is advanced to the purge timestamp

#### Scenario: Engine run row retention
- **WHEN** `full` purge commits
- **THEN** the `engine.runs` row remains (so read endpoints can still return the terminal shell)
- **THEN** the `engine.instances` row remains

---

### Requirement: Projection state barriers block detail writes

The projector MUST treat `engine_projection_state IN ('summary_only', 'journal_expired')` as a hard write barrier and MUST NOT recreate detailed projection rows for that trace.

#### Scenario: Projector checks barrier via centralized write wrapper on every detail write
- **WHEN** the projector is about to write to `public.spans` or `public.span_events` for a trace
- **THEN** the write goes through a single centralized wrapper that reads/locks the `public.traces` row within the same transaction
- **THEN** the wrapper aborts the write if `engine_projection_state` is `summary_only` or `journal_expired`
- **THEN** ALL projector detail-write paths (`projectHistoryRows`, terminal projection, etc.) MUST call through this wrapper — no code path may bypass the barrier check

#### Scenario: Checkpoint does not advance past barrier
- **WHEN** the projector encounters the barrier during a catch-up loop for a run
- **THEN** `engine_last_projected_history_id` for that run is NOT advanced past events whose detail writes were skipped
- **THEN** the projector does not mark the run `up_to_date` on the basis of skipped events

#### Scenario: Barrier persists across projector restarts
- **WHEN** the projector process restarts
- **THEN** the barrier is re-read from `public.traces.engine_projection_state`
- **THEN** no in-memory flag is needed to enforce the barrier

#### Scenario: Non-barrier runs are unaffected
- **WHEN** the projector processes a trace whose `engine_projection_state` is `up_to_date` or `catching_up`
- **THEN** normal detail writes proceed as before
- **THEN** the barrier check is a no-op for non-purged traces

---

### Requirement: Purge wins races with catching_up projector

Purge MUST serialize with the projector via row lock (or equivalent CAS guard) so the projector cannot recreate detail rows after purge has committed.

#### Scenario: Purge acquires trace row lock
- **WHEN** a purge transaction runs
- **THEN** it takes `SELECT ... FOR UPDATE` on `public.traces` for the target run's trace before any deletion
- **THEN** it flips `engine_projection_state` and deletes detail rows inside the same transaction

#### Scenario: Projector write blocked until purge commits
- **WHEN** the projector is mid-catchup on the same trace
- **THEN** its write takes the same row lock (or detects state change via CAS)
- **THEN** it either blocks until purge commits and then aborts on the barrier, or detects the state mismatch via CAS and aborts without writing

#### Scenario: Post-purge projector iteration is a no-op
- **WHEN** the projector's next iteration reads the barrier
- **THEN** it skips detail writes for that run
- **THEN** no re-inserted span or span-event row appears for the purged trace

#### Scenario: Purge during catching_up completes cleanly
- **WHEN** purge commits while `engine_projection_state` was `catching_up`
- **THEN** the state flips from `catching_up` directly to `summary_only` (for projection_only) or `journal_expired` (for full)
- **THEN** partial detail rows written before the purge transaction started are deleted by the purge itself

---

### Requirement: Repair resumes the normal projector path from checkpoint (catching_up recovery only)

Repair MUST reuse the projector's existing catch-up mechanism indirectly by returning the trace to `catching_up`, MUST only rebuild when history exists beyond the checkpoint, and MUST NOT introduce a separate write path. Full checkpoint rewind is explicitly out of scope.

#### Scenario: Repair clears the barrier for summary_only with history beyond checkpoint
- **WHEN** repair runs against a `summary_only` trace AND `engine_last_projected_history_id < engine_latest_history_id` (i.e. the trace was still `catching_up` before purge)
- **THEN** the repair request flips the trace back to `catching_up`
- **THEN** the separate `continua-engine` projector later applies events from `engine_last_projected_history_id + 1` onwards
- **THEN** the state becomes `up_to_date` when the checkpoint reaches `engine_latest_history_id`

#### Scenario: Repair on summary_only with checkpoint at latest is a no-op
- **WHEN** repair runs against a `summary_only` trace AND `engine_last_projected_history_id == engine_latest_history_id` (trace was fully `up_to_date` before purge)
- **THEN** the barrier is NOT cleared
- **THEN** repair returns `no_events_to_project` and the trace stays `summary_only`
- **THEN** the operator's deliberate purge decision is preserved

#### Scenario: Repair respects journal_expired barrier
- **WHEN** repair runs against a `journal_expired` trace
- **THEN** the barrier is NOT cleared
- **THEN** repair returns a `history_expired` reason without attempting to rebuild detail
- **THEN** the projection state remains `journal_expired`

#### Scenario: Repair on catching_up is accepted but does not duplicate work
- **WHEN** repair runs against a trace already in `catching_up`
- **THEN** the trace remains `catching_up`
- **THEN** the existing projector loop continues its current catch-up work
- **THEN** no duplicate rebuild path is started

#### Scenario: Repair never bypasses single-writer invariant
- **WHEN** repair eventually results in rebuilt detail
- **THEN** all writes to `public.spans`, `public.span_events`, and engine projection columns go through the projector's existing write path
- **THEN** no handwritten detail-write SQL is introduced by repair

#### Scenario: Checkpoint-forward rebuild is deterministic
- **WHEN** accepted repair causes the projector to apply events from the retained checkpoint forward
- **THEN** re-running repair on the same run produces the same rebuilt detail
- **THEN** synthetic terminal-cleanup events (from Proposal 1) are re-emitted idempotently via the existing `VariantKey` mechanism

---

### Requirement: Retained operator shell after purge

After any purge mode, the trace row MUST retain a complete operator-useful shell.

#### Scenario: Required shell fields
- **WHEN** a purge completes
- **THEN** the `public.traces` row still carries `engine_run_id`, `engine_instance_key`, `engine_definition_name`, `engine_definition_version`, `engine_run_status`, `engine_projection_state`, `engine_projection_updated_at`, and the terminal summary
- **THEN** the root span still carries terminal status, timestamps, and the terminal result/failure payload
- **THEN** the debugger can display the trace's identity, terminal outcome, and engine linkage from retained fields alone

#### Scenario: Non-engine traces are untouched
- **WHEN** purge targets an engine run
- **THEN** only the trace linked via `engine_run_id` is affected
- **THEN** other traces, sessions, and non-engine projection rows are unchanged

---

### Requirement: Projection state transitions driven by retention

Retention MUST drive engine projection state transitions in a specific order and MUST remain idempotent across restarts.

#### Scenario: Stage 1 transition
- **WHEN** retention stage 1 targets a terminal run whose current state is `up_to_date` or `catching_up` and whose completion age exceeds `ENGINE_PROJECTION_RETENTION_AFTER`
- **THEN** the trace's `engine_projection_state` transitions to `summary_only`
- **THEN** the detail rows are deleted per the `projection_only` purge mapping

#### Scenario: Stage 2 transition
- **WHEN** retention stage 2 targets a terminal run whose completion age exceeds `ENGINE_HISTORY_RETENTION_AFTER`
- **THEN** the trace's `engine_projection_state` transitions to `journal_expired`
- **THEN** engine history rows for the run are deleted per the `full` purge mapping
- **THEN** stage 2 may apply to runs in `summary_only` (most common case) or directly to runs in `up_to_date`/`catching_up` if they are already past the history window

#### Scenario: Retention is idempotent across restarts
- **WHEN** the retention worker restarts mid-scan
- **THEN** runs already transitioned to the target state are skipped
- **THEN** rescanning the same window produces the same end state with no duplicate work

#### Scenario: Retention does not transition non-terminal runs
- **WHEN** retention scans the eligible run set
- **THEN** only runs whose authoritative `engine.runs.status IN ('completed','failed','cancelled','terminated')` are transitioned
- **THEN** non-terminal runs are never purged by retention
