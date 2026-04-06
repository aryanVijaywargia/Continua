package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/jobargs"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestRetentionWorker_Stage1OnlyPurgesProjectionDetail(t *testing.T) {
	fixture := newRetentionFixture(t)
	completedAt := time.Now().UTC().Add(-96 * time.Hour).Round(time.Microsecond)

	record := fixture.seedCompletedRun(t, completedAt, "up_to_date")
	worker := NewRetentionWorker(fixture.store, enginecontrol.NewService(fixture.store), 24*time.Hour, 0)

	err := worker.Work(fixture.ctx, &river.Job[jobargs.RetentionArgs]{Args: jobargs.RetentionArgs{}})
	require.NoError(t, err)

	assertTraceProjectionState(t, fixture, record.trace.ID, "summary_only")
	assertSpanCounts(t, fixture, record.trace.ID, 1, 0)
	assertHistoryCount(t, fixture, record.run.ID, 1)
}

func TestRetentionWorker_BothStagesPromoteToJournalExpiredAndSkipExistingJournalExpired(t *testing.T) {
	fixture := newRetentionFixture(t)
	completedAt := time.Now().UTC().Add(-96 * time.Hour).Round(time.Microsecond)

	promoted := fixture.seedCompletedRun(t, completedAt, "up_to_date")
	skipped := fixture.seedCompletedRun(t, completedAt.Add(-time.Minute), "journal_expired")
	worker := NewRetentionWorker(fixture.store, enginecontrol.NewService(fixture.store), 24*time.Hour, 48*time.Hour)

	err := worker.Work(fixture.ctx, &river.Job[jobargs.RetentionArgs]{Args: jobargs.RetentionArgs{}})
	require.NoError(t, err)

	assertTraceProjectionState(t, fixture, promoted.trace.ID, "journal_expired")
	assertSpanCounts(t, fixture, promoted.trace.ID, 1, 0)
	assertHistoryCount(t, fixture, promoted.run.ID, 0)

	assertTraceProjectionState(t, fixture, skipped.trace.ID, "journal_expired")
	assertSpanCounts(t, fixture, skipped.trace.ID, 2, 1)
	assertHistoryCount(t, fixture, skipped.run.ID, 1)
}

func TestRetentionWorker_AdvisoryLockStaysBoundToHeldConnection(t *testing.T) {
	fixture := newRetentionFixture(t)
	worker := NewRetentionWorker(fixture.store, enginecontrol.NewService(fixture.store), 24*time.Hour, 0)

	lockConn, err := fixture.store.Pool().Acquire(fixture.ctx)
	require.NoError(t, err)
	defer lockConn.Release()

	locked, err := worker.tryAdvisoryLock(fixture.ctx, lockConn)
	require.NoError(t, err)
	require.True(t, locked)

	var secondSessionLocked bool
	err = fixture.store.Pool().QueryRow(fixture.ctx, "SELECT pg_try_advisory_lock($1)", retentionAdvisoryLockKey).Scan(&secondSessionLocked)
	require.NoError(t, err)
	assert.False(t, secondSessionLocked)

	worker.unlockAdvisoryLock(fixture.ctx, lockConn)

	err = fixture.store.Pool().QueryRow(fixture.ctx, "SELECT pg_try_advisory_lock($1)", retentionAdvisoryLockKey).Scan(&secondSessionLocked)
	require.NoError(t, err)
	assert.True(t, secondSessionLocked)

	var unlocked bool
	err = fixture.store.Pool().QueryRow(fixture.ctx, "SELECT pg_advisory_unlock($1)", retentionAdvisoryLockKey).Scan(&unlocked)
	require.NoError(t, err)
	assert.True(t, unlocked)
}

func TestRetentionWorker_RestartAfterProjectionPurgePromotesToJournalExpiredOnce(t *testing.T) {
	fixture := newRetentionFixture(t)
	completedAt := time.Now().UTC().Add(-96 * time.Hour).Round(time.Microsecond)
	record := fixture.seedCompletedRun(t, completedAt, "up_to_date")

	control := enginecontrol.NewService(fixture.store)
	_, err := control.PurgeRun(
		fixture.ctx,
		record.run.ProjectID,
		record.run.ID,
		enginecontrol.PurgeModeProjectionOnly,
	)
	require.NoError(t, err)

	assertTraceProjectionState(t, fixture, record.trace.ID, "summary_only")
	assertSpanCounts(t, fixture, record.trace.ID, 1, 0)
	assertHistoryCount(t, fixture, record.run.ID, 1)

	worker := NewRetentionWorker(fixture.store, control, 24*time.Hour, 48*time.Hour)
	job := &river.Job[jobargs.RetentionArgs]{Args: jobargs.RetentionArgs{}}

	err = worker.Work(fixture.ctx, job)
	require.NoError(t, err)

	assertTraceProjectionState(t, fixture, record.trace.ID, "journal_expired")
	assertSpanCounts(t, fixture, record.trace.ID, 1, 0)
	assertHistoryCount(t, fixture, record.run.ID, 0)

	err = worker.Work(fixture.ctx, job)
	require.NoError(t, err)

	assertTraceProjectionState(t, fixture, record.trace.ID, "journal_expired")
	assertSpanCounts(t, fixture, record.trace.ID, 1, 0)
	assertHistoryCount(t, fixture, record.run.ID, 0)
}

type retentionFixture struct {
	ctx       context.Context
	store     *store.Store
	engine    *enginedb.Queries
	projectID uuid.UUID
}

type retentionRecord struct {
	trace platformdb.Trace
	run   enginedb.EngineRun
}

func newRetentionFixture(t *testing.T) *retentionFixture {
	t.Helper()

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)

	return &retentionFixture{
		ctx:       ctx,
		store:     s,
		engine:    enginedb.New(pool),
		projectID: testutil.CreateTestProject(t, ctx, s.Queries()),
	}
}

func (f *retentionFixture) seedCompletedRun(
	t *testing.T,
	completedAt time.Time,
	projectionState string,
) retentionRecord {
	t.Helper()

	instanceKey := "retention-instance-" + uuid.NewString()[:8]
	traceExternalID := "engine:" + uuid.NewString()

	instance, err := f.engine.CreateInstance(f.ctx, enginedb.CreateInstanceParams{
		ProjectID:      f.projectID,
		InstanceKey:    instanceKey,
		DefinitionName: "checkout",
	})
	require.NoError(t, err)

	run, err := f.engine.CreateRun(f.ctx, enginedb.CreateRunParams{
		ProjectID:         f.projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           completedAt.Add(-time.Hour),
	})
	require.NoError(t, err)

	startedPayload, err := publichistory.MarshalPayload(publichistory.WorkflowStartedPayload{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       instanceKey,
		Input:             []byte(`{"cart_id":"retention"}`),
	})
	require.NoError(t, err)
	history, err := f.engine.AppendHistory(f.ctx, enginedb.AppendHistoryParams{
		ProjectID:  f.projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  publichistory.EventWorkflowStarted,
		Payload:    startedPayload,
	})
	require.NoError(t, err)

	_, err = f.store.Pool().Exec(f.ctx, `
		UPDATE engine.runs
		SET status = 'completed',
		    waiting_for = NULL,
		    result = '{"ok":true}',
		    completed_at = $2,
		    updated_at = $2
		WHERE id = $1
	`, run.ID, completedAt)
	require.NoError(t, err)
	_, err = f.store.Pool().Exec(f.ctx, `
		UPDATE engine.instances
		SET status = 'completed',
		    updated_at = $2
		WHERE id = $1
	`, instance.ID, completedAt)
	require.NoError(t, err)

	trace, err := f.store.Queries().UpsertTrace(f.ctx, platformdb.UpsertTraceParams{
		ProjectID: f.projectID,
		TraceID:   traceExternalID,
		Name:      testutil.StrPtr("Retention Trace"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(completedAt.Add(-time.Hour)),
		EndTime:   testutil.PgtypeTimestamptz(completedAt),
		Output:    []byte(`{"ok":true}`),
	})
	require.NoError(t, err)

	_, err = f.store.Pool().Exec(f.ctx, `
		UPDATE traces
		SET engine_run_id = $2,
		    engine_instance_key = $3,
		    engine_run_status = 'completed',
		    engine_definition_name = 'checkout',
		    engine_definition_version = 'v1',
		    engine_projection_state = $4,
		    engine_latest_history_id = $5,
		    engine_last_projected_history_id = $5,
		    engine_projection_updated_at = $6,
		    updated_at = $6
		WHERE id = $1
	`, trace.ID, run.ID, instanceKey, projectionState, history.ID, completedAt)
	require.NoError(t, err)

	_, err = f.store.Queries().CreateSpan(f.ctx, platformdb.CreateSpanParams{
		ProjectID: f.projectID,
		TraceID:   trace.ID,
		SpanID:    "engine:root:" + run.ID.String(),
		Name:      "Retention Trace",
		Type:      "chain",
		Status:    "completed",
		Level:     "default",
		StartTime: completedAt.Add(-time.Hour),
		EndTime:   testutil.PgtypeTimestamptz(completedAt),
		Depth:     testutil.Int32Ptr(0),
	})
	require.NoError(t, err)
	_, err = f.store.Queries().CreateSpan(f.ctx, platformdb.CreateSpanParams{
		ProjectID:    f.projectID,
		TraceID:      trace.ID,
		SpanID:       "engine:activity:" + run.ID.String() + ":ship-order",
		ParentSpanID: testutil.StrPtr("engine:root:" + run.ID.String()),
		Name:         "ship-order",
		Type:         "tool",
		Status:       "completed",
		Level:        "default",
		StartTime:    completedAt.Add(-30 * time.Minute),
		EndTime:      testutil.PgtypeTimestamptz(completedAt.Add(-20 * time.Minute)),
		Depth:        testutil.Int32Ptr(1),
	})
	require.NoError(t, err)
	_, err = f.store.Queries().InsertSpanEvent(f.ctx, platformdb.InsertSpanEventParams{
		ProjectID: f.projectID,
		TraceID:   trace.ID,
		SpanID:    "engine:root:" + run.ID.String(),
		EventType: "custom",
		Level:     "info",
		EventTs:   testutil.PgtypeTimestamptz(completedAt.Add(-10 * time.Minute)),
		Message:   testutil.StrPtr("retained detail"),
		Payload:   []byte(`{"detail":true}`),
	})
	require.NoError(t, err)

	return retentionRecord{trace: trace, run: run}
}

func assertTraceProjectionState(t *testing.T, fixture *retentionFixture, traceID uuid.UUID, want string) {
	t.Helper()

	var state string
	err := fixture.store.Pool().QueryRow(fixture.ctx, `
		SELECT engine_projection_state
		FROM traces
		WHERE id = $1
	`, traceID).Scan(&state)
	require.NoError(t, err)
	assert.Equal(t, want, state)
}

func assertSpanCounts(t *testing.T, fixture *retentionFixture, traceID uuid.UUID, wantSpans int, wantEvents int) {
	t.Helper()

	var spanCount int
	err := fixture.store.Pool().QueryRow(fixture.ctx, `
		SELECT COUNT(*)
		FROM spans
		WHERE trace_id = $1
	`, traceID).Scan(&spanCount)
	require.NoError(t, err)
	assert.Equal(t, wantSpans, spanCount)

	var eventCount int
	err = fixture.store.Pool().QueryRow(fixture.ctx, `
		SELECT COUNT(*)
		FROM span_events
		WHERE trace_id = $1
	`, traceID).Scan(&eventCount)
	require.NoError(t, err)
	assert.Equal(t, wantEvents, eventCount)
}

func assertHistoryCount(t *testing.T, fixture *retentionFixture, runID uuid.UUID, want int) {
	t.Helper()

	rows, err := fixture.engine.GetHistoryByRun(fixture.ctx, runID)
	require.NoError(t, err)
	assert.Len(t, rows, want)
}
