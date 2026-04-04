package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

type StartRequestDedupeClaimState string

const (
	StartRequestDedupeClaimStateClaimedNew         StartRequestDedupeClaimState = "claimed_new"
	StartRequestDedupeClaimStateClaimedReclaimed   StartRequestDedupeClaimState = "claimed_reclaimed"
	StartRequestDedupeClaimStateExistingFinalized  StartRequestDedupeClaimState = "existing_finalized"
	StartRequestDedupeClaimStateExistingInProgress StartRequestDedupeClaimState = "existing_in_progress"
)

type StartRequestDedupeClaim struct {
	Row   enginedb.EngineRequestDedupe
	State StartRequestDedupeClaimState
}

func (c *StartRequestDedupeClaim) NeedsExecution() bool {
	return c.State == StartRequestDedupeClaimStateClaimedNew || c.State == StartRequestDedupeClaimStateClaimedReclaimed
}

type ClaimStartRequestDedupeParams struct {
	ProjectID    uuid.UUID
	RequestScope string
	RequestKey   string
	ExpiresAt    time.Time
}

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateRequestDedupe(
	ctx context.Context,
	arg enginedb.CreateRequestDedupeParams,
) (enginedb.EngineRequestDedupe, error) {
	return mapResult(o.q.CreateRequestDedupe(ctx, arg))
}

func (o *storeOps) GetRequestDedupeByScopeAndKey(
	ctx context.Context,
	arg enginedb.GetRequestDedupeByScopeAndKeyParams,
) (enginedb.EngineRequestDedupe, error) {
	return mapResult(o.q.GetRequestDedupeByScopeAndKey(ctx, arg))
}

func (o *storeOps) FinalizeRequestDedupeWithResponse(
	ctx context.Context,
	arg enginedb.FinalizeRequestDedupeWithResponseParams,
) (enginedb.EngineRequestDedupe, error) {
	return mapResult(o.q.FinalizeRequestDedupeWithResponse(ctx, arg))
}

func (o *storeOps) FinalizeRequestDedupeWithError(
	ctx context.Context,
	arg enginedb.FinalizeRequestDedupeWithErrorParams,
) (enginedb.EngineRequestDedupe, error) {
	return mapResult(o.q.FinalizeRequestDedupeWithError(ctx, arg))
}

func (o *storeOps) ExpireRequestDedupe(ctx context.Context) (int64, error) {
	return o.q.ExpireRequestDedupe(ctx)
}

func (tx *Tx) ClaimStartRequestDedupe(
	ctx context.Context,
	arg ClaimStartRequestDedupeParams,
) (StartRequestDedupeClaim, error) {
	createArg := enginedb.CreateStartRequestDedupeClaimParams{
		ProjectID:    arg.ProjectID,
		RequestScope: arg.RequestScope,
		RequestKey:   arg.RequestKey,
		ExpiresAt:    arg.ExpiresAt,
	}
	lockArg := enginedb.GetRequestDedupeByScopeAndKeyForUpdateParams{
		ProjectID:    arg.ProjectID,
		RequestScope: arg.RequestScope,
		RequestKey:   arg.RequestKey,
	}

	for attempts := 0; attempts < 2; attempts++ {
		row, err := tx.q.CreateStartRequestDedupeClaim(ctx, createArg)
		if err == nil {
			return StartRequestDedupeClaim{
				Row:   row,
				State: StartRequestDedupeClaimStateClaimedNew,
			}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return StartRequestDedupeClaim{}, normalizeError(err)
		}

		row, err = tx.q.GetRequestDedupeByScopeAndKeyForUpdate(ctx, lockArg)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return StartRequestDedupeClaim{}, normalizeError(err)
		}

		switch row.Status {
		case enginedb.EngineRequestDedupeStatusCompleted, enginedb.EngineRequestDedupeStatusFailed:
			return StartRequestDedupeClaim{
				Row:   row,
				State: StartRequestDedupeClaimStateExistingFinalized,
			}, nil
		case enginedb.EngineRequestDedupeStatusInProgress:
			if !row.ExpiresAt.Before(time.Now()) {
				return StartRequestDedupeClaim{
					Row:   row,
					State: StartRequestDedupeClaimStateExistingInProgress,
				}, nil
			}
		case enginedb.EngineRequestDedupeStatusExpired:
		default:
			return StartRequestDedupeClaim{}, fmt.Errorf("engine store: unsupported request dedupe status %q", row.Status)
		}

		renewed, renewErr := tx.q.RenewRequestDedupeClaim(ctx, enginedb.RenewRequestDedupeClaimParams{
			ID:        row.ID,
			ExpiresAt: arg.ExpiresAt,
		})
		if renewErr != nil {
			return StartRequestDedupeClaim{}, normalizeError(renewErr)
		}

		return StartRequestDedupeClaim{
			Row:   renewed,
			State: StartRequestDedupeClaimStateClaimedReclaimed,
		}, nil
	}

	return StartRequestDedupeClaim{}, fmt.Errorf(
		"engine store: could not acquire request dedupe claim for scope %q key %q",
		arg.RequestScope,
		arg.RequestKey,
	)
}
