package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

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

// GetBatch retrieves a batch by its internal UUID.
func (s *Store) GetBatch(ctx context.Context, id uuid.UUID) (platform.IngestBatch, error) {
	batch, err := s.q.GetBatch(ctx, id)
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

// ListBatches returns paginated batches for a project.
func (s *Store) ListBatches(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]platform.IngestBatch, error) {
	return s.q.ListBatches(ctx, platform.ListBatchesParams{
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
}
