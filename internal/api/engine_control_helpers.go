package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

const uniqueViolationCode = "23505"

type startRequestDedupeClaimState string

const (
	startRequestDedupeClaimStateClaimedNew         startRequestDedupeClaimState = "claimed_new"
	startRequestDedupeClaimStateClaimedReclaimed   startRequestDedupeClaimState = "claimed_reclaimed"
	startRequestDedupeClaimStateExistingFinalized  startRequestDedupeClaimState = "existing_finalized"
	startRequestDedupeClaimStateExistingInProgress startRequestDedupeClaimState = "existing_in_progress"
)

type claimStartRequestDedupeParams struct {
	ProjectID    uuid.UUID
	RequestScope string
	RequestKey   string
	ExpiresAt    time.Time
}

type startRequestDedupeClaim struct {
	Row   enginedb.EngineRequestDedupe
	State startRequestDedupeClaimState
}

func claimStartRequestDedupe(
	ctx context.Context,
	engineTx *enginedb.Queries,
	arg claimStartRequestDedupeParams,
) (startRequestDedupeClaim, error) {
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
		row, err := engineTx.CreateStartRequestDedupeClaim(ctx, createArg)
		if err == nil {
			return startRequestDedupeClaim{
				Row:   row,
				State: startRequestDedupeClaimStateClaimedNew,
			}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return startRequestDedupeClaim{}, err
		}

		row, err = engineTx.GetRequestDedupeByScopeAndKeyForUpdate(ctx, lockArg)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return startRequestDedupeClaim{}, err
		}

		switch row.Status {
		case enginedb.EngineRequestDedupeStatusCompleted, enginedb.EngineRequestDedupeStatusFailed:
			return startRequestDedupeClaim{
				Row:   row,
				State: startRequestDedupeClaimStateExistingFinalized,
			}, nil
		case enginedb.EngineRequestDedupeStatusInProgress:
			if !row.ExpiresAt.Before(time.Now()) {
				return startRequestDedupeClaim{
					Row:   row,
					State: startRequestDedupeClaimStateExistingInProgress,
				}, nil
			}
		case enginedb.EngineRequestDedupeStatusExpired:
		default:
			return startRequestDedupeClaim{}, errors.New("unsupported request dedupe status")
		}

		renewed, renewErr := engineTx.RenewRequestDedupeClaim(ctx, enginedb.RenewRequestDedupeClaimParams{
			ID:        row.ID,
			ExpiresAt: arg.ExpiresAt,
		})
		if renewErr != nil {
			return startRequestDedupeClaim{}, renewErr
		}

		return startRequestDedupeClaim{
			Row:   renewed,
			State: startRequestDedupeClaimStateClaimedReclaimed,
		}, nil
	}

	return startRequestDedupeClaim{}, errors.New("could not acquire request dedupe claim")
}

func decodeStartRunReplay(row *enginedb.EngineRequestDedupe) (engineStartRunResult, error) {
	if row == nil {
		return engineStartRunResult{}, errors.New("request dedupe row is required")
	}
	if row.Status == enginedb.EngineRequestDedupeStatusFailed {
		apiErr := &engineAPIError{
			Code:       derefString(row.ErrorCode),
			Message:    derefString(row.ErrorMessage),
			HTTPStatus: statusForEngineErrorCode(derefString(row.ErrorCode)),
		}
		return engineStartRunResult{}, apiErr
	}

	var result engineStartRunResult
	if err := json.Unmarshal(row.ResponsePayload, &result); err != nil {
		return engineStartRunResult{}, err
	}
	return result, nil
}

func finalizeStartSuccess(
	ctx context.Context,
	engineTx *enginedb.Queries,
	tx pgx.Tx,
	dedupeID uuid.UUID,
	result engineStartRunResult,
) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}

	if _, err := engineTx.FinalizeRequestDedupeWithResponse(ctx, enginedb.FinalizeRequestDedupeWithResponseParams{
		ID:              dedupeID,
		ResponsePayload: payload,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func finalizeStartFailure(
	ctx context.Context,
	engineTx *enginedb.Queries,
	tx pgx.Tx,
	dedupeID uuid.UUID,
	apiErr *engineAPIError,
) error {
	if _, err := engineTx.FinalizeRequestDedupeWithError(ctx, enginedb.FinalizeRequestDedupeWithErrorParams{
		ID:           dedupeID,
		ErrorCode:    stringPtr(apiErr.Code),
		ErrorMessage: stringPtr(apiErr.Message),
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return apiErr
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}

func stringsTrimSpaceEmpty(value string) bool {
	return strings.TrimSpace(value) == ""
}

func stringPtr(value string) *string {
	if stringsTrimSpaceEmpty(value) {
		return nil
	}
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func int32Pointer(value int32) *int32 {
	return &value
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func statusForEngineErrorCode(code string) int {
	switch code {
	case "definition_not_registered", "invalid_request":
		return 400
	case "instance_conflict", "request_in_progress", "run_not_terminal", "run_terminal":
		return 409
	case "not_found":
		return 404
	default:
		return 400
	}
}
