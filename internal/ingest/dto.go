package ingest

import (
	"time"

	"github.com/google/uuid"
)

// IngestRequest represents the request body for the ingest endpoint.
//
//nolint:revive // IngestRequest is clearer than Request in import context
type IngestRequest struct {
	BatchKey string       `json:"batch_key"`
	Traces   []TraceInput `json:"traces,omitempty"`
	Spans    []SpanInput  `json:"spans,omitempty"`
	Events   []EventInput `json:"events,omitempty"`
}

// TraceInput represents a trace in the ingest request.
type TraceInput struct {
	TraceID     string         `json:"trace_id"`
	SessionID   *string        `json:"session_id,omitempty"`
	Name        *string        `json:"name,omitempty"`
	UserID      *string        `json:"user_id,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Environment *string        `json:"environment,omitempty"`
	Release     *string        `json:"release,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Input       any            `json:"input,omitempty"`
	Output      any            `json:"output,omitempty"`
	Status      *string        `json:"status,omitempty"`
	StartTime   *time.Time     `json:"start_time,omitempty"`
	EndTime     *time.Time     `json:"end_time,omitempty"`
}

// SpanInput represents a span in the ingest request.
type SpanInput struct {
	TraceID          string         `json:"trace_id"`
	SpanID           string         `json:"span_id"`
	ParentSpanID     *string        `json:"parent_span_id,omitempty"`
	Name             string         `json:"name"`
	Type             *string        `json:"type,omitempty"`
	Status           *string        `json:"status,omitempty"`
	StatusMessage    *string        `json:"status_message,omitempty"`
	Level            *string        `json:"level,omitempty"`
	StartTime        time.Time      `json:"start_time"`
	EndTime          *time.Time     `json:"end_time,omitempty"`
	Input            any            `json:"input,omitempty"`
	Output           any            `json:"output,omitempty"`
	Model            *string        `json:"model,omitempty"`
	Provider         *string        `json:"provider,omitempty"`
	PromptTokens     *int64         `json:"prompt_tokens,omitempty"`
	CompletionTokens *int64         `json:"completion_tokens,omitempty"`
	TotalTokens      *int64         `json:"total_tokens,omitempty"`
	TotalCost        *float64       `json:"total_cost,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Sequence         *int32         `json:"sequence,omitempty"`
	Depth            *int32         `json:"depth,omitempty"`
}

// EventInput represents a span event in the ingest request.
type EventInput struct {
	TraceID        string         `json:"trace_id"`
	SpanID         string         `json:"span_id"`
	EventType      *string        `json:"event_type,omitempty"`
	Level          *string        `json:"level,omitempty"`
	EventTs        *time.Time     `json:"event_ts,omitempty"`
	Sequence       *int32         `json:"sequence,omitempty"`
	Message        *string        `json:"message,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	IdempotencyKey *string        `json:"idempotency_key,omitempty"`
}

// IngestResponse represents the response from the ingest endpoint.
//
//nolint:revive // IngestResponse is clearer than Response in import context
type IngestResponse struct {
	Status        string    `json:"status"`
	BatchKey      string    `json:"batch_key"`
	BatchID       uuid.UUID `json:"batch_id,omitempty"`
	TraceCount    int32     `json:"trace_count,omitempty"`
	SpanCount     int32     `json:"span_count,omitempty"`
	EventCount    int32     `json:"event_count,omitempty"`
	AcceptedCount int32     `json:"accepted_count,omitempty"`
	RejectedCount int32     `json:"rejected_count,omitempty"`
	Errors        []string  `json:"errors,omitempty"`
}

// IngestStatus represents the processing status.
//
//nolint:revive // IngestStatus is clearer than Status in import context
type IngestStatus string

const (
	IngestStatusOK        IngestStatus = "ok"
	IngestStatusAccepted  IngestStatus = "accepted"
	IngestStatusDuplicate IngestStatus = "duplicate"
	IngestStatusFailed    IngestStatus = "failed"
)

// BatchStatus is the public-facing view returned for batch status polling.
type BatchStatus struct {
	BatchID               uuid.UUID  `json:"batch_id"`
	BatchKey              string     `json:"batch_key"`
	Status                string     `json:"status"`
	AttemptCount          int32      `json:"attempt_count"`
	ServerReceivedAt      time.Time  `json:"server_received_at"`
	ProcessingStartedAt   *time.Time `json:"processing_started_at,omitempty"`
	ProcessingCompletedAt *time.Time `json:"processing_completed_at,omitempty"`
	TraceCount            *int32     `json:"trace_count,omitempty"`
	SpanCount             *int32     `json:"span_count,omitempty"`
	EventCount            *int32     `json:"event_count,omitempty"`
	AcceptedCount         *int32     `json:"accepted_count,omitempty"`
	RejectedCount         *int32     `json:"rejected_count,omitempty"`
	LastErrorCode         *string    `json:"last_error_code,omitempty"`
	LastErrorMessage      *string    `json:"last_error_message,omitempty"`
}
