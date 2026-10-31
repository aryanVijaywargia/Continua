package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

const (
	conflictAttempts = 3
	// conflictBackoff is the backoff step between conflict retries. Retry n waits n
	// steps plus up to one step of jitter.
	conflictBackoff = 20 * time.Millisecond

	sqlStateDeadlock             = "40P01"
	sqlStateSerializationFailure = "40001"
)

var errUnknownBatchStatus = errors.New("unknown batch status")

// ErrWriteConflict reports that an inline batch lost a lock conflict with concurrent
// writers on every attempt. Nothing was written, and the client can resend the batch.
var ErrWriteConflict = errors.New("ingest kept conflicting with concurrent writes")

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

// Ingest processes a batch inline and returns once all writes are committed. A batch
// whose transaction loses a deadlock or serialization conflict runs again from the
// start, batch claim included; when every attempt loses, the error wraps
// ErrWriteConflict.
func (s *Service) Ingest(ctx context.Context, projectID uuid.UUID, req *IngestRequest) (*IngestResponse, error) {
	if err := s.processor.Validate(req); err != nil {
		return nil, err
	}

	var resp *IngestResponse
	err := retryOnConflict(ctx, projectID, req.BatchKey, func() error {
		var err error
		resp, err = s.ingestOnce(ctx, projectID, req)
		return err
	})
	return resp, err
}

// ingestOnce claims and processes a batch in one transaction. A failed attempt rolls
// back its claim along with every other write, so the next attempt claims afresh and
// reports a duplicate only if another request committed the same batch key meanwhile.
func (s *Service) ingestOnce(ctx context.Context, projectID uuid.UUID, req *IngestRequest) (*IngestResponse, error) {
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
		slog.Info("batch_duplicate",
			"batch_id", claim.Batch.ID,
			"batch_key", claim.Batch.BatchKey,
			"project_id", projectID,
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

	slog.Info("batch_processing_completed",
		"batch_id", claim.Batch.ID,
		"batch_key", req.BatchKey,
		"project_id", projectID,
		"attempt_count", 0,
		"duration_ms", 0,
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

// retryOnConflict runs op up to conflictAttempts times while it fails with a deadlock
// or serialization failure. Ingest locks rows in one global order, so ingest batches
// never deadlock each other, but other writers do not follow it: engine run
// termination and project deletion can still close a cycle with a batch, and Postgres
// then aborts one transaction. Each retry waits a short jittered backoff, so the
// winner can commit first.
func retryOnConflict(ctx context.Context, projectID uuid.UUID, batchKey string, op func() error) error {
	for attempt := 1; ; attempt++ {
		err := op()
		code := conflictSQLState(err)
		if code == "" {
			return err
		}
		if attempt == conflictAttempts {
			return fmt.Errorf("%w: %d attempts, last SQLSTATE %s: %w", ErrWriteConflict, attempt, code, err)
		}

		slog.Warn("ingest_conflict_retry",
			"batch_key", batchKey,
			"project_id", projectID,
			"attempt", attempt,
			"sqlstate", code,
			"err", err,
		)
		backoff := time.NewTimer(time.Duration(attempt)*conflictBackoff + rand.N(conflictBackoff))
		select {
		case <-ctx.Done():
			backoff.Stop()
			return fmt.Errorf("ingest retry after SQLSTATE %s: %w", code, ctx.Err())
		case <-backoff.C:
		}
	}
}

// conflictSQLState returns the SQLSTATE of the deadlock or serialization failure in
// err's chain, or "" when err is not one.
func conflictSQLState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == sqlStateDeadlock || pgErr.Code == sqlStateSerializationFailure) {
		return pgErr.Code
	}
	return ""
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
		slog.Info("batch_duplicate",
			"batch_id", claim.Batch.ID,
			"batch_key", claim.Batch.BatchKey,
			"project_id", projectID,
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

	slog.Info("batch_accepted",
		"batch_id", claim.Batch.ID,
		"batch_key", req.BatchKey,
		"project_id", projectID,
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
		if _, err := s.riverClient.InsertTx(ctx, tx, jobargs.TraceRollupArgs{TraceID: traceID}, nil); err != nil {
			return err
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

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return data, nil
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

// IsValidationError reports whether err, or any error it wraps, is a
// *ValidationError. Handlers use it to choose 400 over 500.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
