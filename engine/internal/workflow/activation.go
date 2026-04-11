package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	engineprojector "github.com/continua-ai/continua/engine/internal/projector"
	"github.com/continua-ai/continua/engine/internal/store"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
)

const (
	engineTracePrefix    = "engine:"
	engineRootSpanPrefix = "engine:root:"
)

type continuationTraceSeed struct {
	SessionID   pgtype.UUID
	Name        *string
	UserID      *string
	Tags        []string
	Environment *string
	Release     *string
	Metadata    []byte
}

type Activator struct {
	store       *store.Store
	definitions *Registry
}

func NewActivator(store *store.Store, definitions *Registry) *Activator {
	return &Activator{
		store:       store,
		definitions: definitions,
	}
}

func (a *Activator) Activate(ctx context.Context, claimedRun *enginedb.EngineRun) error {
	tx, err := a.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	run, err := tx.GetRunForUpdate(ctx, claimedRun.ID)
	if err != nil {
		return err
	}
	if run.Status != enginedb.EngineRunLifecycleStatusRunning || !sameClaimedBy(run.ClaimedBy, claimedRun.ClaimedBy) {
		return store.ErrStaleClaim
	}

	workerClaimedBy := claimedRun.ClaimedBy
	instance, err := tx.GetInstance(ctx, run.InstanceID)
	if err != nil {
		return err
	}
	historyRows, err := tx.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		return err
	}
	activityTasks, err := tx.ListActivityTasksByRun(ctx, run.ID)
	if err != nil {
		return err
	}
	inboxRows, err := tx.ListPendingInboxByRun(ctx, run.ID)
	if err != nil {
		return err
	}

	definition, ok := a.definitions.Get(instance.DefinitionName, run.DefinitionVersion)
	if !ok {
		decision := activationDecision{
			Kind:         decisionFailed,
			NextSequence: historyRows[len(historyRows)-1].SequenceNo + 1,
			Events: []queuedHistoryEvent{{
				EventType: enginehistory.EventWorkflowFailed,
				Payload: mustMarshalPayload(enginehistory.WorkflowFailedPayload{
					ErrorCode:    "definition_version_mismatch",
					ErrorMessage: fmt.Sprintf("definition %s@%s is not registered", instance.DefinitionName, run.DefinitionVersion),
				}),
			}},
			CustomStatus:   cloneRaw(run.CustomStatus),
			FailureCode:    "definition_version_mismatch",
			FailureMessage: fmt.Sprintf("definition %s@%s is not registered", instance.DefinitionName, run.DefinitionVersion),
		}
		if err := a.commitDecision(ctx, tx, &instance, &run, workerClaimedBy, &decision); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	decision, err := replayDefinition(definition, historyRows, activityTasks, inboxRows)
	if err != nil {
		return err
	}

	if err := a.commitDecision(ctx, tx, &instance, &run, workerClaimedBy, &decision); err != nil {
		return err
	}
	if decision.Kind == decisionCompleted {
		if err := applyTestFinalHook(ctx); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (a *Activator) commitDecision(
	ctx context.Context,
	tx *store.Tx,
	instance *enginedb.EngineInstance,
	run *enginedb.EngineRun,
	workerClaimedBy *string,
	decision *activationDecision,
) error {
	sequence := decision.NextSequence
	var activityHistoryID *int64
	var timerHistoryID *int64
	var latestHistoryID *int64

	for _, inboxID := range decision.ConsumedInboxIDs {
		if _, err := tx.MarkInboxProcessed(ctx, inboxID); err != nil && err != store.ErrNotFound {
			return err
		}
	}

	for i := range decision.Events {
		event := decision.Events[i]
		appended, err := tx.AppendHistory(ctx, enginedb.AppendHistoryParams{
			ProjectID:  run.ProjectID,
			InstanceID: run.InstanceID,
			RunID:      run.ID,
			SequenceNo: sequence,
			EventType:  event.EventType,
			Payload:    event.Payload,
		})
		if err != nil {
			return err
		}

		switch event.EventType {
		case enginehistory.EventActivityScheduled:
			activityHistoryID = &appended.ID
		case enginehistory.EventTimerScheduled:
			timerHistoryID = &appended.ID
		}
		latestHistoryID = &appended.ID
		sequence++
	}

	// Non-cancel activation paths still maintain the stored per-trace freshness
	// checkpoint so projected shells can move into catching_up before the
	// projector catches up. decisionCancelled remains engine-only per the
	// operational hardening change.
	if latestHistoryID != nil && decision.Kind != decisionCancelled {
		if err := engineprojector.UpdateLatestHistory(ctx, tx.Tx(), run.ProjectID, run.ID, *latestHistoryID); err != nil {
			return err
		}
	}

	if decision.NewActivity != nil {
		if activityHistoryID == nil {
			return fmt.Errorf("activity schedule event missing history id")
		}
		if _, err := tx.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
			ProjectID:         run.ProjectID,
			InstanceID:        run.InstanceID,
			RunID:             run.ID,
			HistoryID:         activityHistoryID,
			ActivityKey:       decision.NewActivity.Scheduled.ActivityKey,
			ActivityType:      decision.NewActivity.Scheduled.ActivityType,
			Input:             decision.NewActivity.Scheduled.Input,
			AvailableAt:       run.ReadyAt,
			MaxAttempts:       decision.NewActivity.Options.MaxAttempts,
			InitialBackoffMs:  decision.NewActivity.Options.InitialBackoffMS,
			MaxBackoffMs:      decision.NewActivity.Options.MaxBackoffMS,
			BackoffMultiplier: decision.NewActivity.Options.BackoffMultiplier,
		}); err != nil {
			return err
		}
	}

	if decision.NewTimer != nil {
		if timerHistoryID == nil {
			return fmt.Errorf("timer schedule event missing history id")
		}
		payload, err := enginehistory.MarshalPayload(*decision.NewTimer)
		if err != nil {
			return err
		}
		if _, err := tx.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
			ProjectID:   run.ProjectID,
			InstanceID:  run.InstanceID,
			RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
			HistoryID:   timerHistoryID,
			Kind:        "timer",
			Payload:     payload,
			AvailableAt: decision.NewTimer.DueAt,
		}); err != nil {
			return err
		}
	}

	switch decision.Kind {
	case decisionWaiting:
		_, err := tx.TransitionRunToWaiting(ctx, enginedb.TransitionRunToWaitingParams{
			ID:           run.ID,
			ClaimedBy:    workerClaimedBy,
			WaitingFor:   decision.WaitingFor,
			CustomStatus: decision.CustomStatus,
		})
		if err != nil {
			return err
		}
		return nil
	case decisionCompleted:
		if _, err := tx.TransitionRunToCompleted(ctx, enginedb.TransitionRunToCompletedParams{
			ID:           run.ID,
			ClaimedBy:    workerClaimedBy,
			Result:       decision.Result,
			CustomStatus: decision.CustomStatus,
		}); err != nil {
			return err
		}
		return updateRunInstanceStatus(ctx, tx, run.InstanceID, enginedb.EngineInstanceLifecycleStatusCompleted)
	case decisionFailed:
		if _, err := tx.TransitionRunToFailed(ctx, enginedb.TransitionRunToFailedParams{
			ID:               run.ID,
			ClaimedBy:        workerClaimedBy,
			CustomStatus:     decision.CustomStatus,
			LastErrorCode:    &decision.FailureCode,
			LastErrorMessage: &decision.FailureMessage,
		}); err != nil {
			return err
		}
		return updateRunInstanceStatus(ctx, tx, run.InstanceID, enginedb.EngineInstanceLifecycleStatusFailed)
	case decisionCancelled:
		if _, err := tx.TransitionRunToCancelled(ctx, enginedb.TransitionRunToCancelledParams{
			ID:           run.ID,
			CustomStatus: decision.CustomStatus,
		}); err != nil {
			return err
		}
		if _, err := tx.CancelOpenActivityTasksByRun(ctx, run.ID); err != nil {
			return err
		}
		if _, err := tx.DiscardOpenInboxItemsByRun(ctx, run.ID); err != nil {
			return err
		}
		return updateRunInstanceStatus(ctx, tx, run.InstanceID, enginedb.EngineInstanceLifecycleStatusCancelled)
	case decisionContinuedAsNew:
		return a.commitContinuationDecision(ctx, tx, instance, run, workerClaimedBy, decision)
	default:
		return fmt.Errorf("unsupported activation decision kind %q", decision.Kind)
	}
}

func (a *Activator) commitContinuationDecision(
	ctx context.Context,
	tx *store.Tx,
	instance *enginedb.EngineInstance,
	run *enginedb.EngineRun,
	workerClaimedBy *string,
	decision *activationDecision,
) error {
	if instance == nil || run == nil || decision == nil {
		return errors.New("continuation commit requires instance, run, and decision")
	}

	traceSeed, err := loadContinuationTraceSeed(ctx, tx, run.ProjectID, run.ID)
	if err != nil {
		return err
	}

	if _, err := tx.CancelOpenActivityTasksByRun(ctx, run.ID); err != nil {
		return err
	}
	if _, err := tx.DiscardOpenInboxItemsByRun(ctx, run.ID); err != nil {
		return err
	}

	startedAt := timeNowUTC()
	nextRun, err := tx.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:          run.ProjectID,
		InstanceID:         run.InstanceID,
		RunNumber:          run.RunNumber + 1,
		DefinitionVersion:  run.DefinitionVersion,
		ReadyAt:            startedAt,
		ContinuedFromRunID: pgtype.UUID{Bytes: run.ID, Valid: true},
	})
	if err != nil {
		return err
	}

	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    instance.DefinitionName,
		DefinitionVersion: run.DefinitionVersion,
		InstanceKey:       instance.InstanceKey,
		Input:             cloneRaw(decision.ContinuationInput),
	})
	if err != nil {
		return err
	}
	startedEvent, err := tx.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  nextRun.ProjectID,
		InstanceID: nextRun.InstanceID,
		RunID:      nextRun.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    startedPayload,
	})
	if err != nil {
		return err
	}

	if _, err := tx.TransitionRunToContinuedAsNew(ctx, enginedb.TransitionRunToContinuedAsNewParams{
		ID:               run.ID,
		ClaimedBy:        workerClaimedBy,
		ContinuedToRunID: pgtype.UUID{Bytes: nextRun.ID, Valid: true},
		CustomStatus:     decision.CustomStatus,
	}); err != nil {
		return err
	}

	if err := createContinuationTraceShell(ctx, tx, instance, &nextRun, traceSeed, &startedEvent, decision.ContinuationInput); err != nil {
		return err
	}

	return updateRunInstanceStatus(ctx, tx, run.InstanceID, enginedb.EngineInstanceLifecycleStatusActive)
}

func updateRunInstanceStatus(
	ctx context.Context,
	tx *store.Tx,
	instanceID uuid.UUID,
	status enginedb.EngineInstanceLifecycleStatus,
) error {
	_, err := tx.UpdateInstanceStatus(ctx, enginedb.UpdateInstanceStatusParams{
		ID:     instanceID,
		Status: status,
	})
	return err
}

func sameClaimedBy(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func loadContinuationTraceSeed(
	ctx context.Context,
	tx *store.Tx,
	projectID uuid.UUID,
	runID uuid.UUID,
) (*continuationTraceSeed, error) {
	if tx == nil {
		return nil, errors.New("transaction is required")
	}

	row := tx.Tx().QueryRow(ctx, `
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
		seed        continuationTraceSeed
		name        pgtype.Text
		userID      pgtype.Text
		tags        []string
		environment pgtype.Text
		release     pgtype.Text
		metadata    []byte
	)
	if err := row.Scan(&seed.SessionID, &name, &userID, &tags, &environment, &release, &metadata); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: projected trace shell missing for run %s", store.ErrInvariant, runID)
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

func createContinuationTraceShell(
	ctx context.Context,
	tx *store.Tx,
	instance *enginedb.EngineInstance,
	run *enginedb.EngineRun,
	seed *continuationTraceSeed,
	startedEvent *enginedb.EngineHistory,
	input json.RawMessage,
) error {
	if tx == nil || instance == nil || run == nil || seed == nil || startedEvent == nil {
		return errors.New("continuation trace shell requires tx, instance, run, seed, and started event")
	}

	runStatus := string(enginedb.EngineRunLifecycleStatusQueued)
	projectionState := publicprojection.StateUpToDate.String()
	traceName := seed.Name
	traceRowID := uuid.UUID{}

	if err := tx.Tx().QueryRow(ctx, `
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
		    $16, $17, $18, $19, $19, $20
		)
		RETURNING id
	`,
		run.ProjectID,
		seed.SessionID,
		engineTraceID(run.ID),
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

	if _, err := tx.Tx().Exec(ctx, `
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
		rootSpanExternalID(run.ID),
		rootSpanName,
		startedEvent.CreatedAt,
		cloneRaw(input),
	); err != nil {
		return err
	}

	return nil
}

func engineTraceID(runID uuid.UUID) string {
	return engineTracePrefix + runID.String()
}

func rootSpanExternalID(runID uuid.UUID) string {
	return engineRootSpanPrefix + runID.String()
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

func timeNowUTC() time.Time {
	return time.Now().UTC()
}
