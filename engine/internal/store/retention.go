package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
)

// RetentionReapCounts reports journal rows deleted for a terminal run.
type RetentionReapCounts struct {
	History       int64
	Inbox         int64
	ActivityTasks int64
}

// ReapRequestDedupe deletes one batch of finalized dedupe rows older than cutoff.
func (o *storeOps) ReapRequestDedupe(ctx context.Context, cutoff time.Time, limit int32) (int64, error) {
	projectFilter := pgtype.UUID{}
	if o.projectFilter != nil {
		projectFilter = pgtype.UUID{Bytes: *o.projectFilter, Valid: true}
	}
	return o.q.ReapRequestDedupe(ctx, enginedb.ReapRequestDedupeParams{
		ProjectFilter: projectFilter,
		Cutoff:        cutoff,
		BatchSize:     limit,
	})
}

// ListRetainableTerminalRunIDs returns oldest-first terminal runs whose
// projected trace has consumed the complete history journal.
func (s *Store) ListRetainableTerminalRunIDs(
	ctx context.Context,
	cutoff time.Time,
	limit int32,
) ([]uuid.UUID, error) {
	projectFilter := pgtype.UUID{}
	if s.projectFilter != nil {
		projectFilter = pgtype.UUID{Bytes: *s.projectFilter, Valid: true}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT r.id
		FROM engine.runs AS r
		JOIN public.traces AS t ON t.engine_run_id = r.id
		WHERE ($1::uuid IS NULL OR r.project_id = $1)
		  AND r.status IN ('completed', 'failed', 'cancelled', 'terminated')
		  AND r.completed_at IS NOT NULL
		  AND r.completed_at < $2
		  AND t.engine_latest_history_id IS NOT NULL
		  AND t.engine_last_projected_history_id IS NOT NULL
		  AND t.engine_last_projected_history_id >= t.engine_latest_history_id
		  AND COALESCE(t.engine_projection_state, '') <> 'journal_expired'
		ORDER BY r.completed_at ASC
		LIMIT $3
	`, projectFilter, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runIDs := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var runID uuid.UUID
		if err := rows.Scan(&runID); err != nil {
			return nil, err
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runIDs, nil
}

// ReapTerminalRunJournal deletes a terminal run's engine journal atomically
// and marks its retained platform projection as journal-expired.
func (s *Store) ReapTerminalRunJournal(ctx context.Context, runID uuid.UUID) (RetentionReapCounts, error) {
	tx, err := s.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RetentionReapCounts{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if tx.projectFilter != nil {
		_, err = tx.q.GetRunByProjectAndIDForUpdate(ctx, enginedb.GetRunByProjectAndIDForUpdateParams{
			ProjectID: *tx.projectFilter,
			ID:        runID,
		})
	} else {
		_, err = tx.q.GetRunForUpdate(ctx, runID)
	}
	if err != nil {
		return RetentionReapCounts{}, normalizeError(err)
	}

	var counts RetentionReapCounts
	counts.ActivityTasks, err = tx.q.DeleteActivityTasksByRun(ctx, runID)
	if err != nil {
		return RetentionReapCounts{}, err
	}
	counts.Inbox, err = tx.q.DeleteInboxByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true})
	if err != nil {
		return RetentionReapCounts{}, err
	}
	counts.History, err = tx.q.DeleteHistoryByRunCounted(ctx, runID)
	if err != nil {
		return RetentionReapCounts{}, err
	}
	if _, err := publicprojection.NewWriter(tx.Tx()).MarkProjectionJournalExpired(ctx, runID); err != nil {
		return RetentionReapCounts{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RetentionReapCounts{}, err
	}
	return counts, nil
}
