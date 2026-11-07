package workflow

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/continua-ai/continua/engine/internal/store"
)

type Worker struct {
	store       *store.Store
	activator   *Activator
	runLeaseTTL time.Duration
	logger      *slog.Logger
}

func NewWorker(engineStore *store.Store, definitions *Registry, runLeaseTTL time.Duration, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		store:       engineStore,
		activator:   NewActivator(engineStore, definitions),
		runLeaseTTL: runLeaseTTL,
		logger:      logger,
	}
}

func (w *Worker) WithDepthLimits(limits DepthLimits) *Worker {
	w.activator.withDepthLimits(limits)
	return w
}

// PollOnce claims and activates at most one ready run and reports whether it claimed one.
func (w *Worker) PollOnce(ctx context.Context, workerID string) (bool, error) {
	metrics := w.store.Metrics()
	run, err := w.store.ClaimNextRun(ctx, workerID, w.runLeaseTTL)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			metrics.IncClaim("run", "empty")
			return false, nil
		}
		return false, err
	}
	metrics.IncClaim("run", "claimed")
	if err := applyTestClaimHook(ctx); err != nil {
		return true, err
	}

	if err := w.activator.Activate(ctx, &run); err != nil {
		if errors.Is(err, store.ErrStaleClaim) {
			metrics.IncClaim("run", "stale")
			w.logger.Warn("workflow worker stale claim",
				"worker", "workflow",
				"worker_id", workerID,
				"project_id", run.ProjectID,
				"run_id", run.ID,
				"event", "stale_claim",
			)
			return true, nil
		}
		return true, err
	}

	return true, nil
}
