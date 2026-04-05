package workflow

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	engineprojector "github.com/continua-ai/continua/engine/internal/projector"
	"github.com/continua-ai/continua/engine/internal/store"
)

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
		if err := a.commitDecision(ctx, tx, &run, workerClaimedBy, &decision); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	decision, err := replayDefinition(definition, historyRows, activityTasks, inboxRows)
	if err != nil {
		return err
	}

	if err := a.commitDecision(ctx, tx, &run, workerClaimedBy, &decision); err != nil {
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
			ProjectID:    run.ProjectID,
			InstanceID:   run.InstanceID,
			RunID:        run.ID,
			HistoryID:    activityHistoryID,
			ActivityKey:  decision.NewActivity.ActivityKey,
			ActivityType: decision.NewActivity.ActivityType,
			Input:        decision.NewActivity.Input,
			AvailableAt:  run.ReadyAt,
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
	default:
		return fmt.Errorf("unsupported activation decision kind %q", decision.Kind)
	}
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
