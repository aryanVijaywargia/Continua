package domain

import (
	"time"

	"github.com/google/uuid"
)

// BatchStatus represents the processing status of an ingest batch.
type BatchStatus string

const (
	BatchStatusProcessing BatchStatus = "processing"
	BatchStatusCompleted  BatchStatus = "completed"
	BatchStatusFailed     BatchStatus = "failed"
)

// Batch represents an ingest batch for idempotency tracking.
type Batch struct {
	ID                    uuid.UUID
	ProjectID             uuid.UUID
	BatchKey              string
	Status                BatchStatus
	ServerReceivedAt      time.Time
	ProcessingCompletedAt *time.Time
	TraceCount            int32
	SpanCount             int32
	EventCount            int32
	AcceptedCount         int32
	RejectedCount         int32
	CreatedAt             time.Time
}

// BatchResult contains the result of processing a batch.
type BatchResult struct {
	BatchID       uuid.UUID
	BatchKey      string
	IsDuplicate   bool
	TraceCount    int32
	SpanCount     int32
	EventCount    int32
	AcceptedCount int32
	RejectedCount int32
	Errors        []string
}
