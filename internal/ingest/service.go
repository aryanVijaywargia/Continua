package ingest

import (
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
	"github.com/continua-ai/continua/internal/jobs"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/pkg/truncation"
)

// Service handles batch ingestion of traces, spans, and events.
type Service struct {
	store       *store.Store
	riverClient *river.Client[pgx.Tx]
}

// NewService creates a new ingest service.
func NewService(s *store.Store, riverClient *river.Client[pgx.Tx]) *Service {
	return &Service{store: s, riverClient: riverClient}
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

// Ingest processes a batch of traces, spans, and events.
// It is idempotent - duplicate batch keys return success without re-processing.
// Per spec v1: rejects entire batch on any validation error (no partial success).
func (s *Service) Ingest(ctx context.Context, projectID uuid.UUID, req *IngestRequest) (*IngestResponse, error) {
	if req.BatchKey == "" {
		return nil, &ValidationError{Errors: []string{"batch_key is required"}}
	}

	// Phase 1: Validate the entire batch BEFORE any database operations
	validationErrors := s.validateBatch(req)
	if len(validationErrors) > 0 {
		return nil, &ValidationError{Errors: validationErrors}
	}

	// Start transaction
	tx, err := s.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Claim the batch (idempotency check) - FIRST operation in transaction
	batchID, err := tx.ClaimBatch(ctx, projectID, req.BatchKey)
	if errors.Is(err, store.ErrDuplicateBatch) {
		// Duplicate batch - return success without processing
		return &IngestResponse{
			Status:   string(IngestStatusDuplicate),
			BatchKey: req.BatchKey,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim batch: %w", err)
	}

	// Phase 2: Process all items (now that batch is claimed)
	// Build trace map: external trace_id -> internal UUID
	traceMap := make(map[string]uuid.UUID)

	// Process traces first to build the external->internal ID map
	for i := range req.Traces {
		trace := &req.Traces[i]
		internalID, err := s.upsertTrace(ctx, tx, projectID, trace)
		if err != nil {
			// Transaction will be rolled back, batch not committed
			return nil, fmt.Errorf("failed to upsert trace %s: %w", trace.TraceID, err)
		}
		traceMap[trace.TraceID] = internalID
	}

	// Process spans - resolve trace UUID from map or DB
	for i := range req.Spans {
		span := &req.Spans[i]
		traceUUID, ok := traceMap[span.TraceID]
		if !ok {
			// Try to find existing trace in DB
			traceUUID, err = tx.GetTraceUUID(ctx, projectID, span.TraceID)
			if err != nil {
				if store.IsNotFound(err) {
					// Per spec: unknown trace reference returns 400 ValidationError
					return nil, &ValidationError{Errors: []string{
						fmt.Sprintf("span %s references unknown trace %s", span.SpanID, span.TraceID),
					}}
				}
				return nil, fmt.Errorf("failed to lookup trace for span %s: %w", span.SpanID, err)
			}
			traceMap[span.TraceID] = traceUUID
		}

		if err := s.upsertSpan(ctx, tx, projectID, traceUUID, span); err != nil {
			return nil, fmt.Errorf("failed to upsert span %s: %w", span.SpanID, err)
		}
	}

	// Process events - track actual inserts vs duplicates
	actualEventInserts := int32(0)
	for i := range req.Events {
		event := &req.Events[i]
		traceUUID, ok := traceMap[event.TraceID]
		if !ok {
			traceUUID, err = tx.GetTraceUUID(ctx, projectID, event.TraceID)
			if err != nil {
				if store.IsNotFound(err) {
					// Per spec: unknown trace reference returns 400 ValidationError
					return nil, &ValidationError{Errors: []string{
						fmt.Sprintf("event for span %s references unknown trace %s", event.SpanID, event.TraceID),
					}}
				}
				return nil, fmt.Errorf("failed to lookup trace for event: %w", err)
			}
			traceMap[event.TraceID] = traceUUID
		}

		inserted, err := s.insertEvent(ctx, tx, projectID, traceUUID, event)
		if err != nil {
			return nil, fmt.Errorf("failed to insert event for span %s: %w", event.SpanID, err)
		}
		if inserted {
			actualEventInserts++
		}
	}

	// Enqueue rollup jobs for all affected traces (async via River)
	// Jobs are enqueued in the same transaction, so they only become visible after commit
	for _, traceUUID := range traceMap {
		inserted, err := jobs.EnqueueRollupInTx(ctx, s.riverClient, tx.Tx(), traceUUID)
		if err != nil {
			log.Printf("Warning: failed to enqueue rollup job for trace %s: %v", traceUUID, err)
			// Continue - rollups will be computed by the next successful enqueue
			continue
		}
		_ = inserted // Duplicate insert is expected for coalescing.
	}

	// Update batch status to "accepted" (spec-compliant vocabulary)
	// event_count reflects actual inserts, not submitted count
	// Per spec: "The insert count reflects 0 new events for that key" (idempotency)
	traceCount := int32(len(req.Traces))
	spanCount := int32(len(req.Spans))
	eventCount := actualEventInserts // actual inserts, not len(req.Events)
	acceptedCount := traceCount + spanCount + actualEventInserts
	// Duplicate events are NOT counted as rejected - they're silently ignored
	rejectedCount := int32(0)

	err = tx.UpdateBatchStatus(ctx, platform.UpdateBatchStatusParams{
		ID:            batchID,
		Status:        "accepted", // Spec: use "accepted" not "completed"
		TraceCount:    &traceCount,
		SpanCount:     &spanCount,
		EventCount:    &eventCount,
		AcceptedCount: &acceptedCount,
		RejectedCount: &rejectedCount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update batch status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &IngestResponse{
		Status:        string(IngestStatusOK),
		BatchKey:      req.BatchKey,
		TraceCount:    traceCount,
		SpanCount:     spanCount,
		EventCount:    eventCount,
		AcceptedCount: acceptedCount,
		RejectedCount: rejectedCount,
	}, nil
}

// validateBatch performs upfront validation of the entire batch.
// Returns validation errors if any items are invalid.
// Per spec v1: entire batch is rejected on any validation error.
func (s *Service) validateBatch(req *IngestRequest) []string {
	var errs []string

	// Track which trace IDs are provided in this batch
	providedTraceIDs := make(map[string]bool)
	for i := range req.Traces {
		trace := &req.Traces[i]
		if trace.TraceID == "" {
			errs = append(errs, "trace missing required field: trace_id")
			continue
		}
		providedTraceIDs[trace.TraceID] = true
	}

	// Validate spans
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
		// Note: we don't validate trace_id references here because the trace
		// might exist in the database from a previous batch. This will be
		// checked during processing. However, if the span references a trace
		// that doesn't exist in this batch AND doesn't exist in DB, processing
		// will fail and the entire batch will be rolled back.
	}

	// Validate events
	for i := range req.Events {
		event := &req.Events[i]
		if event.TraceID == "" {
			errs = append(errs, fmt.Sprintf("event[%d] missing required field: trace_id", i))
		}
		if event.SpanID == "" {
			errs = append(errs, fmt.Sprintf("event[%d] missing required field: span_id", i))
		}
	}

	return errs
}

func (s *Service) upsertTrace(ctx context.Context, tx *store.Tx, projectID uuid.UUID, input *TraceInput) (uuid.UUID, error) {
	// Process metadata
	var metadata []byte
	if input.Metadata != nil {
		metadata, _ = json.Marshal(input.Metadata)
	}

	// Process input/output with truncation
	inputData, _, _, _ := processPayload(input.Input, truncation.DefaultMaxBytes)
	outputData, _, _, _ := processPayload(input.Output, truncation.DefaultMaxBytes)

	// Convert times to pgtype
	var startTime, endTime pgtype.Timestamptz
	if input.StartTime != nil {
		startTime = pgtype.Timestamptz{Time: *input.StartTime, Valid: true}
	}
	if input.EndTime != nil {
		endTime = pgtype.Timestamptz{Time: *input.EndTime, Valid: true}
	}

	// Convert session ID
	var sessionID pgtype.UUID
	if input.SessionID != nil {
		parsed, err := uuid.Parse(*input.SessionID)
		if err == nil {
			sessionID = pgtype.UUID{Bytes: parsed, Valid: true}
		}
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

func (s *Service) upsertSpan(ctx context.Context, tx *store.Tx, projectID, traceUUID uuid.UUID, input *SpanInput) error {
	// Process metadata
	var metadata []byte
	if input.Metadata != nil {
		metadata, _ = json.Marshal(input.Metadata)
	}

	// Process input/output with truncation
	inputData, inputTruncated, inputOrigSize, inputTruncReason := processPayload(input.Input, truncation.DefaultMaxBytes)
	outputData, outputTruncated, outputOrigSize, outputTruncReason := processPayload(input.Output, truncation.DefaultMaxBytes)

	// Convert end time
	var endTime pgtype.Timestamptz
	if input.EndTime != nil {
		endTime = pgtype.Timestamptz{Time: *input.EndTime, Valid: true}
	}

	// Convert cost to numeric
	var totalCost pgtype.Numeric
	if input.TotalCost != nil {
		totalCost.Valid = true
		_ = totalCost.Scan(fmt.Sprintf("%f", *input.TotalCost))
	}

	spanType := defaultString(input.Type, "default")
	status := defaultString(input.Status, "running")
	level := defaultString(input.Level, "default")

	// Use provided start time (required field, validated earlier)
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

func (s *Service) insertEvent(ctx context.Context, tx *store.Tx, projectID, traceUUID uuid.UUID, input *EventInput) (bool, error) {
	// Process payload with truncation
	var payloadData []byte
	var truncated bool
	var origSize *int64
	var truncReason *string

	if input.Payload != nil {
		payloadData, truncated, origSize, truncReason = processPayload(input.Payload, truncation.DefaultMaxBytes)
	}

	// Convert event time
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
	// id == uuid.Nil means it was a duplicate (ON CONFLICT DO NOTHING)
	return id != uuid.Nil, nil
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
