package jobs

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/jobargs"
	"github.com/continua-ai/continua/internal/store"
)

const (
	defaultRetentionBatchSize = 100
	retentionAdvisoryLockKey  = int64(62026040601)
)

type RetentionWorker struct {
	river.WorkerDefaults[jobargs.RetentionArgs]
	store               *store.Store
	control             *enginecontrol.Service
	projectionRetention time.Duration
	historyRetention    time.Duration
	batchSize           int
}

func NewRetentionWorker(
	s *store.Store,
	control *enginecontrol.Service,
	projectionRetention time.Duration,
	historyRetention time.Duration,
) *RetentionWorker {
	return &RetentionWorker{
		store:               s,
		control:             control,
		projectionRetention: projectionRetention,
		historyRetention:    historyRetention,
		batchSize:           defaultRetentionBatchSize,
	}
}

func (w *RetentionWorker) Timeout(*river.Job[jobargs.RetentionArgs]) time.Duration {
	return 5 * time.Minute
}

func (w *RetentionWorker) Work(ctx context.Context, _ *river.Job[jobargs.RetentionArgs]) error {
	if w == nil || w.store == nil || w.control == nil {
		return nil
	}

	lockConn, err := w.store.Pool().Acquire(ctx)
	if err != nil {
		return err
	}
	defer lockConn.Release()

	locked, err := w.tryAdvisoryLock(ctx, lockConn)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer w.unlockAdvisoryLock(context.Background(), lockConn)

	startedAt := time.Now().UTC()
	var projectionCount int
	var historyCount int

	if w.projectionRetention > 0 {
		candidates, err := w.store.ListProjectionRetentionCandidates(
			ctx,
			startedAt.Add(-w.projectionRetention),
			w.batchSize,
		)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			if _, err := w.control.PurgeRun(ctx, candidate.ProjectID, candidate.RunID, enginecontrol.PurgeModeProjectionOnly); err != nil {
				return err
			}
			projectionCount++
		}
	}

	if w.historyRetention > 0 {
		candidates, err := w.store.ListHistoryRetentionCandidates(
			ctx,
			startedAt.Add(-w.historyRetention),
			w.batchSize,
		)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			if _, err := w.control.PurgeRun(ctx, candidate.ProjectID, candidate.RunID, enginecontrol.PurgeModeFull); err != nil {
				return err
			}
			historyCount++
		}
	}

	log.Printf(
		"event=engine_retention_completed projection_count=%d history_count=%d duration_ms=%d",
		projectionCount,
		historyCount,
		time.Since(startedAt).Milliseconds(),
	)
	return nil
}

func (w *RetentionWorker) tryAdvisoryLock(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	var locked bool
	if conn == nil {
		return false, nil
	}
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", retentionAdvisoryLockKey).Scan(&locked); err != nil {
		return false, err
	}
	return locked, nil
}

func (w *RetentionWorker) unlockAdvisoryLock(ctx context.Context, conn *pgxpool.Conn) {
	if w == nil || conn == nil {
		return
	}
	var unlocked bool
	if err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", retentionAdvisoryLockKey).Scan(&unlocked); err != nil {
		log.Printf("event=engine_retention_unlock_failed err=%v", err)
	}
}
