package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// ClaimedBatch contains a claimed or previously existing batch plus whether the row was newly inserted.
type ClaimedBatch struct {
	Batch    platform.IngestBatch
	Inserted bool
}

func claimBatchRowToModel(row platform.ClaimBatchOrGetExistingRow) ClaimedBatch {
	return ClaimedBatch{
		Batch: platform.IngestBatch{
			ID:                    row.ID,
			ProjectID:             row.ProjectID,
			BatchKey:              row.BatchKey,
			Status:                row.Status,
			ServerReceivedAt:      row.ServerReceivedAt,
			ProcessingStartedAt:   row.ProcessingStartedAt,
			ProcessingCompletedAt: row.ProcessingCompletedAt,
			TraceCount:            row.TraceCount,
			SpanCount:             row.SpanCount,
			EventCount:            row.EventCount,
			AcceptedCount:         row.AcceptedCount,
			RejectedCount:         row.RejectedCount,
			CreatedAt:             row.CreatedAt,
			AttemptCount:          row.AttemptCount,
			LastErrorCode:         row.LastErrorCode,
			LastErrorMessage:      row.LastErrorMessage,
			LastErrorAt:           row.LastErrorAt,
		},
		Inserted: row.Inserted,
	}
}

// ClaimBatch attempts to claim a batch for processing.
// Returns the batch ID if successful, or ErrDuplicateBatch if the batch already exists.
func (s *Store) ClaimBatch(ctx context.Context, projectID uuid.UUID, batchKey string) (uuid.UUID, error) {
	id, err := s.q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrDuplicateBatch
	}
	return id, err
}

// ClaimBatchOrGetExisting inserts a new queued batch or returns the existing durable idempotency record.
func (s *Store) ClaimBatchOrGetExisting(ctx context.Context, projectID uuid.UUID, batchKey string) (ClaimedBatch, error) {
	row, err := s.q.ClaimBatchOrGetExisting(ctx, platform.ClaimBatchOrGetExistingParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	if err != nil {
		return ClaimedBatch{}, err
	}
	return claimBatchRowToModel(row), nil
}

// ClaimBatchTx attempts to claim a batch within a transaction.
func (t *Tx) ClaimBatch(ctx context.Context, projectID uuid.UUID, batchKey string) (uuid.UUID, error) {
	id, err := t.q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrDuplicateBatch
	}
	return id, err
}

// ClaimBatchOrGetExisting inserts a new queued batch or returns the existing durable idempotency record within a transaction.
func (t *Tx) ClaimBatchOrGetExisting(ctx context.Context, projectID uuid.UUID, batchKey string) (ClaimedBatch, error) {
	row, err := t.q.ClaimBatchOrGetExisting(ctx, platform.ClaimBatchOrGetExistingParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	if err != nil {
		return ClaimedBatch{}, err
	}
	return claimBatchRowToModel(row), nil
}

// GetBatch retrieves a batch by its internal UUID.
func (s *Store) GetBatch(ctx context.Context, id uuid.UUID) (platform.IngestBatch, error) {
	batch, err := s.q.GetBatch(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.IngestBatch{}, ErrNotFound
	}
	return batch, err
}

// GetBatchForProject retrieves a batch by project and internal UUID.
func (s *Store) GetBatchForProject(ctx context.Context, projectID, id uuid.UUID) (platform.IngestBatch, error) {
	batch, err := s.q.GetBatchForProject(ctx, platform.GetBatchForProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.IngestBatch{}, ErrNotFound
	}
	return batch, err
}

// GetBatchByKey retrieves a batch by project ID and batch key.
func (s *Store) GetBatchByKey(ctx context.Context, projectID uuid.UUID, batchKey string) (platform.IngestBatch, error) {
	batch, err := s.q.GetBatchByKey(ctx, platform.GetBatchByKeyParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.IngestBatch{}, ErrNotFound
	}
	return batch, err
}

// UpdateBatchStatus updates a batch's processing status and counts.
func (s *Store) UpdateBatchStatus(ctx context.Context, params platform.UpdateBatchStatusParams) error {
	return s.q.UpdateBatchStatus(ctx, params)
}

// UpdateBatchStatusTx updates a batch's processing status within a transaction.
func (t *Tx) UpdateBatchStatus(ctx context.Context, params platform.UpdateBatchStatusParams) error {
	return t.q.UpdateBatchStatus(ctx, params)
}

// InsertBatchPayload stores the compressed ingest payload for an async batch.
func (s *Store) InsertBatchPayload(ctx context.Context, params platform.InsertBatchPayloadParams) error {
	return s.q.InsertBatchPayload(ctx, params)
}

// InsertBatchPayload stores the compressed ingest payload for an async batch within a transaction.
func (t *Tx) InsertBatchPayload(ctx context.Context, params platform.InsertBatchPayloadParams) error {
	return t.q.InsertBatchPayload(ctx, params)
}

// GetBatchPayload loads a stored async batch payload.
func (s *Store) GetBatchPayload(ctx context.Context, batchID uuid.UUID) (platform.IngestBatchPayload, error) {
	payload, err := s.q.GetBatchPayload(ctx, batchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.IngestBatchPayload{}, ErrNotFound
	}
	return payload, err
}

// DeleteBatchPayload removes a stored async batch payload.
func (s *Store) DeleteBatchPayload(ctx context.Context, batchID uuid.UUID) error {
	return s.q.DeleteBatchPayload(ctx, batchID)
}

// DeleteBatchPayload removes a stored async batch payload within a transaction.
func (t *Tx) DeleteBatchPayload(ctx context.Context, batchID uuid.UUID) error {
	return t.q.DeleteBatchPayload(ctx, batchID)
}

// MarkBatchProcessingIfQueued transitions a queued batch to processing.
func (s *Store) MarkBatchProcessingIfQueued(ctx context.Context, batchID uuid.UUID) (platform.IngestBatch, error) {
	batch, err := s.q.MarkBatchProcessingIfQueued(ctx, batchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.IngestBatch{}, ErrNotFound
	}
	return batch, err
}

// MarkBatchProcessingIfQueued transitions a queued batch to processing within a transaction.
func (t *Tx) MarkBatchProcessingIfQueued(ctx context.Context, batchID uuid.UUID) (platform.IngestBatch, error) {
	batch, err := t.q.MarkBatchProcessingIfQueued(ctx, batchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.IngestBatch{}, ErrNotFound
	}
	return batch, err
}

// MarkBatchCompleted records a completed batch.
func (s *Store) MarkBatchCompleted(ctx context.Context, params platform.MarkBatchCompletedParams) error {
	return s.q.MarkBatchCompleted(ctx, params)
}

// MarkBatchCompleted records a completed batch within a transaction.
func (t *Tx) MarkBatchCompleted(ctx context.Context, params platform.MarkBatchCompletedParams) error {
	return t.q.MarkBatchCompleted(ctx, params)
}

// MarkBatchFailed records a terminal batch failure.
func (s *Store) MarkBatchFailed(ctx context.Context, params platform.MarkBatchFailedParams) error {
	return s.q.MarkBatchFailed(ctx, params)
}

// MarkBatchQueued moves a batch back to queued with retry metadata.
func (s *Store) MarkBatchQueued(ctx context.Context, params platform.MarkBatchQueuedParams) error {
	return s.q.MarkBatchQueued(ctx, params)
}

// CleanupExpiredPayloads deletes failed payload rows older than the provided completion cutoff.
func (s *Store) CleanupExpiredPayloads(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	return s.q.CleanupExpiredPayloads(ctx, pgtype.Timestamptz{
		Time:  cutoff,
		Valid: true,
	})
}

// ListBatches returns paginated batches for a project.
func (s *Store) ListBatches(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]platform.IngestBatch, error) {
	return s.q.ListBatches(ctx, platform.ListBatchesParams{
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
}
