package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestReapRequestDedupe_DeletesFinalizedPastCutoff(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	now := time.Now()

	seedRetentionDedupe(t, ts, projectID, "old-expired", "expired", now.Add(-48*time.Hour))
	seedRetentionDedupe(t, ts, projectID, "old-completed", "completed", now.Add(-48*time.Hour))
	seedRetentionDedupe(t, ts, projectID, "recent-expired", "expired", now.Add(time.Hour))
	seedRetentionDedupe(t, ts, projectID, "old-in-progress", "in_progress", now.Add(-48*time.Hour))

	reaped, err := ts.store.ReapRequestDedupe(ts.ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("ReapRequestDedupe() error = %v", err)
	}
	if reaped != 2 {
		t.Fatalf("ReapRequestDedupe() = %d, want 2", reaped)
	}
	if got, want := retentionDedupeKeys(t, ts, projectID), []string{"old-in-progress", "recent-expired"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("surviving request_dedupe keys = %v, want %v", got, want)
	}
}

func TestReapRequestDedupe_HonorsBatchLimit(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		seedRetentionDedupe(t, ts, projectID, "eligible-"+string(rune('a'+i)), "expired", now.Add(-48*time.Hour))
	}

	reaped, err := ts.store.ReapRequestDedupe(ts.ctx, now.Add(-24*time.Hour), 2)
	if err != nil {
		t.Fatalf("ReapRequestDedupe() first call error = %v", err)
	}
	if reaped != 2 {
		t.Fatalf("ReapRequestDedupe() first call = %d, want 2", reaped)
	}
	if remaining := retentionDedupeCount(t, ts, projectID); remaining != 3 {
		t.Fatalf("request_dedupe rows after first call = %d, want 3", remaining)
	}

	reaped, err = ts.store.ReapRequestDedupe(ts.ctx, now.Add(-24*time.Hour), 2)
	if err != nil {
		t.Fatalf("ReapRequestDedupe() second call error = %v", err)
	}
	if reaped != 2 {
		t.Fatalf("ReapRequestDedupe() second call = %d, want 2", reaped)
	}
	if remaining := retentionDedupeCount(t, ts, projectID); remaining != 1 {
		t.Fatalf("request_dedupe rows after second call = %d, want 1", remaining)
	}
}

func TestReapRequestDedupe_HonorsProjectFilter(t *testing.T) {
	ts := newTestStore(t)
	projectA := uuidOrFatal(t)
	projectB := uuidOrFatal(t)
	now := time.Now()
	seedRetentionDedupe(t, ts, projectA, "project-a", "failed", now.Add(-48*time.Hour))
	seedRetentionDedupe(t, ts, projectB, "project-b", "failed", now.Add(-48*time.Hour))

	reaped, err := ts.store.WithProjectFilter(projectA).ReapRequestDedupe(ts.ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("ReapRequestDedupe() error = %v", err)
	}
	if reaped != 1 {
		t.Fatalf("ReapRequestDedupe() = %d, want 1", reaped)
	}
	if remaining := retentionDedupeCount(t, ts, projectA); remaining != 0 {
		t.Fatalf("project A request_dedupe rows = %d, want 0", remaining)
	}
	if remaining := retentionDedupeCount(t, ts, projectB); remaining != 1 {
		t.Fatalf("project B request_dedupe rows = %d, want 1", remaining)
	}
}

func TestListRetainableTerminalRunIDs_RequiresProjectionComplete(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	enginetest.EnsurePlatformProject(t, ts.db.Pool, projectID)
	completedAt := time.Now().Add(-48 * time.Hour)
	incomplete := seedRetentionRun(t, ts, projectID, "projection-incomplete", "completed", completedAt)
	complete := seedRetentionRun(t, ts, projectID, "projection-complete", "completed", completedAt.Add(time.Minute))
	seedRetentionTrace(t, ts, incomplete, "completed", "up_to_date", 20, 19, false)
	seedRetentionTrace(t, ts, complete, "completed", "up_to_date", 30, 30, false)

	ids, err := ts.store.ListRetainableTerminalRunIDs(ts.ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("ListRetainableTerminalRunIDs() error = %v", err)
	}
	if got, want := ids, []uuid.UUID{complete.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListRetainableTerminalRunIDs() = %v, want %v", got, want)
	}
}

func TestListRetainableTerminalRunIDs_ExcludesIneligibleRuns(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	enginetest.EnsurePlatformProject(t, ts.db.Pool, projectID)
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)

	recent := seedRetentionRun(t, ts, projectID, "recent-completed", "completed", now.Add(-time.Hour))
	seedRetentionTrace(t, ts, recent, "completed", "up_to_date", 10, 10, false)
	running := seedRetentionRun(t, ts, projectID, "running", "running", now.Add(-48*time.Hour))
	seedRetentionTrace(t, ts, running, "running", "up_to_date", 10, 10, false)
	quarantined := seedRetentionRun(t, ts, projectID, "quarantined", "quarantined", now.Add(-48*time.Hour))
	seedRetentionTrace(t, ts, quarantined, "quarantined", "up_to_date", 10, 10, false)
	continued := seedRetentionRun(t, ts, projectID, "continued", "continued_as_new", now.Add(-48*time.Hour))
	seedRetentionTrace(t, ts, continued, "continued_as_new", "up_to_date", 10, 10, false)
	seedRetentionRun(t, ts, projectID, "no-trace", "completed", now.Add(-48*time.Hour))
	alreadyExpired := seedRetentionRun(t, ts, projectID, "already-expired", "completed", now.Add(-48*time.Hour))
	seedRetentionTrace(t, ts, alreadyExpired, "completed", "journal_expired", 10, 10, false)
	control := seedRetentionRun(t, ts, projectID, "included-control", "completed", now.Add(-72*time.Hour))
	seedRetentionTrace(t, ts, control, "completed", "up_to_date", 10, 10, false)

	ids, err := ts.store.ListRetainableTerminalRunIDs(ts.ctx, cutoff, 20)
	if err != nil {
		t.Fatalf("ListRetainableTerminalRunIDs() error = %v", err)
	}
	if got, want := ids, []uuid.UUID{control.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListRetainableTerminalRunIDs() = %v, want only control %v", got, want)
	}
}

func TestListRetainableTerminalRunIDs_BatchLimitAndOrder(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	enginetest.EnsurePlatformProject(t, ts.db.Pool, projectID)
	now := time.Now()
	oldest := seedRetentionRun(t, ts, projectID, "oldest", "completed", now.Add(-72*time.Hour))
	middle := seedRetentionRun(t, ts, projectID, "middle", "completed", now.Add(-48*time.Hour))
	newest := seedRetentionRun(t, ts, projectID, "newest", "completed", now.Add(-36*time.Hour))
	for _, run := range []enginedb.EngineRun{oldest, middle, newest} {
		seedRetentionTrace(t, ts, run, "completed", "up_to_date", 10, 10, false)
	}

	ids, err := ts.store.ListRetainableTerminalRunIDs(ts.ctx, now.Add(-24*time.Hour), 2)
	if err != nil {
		t.Fatalf("ListRetainableTerminalRunIDs() error = %v", err)
	}
	if got, want := ids, []uuid.UUID{oldest.ID, middle.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListRetainableTerminalRunIDs() = %v, want oldest-first %v", got, want)
	}
}

func TestReapTerminalRunJournal_DeletesJournalAndMarksExpired(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	fixture := seedRetentionJournal(t, ts, projectID, "reap-journal")

	counts, err := ts.store.ReapTerminalRunJournal(ts.ctx, fixture.run.ID)
	if err != nil {
		t.Fatalf("ReapTerminalRunJournal() error = %v", err)
	}
	if counts != (RetentionReapCounts{History: 2, Inbox: 1, ActivityTasks: 1}) {
		t.Fatalf("ReapTerminalRunJournal() counts = %+v, want history=2 inbox=1 activity_tasks=1", counts)
	}
	assertRetentionRunRows(t, ts, fixture.run.ID, 0, 0, 0)

	var runExists, instanceExists, traceExists, rootSpanExists bool
	var traceState, traceStatus string
	if err := ts.db.Pool.QueryRow(ts.ctx, `SELECT EXISTS (SELECT 1 FROM engine.runs WHERE id = $1)`, fixture.run.ID).Scan(&runExists); err != nil {
		t.Fatalf("query run existence: %v", err)
	}
	if err := ts.db.Pool.QueryRow(ts.ctx, `SELECT EXISTS (SELECT 1 FROM engine.instances WHERE id = $1)`, fixture.instance.ID).Scan(&instanceExists); err != nil {
		t.Fatalf("query instance existence: %v", err)
	}
	if err := ts.db.Pool.QueryRow(ts.ctx, `
		SELECT EXISTS (SELECT 1 FROM public.traces WHERE id = $1),
		       COALESCE(engine_projection_state, ''), status
		FROM public.traces WHERE id = $1
	`, fixture.traceID).Scan(&traceExists, &traceState, &traceStatus); err != nil {
		t.Fatalf("query retained trace: %v", err)
	}
	if err := ts.db.Pool.QueryRow(ts.ctx, `SELECT EXISTS (SELECT 1 FROM public.spans WHERE id = $1)`, fixture.rootSpanID).Scan(&rootSpanExists); err != nil {
		t.Fatalf("query root span existence: %v", err)
	}
	if !runExists || !instanceExists || !traceExists || !rootSpanExists {
		t.Fatalf("retained rows run=%t instance=%t trace=%t root_span=%t, want all true", runExists, instanceExists, traceExists, rootSpanExists)
	}
	if traceState != "journal_expired" || traceStatus != "completed" {
		t.Fatalf("trace projection_state/status = %q/%q, want journal_expired/completed", traceState, traceStatus)
	}
}

func TestReapTerminalRunJournal_ProjectFilterMismatchLeavesData(t *testing.T) {
	ts := newTestStore(t)
	projectA := uuidOrFatal(t)
	projectB := uuidOrFatal(t)
	fixture := seedRetentionJournal(t, ts, projectA, "project-filter-mismatch")

	_, _ = ts.store.WithProjectFilter(projectB).ReapTerminalRunJournal(ts.ctx, fixture.run.ID)

	assertRetentionRunRows(t, ts, fixture.run.ID, 2, 1, 1)
	var traceState string
	if err := ts.db.Pool.QueryRow(ts.ctx, `SELECT COALESCE(engine_projection_state, '') FROM public.traces WHERE id = $1`, fixture.traceID).Scan(&traceState); err != nil {
		t.Fatalf("query trace projection state: %v", err)
	}
	if traceState == "journal_expired" {
		t.Fatal("project-filter mismatch marked trace journal_expired; want journal data and projection state untouched")
	}
}

type retentionJournalFixture struct {
	instance   enginedb.EngineInstance
	run        enginedb.EngineRun
	traceID    uuid.UUID
	rootSpanID uuid.UUID
}

func seedRetentionDedupe(t *testing.T, ts *testStore, projectID uuid.UUID, key, status string, expiresAt time.Time) {
	t.Helper()
	if _, err := ts.db.Pool.Exec(ts.ctx, `
		INSERT INTO engine.request_dedupe (project_id, request_scope, request_key, status, expires_at)
		VALUES ($1, 'engine.start', $2, $3::engine.request_dedupe_status, $4)
	`, projectID, key, status, expiresAt); err != nil {
		t.Fatalf("seed request_dedupe %q: %v", key, err)
	}
}

func retentionDedupeKeys(t *testing.T, ts *testStore, projectID uuid.UUID) []string {
	t.Helper()
	rows, err := ts.db.Pool.Query(ts.ctx, `SELECT request_key FROM engine.request_dedupe WHERE project_id = $1 ORDER BY request_key`, projectID)
	if err != nil {
		t.Fatalf("query request_dedupe keys: %v", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatalf("scan request_dedupe key: %v", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate request_dedupe keys: %v", err)
	}
	return keys
}

func retentionDedupeCount(t *testing.T, ts *testStore, projectID uuid.UUID) int64 {
	t.Helper()
	var count int64
	if err := ts.db.Pool.QueryRow(ts.ctx, `SELECT COUNT(*) FROM engine.request_dedupe WHERE project_id = $1`, projectID).Scan(&count); err != nil {
		t.Fatalf("count request_dedupe rows: %v", err)
	}
	return count
}

func seedRetentionRun(t *testing.T, ts *testStore, projectID uuid.UUID, key, status string, completedAt time.Time) enginedb.EngineRun {
	t.Helper()
	instance := ts.createInstance(t, projectID, key+"-"+uuid.NewString()[:8])
	run := ts.createRun(t, instance, 1)
	if _, err := ts.db.Pool.Exec(ts.ctx, `
		UPDATE engine.runs
		SET status = $2::engine.run_lifecycle_status, completed_at = $3, updated_at = $3
		WHERE id = $1
	`, run.ID, status, completedAt); err != nil {
		t.Fatalf("set run %s status %s: %v", run.ID, status, err)
	}
	run.Status = enginedb.EngineRunLifecycleStatus(status)
	return run
}

func seedRetentionTrace(
	t *testing.T,
	ts *testStore,
	run enginedb.EngineRun,
	status, projectionState string,
	latestHistoryID, lastProjectedHistoryID int64,
	withRootSpan bool,
) (uuid.UUID, uuid.UUID) {
	t.Helper()
	traceID := uuid.New()
	if _, err := ts.db.Pool.Exec(ts.ctx, `
		INSERT INTO public.traces (
			id, project_id, trace_id, name, status, start_time, end_time,
			engine_run_id, engine_run_status, engine_projection_state,
			engine_latest_history_id, engine_last_projected_history_id, engine_projection_updated_at
		)
		VALUES ($1, $2, $3, 'Retention test', $4, NOW() - INTERVAL '2 days', NOW(),
		        $5, $4, $6, $7, $8, NOW())
	`, traceID, run.ProjectID, "engine:"+run.ID.String(), status, run.ID, projectionState, latestHistoryID, lastProjectedHistoryID); err != nil {
		t.Fatalf("seed projected trace for run %s: %v", run.ID, err)
	}
	if !withRootSpan {
		return traceID, uuid.Nil
	}
	rootSpanID := uuid.New()
	if _, err := ts.db.Pool.Exec(ts.ctx, `
		INSERT INTO public.spans (id, project_id, trace_id, span_id, name, type, status, level, start_time, end_time, depth)
		VALUES ($1, $2, $3, $4, 'Retention root', 'chain', 'completed', 'default', NOW() - INTERVAL '2 days', NOW(), 0)
	`, rootSpanID, run.ProjectID, traceID, "engine:run:"+run.ID.String()); err != nil {
		t.Fatalf("seed root span for run %s: %v", run.ID, err)
	}
	return traceID, rootSpanID
}

func seedRetentionJournal(t *testing.T, ts *testStore, projectID uuid.UUID, key string) retentionJournalFixture {
	t.Helper()
	enginetest.EnsurePlatformProject(t, ts.db.Pool, projectID)
	instance := ts.createInstance(t, projectID, key+"-"+uuid.NewString()[:8])
	run := ts.createRun(t, instance, 1)
	first := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "workflow.started")
	second := ts.createHistory(t, projectID, instance.ID, run.ID, 2, "workflow.completed")
	if _, err := ts.store.CreateInboxItem(ts.ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  instance.ID,
		RunID:       enginetest.NullableUUID(run.ID),
		HistoryID:   &first.ID,
		Kind:        "signal",
		Payload:     []byte(`{"signal":"done"}`),
		AvailableAt: time.Now().Add(-48 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateInboxItem() error = %v", err)
	}
	if _, err := ts.db.Pool.Exec(ts.ctx, `UPDATE engine.inbox SET status = 'processed', resolved_at = NOW() WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("mark inbox processed: %v", err)
	}
	if _, err := ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &second.ID,
		ActivityKey:     "retention-activity",
		ActivityType:    "retention.test",
		Input:           []byte(`{"value":1}`),
		AvailableAt:     time.Now().Add(-48 * time.Hour),
		ExecutionTarget: "local",
		MaxAttempts:     1,
	}); err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}
	if _, err := ts.db.Pool.Exec(ts.ctx, `UPDATE engine.activity_tasks SET status = 'completed', completed_at = NOW() WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("mark activity task completed: %v", err)
	}
	completedAt := time.Now().Add(-48 * time.Hour)
	if _, err := ts.db.Pool.Exec(ts.ctx, `UPDATE engine.runs SET status = 'completed', completed_at = $2, updated_at = $2 WHERE id = $1`, run.ID, completedAt); err != nil {
		t.Fatalf("mark run completed: %v", err)
	}
	run.Status = enginedb.EngineRunLifecycleStatusCompleted
	traceID, rootSpanID := seedRetentionTrace(t, ts, run, "completed", "up_to_date", second.ID, second.ID, true)
	return retentionJournalFixture{instance: instance, run: run, traceID: traceID, rootSpanID: rootSpanID}
}

func assertRetentionRunRows(t *testing.T, ts *testStore, runID uuid.UUID, wantHistory, wantInbox, wantTasks int64) {
	t.Helper()
	queries := []struct {
		name  string
		query string
		want  int64
	}{
		{name: "history", query: `SELECT COUNT(*) FROM engine.history WHERE run_id = $1`, want: wantHistory},
		{name: "inbox", query: `SELECT COUNT(*) FROM engine.inbox WHERE run_id = $1`, want: wantInbox},
		{name: "activity_tasks", query: `SELECT COUNT(*) FROM engine.activity_tasks WHERE run_id = $1`, want: wantTasks},
	}
	for _, check := range queries {
		var got int64
		if err := ts.db.Pool.QueryRow(ts.ctx, check.query, runID).Scan(&got); err != nil {
			t.Fatalf("count %s rows: %v", check.name, err)
		}
		if got != check.want {
			t.Errorf("%s rows for run = %d, want %d", check.name, got, check.want)
		}
	}
}
