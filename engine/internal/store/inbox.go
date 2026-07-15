package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
)

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateInboxItem(ctx context.Context, arg enginedb.CreateInboxItemParams) (enginedb.EngineInbox, error) {
	inbox, err := mapResult(o.q.CreateInboxItem(ctx, arg))
	if err != nil {
		return enginedb.EngineInbox{}, err
	}
	if err := o.emitNotify(ctx, publicnotify.ChannelInbox); err != nil {
		return enginedb.EngineInbox{}, err
	}
	return inbox, nil
}

func (o *storeOps) ClaimNextInboxItem(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (enginedb.EngineInbox, error) {
	return mapResult(o.q.ClaimNextInboxItem(ctx, enginedb.ClaimNextInboxItemParams{
		ClaimedBy:           nullableWorkerID(workerID),
		LeaseDurationMicros: leaseDurationMicros(leaseDuration),
	}))
}

func (o *storeOps) ListPendingInboxByRun(
	ctx context.Context,
	runID uuid.UUID,
) ([]enginedb.EngineInbox, error) {
	return o.q.ListPendingInboxByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true})
}

func (o *storeOps) ListOpenInboxItemsByRunAndKind(
	ctx context.Context,
	runID uuid.UUID,
	kind string,
) ([]enginedb.EngineInbox, error) {
	return o.q.ListOpenInboxItemsByRunAndKind(ctx, enginedb.ListOpenInboxItemsByRunAndKindParams{
		RunID: pgtype.UUID{Bytes: runID, Valid: true},
		Kind:  kind,
	})
}

func (o *storeOps) ListDiscardedTimerInboxItemsByRun(
	ctx context.Context,
	runID uuid.UUID,
) ([]enginedb.EngineInbox, error) {
	return o.q.ListDiscardedTimerInboxItemsByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true})
}

func (o *storeOps) ListDueTimerRunIDs(ctx context.Context) ([]uuid.UUID, error) {
	var (
		rawIDs []pgtype.UUID
		err    error
	)
	if o.projectFilter != nil {
		rawIDs, err = o.q.ListDueTimerRunIDsByProject(ctx, *o.projectFilter)
	} else {
		rawIDs, err = o.q.ListDueTimerRunIDs(ctx)
	}
	if err != nil {
		return nil, err
	}

	runIDs := make([]uuid.UUID, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		if !rawID.Valid {
			continue
		}
		runIDs = append(runIDs, rawID.Bytes)
	}

	return runIDs, nil
}

func (o *storeOps) MarkInboxProcessed(ctx context.Context, id uuid.UUID) (enginedb.EngineInbox, error) {
	return mapResult(o.q.MarkInboxProcessed(ctx, id))
}

func (o *storeOps) MarkInboxDiscarded(ctx context.Context, id uuid.UUID) (enginedb.EngineInbox, error) {
	return mapResult(o.q.MarkInboxDiscarded(ctx, id))
}

func (o *storeOps) DiscardOpenInboxItemsByRun(
	ctx context.Context,
	runID uuid.UUID,
) ([]enginedb.EngineInbox, error) {
	return o.q.DiscardOpenInboxItemsByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true})
}
