package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/jobargs"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestRetentionArgs_InsertOpts_UsesMaintenanceQueueAndActiveUniqueStates(t *testing.T) {
	opts := jobargs.RetentionArgs{}.InsertOpts()

	assert.Equal(t, jobargs.QueueMaintenance, opts.Queue)
	require.True(t, opts.UniqueOpts.ByArgs)
	assert.ElementsMatch(t, jobargs.ActiveUniqueStates(), opts.UniqueOpts.ByState)
	assert.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateCompleted)
	assert.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateCancelled)
	assert.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateDiscarded)
}

func TestRetentionInterval_IsDaily(t *testing.T) {
	assert.Equal(t, 24*time.Hour, jobargs.RetentionInterval)
}

func TestNewClient_WithoutRetentionConfig_DoesNotRegisterRetentionWorker(t *testing.T) {
	client := newRetentionModuleTestClient(t, nil)

	_, err := client.Insert(context.Background(), jobargs.RetentionArgs{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), jobargs.RetentionArgs{}.Kind())
}

func TestNewClient_WithRetentionConfig_RegistersRetentionWorker(t *testing.T) {
	client := newRetentionModuleTestClient(t, &config.Config{
		Engine: config.EngineConfig{
			ProjectionRetentionAfter: 24 * time.Hour,
		},
		Jobs: config.JobsConfig{
			IngestWorkers:      1,
			RollupWorkers:      1,
			MaintenanceWorkers: 1,
			DefaultWorkers:     1,
		},
	})

	inserted, err := client.Insert(context.Background(), jobargs.RetentionArgs{}, nil)
	require.NoError(t, err)
	assert.Equal(t, jobargs.RetentionArgs{}.Kind(), inserted.Job.Kind)
	assert.Equal(t, jobargs.QueueMaintenance, inserted.Job.Queue)
	assert.ElementsMatch(t, jobargs.ActiveUniqueStates(), inserted.Job.UniqueStates)
}

func TestNewClient_WithRetentionConfig_DoesNotRunRetentionOnStart(t *testing.T) {
	client := newRetentionModuleTestClient(t, &config.Config{
		Engine: config.EngineConfig{
			ProjectionRetentionAfter: 24 * time.Hour,
		},
		Jobs: config.JobsConfig{
			IngestWorkers:      1,
			RollupWorkers:      1,
			MaintenanceWorkers: 1,
			DefaultWorkers:     1,
		},
	})

	ctx := context.Background()
	require.NoError(t, client.Start(ctx))
	t.Cleanup(func() {
		_ = client.Stop(context.Background())
	})

	waitForJobKindCount(t, client, jobargs.CleanupArgs{}.Kind(), 1)
	assertJobKindCount(t, client, jobargs.RetentionArgs{}.Kind(), 0)
}

// River's 100ms default fetch cooldown capped a 10-worker queue at 100 jobs/s
// no matter how cheap each job was (#316).
func TestNewClient_DefaultsDrainRollupBurstFasterThanFetchCadence(t *testing.T) {
	const (
		jobCount       = 500
		rollupWorkers  = 10
		riverCooldown  = 100 * time.Millisecond
		cadenceMinimum = (jobCount/rollupWorkers - 1) * riverCooldown
	)
	client := newRetentionModuleTestClient(t, nil)
	ctx := context.Background()
	q := store.New(testutil.TestDB(t)).Queries()
	projectID := testutil.CreateTestProject(t, ctx, q)

	params := make([]river.InsertManyParams, jobCount)
	for i := range params {
		traceID := testutil.CreateTestTrace(t, ctx, q, projectID, "drain-"+uuid.NewString())
		params[i] = river.InsertManyParams{Args: jobargs.TraceRollupArgs{TraceID: traceID}}
	}
	inserted, err := client.InsertMany(ctx, params)
	require.NoError(t, err)
	pending := make(map[int64]struct{}, jobCount)
	for _, res := range inserted {
		pending[res.Job.ID] = struct{}{}
	}

	completed, cancel := client.Subscribe(river.EventKindJobCompleted)
	t.Cleanup(cancel)
	require.NoError(t, client.Start(ctx))
	t.Cleanup(func() {
		_ = client.Stop(context.Background())
	})

	deadline := time.After(cadenceMinimum / 2)
	for len(pending) > 0 {
		select {
		case event := <-completed:
			delete(pending, event.Job.ID)
		case <-deadline:
			t.Fatalf("%d of %d rollup jobs still pending after %s; the queue drains at fetch cadence, not job cost",
				len(pending), jobCount, cadenceMinimum/2)
		}
	}
}

func newRetentionModuleTestClient(t *testing.T, cfg *config.Config) *river.Client[pgx.Tx] {
	t.Helper()

	pool := testutil.TestDB(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		DELETE FROM river_job
		WHERE kind IN ($1, $2)
	`, jobargs.RetentionArgs{}.Kind(), jobargs.CleanupArgs{}.Kind())
	require.NoError(t, err)

	s := store.New(pool)
	processor := ingest.NewProcessor(s, cfg)
	client, err := NewClient(pool, s, processor, enginecontrol.NewService(s), cfg)
	require.NoError(t, err)
	return client
}

func waitForJobKindCount(t *testing.T, client *river.Client[pgx.Tx], kind string, want int) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		result, err := client.JobList(context.Background(), river.NewJobListParams().Kinds(kind).First(10))
		require.NoError(t, err)
		if len(result.Jobs) >= want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	result, err := client.JobList(context.Background(), river.NewJobListParams().Kinds(kind).First(10))
	require.NoError(t, err)
	t.Fatalf("timed out waiting for %d %s jobs, found %d", want, kind, len(result.Jobs))
}

func assertJobKindCount(t *testing.T, client *river.Client[pgx.Tx], kind string, want int) {
	t.Helper()

	result, err := client.JobList(context.Background(), river.NewJobListParams().Kinds(kind).First(10))
	require.NoError(t, err)
	assert.Len(t, result.Jobs, want)
}
