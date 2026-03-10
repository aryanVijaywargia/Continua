package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/pkg/truncation"
)

const defaultDependencyRetryWindow = 15 * time.Minute

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

// DependencyRetryWindow returns the configured retry window for dependency-not-ready batches.
func (p *Processor) DependencyRetryWindow() time.Duration {
	return p.dependencyRetryWindow
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

// processPayload processes an input payload, ensuring it's valid JSON
// and truncating if necessary.
//
//nolint:unparam // maxBytes is configurable for testing and future flexibility
func processPayload(data any, maxBytes int) (jsonData []byte, truncated bool, origSize *int64, reason *string) {
	if data == nil {
		return nil, false, nil, nil
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		wrapped := map[string]string{"error": "failed to serialize", "type": "unknown"}
		jsonBytes, _ = json.Marshal(wrapped)
	}

	jsonBytes, _ = truncation.EnsureJSON(jsonBytes)

	if maxBytes <= 0 {
		maxBytes = truncation.DefaultMaxBytes
	}

	result := truncation.TruncateWithLimit(jsonBytes, maxBytes)

	if !result.Truncated {
		return result.Data, false, nil, nil
	}

	reasonStr := string(result.Reason)
	return result.Data, true, &result.OriginalSizeBytes, &reasonStr
}

// defaultString returns the value if non-nil, otherwise returns the default.
func defaultString(val *string, def string) string {
	if val == nil {
		return def
	}
	return *val
}
