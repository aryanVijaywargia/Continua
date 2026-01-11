package domain

import (
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of span event.
type EventType string

const (
	EventTypeLog       EventType = "log"
	EventTypeError     EventType = "error"
	EventTypeException EventType = "exception"
	EventTypeMessage   EventType = "message"
	EventTypeMetric    EventType = "metric"
	EventTypeCustom    EventType = "custom"
)

// EventLevel represents the severity level of an event.
type EventLevel string

const (
	EventLevelDebug   EventLevel = "debug"
	EventLevelInfo    EventLevel = "info"
	EventLevelWarning EventLevel = "warning"
	EventLevelError   EventLevel = "error"
)

// SpanEvent represents an event that occurred during a span's execution.
type SpanEvent struct {
	// Internal UUID (primary key)
	ID uuid.UUID

	// Project this event belongs to
	ProjectID uuid.UUID

	// Internal UUID of the parent trace
	TraceUUID uuid.UUID

	// External span ID this event belongs to
	// Note: No FK constraint - spans may arrive after events
	SpanID string

	// Event classification
	EventType EventType
	Level     EventLevel

	// Timing
	EventTs          *time.Time
	ServerIngestedAt time.Time

	// Ordering hint
	Sequence *int32

	// Event content
	Message *string
	Payload []byte

	// Truncation info
	Truncated         bool
	OriginalSizeBytes *int64
	TruncationReason  *string

	// Idempotency key for deduplication
	IdempotencyKey *string

	// Timestamps
	CreatedAt time.Time
}

// SpanEventInput represents input data for creating a span event.
type SpanEventInput struct {
	TraceID        string
	SpanID         string
	EventType      EventType
	Level          EventLevel
	EventTs        *time.Time
	Sequence       *int32
	Message        *string
	Payload        []byte
	IdempotencyKey *string
}
