package jobs

import (
	"context"
	"log/slog"
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

	slog.Info("engine_retention_completed",
		"projection_count", projectionCount,
		"history_count", historyCount,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
	return nil
}

// tryAdvisoryLock takes the process-wide retention advisory lock on the given
// session. conn must be a connection returned by Pool().Acquire and held for
// the duration of the run, and the caller must release the lock explicitly.
func (w *RetentionWorker) tryAdvisoryLock(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", retentionAdvisoryLockKey).Scan(&locked); err != nil {
		return false, err
	}
	return locked, nil
}

// Postgres binds advisory locks to the session that took them, so releasing
// must happen on the same connection. A session-level lock lives until
// pg_advisory_unlock runs or the session ends. Returning the connection to the
// pool does not end the session, so skipping this call would leave the lock
// held and hand it to whichever caller borrows that connection next.
func (w *RetentionWorker) unlockAdvisoryLock(ctx context.Context, conn *pgxpool.Conn) {
	var unlocked bool
	if err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", retentionAdvisoryLockKey).Scan(&unlocked); err != nil {
		slog.Warn("engine_retention_unlock_failed", "err", err)
	}
}
