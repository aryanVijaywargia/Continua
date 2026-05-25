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

type childWorkflowBindingState struct {
	relationship  *enginedb.EngineChildWorkflow
	childInstance *enginedb.EngineInstance
}

func NewActivator(st *store.Store, definitions *Registry) *Activator {
	return &Activator{
		store:       st,
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
	childWorkflows, err := tx.ListChildWorkflowOutcomesByParentRun(ctx, run.ProjectID, run.ID)
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
			NextSequence: nextHistorySequence(historyRows),
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

	decision, err := replayDefinitionForRunWithValidator(
		definition,
		&run,
		historyRows,
		activityTasks,
		childWorkflows,
		inboxRows,
		func(scheduled enginehistory.ChildWorkflowScheduledPayload) error {
			return a.validateChildWorkflowScheduling(ctx, tx, &run, &scheduled)
		},
	)
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

func nextHistorySequence(historyRows []enginedb.EngineHistory) int32 {
	if len(historyRows) == 0 {
		return 1
	}
	return historyRows[len(historyRows)-1].SequenceNo + 1
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
	var childScheduleHistoryID *int64
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
		case enginehistory.EventChildWorkflowScheduled:
			childScheduleHistoryID = &appended.ID
		}
		latestHistoryID = &appended.ID
		sequence++
	}

	if len(decision.ChildWaitFailures) > 0 {
		if err := a.commitChildWaitFailures(ctx, tx, run, decision.ChildWaitFailures); err != nil {
			return err
		}
	}

	if decision.NewChildWorkflow != nil {
		if childScheduleHistoryID == nil {
			return fmt.Errorf("child workflow schedule event missing history id")
		}
		parentStartedEvent, err := a.commitNewChildWorkflow(ctx, tx, instance, run, decision.NewChildWorkflow, sequence)
		if err != nil {
			return err
		}
		latestHistoryID = &parentStartedEvent.ID
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
			ExecutionTarget:   decision.NewActivity.Options.ExecutionTarget,
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
		updatedRun, err := tx.TransitionRunToCompleted(ctx, enginedb.TransitionRunToCompletedParams{
			ID:           run.ID,
			ClaimedBy:    workerClaimedBy,
			Result:       decision.Result,
			CustomStatus: decision.CustomStatus,
		})
		if err != nil {
			return err
		}
		if err := a.commitChildTerminalTransition(ctx, tx, &updatedRun, enginedb.EngineChildWorkflowStatusCompleted); err != nil {
			return err
		}
		return updateRunInstanceStatus(ctx, tx, run.InstanceID, enginedb.EngineInstanceLifecycleStatusCompleted)
	case decisionFailed:
		updatedRun, err := tx.TransitionRunToFailed(ctx, enginedb.TransitionRunToFailedParams{
			ID:               run.ID,
			ClaimedBy:        workerClaimedBy,
			CustomStatus:     decision.CustomStatus,
			LastErrorCode:    &decision.FailureCode,
			LastErrorMessage: &decision.FailureMessage,
		})
		if err != nil {
			return err
		}
		if err := a.commitChildTerminalTransition(ctx, tx, &updatedRun, enginedb.EngineChildWorkflowStatusFailed); err != nil {
			return err
		}
		return updateRunInstanceStatus(ctx, tx, run.InstanceID, enginedb.EngineInstanceLifecycleStatusFailed)
	case decisionCancelled:
		updatedRun, err := tx.TransitionRunToCancelled(ctx, enginedb.TransitionRunToCancelledParams{
			ID:           run.ID,
			CustomStatus: decision.CustomStatus,
		})
		if err != nil {
			return err
		}
		if _, err := tx.CancelOpenActivityTasksByRun(ctx, run.ID); err != nil {
			return err
		}
		if _, err := tx.DiscardOpenInboxItemsByRun(ctx, run.ID); err != nil {
			return err
		}
		if err := a.enqueueChildCancelCascade(ctx, tx, &updatedRun); err != nil {
			return err
		}
		if err := a.commitChildTerminalTransition(ctx, tx, &updatedRun, enginedb.EngineChildWorkflowStatusCancelled); err != nil {
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
	var nextRun enginedb.EngineRun
	if run.ParentRunID.Valid {
		nextRun, err = tx.CreateChildRun(ctx, enginedb.CreateChildRunParams{
			ProjectID:          run.ProjectID,
			InstanceID:         run.InstanceID,
			RunNumber:          run.RunNumber + 1,
			DefinitionVersion:  run.DefinitionVersion,
			ReadyAt:            startedAt,
			ContinuedFromRunID: pgtype.UUID{Bytes: run.ID, Valid: true},
			ParentRunID:        run.ParentRunID,
			RootRunID:          run.RootRunID,
			ChildKey:           cloneStringPtr(run.ChildKey),
			ChildDepth:         run.ChildDepth,
		})
	} else {
		nextRun, err = tx.CreateRun(ctx, enginedb.CreateRunParams{
			ProjectID:          run.ProjectID,
			InstanceID:         run.InstanceID,
			RunNumber:          run.RunNumber + 1,
			DefinitionVersion:  run.DefinitionVersion,
			ReadyAt:            startedAt,
			ContinuedFromRunID: pgtype.UUID{Bytes: run.ID, Valid: true},
		})
	}
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

	if run.ParentRunID.Valid {
		childWorkflow, err := tx.UpdateChildWorkflowContinuation(ctx, enginedb.UpdateChildWorkflowContinuationParams{
			ProjectID:          run.ProjectID,
			PreviousChildRunID: run.ID,
			NextChildRunID:     nextRun.ID,
		})
		if err != nil {
			return err
		}
		if childWorkflow.ContinuationCount >= maxContinuationFollowDepth && !childWorkflow.ParentWaitFailedAt.Valid {
			if _, err := tx.WakeWaitingChildWorkflowRun(ctx, enginedb.WakeWaitingChildWorkflowRunParams{
				ID:       childWorkflow.ParentRunID,
				ChildKey: childWorkflow.ChildKey,
			}); err != nil {
				return err
			}
		}
	}

	if err := createContinuationTraceShell(ctx, tx, instance, &nextRun, traceSeed, &startedEvent, decision.ContinuationInput); err != nil {
		return err
	}

	return updateRunInstanceStatus(ctx, tx, run.InstanceID, enginedb.EngineInstanceLifecycleStatusActive)
}

func (a *Activator) commitChildWaitFailures(
	ctx context.Context,
	tx *store.Tx,
	run *enginedb.EngineRun,
	failures []childWaitFailure,
) error {
	if run == nil {
		return errors.New("run is required")
	}
	for i := range failures {
		failure := failures[i]
		errorCode := failure.ErrorCode
		errorMessage := failure.ErrorMessage
		if _, err := tx.MarkChildWorkflowParentWaitFailed(ctx, enginedb.MarkChildWorkflowParentWaitFailedParams{
			ProjectID:              run.ProjectID,
			ParentRunID:            run.ID,
			ChildKey:               failure.ChildKey,
			ParentWaitErrorCode:    &errorCode,
			ParentWaitErrorMessage: &errorMessage,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (a *Activator) commitNewChildWorkflow(
	ctx context.Context,
	tx *store.Tx,
	parentInstance *enginedb.EngineInstance,
	parentRun *enginedb.EngineRun,
	child *newChildWorkflow,
	sequence int32,
) (enginedb.EngineHistory, error) {
	if parentInstance == nil || parentRun == nil || child == nil {
		return enginedb.EngineHistory{}, errors.New("parent instance, parent run, and child workflow are required")
	}

	childInstance, childRun, childStartedEvent, createdChild, err := a.createOrAttachChildExecution(ctx, tx, parentInstance, parentRun, child)
	if err != nil {
		return enginedb.EngineHistory{}, err
	}

	parentStartedPayload, err := enginehistory.MarshalPayload(enginehistory.ChildWorkflowStartedPayload{
		ChildKey:         child.Scheduled.ChildKey,
		ChildInstanceID:  childInstance.ID.String(),
		ChildInstanceKey: childInstance.InstanceKey,
		ChildRunID:       childRun.ID.String(),
		RootRunID:        childRun.RootRunID.String(),
		ChildDepth:       childRun.ChildDepth,
	})
	if err != nil {
		return enginedb.EngineHistory{}, err
	}
	parentStartedEvent, err := tx.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  parentRun.ProjectID,
		InstanceID: parentRun.InstanceID,
		RunID:      parentRun.ID,
		SequenceNo: sequence,
		EventType:  enginehistory.EventChildWorkflowStarted,
		Payload:    parentStartedPayload,
	})
	if err != nil {
		return enginedb.EngineHistory{}, err
	}

	if createdChild && childStartedEvent != nil {
		traceSeed, err := loadContinuationTraceSeed(ctx, tx, parentRun.ProjectID, parentRun.ID)
		if err != nil {
			return enginedb.EngineHistory{}, err
		}
		traceName := &childInstance.DefinitionName
		if err := createProjectedTraceShell(ctx, tx, &childInstance, &childRun, traceSeed, childStartedEvent, child.Scheduled.Input, traceName); err != nil {
			return enginedb.EngineHistory{}, err
		}
	}

	return parentStartedEvent, nil
}

func (a *Activator) createOrAttachChildExecution(
	ctx context.Context,
	tx *store.Tx,
	parentInstance *enginedb.EngineInstance,
	parentRun *enginedb.EngineRun,
	child *newChildWorkflow,
) (enginedb.EngineInstance, enginedb.EngineRun, *enginedb.EngineHistory, bool, error) {
	if parentInstance == nil || parentRun == nil || child == nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, false, errors.New("parent instance, parent run, and child workflow are required")
	}

	bindingState, err := a.loadChildWorkflowBindingState(ctx, tx, parentRun, &child.Scheduled)
	if err != nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, false, err
	}

	if bindingState.relationship != nil {
		childRun, err := tx.GetRun(ctx, bindingState.relationship.CurrentChildRunID)
		if err != nil {
			return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, false, err
		}
		return *bindingState.childInstance, childRun, nil, false, nil
	}

	var (
		childInstance enginedb.EngineInstance
	)
	if bindingState.childInstance != nil {
		childInstance = *bindingState.childInstance
	} else {
		childInstance, _, err = a.getOrCreateChildInstance(ctx, tx, parentRun, child)
		if err != nil {
			return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, false, err
		}
	}

	startedAt := timeNowUTC()
	childKey := child.Scheduled.ChildKey
	childRun, err := tx.CreateChildRun(ctx, enginedb.CreateChildRunParams{
		ProjectID:          parentRun.ProjectID,
		InstanceID:         childInstance.ID,
		RunNumber:          1,
		DefinitionVersion:  child.Scheduled.DefinitionVersion,
		ReadyAt:            startedAt,
		ContinuedFromRunID: pgtype.UUID{},
		ParentRunID:        pgtype.UUID{Bytes: parentRun.ID, Valid: true},
		RootRunID:          parentRun.RootRunID,
		ChildKey:           &childKey,
		ChildDepth:         parentRun.ChildDepth + 1,
	})
	if err != nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, false, err
	}

	childStartedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    child.Scheduled.DefinitionName,
		DefinitionVersion: child.Scheduled.DefinitionVersion,
		InstanceKey:       child.Scheduled.ChildInstanceKey,
		Input:             cloneRaw(child.Scheduled.Input),
	})
	if err != nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, false, err
	}
	childStartedEvent, err := tx.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  childRun.ProjectID,
		InstanceID: childRun.InstanceID,
		RunID:      childRun.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    childStartedPayload,
	})
	if err != nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, false, err
	}

	if _, err := tx.CreateChildWorkflow(ctx, enginedb.CreateChildWorkflowParams{
		ProjectID:                  parentRun.ProjectID,
		ParentInstanceID:           parentInstance.ID,
		ParentRunID:                parentRun.ID,
		ChildKey:                   child.Scheduled.ChildKey,
		RequestedDefinitionName:    child.Scheduled.DefinitionName,
		RequestedDefinitionVersion: child.Scheduled.DefinitionVersion,
		ChildInstanceID:            childInstance.ID,
		ChildInstanceKey:           child.Scheduled.ChildInstanceKey,
		CurrentChildRunID:          childRun.ID,
		RootRunID:                  childRun.RootRunID,
		ChildDepth:                 childRun.ChildDepth,
	}); err != nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, false, err
	}

	return childInstance, childRun, &childStartedEvent, true, nil
}

func (a *Activator) validateChildWorkflowScheduling(
	ctx context.Context,
	tx *store.Tx,
	parentRun *enginedb.EngineRun,
	scheduled *enginehistory.ChildWorkflowScheduledPayload,
) error {
	_, err := a.loadChildWorkflowBindingState(ctx, tx, parentRun, scheduled)
	return err
}

func (a *Activator) loadChildWorkflowBindingState(
	ctx context.Context,
	tx *store.Tx,
	parentRun *enginedb.EngineRun,
	scheduled *enginehistory.ChildWorkflowScheduledPayload,
) (childWorkflowBindingState, error) {
	if parentRun == nil || scheduled == nil {
		return childWorkflowBindingState{}, errors.New("parent run and scheduled child workflow are required")
	}

	if existing, err := tx.GetChildWorkflowByParentRunAndKeyForUpdate(ctx, enginedb.GetChildWorkflowByParentRunAndKeyForUpdateParams{
		ProjectID:   parentRun.ProjectID,
		ParentRunID: parentRun.ID,
		ChildKey:    scheduled.ChildKey,
	}); err == nil {
		if existing.RequestedDefinitionName != scheduled.DefinitionName ||
			existing.RequestedDefinitionVersion != scheduled.DefinitionVersion ||
			existing.ChildInstanceKey != scheduled.ChildInstanceKey {
			return childWorkflowBindingState{}, codedWorkflowError{
				code:    "instance_conflict",
				message: "child workflow binding does not match the existing child relationship",
			}
		}
		childInstance, err := tx.GetInstance(ctx, existing.ChildInstanceID)
		if err != nil {
			return childWorkflowBindingState{}, err
		}
		return childWorkflowBindingState{
			relationship:  &existing,
			childInstance: &childInstance,
		}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return childWorkflowBindingState{}, err
	}

	childInstance, err := tx.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   parentRun.ProjectID,
		InstanceKey: scheduled.ChildInstanceKey,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return childWorkflowBindingState{}, nil
		}
		return childWorkflowBindingState{}, err
	}
	if childInstance.DefinitionName != scheduled.DefinitionName {
		return childWorkflowBindingState{}, codedWorkflowError{
			code:    "instance_conflict",
			message: "child instance key is already bound to a different definition",
		}
	}

	relationship, err := tx.GetChildWorkflowByChildInstanceForUpdate(ctx, enginedb.GetChildWorkflowByChildInstanceForUpdateParams{
		ProjectID:       parentRun.ProjectID,
		ChildInstanceID: childInstance.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return childWorkflowBindingState{}, codedWorkflowError{
				code:    "instance_conflict",
				message: "child instance key is already attached to a different workflow relationship",
			}
		}
		return childWorkflowBindingState{}, err
	}
	if relationship.ParentRunID != parentRun.ID ||
		relationship.ChildKey != scheduled.ChildKey ||
		relationship.RequestedDefinitionName != scheduled.DefinitionName ||
		relationship.RequestedDefinitionVersion != scheduled.DefinitionVersion ||
		relationship.ChildInstanceKey != scheduled.ChildInstanceKey {
		return childWorkflowBindingState{}, codedWorkflowError{
			code:    "instance_conflict",
			message: "child instance key is already attached to a different workflow relationship",
		}
	}
	return childWorkflowBindingState{
		relationship:  &relationship,
		childInstance: &childInstance,
	}, nil
}

func (a *Activator) getOrCreateChildInstance(
	ctx context.Context,
	tx *store.Tx,
	parentRun *enginedb.EngineRun,
	child *newChildWorkflow,
) (enginedb.EngineInstance, bool, error) {
	if _, err := tx.Tx().Exec(ctx, "SAVEPOINT child_create_instance"); err != nil {
		return enginedb.EngineInstance{}, false, err
	}
	instance, err := tx.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      parentRun.ProjectID,
		InstanceKey:    child.Scheduled.ChildInstanceKey,
		DefinitionName: child.Scheduled.DefinitionName,
	})
	if err == nil {
		if _, releaseErr := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT child_create_instance"); releaseErr != nil {
			return enginedb.EngineInstance{}, false, releaseErr
		}
		return instance, true, nil
	}

	if _, rollbackErr := tx.Tx().Exec(ctx, "ROLLBACK TO SAVEPOINT child_create_instance"); rollbackErr != nil {
		return enginedb.EngineInstance{}, false, rollbackErr
	}
	if _, releaseErr := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT child_create_instance"); releaseErr != nil {
		return enginedb.EngineInstance{}, false, releaseErr
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		return enginedb.EngineInstance{}, false, err
	}

	instance, err = tx.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   parentRun.ProjectID,
		InstanceKey: child.Scheduled.ChildInstanceKey,
	})
	if err != nil {
		return enginedb.EngineInstance{}, false, err
	}
	if instance.DefinitionName != child.Scheduled.DefinitionName {
		return enginedb.EngineInstance{}, false, codedWorkflowError{
			code:    "instance_conflict",
			message: "child instance key is already bound to a different definition",
		}
	}
	return instance, false, nil
}

func (a *Activator) enqueueChildCancelCascade(ctx context.Context, tx *store.Tx, run *enginedb.EngineRun) error {
	if run == nil {
		return errors.New("run is required")
	}
	children, err := tx.ListActiveChildWorkflowsByParentRun(ctx, run.ProjectID, run.ID)
	if err != nil {
		return err
	}
	payload, err := enginehistory.MarshalPayload(enginehistory.CancelRequestedPayload{})
	if err != nil {
		return err
	}
	for i := range children {
		child := children[i]
		dedupeKey := "cancel:" + child.CurrentChildRunID.String()
		if _, err := tx.Tx().Exec(ctx, "SAVEPOINT child_cancel_inbox"); err != nil {
			return err
		}
		_, err := tx.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
			ProjectID:   child.ProjectID,
			InstanceID:  child.ChildInstanceID,
			RunID:       pgtype.UUID{Bytes: child.CurrentChildRunID, Valid: true},
			Kind:        "cancel",
			Payload:     payload,
			AvailableAt: timeNowUTC(),
			DedupeKey:   &dedupeKey,
		})
		if err == nil {
			if _, releaseErr := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT child_cancel_inbox"); releaseErr != nil {
				return releaseErr
			}
			continue
		}
		if _, rollbackErr := tx.Tx().Exec(ctx, "ROLLBACK TO SAVEPOINT child_cancel_inbox"); rollbackErr != nil {
			return rollbackErr
		}
		if _, releaseErr := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT child_cancel_inbox"); releaseErr != nil {
			return releaseErr
		}
		if !errors.Is(err, store.ErrAlreadyExists) {
			return err
		}
	}
	return nil
}

func (a *Activator) commitChildTerminalTransition(
	ctx context.Context,
	tx *store.Tx,
	run *enginedb.EngineRun,
	status enginedb.EngineChildWorkflowStatus,
) error {
	if run == nil || !run.ParentRunID.Valid {
		return nil
	}
	childWorkflow, err := tx.UpdateChildWorkflowTerminal(ctx, enginedb.UpdateChildWorkflowTerminalParams{
		ProjectID:          run.ProjectID,
		CurrentChildRunID:  run.ID,
		TerminalChildRunID: pgtype.UUID{Bytes: run.ID, Valid: true},
		Status:             status,
	})
	if err != nil {
		return err
	}
	if childWorkflow.ParentWaitFailedAt.Valid {
		return nil
	}
	_, err = tx.WakeWaitingChildWorkflowRun(ctx, enginedb.WakeWaitingChildWorkflowRunParams{
		ID:       childWorkflow.ParentRunID,
		ChildKey: childWorkflow.ChildKey,
	})
	return err
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

	return createProjectedTraceShell(ctx, tx, instance, run, seed, startedEvent, input, seed.Name)
}

func createProjectedTraceShell(
	ctx context.Context,
	tx *store.Tx,
	instance *enginedb.EngineInstance,
	run *enginedb.EngineRun,
	seed *continuationTraceSeed,
	startedEvent *enginedb.EngineHistory,
	input json.RawMessage,
	traceName *string,
) error {
	if tx == nil || instance == nil || run == nil || seed == nil || startedEvent == nil {
		return errors.New("projected trace shell requires tx, instance, run, seed, and started event")
	}

	runStatus := string(enginedb.EngineRunLifecycleStatusQueued)
	projectionState := publicprojection.StateUpToDate.String()
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

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}
