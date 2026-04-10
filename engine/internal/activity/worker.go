package activity

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	engineprojector "github.com/continua-ai/continua/engine/internal/projector"
	"github.com/continua-ai/continua/engine/internal/store"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

type Worker struct {
	store            *store.Store
	registry         *Registry
	activityLeaseTTL time.Duration
}

func NewWorker(store *store.Store, registry *Registry, activityLeaseTTL time.Duration) *Worker {
	return &Worker{
		store:            store,
		registry:         registry,
		activityLeaseTTL: activityLeaseTTL,
	}
}

func (w *Worker) PollOnce(ctx context.Context, workerID string) error {
	task, err := w.store.ClaimNextActivityTask(ctx, workerID, w.activityLeaseTTL)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}

	handler, ok := w.registry.Get(task.ActivityType)
	if !ok {
		code := "activity_not_registered"
		message := "no handler registered for activity type " + task.ActivityType
		return w.failTask(ctx, task.ID, task.RunID, workerID, &code, &message)
	}

	output, handlerErr := handler(ctx, task.Input)
	if handlerErr != nil {
		if w.shouldRetryTask(&task, handlerErr) {
			return w.retryTask(ctx, &task, workerID, handlerErr)
		}
		code := "activity_failed"
		message := handlerErr.Error()
		return w.failTask(ctx, task.ID, task.RunID, workerID, &code, &message)
	}

	tx, err := w.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.CompleteActivityTask(ctx, task.ID, workerID, output)
	if err != nil {
		if errors.Is(err, store.ErrStaleClaim) {
			log.Printf("activity worker stale completion for task %s", task.ID)
			return nil
		}
		return err
	}

	wake, err := tx.WakeWaitingRun(ctx, task.RunID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err == nil {
		if syncErr := engineprojector.SyncProjectedRunSummary(ctx, tx.Tx(), &wake.Run); syncErr != nil {
			return syncErr
		}
	}
	return tx.Commit(ctx)
}

func (w *Worker) retryTask(
	ctx context.Context,
	task *enginedb.EngineActivityTask,
	workerID string,
	handlerErr error,
) error {
	if task == nil {
		return errors.New("activity task is required")
	}

	retryDelayMS, err := computeRetryDelayMS(task)
	if err != nil {
		return err
	}

	tx, err := w.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	run, err := tx.GetRunForUpdate(ctx, task.RunID)
	if err != nil {
		return err
	}

	retriedTask, err := tx.RetryActivityTask(ctx, task.ID, workerID, retryDelayMS)
	if err != nil {
		if errors.Is(err, store.ErrStaleClaim) {
			log.Printf("activity worker stale retry for task %s", task.ID)
			return nil
		}
		return err
	}

	nextSequence, err := nextHistorySequence(ctx, tx, run.ID)
	if err != nil {
		return err
	}

	payload, err := enginehistory.MarshalPayload(enginehistory.ActivityRetryScheduledPayload{
		ActivityKey:     retriedTask.ActivityKey,
		ActivityType:    retriedTask.ActivityType,
		FailedAttempt:   retriedTask.AttemptCount,
		NextAvailableAt: retriedTask.AvailableAt,
		ErrorCode:       activityErrorCode(handlerErr),
		ErrorMessage:    handlerErr.Error(),
	})
	if err != nil {
		return err
	}

	appended, err := tx.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  run.ProjectID,
		InstanceID: run.InstanceID,
		RunID:      run.ID,
		SequenceNo: nextSequence,
		EventType:  enginehistory.EventActivityRetryScheduled,
		Payload:    payload,
	})
	if err != nil {
		return err
	}

	if err := engineprojector.UpdateLatestHistory(ctx, tx.Tx(), run.ProjectID, run.ID, appended.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Worker) shouldRetryTask(task *enginedb.EngineActivityTask, handlerErr error) bool {
	if task == nil || handlerErr == nil {
		return false
	}
	if publicworkflow.IsNonRetryable(handlerErr) {
		return false
	}
	return task.AttemptCount < task.MaxAttempts
}

func nextHistorySequence(ctx context.Context, tx *store.Tx, runID uuid.UUID) (int32, error) {
	historyRows, err := tx.GetHistoryByRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	if len(historyRows) == 0 {
		return 1, nil
	}
	return historyRows[len(historyRows)-1].SequenceNo + 1, nil
}

func activityErrorCode(err error) string {
	if err == nil {
		return ""
	}
	return "activity_failed"
}

func computeRetryDelayMS(task *enginedb.EngineActivityTask) (int64, error) {
	if task == nil {
		return 0, errors.New("activity task is required")
	}
	if task.InitialBackoffMs == nil || task.MaxBackoffMs == nil || task.BackoffMultiplier == nil {
		return 0, fmt.Errorf("activity task %s is missing retry policy fields", task.ID)
	}

	exponent := float64(task.AttemptCount - 1)
	rawDelayMS := float64(*task.InitialBackoffMs) * math.Pow(*task.BackoffMultiplier, exponent)
	if maxBackoffMS := float64(*task.MaxBackoffMs); rawDelayMS > maxBackoffMS {
		rawDelayMS = maxBackoffMS
	}
	return int64(math.Ceil(rawDelayMS)), nil
}

func (w *Worker) failTask(
	ctx context.Context,
	taskID uuid.UUID,
	runID uuid.UUID,
	workerID string,
	errorCode *string,
	errorMessage *string,
) error {
	tx, err := w.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.FailActivityTask(ctx, taskID, workerID, errorCode, errorMessage)
	if err != nil {
		if errors.Is(err, store.ErrStaleClaim) {
			log.Printf("activity worker stale failure for task %s", taskID)
			return nil
		}
		return err
	}

	wake, err := tx.WakeWaitingRun(ctx, runID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err == nil {
		if syncErr := engineprojector.SyncProjectedRunSummary(ctx, tx.Tx(), &wake.Run); syncErr != nil {
			return syncErr
		}
	}
	return tx.Commit(ctx)
}
