package activity

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/continua-ai/continua/engine/internal/store"
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
		code := "activity_failed"
		message := handlerErr.Error()
		return w.failTask(ctx, task.ID, task.RunID, workerID, &code, &message)
	}

	_, err = w.store.CompleteActivityTask(ctx, task.ID, workerID, output)
	if err != nil {
		if errors.Is(err, store.ErrStaleClaim) {
			log.Printf("activity worker stale completion for task %s", task.ID)
			return nil
		}
		return err
	}

	_, err = w.store.WakeWaitingRun(ctx, task.RunID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return nil
}

func (w *Worker) failTask(
	ctx context.Context,
	taskID uuid.UUID,
	runID uuid.UUID,
	workerID string,
	errorCode *string,
	errorMessage *string,
) error {
	_, err := w.store.FailActivityTask(ctx, taskID, workerID, errorCode, errorMessage)
	if err != nil {
		if errors.Is(err, store.ErrStaleClaim) {
			log.Printf("activity worker stale failure for task %s", taskID)
			return nil
		}
		return err
	}

	_, err = w.store.WakeWaitingRun(ctx, runID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return nil
}
