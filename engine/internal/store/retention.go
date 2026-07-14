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

	return s.q.ListRetainableTerminalRunIDs(ctx, enginedb.ListRetainableTerminalRunIDsParams{
		ProjectFilter: projectFilter,
		Cutoff:        cutoff,
		BatchSize:     limit,
	})
}

// ReapTerminalRunJournal deletes a terminal run's engine journal atomically
// and marks its retained platform projection as journal-expired.
func (s *Store) ReapTerminalRunJournal(ctx context.Context, runID uuid.UUID) (RetentionReapCounts, error) {
	tx, err := s.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RetentionReapCounts{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	projectFilter := pgtype.UUID{}
	if tx.projectFilter != nil {
		projectFilter = pgtype.UUID{Bytes: *tx.projectFilter, Valid: true}
	}
	_, err = tx.q.GetRetainableTerminalRunForUpdate(ctx, enginedb.GetRetainableTerminalRunForUpdateParams{
		RunID:         runID,
		ProjectFilter: projectFilter,
	})
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
