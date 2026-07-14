package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginemetrics "github.com/continua-ai/continua/engine/internal/metrics"
	"github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestRetentionWorkerPollOnce_DisabledIsNoop(t *testing.T) {
	fixture := newRetentionWorkerFixture(t)
	worker := NewRetentionWorker(fixture.store, RetentionConfig{
		TerminalRuns: 0,
		DedupeGrace:  0,
		BatchSize:    10,
	})

	if err := worker.PollOnce(fixture.ctx, "retention-test"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	fixture.assertRows(t, 1, 2, 1, 1)
	if state := fixture.traceProjectionState(t); state == "journal_expired" {
		t.Fatal("disabled PollOnce() marked trace journal_expired")
	}
}

func TestRetentionWorkerPollOnce_ReapsAndRecordsMetrics(t *testing.T) {
	fixture := newRetentionWorkerFixture(t)
	registry := prometheus.NewRegistry()
	fixture.store = fixture.store.WithMetrics(enginemetrics.New(registry))
	worker := NewRetentionWorker(fixture.store, RetentionConfig{
		TerminalRuns: 24 * time.Hour,
		DedupeGrace:  24 * time.Hour,
		BatchSize:    10,
	})

	if err := worker.PollOnce(fixture.ctx, "retention-test"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	fixture.assertRows(t, 0, 0, 0, 0)
	if state := fixture.traceProjectionState(t); state != "journal_expired" {
		t.Errorf("trace engine_projection_state = %q, want journal_expired", state)
	}

	got := gatherRetentionReaped(t, registry)
	want := map[string]float64{
		"request_dedupe": 1,
		"history":        2,
		"inbox":          1,
		"activity_tasks": 1,
	}
	for table, wantValue := range want {
		if gotValue := got[table]; gotValue != wantValue {
			t.Errorf("continua_engine_retention_reaped_rows_total{table=%q} = %v, want %v", table, gotValue, wantValue)
		}
	}
}

type retentionWorkerFixture struct {
	ctx     context.Context
	db      *enginetest.TestDatabase
	store   *store.Store
	project uuid.UUID
	runID   uuid.UUID
	traceID uuid.UUID
}

func newRetentionWorkerFixture(t *testing.T) *retentionWorkerFixture {
	t.Helper()
	ctx := context.Background()
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	engineStore := store.New(db.Pool)
	instance, err := engineStore.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "retention-worker-" + uuid.NewString()[:8],
		DefinitionName: "retention.worker",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := engineStore.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now().Add(-48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	first, err := engineStore.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID: projectID, InstanceID: instance.ID, RunID: run.ID,
		SequenceNo: 1, EventType: "workflow.started", Payload: []byte(`{"event":"started"}`),
	})
	if err != nil {
		t.Fatalf("AppendHistory(first) error = %v", err)
	}
	second, err := engineStore.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID: projectID, InstanceID: instance.ID, RunID: run.ID,
		SequenceNo: 2, EventType: "workflow.completed", Payload: []byte(`{"event":"completed"}`),
	})
	if err != nil {
		t.Fatalf("AppendHistory(second) error = %v", err)
	}
	if _, err := engineStore.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID: projectID, InstanceID: instance.ID, RunID: enginetest.NullableUUID(run.ID),
		HistoryID: &first.ID, Kind: "signal", Payload: []byte(`{"signal":"done"}`), AvailableAt: time.Now().Add(-48 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateInboxItem() error = %v", err)
	}
	if _, err := engineStore.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID: projectID, InstanceID: instance.ID, RunID: run.ID, HistoryID: &second.ID,
		ActivityKey: "retention", ActivityType: "retention.test", Input: []byte(`{"value":1}`),
		AvailableAt: time.Now().Add(-48 * time.Hour), ExecutionTarget: "local", MaxAttempts: 1,
	}); err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}
	fixtureUpdates := []struct {
		name  string
		query string
	}{
		{name: "run", query: `UPDATE engine.runs SET status = 'completed', completed_at = NOW() - INTERVAL '48 hours' WHERE id = $1`},
		{name: "inbox", query: `UPDATE engine.inbox SET status = 'processed', resolved_at = NOW() WHERE run_id = $1`},
		{name: "activity task", query: `UPDATE engine.activity_tasks SET status = 'completed', completed_at = NOW() WHERE run_id = $1`},
	}
	for _, update := range fixtureUpdates {
		if _, err := db.Pool.Exec(ctx, update.query, run.ID); err != nil {
			t.Fatalf("finalize retention worker %s fixture: %v", update.name, err)
		}
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO engine.request_dedupe (project_id, request_scope, request_key, status, expires_at)
		VALUES ($1, 'engine.start', 'retention-worker', 'expired', NOW() - INTERVAL '48 hours')
	`, projectID); err != nil {
		t.Fatalf("seed request_dedupe: %v", err)
	}
	traceID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO public.traces (
			id, project_id, trace_id, name, status, start_time, end_time,
			engine_run_id, engine_run_status, engine_projection_state,
			engine_latest_history_id, engine_last_projected_history_id, engine_projection_updated_at
		)
		VALUES ($1, $2, $3, 'Retention worker', 'completed', NOW() - INTERVAL '48 hours', NOW(),
		        $4, 'completed', 'up_to_date', $5, $5, NOW())
	`, traceID, projectID, "engine:"+run.ID.String(), run.ID, second.ID); err != nil {
		t.Fatalf("seed projected trace: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO public.spans (project_id, trace_id, span_id, name, type, status, level, start_time, end_time, depth)
		VALUES ($1, $2, $3, 'Retention root', 'chain', 'completed', 'default', NOW() - INTERVAL '48 hours', NOW(), 0)
	`, projectID, traceID, "engine:run:"+run.ID.String()); err != nil {
		t.Fatalf("seed projected root span: %v", err)
	}

	return &retentionWorkerFixture{ctx: ctx, db: db, store: engineStore, project: projectID, runID: run.ID, traceID: traceID}
}

func (f *retentionWorkerFixture) assertRows(t *testing.T, wantDedupe, wantHistory, wantInbox, wantTasks int64) {
	t.Helper()
	checks := []struct {
		name  string
		query string
		arg   any
		want  int64
	}{
		{name: "request_dedupe", query: `SELECT COUNT(*) FROM engine.request_dedupe WHERE project_id = $1`, arg: f.project, want: wantDedupe},
		{name: "history", query: `SELECT COUNT(*) FROM engine.history WHERE run_id = $1`, arg: f.runID, want: wantHistory},
		{name: "inbox", query: `SELECT COUNT(*) FROM engine.inbox WHERE run_id = $1`, arg: f.runID, want: wantInbox},
		{name: "activity_tasks", query: `SELECT COUNT(*) FROM engine.activity_tasks WHERE run_id = $1`, arg: f.runID, want: wantTasks},
	}
	for _, check := range checks {
		var got int64
		if err := f.db.Pool.QueryRow(f.ctx, check.query, check.arg).Scan(&got); err != nil {
			t.Fatalf("count %s rows: %v", check.name, err)
		}
		if got != check.want {
			t.Errorf("%s rows = %d, want %d", check.name, got, check.want)
		}
	}
}

func (f *retentionWorkerFixture) traceProjectionState(t *testing.T) string {
	t.Helper()
	var state string
	if err := f.db.Pool.QueryRow(f.ctx, `SELECT COALESCE(engine_projection_state, '') FROM public.traces WHERE id = $1`, f.traceID).Scan(&state); err != nil {
		t.Fatalf("query trace projection state: %v", err)
	}
	return state
}

func gatherRetentionReaped(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("registry.Gather() error = %v", err)
	}
	values := make(map[string]float64)
	for _, family := range families {
		if family.GetName() != "continua_engine_retention_reaped_rows_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			var table string
			for _, label := range metric.GetLabel() {
				if label.GetName() == "table" {
					table = label.GetValue()
				}
			}
			values[table] = metric.GetCounter().GetValue()
		}
	}
	return values
}
