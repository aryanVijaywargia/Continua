package projector

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

// A run that completes or fails with an open activity and an open timer cancels the task
// and discards the timer (#359). The projector pass for the terminal history row closes
// the activity span as cancelled and resolves both waits, and projecting that row again
// changes nothing (#362).
//
// The first activation swallows the engine's wait for the activity, so it schedules the
// activity and then waits on the timer. A signal wakes the run, and the next activation
// ends it: "completed" because the code swallows both waits, "failed" because the
// definition version is gone.
func TestProjectorPollOnce_FinishedRunClosesCancelledWork(t *testing.T) {
	scheduleWork := func(ctx publicworkflow.Context) {
		defer func() { _ = recover() }()
		_ = ctx.Activity("work", "demo.work", nil, nil)
	}
	waitOnTimer := func(ctx publicworkflow.Context) {
		defer func() { _ = recover() }()
		_ = ctx.Sleep("deadline", time.Hour)
	}
	endings := []struct {
		name        string
		definitions []publicworkflow.Definition
		status      enginedb.EngineRunLifecycleStatus
		event       string
		message     string
	}{
		{
			name: "completed",
			definitions: []publicworkflow.Definition{{
				Name:    "projector-demo",
				Version: "v1",
				Run: func(ctx publicworkflow.Context) error {
					scheduleWork(ctx)
					waitOnTimer(ctx)
					return nil
				},
			}},
			status:  enginedb.EngineRunLifecycleStatusCompleted,
			event:   publichistory.EventWorkflowCompleted,
			message: "activity cancelled: workflow completed",
		},
		{
			name:    "failed",
			status:  enginedb.EngineRunLifecycleStatusFailed,
			event:   publichistory.EventWorkflowFailed,
			message: "activity cancelled: workflow failed",
		},
	}
	for _, ending := range endings {
		t.Run(ending.name, func(t *testing.T) {
			fixture := newProjectorFixture(t)
			activate(t, fixture, publicworkflow.Definition{
				Name:    "projector-demo",
				Version: "v1",
				Run: func(ctx publicworkflow.Context) error {
					scheduleWork(ctx)
					return ctx.Sleep("deadline", time.Hour)
				},
			})
			if err := fixture.projector.PollOnce(fixture.ctx, "projector-open-work"); err != nil {
				t.Fatalf("PollOnce() open work error = %v", err)
			}

			signal, err := publichistory.MarshalPayload(publichistory.SignalReceivedPayload{SignalName: "nudge"})
			if err != nil {
				t.Fatalf("MarshalPayload(signal) error = %v", err)
			}
			if _, err := fixture.store.CreateInboxItem(fixture.ctx, enginedb.CreateInboxItemParams{
				ProjectID:   fixture.projectID,
				InstanceID:  fixture.instance.ID,
				RunID:       pgtype.UUID{Bytes: fixture.run.ID, Valid: true},
				Kind:        "signal",
				Payload:     signal,
				AvailableAt: time.Now().Add(-time.Second),
			}); err != nil {
				t.Fatalf("CreateInboxItem(signal) error = %v", err)
			}
			if _, err := fixture.store.WakeWaitingRun(fixture.ctx, fixture.run.ID); err != nil {
				t.Fatalf("WakeWaitingRun() error = %v", err)
			}
			activate(t, fixture, ending.definitions...)

			finished, err := fixture.store.GetRun(fixture.ctx, fixture.run.ID)
			if err != nil {
				t.Fatalf("GetRun() error = %v", err)
			}
			if finished.Status != ending.status {
				t.Fatalf("run status = %s, want %s", finished.Status, ending.status)
			}
			history, err := fixture.store.GetHistoryByRun(fixture.ctx, fixture.run.ID)
			if err != nil {
				t.Fatalf("GetHistoryByRun() error = %v", err)
			}
			terminal := history[len(history)-1]
			if terminal.EventType != ending.event {
				t.Fatalf("last history event = %s, want %s", terminal.EventType, ending.event)
			}

			if err := fixture.projector.PollOnce(fixture.ctx, "projector-finished-run"); err != nil {
				t.Fatalf("PollOnce() finished run error = %v", err)
			}
			span := queryActivitySpan(t, fixture, "work")
			if span.Status != "failed" || span.StatusMessage == nil || *span.StatusMessage != ending.message {
				t.Fatalf("activity span status = %q, message %v; want failed, %q", span.Status, span.StatusMessage, ending.message)
			}
			if span.EndTime == nil || !span.EndTime.Equal(terminal.CreatedAt) {
				t.Fatalf("activity span end time = %v, want the terminal history time %v", span.EndTime, terminal.CreatedAt)
			}
			var output map[string]any
			if err := json.Unmarshal(span.Output, &output); err != nil {
				t.Fatalf("json.Unmarshal(activity span output) error = %v", err)
			}
			if output["error_code"] != "cancelled" || output["error_message"] != ending.message {
				t.Fatalf("activity span output = %v, want error_code cancelled", output)
			}
			var metadata map[string]any
			if err := json.Unmarshal(span.Metadata, &metadata); err != nil {
				t.Fatalf("json.Unmarshal(activity span metadata) error = %v", err)
			}
			if metadata[publicprojection.TerminalHistoryMetadataKey] != float64(terminal.ID) {
				t.Fatalf("activity span metadata = %v, want terminal history id %d", metadata, terminal.ID)
			}
			waits := queryResolvedWaitEvents(t, fixture, "cancelled")
			want := []resolvedWaitEvent{
				{Sequence: terminal.SequenceNo*10 + 1, WaitID: "activity:work"},
				{Sequence: terminal.SequenceNo*10 + 2, WaitID: "timer:deadline"},
			}
			if len(waits) != len(want) || waits[0] != want[0] || waits[1] != want[1] {
				t.Fatalf("cancelled waits = %+v, want %+v", waits, want)
			}
			eventCount := countSpanEvents(t, fixture)

			// Crash recovery projects the terminal history row again.
			fixture.setTraceProjection(terminal.ID, history[len(history)-2].ID, publicprojection.StateCatchingUp.String())
			if err := fixture.projector.PollOnce(fixture.ctx, "projector-finished-run-again"); err != nil {
				t.Fatalf("PollOnce() re-projection error = %v", err)
			}
			assertProjectedDetailState(t, fixture, publicprojection.StateUpToDate.String(), terminal.ID, 1, eventCount)
			if again := queryActivitySpan(t, fixture, "work"); !again.equal(&span) {
				t.Fatalf("activity span after re-projection = %+v, want %+v", again, span)
			}
		})
	}
}

func activate(t *testing.T, fixture *projectorFixture, definitions ...publicworkflow.Definition) {
	t.Helper()

	registry, err := engineworkflow.NewRegistry(definitions...)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	claimed, err := fixture.store.ClaimNextRun(fixture.ctx, "workflow-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	if err := engineworkflow.NewActivator(fixture.store, registry).Activate(fixture.ctx, &claimed); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
}

type activitySpanRow struct {
	Status        string
	StatusMessage *string
	EndTime       *time.Time
	Output        []byte
	Metadata      []byte
	Version       int32
}

func (r *activitySpanRow) equal(other *activitySpanRow) bool {
	return r.Status == other.Status &&
		*r.StatusMessage == *other.StatusMessage &&
		r.EndTime.Equal(*other.EndTime) &&
		jsonEqual(r.Output, other.Output) &&
		jsonEqual(r.Metadata, other.Metadata) &&
		r.Version == other.Version
}

func queryActivitySpan(t *testing.T, fixture *projectorFixture, activityKey string) activitySpanRow {
	t.Helper()

	var row activitySpanRow
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT status, status_message, end_time, output, metadata, version
		FROM public.spans
		WHERE trace_id = $1
		  AND span_id = $2
	`, fixture.traceID, activitySpanExternalID(fixture.run.ID, activityKey)).Scan(
		&row.Status, &row.StatusMessage, &row.EndTime, &row.Output, &row.Metadata, &row.Version,
	); err != nil {
		t.Fatalf("query activity span: %v", err)
	}
	return row
}

func countSpanEvents(t *testing.T, fixture *projectorFixture) int {
	t.Helper()

	var count int
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*)
		FROM public.span_events
		WHERE trace_id = $1
	`, fixture.traceID).Scan(&count); err != nil {
		t.Fatalf("count span events: %v", err)
	}
	return count
}
