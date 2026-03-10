package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/jobargs"
	"github.com/continua-ai/continua/internal/store"
)

// IngestBatchArgs contains the arguments for an async ingest job.
type IngestBatchArgs = jobargs.IngestBatchArgs

// IngestBatchWorker processes accepted ingest batches in the background.
type IngestBatchWorker struct {
	river.WorkerDefaults[IngestBatchArgs]
	store     *store.Store
	processor *ingest.Processor
	client    *river.Client[pgx.Tx]
}

// Timeout bounds async batch processing.
func (w *IngestBatchWorker) Timeout(*river.Job[IngestBatchArgs]) time.Duration {
	return 5 * time.Minute
}

// Work processes an accepted ingest batch.
func (w *IngestBatchWorker) Work(ctx context.Context, job *river.Job[IngestBatchArgs]) error {
	startedAt := time.Now()

	batch, shouldProcess, err := w.claimBatch(ctx, job.Args.BatchID)
	if err != nil {
		return err
	}
	if !shouldProcess {
		return nil
	}

	req, err := w.loadRequest(ctx, batch.ID)
	if err != nil {
		return w.finishTerminalFailure(ctx, &batch, startedAt, err)
	}

	tx, err := w.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return w.retryBatch(ctx, &batch, startedAt, classifyRetryableError(err), err.Error(), err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := w.processor.ProcessBatch(ctx, tx, batch.ProjectID, req)
	if err != nil {
		return w.handleProcessingError(ctx, &batch, startedAt, err)
	}

	if err := enqueueRollupsInTx(ctx, w.client, tx.Tx(), result.TraceIDs); err != nil {
		return w.retryBatch(ctx, &batch, startedAt, classifyRetryableError(err), err.Error(), err)
	}

	if err := tx.MarkBatchCompleted(ctx, platform.MarkBatchCompletedParams{
		ID:            batch.ID,
		TraceCount:    &result.TraceCount,
		SpanCount:     &result.SpanCount,
		EventCount:    &result.EventCount,
		AcceptedCount: &result.AcceptedCount,
		RejectedCount: &result.RejectedCount,
	}); err != nil {
		return w.retryBatch(ctx, &batch, startedAt, classifyRetryableError(err), err.Error(), err)
	}

	if err := tx.DeleteBatchPayload(ctx, batch.ID); err != nil {
		return w.retryBatch(ctx, &batch, startedAt, classifyRetryableError(err), err.Error(), err)
	}

	if err := tx.Commit(ctx); err != nil {
		return w.resolveCommitOutcome(ctx, &batch, startedAt, err)
	}

	log.Printf(
		"event=batch_processing_completed batch_id=%s batch_key=%s project_id=%s attempt_count=%d duration_ms=%d",
		batch.ID,
		batch.BatchKey,
		batch.ProjectID,
		batch.AttemptCount,
		time.Since(startedAt).Milliseconds(),
	)
	return nil
}

func (w *IngestBatchWorker) resolveCommitOutcome(
	ctx context.Context,
	batch *platform.IngestBatch,
	startedAt time.Time,
	commitErr error,
) error {
	persistedBatch, err := w.store.GetBatch(ctx, batch.ID)
	if err == nil {
		_, payloadErr := w.store.GetBatchPayload(ctx, batch.ID)
		payloadMissing := store.IsNotFound(payloadErr)

		if persistedBatch.Status == "completed" && payloadMissing {
			log.Printf(
				"event=batch_processing_commit_reconciled batch_id=%s batch_key=%s project_id=%s attempt_count=%d duration_ms=%d",
				batch.ID,
				batch.BatchKey,
				batch.ProjectID,
				persistedBatch.AttemptCount,
				time.Since(startedAt).Milliseconds(),
			)
			return nil
		}

		if persistedBatch.Status == "failed" {
			return nil
		}
	}

	log.Printf(
		"event=batch_processing_commit_ambiguous batch_id=%s batch_key=%s project_id=%s attempt_count=%d duration_ms=%d error_code=commit_uncertain",
		batch.ID,
		batch.BatchKey,
		batch.ProjectID,
		batch.AttemptCount,
		time.Since(startedAt).Milliseconds(),
	)
	return commitErr
}

func (w *IngestBatchWorker) claimBatch(ctx context.Context, batchID uuid.UUID) (platform.IngestBatch, bool, error) {
	batch, err := w.store.GetBatch(ctx, batchID)
	if err != nil {
		if store.IsNotFound(err) {
			return platform.IngestBatch{}, false, nil
		}
		return platform.IngestBatch{}, false, err
	}

	switch batch.Status {
	case "completed", "failed", "accepted":
		return batch, false, nil
	case "processing":
		log.Printf(
			"event=batch_processing_resumed batch_id=%s batch_key=%s project_id=%s attempt_count=%d",
			batch.ID,
			batch.BatchKey,
			batch.ProjectID,
			batch.AttemptCount,
		)
		return batch, true, nil
	case "queued":
		claimed, err := w.store.MarkBatchProcessingIfQueued(ctx, batchID)
		if err != nil {
			if store.IsNotFound(err) {
				return w.claimBatch(ctx, batchID)
			}
			return platform.IngestBatch{}, false, err
		}
		log.Printf(
			"event=batch_processing_started batch_id=%s batch_key=%s project_id=%s attempt_count=%d",
			claimed.ID,
			claimed.BatchKey,
			claimed.ProjectID,
			claimed.AttemptCount,
		)
		return claimed, true, nil
	default:
		return batch, true, nil
	}
}

func (w *IngestBatchWorker) loadRequest(ctx context.Context, batchID uuid.UUID) (*ingest.IngestRequest, error) {
	payload, err := w.store.GetBatchPayload(ctx, batchID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, &ingest.TerminalError{
				Code:    "payload_missing",
				Message: "async batch payload missing",
				Err:     err,
			}
		}
		return nil, err
	}

	rawPayload, err := ingest.DecompressPayload(payload.PayloadBytes)
	if err != nil {
		return nil, &ingest.TerminalError{
			Code:    "payload_decode_error",
			Message: "failed to decompress ingest payload",
			Err:     err,
		}
	}

	var req ingest.IngestRequest
	if err := json.Unmarshal(rawPayload, &req); err != nil {
		return nil, &ingest.TerminalError{
			Code:    "payload_decode_error",
			Message: "failed to decode ingest payload",
			Err:     err,
		}
	}

	return &req, nil
}

func (w *IngestBatchWorker) handleProcessingError(
	ctx context.Context,
	batch *platform.IngestBatch,
	startedAt time.Time,
	err error,
) error {
	var dependencyErr *ingest.DependencyNotReadyError
	if errors.As(err, &dependencyErr) {
		deadline := batch.ServerReceivedAt.Add(w.processor.DependencyRetryWindow())
		if time.Now().Before(deadline) {
			return w.retryBatch(ctx, batch, startedAt, "dependency_not_ready", dependencyErr.Error(), err)
		}
		return w.finishTerminalFailure(ctx, batch, startedAt, &ingest.TerminalError{
			Code:    "reference_timeout",
			Message: dependencyErr.Error(),
			Err:     err,
		})
	}

	var terminalErr *ingest.TerminalError
	if errors.As(err, &terminalErr) {
		return w.finishTerminalFailure(ctx, batch, startedAt, terminalErr)
	}

	return w.retryBatch(ctx, batch, startedAt, classifyRetryableError(err), err.Error(), err)
}

func (w *IngestBatchWorker) finishTerminalFailure(
	ctx context.Context,
	batch *platform.IngestBatch,
	startedAt time.Time,
	err error,
) error {
	var terminalErr *ingest.TerminalError
	if !errors.As(err, &terminalErr) {
		terminalErr = &ingest.TerminalError{
			Code:    "internal_terminal",
			Message: err.Error(),
			Err:     err,
		}
	}

	if statusErr := w.store.MarkBatchFailed(ctx, platform.MarkBatchFailedParams{
		ID:               batch.ID,
		LastErrorCode:    stringPtr(terminalErr.Code),
		LastErrorMessage: stringPtr(terminalErr.Message),
	}); statusErr != nil {
		return statusErr
	}

	log.Printf(
		"event=batch_processing_failed batch_id=%s batch_key=%s project_id=%s attempt_count=%d duration_ms=%d error_code=%s",
		batch.ID,
		batch.BatchKey,
		batch.ProjectID,
		batch.AttemptCount,
		time.Since(startedAt).Milliseconds(),
		terminalErr.Code,
	)
	return nil
}

func (w *IngestBatchWorker) retryBatch(
	ctx context.Context,
	batch *platform.IngestBatch,
	startedAt time.Time,
	errorCode string,
	errorMessage string,
	err error,
) error {
	if statusErr := w.store.MarkBatchQueued(ctx, platform.MarkBatchQueuedParams{
		ID:               batch.ID,
		LastErrorCode:    stringPtr(errorCode),
		LastErrorMessage: stringPtr(errorMessage),
	}); statusErr != nil {
		return statusErr
	}

	log.Printf(
		"event=batch_processing_retried batch_id=%s batch_key=%s project_id=%s attempt_count=%d duration_ms=%d error_code=%s",
		batch.ID,
		batch.BatchKey,
		batch.ProjectID,
		batch.AttemptCount,
		time.Since(startedAt).Milliseconds(),
		errorCode,
	)
	return err
}

func enqueueRollupsInTx(ctx context.Context, client *river.Client[pgx.Tx], tx pgx.Tx, traceIDs []uuid.UUID) error {
	for _, traceID := range traceIDs {
		if _, err := EnqueueRollupInTx(ctx, client, tx, traceID); err != nil {
			return err
		}
	}
	return nil
}

func classifyRetryableError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "worker_timeout"
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return "db_error"
	}

	return "internal_retryable"
}

func stringPtr(value string) *string {
	return &value
}
