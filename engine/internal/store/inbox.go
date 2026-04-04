package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateInboxItem(ctx context.Context, arg enginedb.CreateInboxItemParams) (enginedb.EngineInbox, error) {
	return mapResult(o.q.CreateInboxItem(ctx, arg))
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

func (o *storeOps) MarkInboxProcessed(ctx context.Context, id uuid.UUID) (enginedb.EngineInbox, error) {
	return mapResult(o.q.MarkInboxProcessed(ctx, id))
}

func (o *storeOps) MarkInboxDiscarded(ctx context.Context, id uuid.UUID) (enginedb.EngineInbox, error) {
	return mapResult(o.q.MarkInboxDiscarded(ctx, id))
}
