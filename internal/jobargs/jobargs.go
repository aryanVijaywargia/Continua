package jobargs

import (
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const (
	QueueIngest      = "ingest"
	QueueRollup      = "rollup"
	QueueMaintenance = "maintenance"
)

var activeUniqueStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateScheduled,
	rivertype.JobStateRetryable,
}

// ActiveUniqueStates returns the River job states used for active uniqueness constraints.
func ActiveUniqueStates() []rivertype.JobState {
	return append([]rivertype.JobState(nil), activeUniqueStates...)
}

// IngestBatchArgs contains the arguments for an async ingest job.
type IngestBatchArgs struct {
	BatchID uuid.UUID `json:"batch_id"`
}

// Kind returns the River job kind.
func (IngestBatchArgs) Kind() string {
	return "ingest_batch"
}

// InsertOpts routes async ingest jobs to the ingest queue.
func (IngestBatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueIngest,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByState: ActiveUniqueStates(),
		},
	}
}

// TraceRollupArgs contains the arguments for a trace rollup job.
type TraceRollupArgs struct {
	TraceID uuid.UUID `json:"trace_id"`
}

// Kind returns the River job kind.
func (TraceRollupArgs) Kind() string {
	return "trace_rollup"
}

// InsertOpts routes trace rollups to the rollup queue with active-state uniqueness.
func (TraceRollupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueRollup,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByState: ActiveUniqueStates(),
		},
	}
}

// CleanupArgs contains the arguments for the payload cleanup job.
type CleanupArgs struct{}

// Kind returns the River job kind.
func (CleanupArgs) Kind() string {
	return "ingest_payload_cleanup"
}

// InsertOpts routes cleanup jobs to the maintenance queue.
func (CleanupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
	}
}

// CleanupInterval is the cadence for payload cleanup jobs.
const CleanupInterval = 24 * time.Hour

// RetentionArgs contains the arguments for the engine retention maintenance job.
type RetentionArgs struct{}

// Kind returns the River job kind.
func (RetentionArgs) Kind() string {
	return "engine_retention_maintenance"
}

// InsertOpts routes retention jobs to the maintenance queue with active-state uniqueness.
func (RetentionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByState: ActiveUniqueStates(),
		},
	}
}

// RetentionInterval is the cadence for engine retention jobs.
const RetentionInterval = 24 * time.Hour
