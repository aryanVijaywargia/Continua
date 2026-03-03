package domain

import (
	"time"

	"github.com/google/uuid"
)

// TraceStatus represents the current status of a trace.
type TraceStatus string

const (
	TraceStatusRunning   TraceStatus = "running"
	TraceStatusCompleted TraceStatus = "completed"
	TraceStatusFailed    TraceStatus = "failed"
)

// Trace represents a top-level execution trace.
type Trace struct {
	// Internal UUID (primary key)
	ID uuid.UUID

	// Project this trace belongs to
	ProjectID uuid.UUID

	// Optional session grouping
	SessionID *uuid.UUID

	// External trace ID provided by the client
	TraceID string

	// Human-readable name for the trace
	Name *string

	// User identifier associated with this trace
	UserID *string

	// Tags for filtering and categorization
	Tags []string

	// Deployment environment (e.g., "production", "staging")
	Environment *string

	// Release version
	Release *string

	// Additional metadata as JSON
	Metadata []byte

	// Input data for the trace (JSON)
	Input []byte

	// Output data from the trace (JSON)
	Output []byte

	// Current status
	Status TraceStatus

	// Timing
	StartTime        *time.Time
	EndTime          *time.Time
	ServerReceivedAt time.Time
	DurationMs       *int64

	// Rollup metrics
	TotalSpans     *int32
	TotalTokensIn  *int64
	TotalTokensOut *int64
	TotalCost      *float64
	ErrorCount     *int32

	// Version for optimistic locking
	Version int32

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TraceInput represents input data for creating or updating a trace.
type TraceInput struct {
	TraceID     string
	SessionID   *uuid.UUID
	Name        *string
	UserID      *string
	Tags        []string
	Environment *string
	Release     *string
	Metadata    []byte
	Input       []byte
	Output      []byte
	Status      TraceStatus
	StartTime   *time.Time
	EndTime     *time.Time
}
