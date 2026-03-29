package store

import "strings"

// TraceStatusBucket is the normalized status bucket shared by API/status summaries.
type TraceStatusBucket string

const (
	TraceStatusBucketRunning   TraceStatusBucket = "running"
	TraceStatusBucketCompleted TraceStatusBucket = "completed"
	TraceStatusBucketFailed    TraceStatusBucket = "failed"
)

// NormalizeTraceStatus buckets raw trace statuses into the user-facing lifecycle groups.
func NormalizeTraceStatus(status string) TraceStatusBucket {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "ok":
		return TraceStatusBucketCompleted
	case "failed", "error", "cancelled":
		return TraceStatusBucketFailed
	default:
		return TraceStatusBucketRunning
	}
}
