package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
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
	// Every row lock this batch takes is on a session or a trace, or is reached through
	// a trace it already holds: the span trigger bumps the parent trace's version, and
	// span, event, and rollup inserts all key on the trace. Locking sessions, then every
	// referenced trace, each sorted by external id, before any span or event write gives
	// all concurrent batches one global lock order, so they queue instead of deadlocking.
	sessionIDs, err := p.upsertSessions(ctx, tx, projectID, req.Traces)
	if err != nil {
		return nil, err
	}

	traceMap, err := p.lockTraces(ctx, tx, projectID, req, sessionIDs)
	if err != nil {
		return nil, err
	}

	for i := range req.Spans {
		span := &req.Spans[i]
		if err := p.upsertSpan(ctx, tx, projectID, traceMap[span.TraceID], span); err != nil {
			return nil, fmt.Errorf("failed to upsert span %s: %w", span.SpanID, err)
		}
	}

	actualEventInserts := int32(0)
	for i := range req.Events {
		event := &req.Events[i]
		inserted, err := p.insertEvent(ctx, tx, projectID, traceMap[event.TraceID], event)
		if err != nil {
			return nil, fmt.Errorf("failed to insert event for span %s: %w", event.SpanID, err)
		}
		if inserted {
			actualEventInserts++
		}
	}

	// Go randomizes map iteration, and callers enqueue a rollup per id inside this same
	// transaction, so an unsorted slice means two concurrent batches touching the same
	// traces can take the rollup insert locks in opposite orders and deadlock.
	traceIDs := slices.Collect(maps.Values(traceMap))
	slices.SortFunc(traceIDs, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

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

// upsertSessions materializes the session of every trace that names one and returns the
// session UUID for each trace index. Traces are visited stably sorted by session id, so
// session row locks follow one global order while each session still merges its traces'
// context in request order.
func (p *Processor) upsertSessions(
	ctx context.Context,
	tx *store.Tx,
	projectID uuid.UUID,
	traces []TraceInput,
) ([]pgtype.UUID, error) {
	order := make([]int, 0, len(traces))
	for i := range traces {
		if traces[i].SessionID != nil && *traces[i].SessionID != "" {
			order = append(order, i)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return strings.Compare(*traces[a].SessionID, *traces[b].SessionID)
	})

	sessionIDs := make([]pgtype.UUID, len(traces))
	for _, i := range order {
		trace := &traces[i]
		session, err := tx.UpsertSessionContext(ctx, projectID, *trace.SessionID, store.SessionContextParams{
			Name:     trace.SessionContext.Name,
			UserID:   trace.SessionContext.UserID,
			UserName: trace.SessionContext.UserName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to upsert trace %s: resolve session: %w", trace.TraceID, err)
		}
		sessionIDs[i] = pgtype.UUID{Bytes: session.ID, Valid: true}
	}
	return sessionIDs, nil
}

// lockTraces upserts or row-locks every trace the batch references, sorted by external
// trace id, and maps each external id to its internal UUID. A trace referenced only by
// spans or events must already exist; otherwise the batch is not ready yet.
func (p *Processor) lockTraces(
	ctx context.Context,
	tx *store.Tx,
	projectID uuid.UUID,
	req *IngestRequest,
	sessionIDs []pgtype.UUID,
) (map[string]uuid.UUID, error) {
	inputs := make(map[string][]int, len(req.Traces))
	externalIDs := make([]string, 0, len(req.Traces)+len(req.Spans)+len(req.Events))
	for i := range req.Traces {
		inputs[req.Traces[i].TraceID] = append(inputs[req.Traces[i].TraceID], i)
		externalIDs = append(externalIDs, req.Traces[i].TraceID)
	}
	for i := range req.Spans {
		externalIDs = append(externalIDs, req.Spans[i].TraceID)
	}
	for i := range req.Events {
		externalIDs = append(externalIDs, req.Events[i].TraceID)
	}
	slices.Sort(externalIDs)
	externalIDs = slices.Compact(externalIDs)

	traceMap := make(map[string]uuid.UUID, len(externalIDs))
	for _, traceID := range externalIDs {
		if indices := inputs[traceID]; len(indices) > 0 {
			for _, i := range indices {
				internalID, err := p.upsertTrace(ctx, tx, projectID, &req.Traces[i], sessionIDs[i])
				if err != nil {
					return nil, fmt.Errorf("failed to upsert trace %s: %w", traceID, err)
				}
				traceMap[traceID] = internalID
			}
			continue
		}

		internalID, err := tx.LockTraceUUID(ctx, projectID, traceID)
		if err != nil {
			if store.IsNotFound(err) {
				return nil, &DependencyNotReadyError{
					Message: fmt.Sprintf("unknown trace reference: %s", traceID),
				}
			}
			return nil, fmt.Errorf("failed to lookup trace %s: %w", traceID, err)
		}
		traceMap[traceID] = internalID
	}
	return traceMap, nil
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

		warnForMissingSemanticPayloadFields(event)
	}

	return errs
}

func isValidIngestEventType(eventType string) bool {
	if eventType == "" {
		return false
	}

	_, blocked := syntheticTimelineOnlyEventTypes[eventType]
	return !blocked
}

var syntheticTimelineOnlyEventTypes = map[string]struct{}{
	// keep in sync with synthetic timeline event types in contracts/openapi/openapi.yaml and internal/api/timeline.go
	"span_started":   {},
	"span_completed": {},
	"span_failed":    {},
}

func warnForMissingSemanticPayloadFields(event *EventInput) {
	eventType := defaultString(event.EventType, "log")

	switch eventType {
	case "state_change":
		if !hasSemanticStringField(event.Payload, "key") {
			log.Printf(
				"[WARN] ingest state_change event missing semantic payload field 'key' for trace %s span %s",
				event.TraceID,
				event.SpanID,
			)
		}
	case "decision":
		missingQuestion := !hasSemanticStringField(event.Payload, "question")
		missingChosen := !hasSemanticStringField(event.Payload, "chosen")
		if missingQuestion || missingChosen {
			log.Printf(
				"[WARN] ingest decision event missing semantic payload fields for trace %s span %s (question=%t chosen=%t)",
				event.TraceID,
				event.SpanID,
				missingQuestion,
				missingChosen,
			)
		}
	}
}

func hasSemanticStringField(payload map[string]any, field string) bool {
	if payload == nil {
		return false
	}

	value, ok := payload[field]
	if !ok {
		return false
	}

	text, ok := value.(string)
	return ok && text != ""
}

func isValidIngestEventLevel(level string) bool {
	switch level {
	case "debug", "info", "warning", "error":
		return true
	default:
		return false
	}
}

func (p *Processor) upsertTrace(
	ctx context.Context,
	tx *store.Tx,
	projectID uuid.UUID,
	input *TraceInput,
	sessionID pgtype.UUID,
) (uuid.UUID, error) {
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

	// A trace with no status starts 'running', but the upsert has to tell that default
	// apart from an explicit 'running' so it can hold a settled trace's status against
	// a status-less update (every OTLP export) without swallowing a real one.
	status := defaultString(input.Status, "running")

	trace, err := tx.UpsertTrace(ctx, &platform.UpsertTraceParams{
		ProjectID:      projectID,
		SessionID:      sessionID,
		TraceID:        input.TraceID,
		Name:           input.Name,
		UserID:         input.UserID,
		Tags:           input.Tags,
		Environment:    input.Environment,
		Release:        input.Release,
		Metadata:       metadata,
		Input:          inputData,
		Output:         outputData,
		Status:         status,
		StatusSupplied: input.Status != nil,
		StartTime:      startTime,
		EndTime:        endTime,
	})
	if err != nil {
		return uuid.Nil, err
	}

	return trace.ID, nil
}

func (p *Processor) upsertSpan(ctx context.Context, tx *store.Tx, projectID, traceUUID uuid.UUID, input *SpanInput) error {
	// Metadata goes through the same bounded path as input/output. It used to be
	// marshaled raw with the error discarded, so an unmarshalable value (an OTLP NaN
	// attribute) silently nulled the whole column and an adapter that copies a shared
	// block onto every span could write unbounded jsonb in one transaction.
	var metadata []byte
	if input.Metadata != nil {
		metadata, _, _, _ = processPayload(input.Metadata, truncation.DefaultMaxBytes)
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
	var eventTs pgtype.Timestamptz
	if input.EventTs != nil {
		eventTs = pgtype.Timestamptz{Time: *input.EventTs, Valid: true}
	}

	eventType := defaultString(input.EventType, "log")
	level := defaultString(input.Level, "info")
	payloadData, truncated, origSize, truncReason := processEventPayload(eventType, input, truncation.DefaultMaxBytes)

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

	jsonBytes := marshalPayloadData(data)

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

func marshalPayloadData(data any) []byte {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		wrapped := map[string]string{"error": "failed to serialize", "type": "unknown"}
		jsonBytes, _ = json.Marshal(wrapped)
	}

	jsonBytes, _ = truncation.EnsureJSON(jsonBytes)
	return jsonBytes
}

// defaultString returns the value if non-nil, otherwise returns the default.
func defaultString(val *string, def string) string {
	if val == nil {
		return def
	}
	return *val
}
