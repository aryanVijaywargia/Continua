// writer_shell.go holds the Writer methods that create projected trace shells:
// the initial trace+root-span pair for new runs, continuations, and child
// workflows, plus the dark-launch CLI bootstrap shell.
package projection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

// TraceShellSeed carries the trace attributes inherited by continuation and
// child-workflow shells from the parent run's projected trace.
type TraceShellSeed struct {
	SessionID   pgtype.UUID
	Name        *string
	UserID      *string
	Tags        []string
	Environment *string
	Release     *string
	Metadata    []byte
}

// LoadContinuationSeed reads (and row-locks) the projected trace of an
// existing run so its attributes can seed a continuation or child shell.
// Returns ErrTraceShellMissing when the projected shell does not exist.
func (w *Writer) LoadContinuationSeed(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
) (*TraceShellSeed, error) {
	row := w.tx.QueryRow(ctx, `
		SELECT session_id,
		       name,
		       user_id,
		       tags,
		       environment,
		       release,
		       metadata
		FROM public.traces
		WHERE project_id = $1
		  AND engine_run_id = $2
		FOR UPDATE
	`, projectID, runID)

	var (
		seed        TraceShellSeed
		name        pgtype.Text
		userID      pgtype.Text
		tags        []string
		environment pgtype.Text
		release     pgtype.Text
		metadata    []byte
	)
	if err := row.Scan(&seed.SessionID, &name, &userID, &tags, &environment, &release, &metadata); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTraceShellMissing
		}
		return nil, err
	}

	seed.Name = pgTextPtr(name)
	seed.UserID = pgTextPtr(userID)
	seed.Tags = append([]string(nil), tags...)
	seed.Environment = pgTextPtr(environment)
	seed.Release = pgTextPtr(release)
	seed.Metadata = cloneBytes(metadata)
	return &seed, nil
}

// CreateTraceShell inserts the projected trace and root span for a freshly
// started run (continuation or child workflow).
func (w *Writer) CreateTraceShell(
	ctx context.Context,
	instance *enginedb.EngineInstance,
	run *enginedb.EngineRun,
	seed *TraceShellSeed,
	startedEvent *enginedb.EngineHistory,
	input json.RawMessage,
	traceName *string,
) error {
	if instance == nil || run == nil || seed == nil || startedEvent == nil {
		return errors.New("projected trace shell requires instance, run, seed, and started event")
	}

	runStatus := string(enginedb.EngineRunLifecycleStatusQueued)
	projectionState := StateUpToDate.String()
	traceRowID := uuid.UUID{}

	if err := w.tx.QueryRow(ctx, `
		INSERT INTO public.traces (
		    project_id,
		    session_id,
		    trace_id,
		    name,
		    user_id,
		    tags,
		    environment,
		    release,
		    metadata,
		    input,
		    output,
		    status,
		    start_time,
		    end_time,
		    engine_run_id,
		    engine_instance_key,
		    engine_run_status,
		    engine_custom_status,
		    engine_wait_state,
		    engine_pending_activity_tasks,
		    engine_pending_inbox_items,
		    engine_definition_name,
		    engine_definition_version,
		    engine_parent_run_id,
		    engine_root_run_id,
		    engine_child_key,
		    engine_child_depth,
		    engine_projection_state,
		    engine_latest_history_id,
		    engine_last_projected_history_id,
		    engine_projection_updated_at
		)
		VALUES (
		    $1, $2, $3, $4, $5, $6,
		    $7, $8, $9, $10, $11,
			    'running', $12, NULL,
			    $13, $14, $15,
			    NULL, NULL, 0, 0,
			    $16, $17, $18, $19, $20, $21,
			    $22, $23, $23, $24
			)
		RETURNING id
	`,
		run.ProjectID,
		seed.SessionID,
		TraceExternalID(run.ID),
		traceName,
		seed.UserID,
		seed.Tags,
		seed.Environment,
		seed.Release,
		cloneBytes(seed.Metadata),
		cloneRaw(input),
		nil,
		startedEvent.CreatedAt,
		run.ID,
		instance.InstanceKey,
		runStatus,
		instance.DefinitionName,
		run.DefinitionVersion,
		run.ParentRunID,
		run.RootRunID,
		run.ChildKey,
		run.ChildDepth,
		projectionState,
		startedEvent.ID,
		startedEvent.CreatedAt,
	).Scan(&traceRowID); err != nil {
		return err
	}

	rootSpanName := strings.TrimSpace(instance.DefinitionName)
	if traceName != nil && strings.TrimSpace(*traceName) != "" {
		rootSpanName = strings.TrimSpace(*traceName)
	}
	if rootSpanName == "" {
		rootSpanName = "workflow"
	}

	if _, err := w.tx.Exec(ctx, `
		INSERT INTO public.spans (
		    project_id,
		    trace_id,
		    span_id,
		    name,
		    type,
		    status,
		    level,
		    start_time,
		    input,
		    depth
		)
		VALUES ($1, $2, $3, $4, 'chain', 'running', 'default', $5, $6, 0)
	`,
		run.ProjectID,
		traceRowID,
		RootSpanExternalID(run.ID),
		rootSpanName,
		startedEvent.CreatedAt,
		cloneRaw(input),
	); err != nil {
		return err
	}

	return nil
}

// EnsureDarkLaunchShell bootstraps the fixed dark-launch demo project and a
// projected shell for a CLI-started run. Dark-launch shells are best-effort;
// see requireProjectedRow for the matching tolerance rule on later writes.
func (w *Writer) EnsureDarkLaunchShell(
	ctx context.Context,
	instance *enginedb.EngineInstance,
	run *enginedb.EngineRun,
	definitionName string,
	definitionVersion string,
	input json.RawMessage,
	startedHistoryID int64,
) error {
	if instance == nil || run == nil || startedHistoryID == 0 {
		return errors.New("workflow.started history row is required before creating projected shell")
	}

	now := time.Now()
	if _, err := w.tx.Exec(ctx, `
		INSERT INTO public.projects (id, name, api_key_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING
	`, DarkLaunchProjectID, "Engine Dark Launch", "engine-dark-launch"); err != nil {
		return err
	}

	traceID := TraceExternalID(run.ID)
	traceUUID := uuid.New()
	if _, err := w.tx.Exec(ctx, `
		INSERT INTO public.traces (
		    id,
		    project_id,
		    trace_id,
		    name,
		    input,
		    status,
		    start_time,
		    engine_run_id,
		    engine_instance_key,
		    engine_run_status,
		    engine_pending_activity_tasks,
		    engine_pending_inbox_items,
		    engine_definition_name,
		    engine_definition_version,
		    engine_parent_run_id,
		    engine_root_run_id,
		    engine_child_key,
		    engine_child_depth,
		    engine_projection_state,
		    engine_latest_history_id,
		    engine_last_projected_history_id,
		    engine_projection_updated_at
		)
		VALUES (
		    $1,
		    $2,
		    $3,
		    $4,
		    $5::jsonb,
		    'running',
		    $6::timestamptz,
		    $7,
		    $8,
		    'queued',
		    0,
		    0,
		    $9,
		    $10,
		    NULL,
		    $11,
		    NULL,
		    0,
		    'up_to_date',
		    $12,
		    $12,
		    $6::timestamptz
		)
	`, traceUUID, DarkLaunchProjectID, traceID, definitionName, cloneRaw(input), now, run.ID, instance.InstanceKey, definitionName, definitionVersion, run.ID, startedHistoryID); err != nil {
		return err
	}

	_, spanErr := w.tx.Exec(ctx, `
		INSERT INTO public.spans (
		    project_id,
		    trace_id,
		    span_id,
		    name,
		    type,
		    status,
		    level,
		    start_time,
		    input,
		    depth
		)
		VALUES ($1, $2, $3, $4, 'chain', 'running', 'default', $5::timestamptz, $6::jsonb, 0)
	`, DarkLaunchProjectID, traceUUID, RootSpanExternalID(run.ID), definitionName, now, cloneRaw(input))
	return spanErr
}

func pgTextPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func cloneBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return append([]byte(nil), raw...)
}
