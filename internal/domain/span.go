package domain

import (
	"time"

	"github.com/google/uuid"
)

// SpanStatus represents the current status of a span.
type SpanStatus string

const (
	SpanStatusRunning   SpanStatus = "running"
	SpanStatusCompleted SpanStatus = "completed"
	SpanStatusFailed    SpanStatus = "failed"
)

// SpanType represents the type/category of a span.
type SpanType string

const (
	SpanTypeDefault    SpanType = "default"
	SpanTypeLLM        SpanType = "llm"
	SpanTypeTool       SpanType = "tool"
	SpanTypeAgent      SpanType = "agent"
	SpanTypeChain      SpanType = "chain"
	SpanTypeRetrieval  SpanType = "retrieval"
	SpanTypeEmbedding  SpanType = "embedding"
	SpanTypeGeneration SpanType = "generation"
)

// SpanLevel represents the logging level of a span.
type SpanLevel string

const (
	SpanLevelDebug   SpanLevel = "debug"
	SpanLevelDefault SpanLevel = "default"
	SpanLevelWarning SpanLevel = "warning"
	SpanLevelError   SpanLevel = "error"
)

// Span represents a unit of work within a trace.
type Span struct {
	// Internal UUID (primary key)
	ID uuid.UUID

	// Project this span belongs to
	ProjectID uuid.UUID

	// Internal UUID of the parent trace
	TraceUUID uuid.UUID

	// External span ID provided by the client
	SpanID string

	// External parent span ID (nil for root spans)
	ParentSpanID *string

	// Human-readable name
	Name string

	// Type of span
	Type SpanType

	// Current status
	Status        SpanStatus
	StatusMessage *string

	// Logging level
	Level SpanLevel

	// Timing
	StartTime        time.Time
	EndTime          *time.Time
	ServerReceivedAt time.Time
	DurationMs       *int64

	// Input/Output data (JSON)
	Input                   []byte
	InputTruncated          bool
	InputOriginalSizeBytes  *int64
	InputTruncationReason   *string
	Output                  []byte
	OutputTruncated         bool
	OutputOriginalSizeBytes *int64
	OutputTruncationReason  *string

	// LLM-specific fields
	Thinking          *string
	ThinkingTruncated bool
	Model             *string
	Provider          *string
	PromptTokens      *int64
	CompletionTokens  *int64
	TotalTokens       *int64
	TotalCost         *float64

	// Additional metadata (JSON)
	Metadata []byte

	// Ordering hints
	Sequence *int32
	Depth    *int32

	// Version for optimistic locking
	Version int32

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SpanInput represents input data for creating or updating a span.
type SpanInput struct {
	TraceID          string
	SpanID           string
	ParentSpanID     *string
	Name             string
	Type             SpanType
	Status           SpanStatus
	StatusMessage    *string
	Level            SpanLevel
	StartTime        time.Time
	EndTime          *time.Time
	Input            []byte
	Output           []byte
	Model            *string
	Provider         *string
	PromptTokens     *int64
	CompletionTokens *int64
	TotalTokens      *int64
	TotalCost        *float64
	Metadata         []byte
	Sequence         *int32
	Depth            *int32
}
