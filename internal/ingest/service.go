package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/jobargs"
	"github.com/continua-ai/continua/internal/store"
)

const (
	batchStatusAcceptedLegacy = "accepted"
	batchStatusCompleted      = "completed"
	batchStatusFailed         = "failed"
	batchStatusProcessing     = "processing"
	batchStatusQueued         = "queued"
)

var errUnknownBatchStatus = errors.New("unknown batch status")

// Service handles synchronous and asynchronous batch ingestion flows.
type Service struct {
	store            *store.Store
	riverClient      *river.Client[pgx.Tx]
	processor        *Processor
	trueAsyncDefault bool
}

// ValidationError represents a batch validation error.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %s", e.Errors[0])
}

// DependencyNotReadyError indicates a trace reference that may become valid after another accepted batch commits.
type DependencyNotReadyError struct {
	Message string
}

func (e *DependencyNotReadyError) Error() string {
	return e.Message
}

// TerminalError indicates a non-retryable async worker failure.
type TerminalError struct {
	Code    string
	Message string
	Err     error
}

func (e *TerminalError) Error() string {
	return e.Message
}

func (e *TerminalError) Unwrap() error {
	return e.Err
}

// NewService creates a new ingest service.
func NewService(
	s *store.Store,
	riverClient *river.Client[pgx.Tx],
	processor *Processor,
	cfg *config.Config,
) *Service {
	trueAsyncDefault := false
	if cfg != nil {
		trueAsyncDefault = cfg.Ingest.TrueAsyncDefault
	}

	return &Service{
		store:            s,
		riverClient:      riverClient,
		processor:        processor,
		trueAsyncDefault: trueAsyncDefault,
	}
}

// TrueAsyncDefault reports whether the server default routes non-sync ingest requests to true async.
func (s *Service) TrueAsyncDefault() bool {
	return s.trueAsyncDefault
}

// Ingest processes a batch inline and returns once all writes are committed.
func (s *Service) Ingest(ctx context.Context, projectID uuid.UUID, req *IngestRequest) (*IngestResponse, error) {
	if err := s.processor.Validate(req); err != nil {
		return nil, err
	}

	tx, err := s.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	claim, err := tx.ClaimBatchOrGetExisting(ctx, projectID, req.BatchKey)
	if err != nil {
		return nil, fmt.Errorf("failed to claim batch: %w", err)
	}

	if !claim.Inserted {
		log.Printf(
			"event=batch_duplicate batch_id=%s batch_key=%s project_id=%s",
			claim.Batch.ID,
			claim.Batch.BatchKey,
			projectID,
		)
		return &IngestResponse{
			Status:   string(IngestStatusDuplicate),
			BatchKey: req.BatchKey,
			BatchID:  claim.Batch.ID,
		}, nil
	}

	result, err := s.processor.ProcessBatch(ctx, tx, projectID, req)
	if err != nil {
		var dependencyErr *DependencyNotReadyError
		if errors.As(err, &dependencyErr) {
			return nil, &ValidationError{Errors: []string{dependencyErr.Message}}
		}
		return nil, err
	}

	if err := s.enqueueRollupsInTx(ctx, tx.Tx(), result.TraceIDs); err != nil {
		return nil, fmt.Errorf("failed to enqueue rollups: %w", err)
	}

	if err := tx.MarkBatchCompleted(ctx, platform.MarkBatchCompletedParams{
		ID:            claim.Batch.ID,
		TraceCount:    &result.TraceCount,
		SpanCount:     &result.SpanCount,
		EventCount:    &result.EventCount,
		AcceptedCount: &result.AcceptedCount,
		RejectedCount: &result.RejectedCount,
	}); err != nil {
		return nil, fmt.Errorf("failed to update batch status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf(
		"event=batch_processing_completed batch_id=%s batch_key=%s project_id=%s attempt_count=0 duration_ms=0",
		claim.Batch.ID,
		req.BatchKey,
		projectID,
	)

	return &IngestResponse{
		Status:        string(IngestStatusOK),
		BatchKey:      req.BatchKey,
		BatchID:       claim.Batch.ID,
		TraceCount:    result.TraceCount,
		SpanCount:     result.SpanCount,
		EventCount:    result.EventCount,
		AcceptedCount: result.AcceptedCount,
		RejectedCount: result.RejectedCount,
	}, nil
}

// AcceptAsync validates a batch, durably stores its payload, and enqueues a background job.
func (s *Service) AcceptAsync(
	ctx context.Context,
	projectID uuid.UUID,
	req *IngestRequest,
	rawPayload []byte,
) (*IngestResponse, error) {
	if err := s.processor.Validate(req); err != nil {
		return nil, err
	}

	tx, err := s.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	claim, err := tx.ClaimBatchOrGetExisting(ctx, projectID, req.BatchKey)
	if err != nil {
		return nil, fmt.Errorf("failed to claim batch: %w", err)
	}

	if !claim.Inserted {
		log.Printf(
			"event=batch_duplicate batch_id=%s batch_key=%s project_id=%s",
			claim.Batch.ID,
			claim.Batch.BatchKey,
			projectID,
		)
		return &IngestResponse{
			Status:   string(IngestStatusDuplicate),
			BatchKey: req.BatchKey,
			BatchID:  claim.Batch.ID,
		}, nil
	}

	compressedPayload, err := CompressPayload(rawPayload)
	if err != nil {
		return nil, fmt.Errorf("compress payload: %w", err)
	}

	if err := tx.InsertBatchPayload(ctx, &platform.InsertBatchPayloadParams{
		BatchID:      claim.Batch.ID,
		PayloadBytes: compressedPayload,
		Compression:  "gzip",
		ContentType:  "application/json",
		ByteSize:     int32(len(rawPayload)),
	}); err != nil {
		return nil, fmt.Errorf("insert batch payload: %w", err)
	}

	if err := s.enqueueAsyncBatchInTx(ctx, tx.Tx(), claim.Batch.ID); err != nil {
		return nil, fmt.Errorf("enqueue ingest batch job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit async acceptance transaction: %w", err)
	}

	log.Printf(
		"event=batch_accepted batch_id=%s batch_key=%s project_id=%s",
		claim.Batch.ID,
		req.BatchKey,
		projectID,
	)

	return &IngestResponse{
		Status:   string(IngestStatusAccepted),
		BatchKey: req.BatchKey,
		BatchID:  claim.Batch.ID,
	}, nil
}

// GetBatchStatus retrieves a batch scoped to the given project and maps its internal status to the public API vocabulary.
func (s *Service) GetBatchStatus(ctx context.Context, projectID, batchID uuid.UUID) (*BatchStatus, error) {
	batch, err := s.store.GetBatchForProject(ctx, projectID, batchID)
	if err != nil {
		return nil, err
	}

	return batchStatusFromModel(&batch)
}

func (s *Service) enqueueRollupsInTx(ctx context.Context, tx pgx.Tx, traceIDs []uuid.UUID) error {
	if s.riverClient == nil {
		return nil
	}

	for _, traceID := range traceIDs {
		res, err := s.riverClient.InsertTx(ctx, tx, jobargs.TraceRollupArgs{TraceID: traceID}, nil)
		if err != nil {
			return err
		}
		if res.UniqueSkippedAsDuplicate {
			continue
		}
	}

	return nil
}

func (s *Service) enqueueAsyncBatchInTx(ctx context.Context, tx pgx.Tx, batchID uuid.UUID) error {
	if s.riverClient == nil {
		return errors.New("river client is nil")
	}

	_, err := s.riverClient.InsertTx(ctx, tx, jobargs.IngestBatchArgs{BatchID: batchID}, nil)
	return err
}

// CompressPayload gzips raw request bytes for async payload storage.
func CompressPayload(rawPayload []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(rawPayload); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecompressPayload expands a gzip-compressed async payload.
func DecompressPayload(compressedPayload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressedPayload))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	return ioReadAll(reader)
}

func batchStatusFromModel(batch *platform.IngestBatch) (*BatchStatus, error) {
	status, err := publicBatchStatus(batch.Status)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errUnknownBatchStatus, batch.Status)
	}

	return &BatchStatus{
		BatchID:               batch.ID,
		BatchKey:              batch.BatchKey,
		Status:                status,
		AttemptCount:          batch.AttemptCount,
		ServerReceivedAt:      batch.ServerReceivedAt,
		ProcessingStartedAt:   timestamptzPtr(batch.ProcessingStartedAt),
		ProcessingCompletedAt: timestamptzPtr(batch.ProcessingCompletedAt),
		TraceCount:            batch.TraceCount,
		SpanCount:             batch.SpanCount,
		EventCount:            batch.EventCount,
		AcceptedCount:         batch.AcceptedCount,
		RejectedCount:         batch.RejectedCount,
		LastErrorCode:         batch.LastErrorCode,
		LastErrorMessage:      batch.LastErrorMessage,
	}, nil
}

func publicBatchStatus(internalStatus string) (string, error) {
	switch internalStatus {
	case batchStatusQueued:
		return string(IngestStatusAccepted), nil
	case batchStatusProcessing:
		return batchStatusProcessing, nil
	case batchStatusCompleted, batchStatusAcceptedLegacy:
		return batchStatusCompleted, nil
	case batchStatusFailed:
		return batchStatusFailed, nil
	default:
		return "", errUnknownBatchStatus
	}
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func ioReadAll(reader *gzip.Reader) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
