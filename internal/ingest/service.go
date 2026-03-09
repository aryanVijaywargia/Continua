package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
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
	"github.com/continua-ai/continua/pkg/truncation"
)

const (
	batchStatusAcceptedLegacy = "accepted"
	batchStatusCompleted      = "completed"
	batchStatusFailed         = "failed"
	batchStatusProcessing     = "processing"
	batchStatusQueued         = "queued"
)

const defaultDependencyRetryWindow = 15 * time.Minute

var errUnknownBatchStatus = errors.New("unknown batch status")

// Service handles synchronous and asynchronous batch ingestion flows.
type Service struct {
	store            *store.Store
	riverClient      *river.Client[pgx.Tx]
	processor        *Processor
	trueAsyncDefault bool
}

// Processor owns the shared trace/span/event write path used by both sync and async ingest.
type Processor struct {
	store                 *store.Store
	dependencyRetryWindow time.Duration
}

// ProcessedBatch is the shared processing result for sync and async ingestion.
type ProcessedBatch struct {
	TraceIDs      []uuid.UUID
	TraceCount    int32
	SpanCount     int32
	EventCount    int32
	AcceptedCount int32
	RejectedCount int32
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

// NewProcessor creates a shared ingest processor from configuration.
func NewProcessor(s *store.Store, cfg *config.Config) *Processor {
	window := defaultDependencyRetryWindow
	if cfg != nil && cfg.Ingest.DependencyRetryWindow > 0 {
		window = cfg.Ingest.DependencyRetryWindow
	}

	return &Processor{
		store:                 s,
		dependencyRetryWindow: window,
	}
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

// DependencyRetryWindow returns the configured retry window for dependency-not-ready batches.
func (p *Processor) DependencyRetryWindow() time.Duration {
	return p.dependencyRetryWindow
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

	if err := tx.InsertBatchPayload(ctx, platform.InsertBatchPayloadParams{
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

	return batchStatusFromModel(batch)
}

// Validate performs request-shape validation that should fail fast at acceptance time.
func (p *Processor) Validate(req *IngestRequest) error {
	if req.BatchKey == "" {
		return &ValidationError{Errors: []string{"batch_key is required"}}
	}

	validationErrors := p.validateBatch(req)
	if len(validationErrors) > 0 {
		return &ValidationError{Errors: validationErrors}
	}

	return nil
}

// ProcessBatch executes the shared trace/span/event write path inside the caller's transaction.
func (p *Processor) ProcessBatch(
	ctx context.Context,
	tx *store.Tx,
	projectID uuid.UUID,
	req *IngestRequest,
) (*ProcessedBatch, error) {
	traceMap := make(map[string]uuid.UUID)
	affectedTraceIDs := make(map[uuid.UUID]struct{})

	for i := range req.Traces {
		trace := &req.Traces[i]
		internalID, err := p.upsertTrace(ctx, tx, projectID, trace)
		if err != nil {
			return nil, fmt.Errorf("failed to upsert trace %s: %w", trace.TraceID, err)
		}
		traceMap[trace.TraceID] = internalID
		affectedTraceIDs[internalID] = struct{}{}
	}

	for i := range req.Spans {
		span := &req.Spans[i]
		traceUUID, err := p.resolveTraceUUID(ctx, tx, projectID, traceMap, span.TraceID)
		if err != nil {
			return nil, err
		}
		if err := p.upsertSpan(ctx, tx, projectID, traceUUID, span); err != nil {
			return nil, fmt.Errorf("failed to upsert span %s: %w", span.SpanID, err)
		}
		affectedTraceIDs[traceUUID] = struct{}{}
	}

	actualEventInserts := int32(0)
	for i := range req.Events {
		event := &req.Events[i]
		traceUUID, err := p.resolveTraceUUID(ctx, tx, projectID, traceMap, event.TraceID)
		if err != nil {
			return nil, err
		}
		inserted, err := p.insertEvent(ctx, tx, projectID, traceUUID, event)
		if err != nil {
			return nil, fmt.Errorf("failed to insert event for span %s: %w", event.SpanID, err)
		}
		if inserted {
			actualEventInserts++
		}
		affectedTraceIDs[traceUUID] = struct{}{}
	}

	traceIDs := make([]uuid.UUID, 0, len(affectedTraceIDs))
	for traceID := range affectedTraceIDs {
		traceIDs = append(traceIDs, traceID)
	}

	traceCount := int32(len(req.Traces))
	spanCount := int32(len(req.Spans))
	acceptedCount := traceCount + spanCount + actualEventInserts

	return &ProcessedBatch{
		TraceIDs:      traceIDs,
		TraceCount:    traceCount,
		SpanCount:     spanCount,
		EventCount:    actualEventInserts,
		AcceptedCount: acceptedCount,
		RejectedCount: 0,
	}, nil
}

func (p *Processor) resolveTraceUUID(
	ctx context.Context,
	tx *store.Tx,
	projectID uuid.UUID,
	traceMap map[string]uuid.UUID,
	traceID string,
) (uuid.UUID, error) {
	if traceUUID, ok := traceMap[traceID]; ok {
		return traceUUID, nil
	}

	traceUUID, err := tx.GetTraceUUID(ctx, projectID, traceID)
	if err != nil {
		if store.IsNotFound(err) {
			return uuid.Nil, &DependencyNotReadyError{
				Message: fmt.Sprintf("unknown trace reference: %s", traceID),
			}
		}
		return uuid.Nil, fmt.Errorf("failed to lookup trace %s: %w", traceID, err)
	}

	traceMap[traceID] = traceUUID
	return traceUUID, nil
}

func (p *Processor) validateBatch(req *IngestRequest) []string {
	var errs []string

	for i := range req.Traces {
		trace := &req.Traces[i]
		if trace.TraceID == "" {
			errs = append(errs, "trace missing required field: trace_id")
		}
	}

	for i := range req.Spans {
		span := &req.Spans[i]
		if span.TraceID == "" {
			errs = append(errs, fmt.Sprintf("span[%d] missing required field: trace_id", i))
		}
		if span.SpanID == "" {
			errs = append(errs, fmt.Sprintf("span[%d] missing required field: span_id", i))
		}
		if span.Name == "" {
			errs = append(errs, fmt.Sprintf("span[%d] missing required field: name", i))
		}
		if span.StartTime.IsZero() {
			errs = append(errs, fmt.Sprintf("span[%d] missing required field: start_time", i))
		}
		if span.TotalTokens != nil && span.PromptTokens == nil && span.CompletionTokens == nil {
			errs = append(errs, fmt.Sprintf(
				"span[%d] unsupported token format: total_tokens without prompt_tokens/completion_tokens",
				i,
			))
		}
	}

	for i := range req.Events {
		event := &req.Events[i]
		if event.TraceID == "" {
			errs = append(errs, fmt.Sprintf("event[%d] missing required field: trace_id", i))
		}
		if event.SpanID == "" {
			errs = append(errs, fmt.Sprintf("event[%d] missing required field: span_id", i))
		}
		if event.EventType != nil && !isValidIngestEventType(*event.EventType) {
			errs = append(errs, fmt.Sprintf("event[%d] invalid event_type: %s", i, *event.EventType))
		}
		if event.Level != nil && !isValidIngestEventLevel(*event.Level) {
			errs = append(errs, fmt.Sprintf("event[%d] invalid level: %s", i, *event.Level))
		}
	}

	return errs
}

func isValidIngestEventType(eventType string) bool {
	switch eventType {
	case "log", "error", "exception", "message", "metric", "custom":
		return true
	default:
		return false
	}
}

func isValidIngestEventLevel(level string) bool {
	switch level {
	case "debug", "info", "warning", "error":
		return true
	default:
		return false
	}
}

func (p *Processor) upsertTrace(ctx context.Context, tx *store.Tx, projectID uuid.UUID, input *TraceInput) (uuid.UUID, error) {
	var metadata []byte
	if input.Metadata != nil {
		metadata, _ = json.Marshal(input.Metadata)
	}

	inputData, _, _, _ := processPayload(input.Input, truncation.DefaultMaxBytes)
	outputData, _, _, _ := processPayload(input.Output, truncation.DefaultMaxBytes)

	var startTime, endTime pgtype.Timestamptz
	if input.StartTime != nil {
		startTime = pgtype.Timestamptz{Time: *input.StartTime, Valid: true}
	}
	if input.EndTime != nil {
		endTime = pgtype.Timestamptz{Time: *input.EndTime, Valid: true}
	}

	var sessionID pgtype.UUID
	if input.SessionID != nil && *input.SessionID != "" {
		session, err := tx.GetOrCreateSessionByExternalID(ctx, projectID, *input.SessionID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("resolve session: %w", err)
		}
		sessionID = pgtype.UUID{Bytes: session.ID, Valid: true}
	}

	status := defaultString(input.Status, "running")

	trace, err := tx.UpsertTrace(ctx, &platform.UpsertTraceParams{
		ProjectID:   projectID,
		SessionID:   sessionID,
		TraceID:     input.TraceID,
		Name:        input.Name,
		UserID:      input.UserID,
		Tags:        input.Tags,
		Environment: input.Environment,
		Release:     input.Release,
		Metadata:    metadata,
		Input:       inputData,
		Output:      outputData,
		Status:      status,
		StartTime:   startTime,
		EndTime:     endTime,
	})
	if err != nil {
		return uuid.Nil, err
	}

	return trace.ID, nil
}

func (p *Processor) upsertSpan(ctx context.Context, tx *store.Tx, projectID, traceUUID uuid.UUID, input *SpanInput) error {
	var metadata []byte
	if input.Metadata != nil {
		metadata, _ = json.Marshal(input.Metadata)
	}

	inputData, inputTruncated, inputOrigSize, inputTruncReason := processPayload(input.Input, truncation.DefaultMaxBytes)
	outputData, outputTruncated, outputOrigSize, outputTruncReason := processPayload(input.Output, truncation.DefaultMaxBytes)

	var endTime pgtype.Timestamptz
	if input.EndTime != nil {
		endTime = pgtype.Timestamptz{Time: *input.EndTime, Valid: true}
	}

	var totalCost pgtype.Numeric
	if input.TotalCost != nil {
		totalCost.Valid = true
		_ = totalCost.Scan(fmt.Sprintf("%f", *input.TotalCost))
	}

	spanType := defaultString(input.Type, "default")
	status := defaultString(input.Status, "running")
	level := defaultString(input.Level, "default")
	startTime := input.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}

	_, err := tx.UpsertSpan(ctx, &platform.UpsertSpanParams{
		ProjectID:               projectID,
		TraceID:                 traceUUID,
		SpanID:                  input.SpanID,
		ParentSpanID:            input.ParentSpanID,
		Name:                    input.Name,
		Type:                    spanType,
		Status:                  status,
		StatusMessage:           input.StatusMessage,
		Level:                   level,
		StartTime:               startTime,
		EndTime:                 endTime,
		Input:                   inputData,
		InputTruncated:          &inputTruncated,
		InputOriginalSizeBytes:  inputOrigSize,
		InputTruncationReason:   inputTruncReason,
		Output:                  outputData,
		OutputTruncated:         &outputTruncated,
		OutputOriginalSizeBytes: outputOrigSize,
		OutputTruncationReason:  outputTruncReason,
		Model:                   input.Model,
		Provider:                input.Provider,
		PromptTokens:            input.PromptTokens,
		CompletionTokens:        input.CompletionTokens,
		TotalTokens:             input.TotalTokens,
		TotalCost:               totalCost,
		Metadata:                metadata,
		Sequence:                input.Sequence,
		Depth:                   input.Depth,
	})

	return err
}

func (p *Processor) insertEvent(ctx context.Context, tx *store.Tx, projectID, traceUUID uuid.UUID, input *EventInput) (bool, error) {
	var payloadData []byte
	var truncated bool
	var origSize *int64
	var truncReason *string

	if input.Payload != nil {
		payloadData, truncated, origSize, truncReason = processPayload(input.Payload, truncation.DefaultMaxBytes)
	}

	var eventTs pgtype.Timestamptz
	if input.EventTs != nil {
		eventTs = pgtype.Timestamptz{Time: *input.EventTs, Valid: true}
	}

	eventType := defaultString(input.EventType, "log")
	level := defaultString(input.Level, "info")

	id, err := tx.InsertSpanEvent(ctx, &platform.InsertSpanEventParams{
		ProjectID:         projectID,
		TraceID:           traceUUID,
		SpanID:            input.SpanID,
		EventType:         eventType,
		Level:             level,
		EventTs:           eventTs,
		Sequence:          input.Sequence,
		Message:           input.Message,
		Payload:           payloadData,
		Truncated:         &truncated,
		OriginalSizeBytes: origSize,
		TruncationReason:  truncReason,
		IdempotencyKey:    input.IdempotencyKey,
	})
	if err != nil {
		return false, err
	}
	return id != uuid.Nil, nil
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

func batchStatusFromModel(batch platform.IngestBatch) (*BatchStatus, error) {
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
