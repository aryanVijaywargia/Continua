package activity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	"github.com/continua-ai/continua/engine/internal/store"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

type Worker struct {
	store            *store.Store
	registry         *Registry
	activityLeaseTTL time.Duration
}

func NewWorker(engineStore *store.Store, registry *Registry, activityLeaseTTL time.Duration) *Worker {
	return &Worker{
		store:            engineStore,
		registry:         registry,
		activityLeaseTTL: activityLeaseTTL,
	}
}

func (w *Worker) PollOnce(ctx context.Context, workerID string) error {
	task, err := w.store.ClaimNextActivityTask(ctx, workerID, w.activityLeaseTTL)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			w.store.Metrics().IncClaim("activity_task", "empty")
			return nil
		}
		return err
	}
	w.store.Metrics().IncClaim("activity_task", "claimed")

	handler, ok := w.registry.Get(task.ActivityType)
	if !ok {
		code := "activity_not_registered"
		message := "no handler registered for activity type " + task.ActivityType
		return w.failTask(ctx, task.ID, task.RunID, workerID, &code, &message)
	}

	output, handlerErr := w.runHandlerWithHeartbeat(ctx, handler, &task, workerID)
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
			w.store.Metrics().IncClaim("activity_task", "stale")
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
		if syncErr := publicprojection.NewWriter(tx.Tx()).SyncRunSummary(ctx, &wake.Run); syncErr != nil {
			return syncErr
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	w.store.Metrics().IncActivityAttempt("completed")
	return nil
}

type handlerResult struct {
	output json.RawMessage
	err    error
}

func (w *Worker) runHandlerWithHeartbeat(
	ctx context.Context,
	handler Handler,
	task *enginedb.EngineActivityTask,
	workerID string,
) (json.RawMessage, error) {
	handlerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan handlerResult, 1)
	go func() {
		output, err := handler(handlerCtx, task.Input)
		results <- handlerResult{output: output, err: err}
	}()

	heartbeatInterval := w.activityLeaseTTL / 2
	if heartbeatInterval <= 0 {
		heartbeatInterval = 50 * time.Millisecond
	}
	heartbeatTimeout := heartbeatInterval / 2
	if heartbeatTimeout <= 0 {
		heartbeatTimeout = heartbeatInterval
	}
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	heartbeatC := ticker.C
	ctxDone := ctx.Done()

	for {
		select {
		case result := <-results:
			return result.output, result.err
		case <-ctxDone:
			cancel()
			ticker.Stop()
			heartbeatC = nil
			ctxDone = nil
		case <-heartbeatC:
			heartbeatCtx, heartbeatCancel := context.WithTimeout(ctx, heartbeatTimeout)
			_, err := w.store.HeartbeatLocalActivityTask(heartbeatCtx, task.ID, workerID)
			heartbeatCancel()
			if err != nil {
				if errors.Is(err, store.ErrStaleClaim) || errors.Is(err, store.ErrNotFound) {
					log.Printf("activity worker lost lease for task %s", task.ID)
					cancel()
					ticker.Stop()
					heartbeatC = nil
					continue
				}
				log.Printf("activity worker heartbeat failed for task %s: %v", task.ID, err)
			}
		}
	}
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
			w.store.Metrics().IncClaim("activity_task", "stale")
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

	if err := publicprojection.NewWriter(tx.Tx()).UpdateLatestHistory(ctx, run.ProjectID, run.ID, appended.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	w.store.Metrics().IncActivityAttempt("retried")
	return nil
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
	maxSequence, err := tx.GetMaxHistorySequenceByRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	return maxSequence + 1, nil
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
	retryDelayMS, err := publicworkflow.ComputeActivityRetryDelayMS(
		task.AttemptCount,
		task.InitialBackoffMs,
		task.MaxBackoffMs,
		task.BackoffMultiplier,
	)
	if err != nil {
		return 0, fmt.Errorf("activity task %s retry policy: %w", task.ID, err)
	}
	return retryDelayMS, nil
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
			w.store.Metrics().IncClaim("activity_task", "stale")
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
		if syncErr := publicprojection.NewWriter(tx.Tx()).SyncRunSummary(ctx, &wake.Run); syncErr != nil {
			return syncErr
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	w.store.Metrics().IncActivityAttempt("failed")
	return nil
}
