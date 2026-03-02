package jobs_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/jobs"
)

func TestTraceRollupArgs_InsertOpts_UsesActiveUniqueStates(t *testing.T) {
	opts := jobs.TraceRollupArgs{}.InsertOpts()

	require.True(t, opts.UniqueOpts.ByArgs)
	assert.ElementsMatch(t, []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateRunning,
		rivertype.JobStateScheduled,
		rivertype.JobStateRetryable,
	}, opts.UniqueOpts.ByState)

	assert.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateCompleted)
	assert.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateCancelled)
	assert.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateDiscarded)
}

func TestEnqueueRollup_NilClientReturnsError(t *testing.T) {
	inserted, err := jobs.EnqueueRollup(context.Background(), nil, uuid.New())
	require.Error(t, err)
	assert.False(t, inserted)
}
