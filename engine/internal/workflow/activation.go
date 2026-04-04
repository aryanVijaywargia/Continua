package workflow

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
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

func (a *Activator) Activate(ctx context.Context, claimedRun enginedb.EngineRun) error {
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
		if err := a.commitDecision(ctx, tx, instance, run, workerClaimedBy, decision); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	decision, err := replayDefinition(definition, historyRows, activityTasks, inboxRows)
	if err != nil {
		return err
	}

	if err := a.commitDecision(ctx, tx, instance, run, workerClaimedBy, decision); err != nil {
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
	instance enginedb.EngineInstance,
	run enginedb.EngineRun,
	workerClaimedBy *string,
	decision activationDecision,
) error {
	sequence := decision.NextSequence
	var activityHistoryID *int64
	var timerHistoryID *int64

	for _, event := range decision.Events {
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
		sequence++
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

	for _, inboxID := range decision.ConsumedInboxIDs {
		if _, err := tx.MarkInboxProcessed(ctx, inboxID); err != nil && err != store.ErrNotFound {
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
		return err
	case decisionCompleted:
		_, err := tx.TransitionRunToCompleted(ctx, enginedb.TransitionRunToCompletedParams{
			ID:           run.ID,
			ClaimedBy:    workerClaimedBy,
			Result:       decision.Result,
			CustomStatus: decision.CustomStatus,
		})
		return err
	case decisionFailed:
		errorCode := decision.FailureCode
		errorMessage := decision.FailureMessage
		_, err := tx.TransitionRunToFailed(ctx, enginedb.TransitionRunToFailedParams{
			ID:               run.ID,
			ClaimedBy:        workerClaimedBy,
			CustomStatus:     decision.CustomStatus,
			LastErrorCode:    &errorCode,
			LastErrorMessage: &errorMessage,
		})
		return err
	default:
		return fmt.Errorf("unsupported activation decision kind %q", decision.Kind)
	}
}

func sameClaimedBy(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
