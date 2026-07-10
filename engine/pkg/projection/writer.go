// writer.go holds the projection Writer: the single owner of every engine-run
// write into the platform debugger tables (public.traces, public.spans, and
// public.span_events).
//
// The Writer sits at the engine→platform seam and has exactly two adapters:
//
//   - the engine runtime (engine/internal/projector, engine/internal/workflow,
//     engine/internal/activity, engine/internal/worker, and the
//     continua-engine CLI), which projects history into debugger shells; and
//   - the platform engine-control service (internal/enginecontrol in the
//     platform module), which performs purge/repair/backfill maintenance.
//
// Neither adapter embeds its own SQL against these tables; they call Writer
// methods over the pgx transaction they already hold. The Writer lives in the
// engine module (which the platform module imports) because the reverse import
// is impossible: engine/ and the platform root are separate Go modules in the
// go.work workspace, and only the platform module depends on the engine
// module.
//
// Authoritative platform dependencies:
// - db/platform/migrations/postgres/000001_initial_schema.up.sql
// - db/platform/migrations/postgres/000013_engine_trace_linkage.up.sql
// - db/platform/queries/traces.sql
// - db/platform/queries/spans.sql
package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

// Sentinel errors surfaced by Writer reads that guard writes.
var (
	// ErrTraceShellMissing reports that the projected trace shell for a run
	// does not exist where an invariant requires it.
	ErrTraceShellMissing = errors.New("projected trace shell missing")
	// ErrRunNotFound reports that the locked run+trace pair does not exist
	// for the requesting project.
	ErrRunNotFound = errors.New("engine run not found")
)

const (
	engineTracePrefix        = "engine:"
	engineRootSpanPrefix     = "engine:root:"
	engineActivitySpanPrefix = "engine:activity:"

	defaultProjectedEventLevel   = "info"
	defaultProjectedSpanLevel    = "default"
	defaultProjectedSpanTypeRoot = "chain"
	defaultProjectedSpanTypeTool = "tool"
)

// TerminalHistoryMetadataKey marks force-closed activity spans with the id of
// the terminal history row that closed them.
const TerminalHistoryMetadataKey = "__continua_terminal_history_id"

// TraceExternalID returns the external trace id projected for an engine run.
func TraceExternalID(runID uuid.UUID) string {
	return engineTracePrefix + runID.String()
}

// RootSpanExternalID returns the external span id of a run's root span.
func RootSpanExternalID(runID uuid.UUID) string {
	return engineRootSpanPrefix + runID.String()
}

// ActivitySpanExternalID returns the external span id of an activity span.
func ActivitySpanExternalID(runID uuid.UUID, activityKey string) string {
	return engineActivitySpanPrefix + runID.String() + ":" + activityKey
}

// Writer owns all engine-run writes into the platform debugger tables within
// a single pgx transaction supplied by the caller.
type Writer struct {
	tx pgx.Tx
}

// NewWriter binds a Writer to the caller's transaction.
func NewWriter(tx pgx.Tx) *Writer {
	return &Writer{tx: tx}
}

// requireProjectedRow ensures a guarded projection write matched an existing
// projected row.
func requireProjectedRow(tag pgconn.CommandTag, missing func() error) error {
	if tag.RowsAffected() == 0 {
		return missing()
	}
	return nil
}

// TraceRef identifies the projected trace shell targeted by a write.
type TraceRef struct {
	ProjectID uuid.UUID
	// TraceID is the internal public.traces.id row id.
	TraceID uuid.UUID
	RunID   uuid.UUID
	// TraceName seeds span names for terminal root-span upserts.
	TraceName string
}

// TerminalUpdate describes a terminal run transition to project.
type TerminalUpdate struct {
	RunStatus    enginedb.EngineRunLifecycleStatus
	Result       json.RawMessage
	ErrorCode    string
	ErrorMessage string
}

// ActivitySpanUpdate describes an activity span upsert.
type ActivitySpanUpdate struct {
	ActivityKey   string
	ActivityType  string
	Status        string
	StartTime     time.Time
	EndTime       *time.Time
	Input         json.RawMessage
	Output        json.RawMessage
	StatusMessage *string
}

// Event is a projected span event derived from an engine history row.
type Event struct {
	ProjectID  uuid.UUID
	TraceID    uuid.UUID
	SpanID     string
	EventType  string
	Message    *string
	EventTS    time.Time
	Sequence   int32
	Payload    json.RawMessage
	HistoryID  int64
	RunID      uuid.UUID
	VariantKey string
}

// UpdateLatestHistory advances the stored per-trace history freshness
// checkpoint after new history rows are appended for a run.
func (w *Writer) UpdateLatestHistory(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
	latestHistoryID int64,
) error {
	commandTag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_latest_history_id = GREATEST(COALESCE(engine_latest_history_id, 0), $2),
		    engine_projection_state = CASE
		        WHEN COALESCE(engine_last_projected_history_id, 0) >= $2 THEN $3
		        ELSE $4
		    END,
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
	`, runID, latestHistoryID, StateUpToDate.String(), StateCatchingUp.String())
	if err != nil {
		return err
	}
	return requireProjectedRow(commandTag, func() error {
		return fmt.Errorf("projected trace not found for run %s", runID)
	})
}

// WriteTerminalSummary applies a terminal run transition to the projected
// trace and its root span in one step, keyed by the run id.
func (w *Writer) WriteTerminalSummary(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
	runStatus enginedb.EngineRunLifecycleStatus,
	completedAt time.Time,
	result json.RawMessage,
	errorCode *string,
	errorMessage *string,
	latestHistoryID int64,
) error {
	traceStatus, spanStatus := TerminalStatuses(string(runStatus))
	outputPayload, err := TerminalOutputPayload(string(runStatus), result, errorCode, errorMessage)
	if err != nil {
		return err
	}

	commandTag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET status = $2,
		    end_time = $3::timestamptz,
		    output = $4::jsonb,
		    error_count = CASE
		        WHEN $2 IN ('failed', 'cancelled') THEN GREATEST(COALESCE(error_count, 0), 1)
		        ELSE error_count
		    END,
		    engine_latest_history_id = GREATEST(COALESCE(engine_latest_history_id, 0), $5),
		    engine_projection_state = CASE
		        WHEN COALESCE(engine_last_projected_history_id, 0) >= $5 THEN $6
		        ELSE $7
		    END,
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
	`, runID, traceStatus, completedAt, outputPayload, latestHistoryID, StateUpToDate.String(), StateCatchingUp.String())
	if err != nil {
		return err
	}
	if err := requireProjectedRow(commandTag, func() error {
		return fmt.Errorf("projected trace not found for run %s", runID)
	}); err != nil {
		return err
	}

	rootSpanID := RootSpanExternalID(runID)
	commandTag, err = w.tx.Exec(ctx, `
		UPDATE public.spans
		SET status = $3,
		    end_time = $4::timestamptz,
		    output = $5::jsonb,
		    status_message = $6::text,
		    duration_ms = CASE
		        WHEN $4::timestamptz IS NOT NULL THEN EXTRACT(EPOCH FROM ($4::timestamptz - start_time)) * 1000
		        ELSE duration_ms
		    END,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE trace_id = (
		        SELECT id
		        FROM public.traces
		        WHERE engine_run_id = $1
		    )
		  AND span_id = $2
	`, runID, rootSpanID, spanStatus, completedAt, outputPayload, nullableText(errorMessage))
	if err != nil {
		return err
	}
	return requireProjectedRow(commandTag, func() error {
		return fmt.Errorf("projected root span not found for run %s", runID)
	})
}

// SyncRunSummary refreshes the projected engine summary columns (run status,
// custom status, wait state, pending-work counters) on the trace shell.
func (w *Writer) SyncRunSummary(ctx context.Context, run *enginedb.EngineRun) error {
	if run == nil {
		return errors.New("run is required")
	}
	queries := enginedb.New(w.tx)
	pendingActivityTasks, err := queries.CountOpenActivityTasksByRun(ctx, run.ID)
	if err != nil {
		return err
	}

	// CountOpenInboxByRun excludes cancel rows so projected counts mirror the
	// public pending-work surface instead of internal control intents.
	pendingInboxItems, err := queries.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: run.ID, Valid: true})
	if err != nil {
		return err
	}

	commandTag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_run_status = $2,
		    engine_custom_status = $3::jsonb,
		    engine_wait_state = $4::jsonb,
		    engine_pending_activity_tasks = $5,
		    engine_pending_inbox_items = $6,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
	`, run.ID, string(run.Status), cloneRaw(run.CustomStatus), cloneRaw(run.WaitingFor), pendingActivityTasks, pendingInboxItems)
	if err != nil {
		return err
	}
	return requireProjectedRow(commandTag, func() error {
		return fmt.Errorf("projected trace not found for run %s", run.ID)
	})
}

// WriteTerminalProjection applies a terminal transition to a known trace row
// (by internal id) and upserts the terminal root span.
func (w *Writer) WriteTerminalProjection(
	ctx context.Context,
	ref *TraceRef,
	completedAt time.Time,
	update *TerminalUpdate,
) error {
	if ref == nil || update == nil {
		return errors.New("projection target is required")
	}

	traceStatus, spanStatus := TerminalStatuses(string(update.RunStatus))
	outputPayload, err := TerminalOutputPayload(
		string(update.RunStatus),
		update.Result,
		stringPtr(update.ErrorCode),
		stringPtr(update.ErrorMessage),
	)
	if err != nil {
		return err
	}

	commandTag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET status = $2,
		    end_time = $3::timestamptz,
		    output = $4::jsonb,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE id = $1
	`, ref.TraceID, traceStatus, completedAt, outputPayload)
	if err != nil {
		return err
	}
	if err := requireProjectedRow(commandTag, func() error {
		return fmt.Errorf("projected trace not found for run %s", ref.RunID)
	}); err != nil {
		return err
	}

	return w.upsertTerminalRootSpan(
		ctx,
		ref,
		spanStatus,
		completedAt,
		outputPayload,
		stringPtr(update.ErrorMessage),
	)
}

func (w *Writer) upsertTerminalRootSpan(
	ctx context.Context,
	ref *TraceRef,
	spanStatus string,
	completedAt time.Time,
	outputPayload json.RawMessage,
	statusMessage *string,
) error {
	if ref == nil {
		return errors.New("projection target is required")
	}

	commandTag, err := w.tx.Exec(ctx, `
		INSERT INTO public.spans (
		    project_id,
		    trace_id,
		    span_id,
		    name,
		    type,
		    status,
		    level,
		    start_time,
		    end_time,
		    output,
		    status_message,
		    duration_ms,
		    depth
		)
		SELECT $1,
		       t.id,
		       $3,
		       COALESCE(NULLIF($4, ''), NULLIF(t.name, ''), 'workflow'),
		       $5,
		       $6,
		       $7,
		       COALESCE(t.start_time, $8::timestamptz),
		       $8::timestamptz,
		       $9::jsonb,
		       $10::text,
		       EXTRACT(EPOCH FROM ($8::timestamptz - COALESCE(t.start_time, $8::timestamptz))) * 1000,
		       0
		FROM public.traces AS t
		WHERE t.id = $2
		ON CONFLICT (trace_id, span_id) DO UPDATE
		SET status = EXCLUDED.status,
		    end_time = EXCLUDED.end_time,
		    output = EXCLUDED.output,
		    status_message = EXCLUDED.status_message,
		    duration_ms = EXCLUDED.duration_ms,
		    updated_at = NOW(),
		    version = COALESCE(spans.version, 1) + 1
	`, ref.ProjectID, ref.TraceID, RootSpanExternalID(ref.RunID), ref.TraceName, defaultProjectedSpanTypeRoot, spanStatus, defaultProjectedSpanLevel, completedAt, outputPayload, statusMessage)
	if err != nil {
		return err
	}
	return requireProjectedRow(commandTag, func() error {
		return fmt.Errorf("projected trace not found for run %s", ref.RunID)
	})
}

// CloseTerminalActivitySpan force-closes a still-open activity span when its
// run reaches a terminal state that abandons the activity.
func (w *Writer) CloseTerminalActivitySpan(
	ctx context.Context,
	ref *TraceRef,
	historyID int64,
	closedAt time.Time,
	activityKey string,
	errorCode string,
	errorMessage string,
) error {
	if ref == nil {
		return errors.New("projection target is required")
	}

	_, err := w.tx.Exec(ctx, `
		UPDATE public.spans
		SET status = 'failed',
		    status_message = $3,
		    end_time = $4::timestamptz,
		    output = $5::jsonb,
		    metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object($6::text, to_jsonb($7::bigint)),
		    duration_ms = CASE
		        WHEN start_time IS NOT NULL THEN EXTRACT(EPOCH FROM ($4::timestamptz - start_time)) * 1000
		        ELSE duration_ms
		    END,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE trace_id = $1
		  AND span_id = $2
		  AND end_time IS NULL
	`, ref.TraceID, ActivitySpanExternalID(ref.RunID, activityKey), errorMessage, closedAt, mustMarshalMap(map[string]any{
		"error_code":    errorCode,
		"error_message": errorMessage,
	}), TerminalHistoryMetadataKey, historyID)
	return err
}

// UpsertActivitySpan creates or patches the projected span for an activity.
func (w *Writer) UpsertActivitySpan(
	ctx context.Context,
	ref *TraceRef,
	update *ActivitySpanUpdate,
) error {
	if ref == nil || update == nil {
		return errors.New("projection target and activity update are required")
	}
	spanID := ActivitySpanExternalID(ref.RunID, update.ActivityKey)
	commandTag, err := w.tx.Exec(ctx, `
		INSERT INTO public.spans (
		    project_id,
		    trace_id,
		    span_id,
		    parent_span_id,
		    name,
		    type,
		    status,
		    status_message,
		    level,
		    start_time,
		    end_time,
		    input,
		    output,
		    depth
		)
		VALUES (
		    $1,
		    $2,
		    $3,
		    $4,
		    $5,
		    $6,
		    $7,
		    $8,
		    $9,
		    $10,
		    $11,
		    $12,
		    $13,
		    1
		)
		ON CONFLICT (trace_id, span_id) DO UPDATE SET
		    name = COALESCE(EXCLUDED.name, public.spans.name),
		    type = COALESCE(EXCLUDED.type, public.spans.type),
		    status = COALESCE(EXCLUDED.status, public.spans.status),
		    status_message = COALESCE(EXCLUDED.status_message, public.spans.status_message),
		    start_time = COALESCE(public.spans.start_time, EXCLUDED.start_time),
		    end_time = COALESCE(EXCLUDED.end_time, public.spans.end_time),
		    input = COALESCE(public.spans.input, EXCLUDED.input),
		    output = COALESCE(EXCLUDED.output, public.spans.output),
		    duration_ms = CASE
		        WHEN COALESCE(EXCLUDED.end_time, public.spans.end_time) IS NOT NULL
		         AND COALESCE(public.spans.start_time, EXCLUDED.start_time) IS NOT NULL
		        THEN EXTRACT(EPOCH FROM (
		            COALESCE(EXCLUDED.end_time, public.spans.end_time) - COALESCE(public.spans.start_time, EXCLUDED.start_time)
		        )) * 1000
		        ELSE public.spans.duration_ms
		    END,
		    updated_at = NOW(),
		    version = COALESCE(public.spans.version, 1) + 1
	`, ref.ProjectID, ref.TraceID, spanID, stringPtr(RootSpanExternalID(ref.RunID)), update.ActivityType, defaultProjectedSpanTypeTool, update.Status, update.StatusMessage, defaultProjectedSpanLevel, update.StartTime, update.EndTime, update.Input, update.Output)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("failed to upsert activity span %s for run %s", update.ActivityKey, ref.RunID)
	}
	return nil
}

// EmitEvent inserts a projected span event, deduplicated by an idempotency
// key derived from the run, history row, and variant.
func (w *Writer) EmitEvent(ctx context.Context, event *Event) error {
	if event == nil {
		return errors.New("projected event is required")
	}
	idempotencyKey := fmt.Sprintf("%s:%d:%s", event.RunID.String(), event.HistoryID, event.VariantKey)
	_, err := w.tx.Exec(ctx, `
		INSERT INTO public.span_events (
		    project_id,
		    trace_id,
		    span_id,
		    event_type,
		    level,
		    event_ts,
		    server_ingested_at,
		    sequence,
		    message,
		    payload,
		    idempotency_key
		)
		VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8, $9, $10)
		ON CONFLICT (project_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, event.ProjectID, event.TraceID, event.SpanID, event.EventType, defaultProjectedEventLevel, event.EventTS, event.Sequence, event.Message, event.Payload, idempotencyKey)
	return err
}

// AdvanceCheckpoint moves the last-projected history checkpoint forward and
// recomputes the projection state against the live history journal.
func (w *Writer) AdvanceCheckpoint(
	ctx context.Context,
	traceID uuid.UUID,
	runID uuid.UUID,
	lastProjectedHistoryID int64,
) error {
	commandTag, err := w.tx.Exec(ctx, `
		WITH latest AS (
		    SELECT COALESCE(MAX(id), 0) AS latest_history_id
		    FROM engine.history
		    WHERE run_id = $2
		)
		UPDATE public.traces
		SET engine_latest_history_id = GREATEST(COALESCE(public.traces.engine_latest_history_id, 0), latest.latest_history_id),
		    engine_last_projected_history_id = CASE
		        WHEN COALESCE(public.traces.engine_last_projected_history_id, 0) < $3 THEN $3
		        ELSE public.traces.engine_last_projected_history_id
		    END,
		    engine_projection_state = CASE
		        WHEN latest.latest_history_id <= GREATEST(COALESCE(public.traces.engine_last_projected_history_id, 0), $3) THEN $4
		        ELSE $5
		    END,
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW()
		FROM latest
		WHERE public.traces.id = $1
		  AND public.traces.engine_run_id = $2
	`, traceID, runID, lastProjectedHistoryID, StateUpToDate.String(), StateCatchingUp.String())
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("projection checkpoint target missing for run %s", runID)
	}
	return nil
}

// WithProjectionBarrier locks the projected trace row, skips the write when
// the trace has been reduced to summary_only or journal_expired, and reports
// whether the write was applied.
func (w *Writer) WithProjectionBarrier(
	ctx context.Context,
	ref *TraceRef,
	write func() error,
) (bool, error) {
	if ref == nil {
		return false, errors.New("projection target is required")
	}
	if write == nil {
		return false, errors.New("projection write is required")
	}

	var projectionState string
	err := w.tx.QueryRow(ctx, `
		SELECT COALESCE(engine_projection_state, $3)
		FROM public.traces
		WHERE id = $1
		  AND engine_run_id = $2
		FOR UPDATE
	`, ref.TraceID, ref.RunID, StateUpToDate.String()).Scan(&projectionState)
	if err != nil {
		return false, err
	}
	if projectionState == StateSummaryOnly.String() ||
		projectionState == StateJournalExpired.String() {
		return false, nil
	}

	if err := write(); err != nil {
		return false, err
	}
	return true, nil
}

// RefreshTraceCounters recomputes span totals and error counts on the trace.
func (w *Writer) RefreshTraceCounters(ctx context.Context, traceID uuid.UUID) error {
	_, err := w.tx.Exec(ctx, `
		WITH counts AS (
		    SELECT COUNT(*)::integer AS total_spans,
		           COUNT(*) FILTER (WHERE status IN ('failed', 'error'))::integer AS error_count
		    FROM public.spans
		    WHERE trace_id = $1
		)
		UPDATE public.traces
		SET total_spans = counts.total_spans,
		    error_count = counts.error_count,
		    updated_at = NOW()
		FROM counts
		WHERE public.traces.id = $1
	`, traceID)
	return err
}

func nullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func mustMarshalMap(value map[string]any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}
