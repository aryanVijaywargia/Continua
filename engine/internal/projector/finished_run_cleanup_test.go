package projector

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	engineactivity "github.com/continua-ai/continua/engine/internal/activity"
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
		(r.StatusMessage == nil) == (other.StatusMessage == nil) &&
		(r.StatusMessage == nil || *r.StatusMessage == *other.StatusMessage) &&
		(r.EndTime == nil) == (other.EndTime == nil) &&
		(r.EndTime == nil || r.EndTime.Equal(*other.EndTime)) &&
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

// A worker can finish an activity task after the run's last activation began, so no
// activation records the outcome in history. When the run then fails without a replay
// (definition_version_mismatch appends only workflow.failed), the projector pass for
// the terminal history row closes the activity span with the task's own result, and
// projecting that row again changes nothing (#364).
func TestProjectorPollOnce_FailedRunClosesUnrecordedActivityOutcome(t *testing.T) {
	outcomes := []struct {
		name          string
		handler       engineactivity.Handler
		taskStatus    enginedb.EngineActivityTaskStatus
		spanStatus    string
		statusMessage *string
		output        map[string]any
		decision      map[string]any
	}{
		{
			name: "completed",
			handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{"charged":true}`), nil
			},
			taskStatus: enginedb.EngineActivityTaskStatusCompleted,
			spanStatus: "completed",
			output:     map[string]any{"charged": true},
			decision:   map[string]any{"question": "activity:charge:outcome", "chosen": "completed"},
		},
		{
			name: "failed",
			handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return nil, publicworkflow.NonRetryableError(errors.New("card declined"))
			},
			taskStatus:    enginedb.EngineActivityTaskStatusFailed,
			spanStatus:    "failed",
			statusMessage: stringPtr("card declined"),
			output:        map[string]any{"error_code": "activity_failed", "error_message": "card declined"},
			decision: map[string]any{
				"question":  "activity:charge:outcome",
				"chosen":    "failed",
				"reasoning": "card declined",
			},
		},
	}
	for _, outcome := range outcomes {
		t.Run(outcome.name, func(t *testing.T) {
			fixture := newProjectorFixture(t)
			activate(t, fixture, publicworkflow.Definition{
				Name:    "projector-demo",
				Version: "v1",
				Run: func(ctx publicworkflow.Context) error {
					return ctx.Activity("charge", "demo.charge", nil, nil)
				},
			})
			if err := fixture.projector.PollOnce(fixture.ctx, "projector-open-work"); err != nil {
				t.Fatalf("PollOnce() open work error = %v", err)
			}

			activities, err := engineactivity.NewRegistry(map[string]engineactivity.Handler{"demo.charge": outcome.handler})
			if err != nil {
				t.Fatalf("NewRegistry(activities) error = %v", err)
			}
			worker := engineactivity.NewWorker(fixture.store, activities, time.Minute, nil)
			if claimed, err := worker.PollOnce(fixture.ctx, "activity-worker"); err != nil || !claimed {
				t.Fatalf("activity worker PollOnce() = %v, %v; want a claimed task", claimed, err)
			}
			// The definition version is gone, so the activation fails the run without
			// replaying it, and no history row records the task's outcome.
			activate(t, fixture)

			finished, err := fixture.store.GetRun(fixture.ctx, fixture.run.ID)
			if err != nil {
				t.Fatalf("GetRun() error = %v", err)
			}
			if finished.Status != enginedb.EngineRunLifecycleStatusFailed {
				t.Fatalf("run status = %s, want failed", finished.Status)
			}
			history, err := fixture.store.GetHistoryByRun(fixture.ctx, fixture.run.ID)
			if err != nil {
				t.Fatalf("GetHistoryByRun() error = %v", err)
			}
			for i := range history {
				switch history[i].EventType {
				case publichistory.EventActivityCompleted, publichistory.EventActivityFailed:
					t.Fatalf("history records the activity outcome (%s); the test needs an unrecorded one", history[i].EventType)
				}
			}
			terminal := history[len(history)-1]
			if terminal.EventType != publichistory.EventWorkflowFailed {
				t.Fatalf("last history event = %s, want %s", terminal.EventType, publichistory.EventWorkflowFailed)
			}
			task, err := fixture.store.GetActivityTaskByRunAndKey(fixture.ctx, enginedb.GetActivityTaskByRunAndKeyParams{
				RunID:       fixture.run.ID,
				ActivityKey: "charge",
			})
			if err != nil {
				t.Fatalf("GetActivityTaskByRunAndKey() error = %v", err)
			}
			if task.Status != outcome.taskStatus || !task.CompletedAt.Valid {
				t.Fatalf("task status = %s, completed at %v; want %s with a completion time", task.Status, task.CompletedAt, outcome.taskStatus)
			}

			if err := fixture.projector.PollOnce(fixture.ctx, "projector-failed-run"); err != nil {
				t.Fatalf("PollOnce() failed run error = %v", err)
			}
			span := queryActivitySpan(t, fixture, "charge")
			if span.Status != outcome.spanStatus {
				t.Fatalf("activity span status = %q, want %q", span.Status, outcome.spanStatus)
			}
			if (span.StatusMessage == nil) != (outcome.statusMessage == nil) ||
				(span.StatusMessage != nil && *span.StatusMessage != *outcome.statusMessage) {
				t.Fatalf("activity span status message = %v, want %v", span.StatusMessage, outcome.statusMessage)
			}
			if span.EndTime == nil || !span.EndTime.Equal(task.CompletedAt.Time) {
				t.Fatalf("activity span end time = %v, want the task completion time %v", span.EndTime, task.CompletedAt.Time)
			}
			if !jsonEqual(span.Output, mustJSON(t, outcome.output)) {
				t.Fatalf("activity span output = %s, want %v", span.Output, outcome.output)
			}
			decisions := queryActivityDecisionEvents(t, fixture)
			if len(decisions) != 1 || !jsonEqual(decisions[0], mustJSON(t, outcome.decision)) {
				t.Fatalf("activity decision events = %s, want one %v", decisions, outcome.decision)
			}
			waits := queryResolvedWaitEvents(t, fixture, outcome.spanStatus)
			want := resolvedWaitEvent{Sequence: terminal.SequenceNo*10 + 2, WaitID: "activity:charge"}
			if len(waits) != 1 || waits[0] != want {
				t.Fatalf("%s waits = %+v, want %+v", outcome.spanStatus, waits, want)
			}
			eventCount := countSpanEvents(t, fixture)

			// Crash recovery projects the terminal history row again.
			fixture.setTraceProjection(terminal.ID, history[len(history)-2].ID, publicprojection.StateCatchingUp.String())
			if err := fixture.projector.PollOnce(fixture.ctx, "projector-failed-run-again"); err != nil {
				t.Fatalf("PollOnce() re-projection error = %v", err)
			}
			assertProjectedDetailState(t, fixture, publicprojection.StateUpToDate.String(), terminal.ID, 1, eventCount)
			if again := queryActivitySpan(t, fixture, "charge"); !again.equal(&span) {
				t.Fatalf("activity span after re-projection = %+v, want %+v", again, span)
			}
		})
	}
}

func queryActivityDecisionEvents(t *testing.T, fixture *projectorFixture) [][]byte {
	t.Helper()

	rows, err := fixture.db.Pool.Query(fixture.ctx, `
		SELECT payload
		FROM public.span_events
		WHERE trace_id = $1
		  AND event_type = 'decision'
		ORDER BY sequence ASC, created_at ASC
	`, fixture.traceID)
	if err != nil {
		t.Fatalf("query decision events: %v", err)
	}
	defer rows.Close()

	var payloads [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan decision event: %v", err)
		}
		payloads = append(payloads, payload)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate decision events: %v", err)
	}
	return payloads
}
