package workflow

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestReplayDefinitionHappyPath(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"name": "Ada"})
	output, _ := json.Marshal(map[string]string{"greeting": "hello, Ada"})
	signal, _ := json.Marshal(map[string]string{"approval": "yes"})
	result, _ := json.Marshal(map[string]string{"greeting": "hello, Ada", "approval": "yes"})
	timerAt := time.Unix(0, 0).UTC()

	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-1",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventActivityScheduled, enginehistory.ActivityScheduledPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			Input:        input,
		}),
		historyRow(t, 3, enginehistory.EventActivityCompleted, enginehistory.ActivityCompletedPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			Output:       output,
		}),
		historyRow(t, 4, enginehistory.EventTimerScheduled, enginehistory.TimerScheduledPayload{
			TimerKey: "deadline",
			DueAt:    timerAt,
		}),
		historyRow(t, 5, enginehistory.EventTimerFired, enginehistory.TimerFiredPayload{
			TimerKey: "deadline",
		}),
		historyRow(t, 6, enginehistory.EventSignalReceived, enginehistory.SignalReceivedPayload{
			SignalName: "approval",
			Payload:    signal,
		}),
		historyRow(t, 7, enginehistory.EventWorkflowCompleted, enginehistory.WorkflowCompletedPayload{
			Result: result,
		}),
	}

	decision, err := replayDefinition(testDefinition(timerAt), historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected no new history events during replay, got %+v", decision.Events)
	}
	if !equalJSON(decision.Result, result) {
		t.Fatalf("expected replayed result %s, got %s", result, decision.Result)
	}
}

func TestReplayDefinitionMismatchProducesFailureDecision(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"name": "Ada"})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-1",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventActivityScheduled, enginehistory.ActivityScheduledPayload{
			ActivityKey:  "different",
			ActivityType: "demo.activity",
			Input:        input,
		}),
	}

	decision, err := replayDefinition(testDefinition(time.Unix(0, 0).UTC()), historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionFailed {
		t.Fatalf("expected failed decision, got %+v", decision)
	}
	if len(decision.Events) != 2 {
		t.Fatalf("expected replay mismatch + workflow failed events, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventWorkflowReplayMismatch {
		t.Fatalf("expected first replay event to be workflow.replay_mismatch, got %+v", decision.Events)
	}
	if decision.Events[1].EventType != enginehistory.EventWorkflowFailed {
		t.Fatalf("expected second replay event to be workflow.failed, got %+v", decision.Events)
	}
}

func TestReplayDefinitionCancellationRequestedRespectsHistoryOrder(t *testing.T) {
	result, _ := json.Marshal(map[string]bool{"early": false, "late": true})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "cancel-order",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-cancel-order",
		}),
		historyRow(t, 2, enginehistory.EventCustomStatusUpdated, enginehistory.CustomStatusUpdatedPayload{
			Status: mustRawJSON(t, map[string]string{"step": "before-cancel"}),
		}),
		historyRow(t, 3, enginehistory.EventCancelRequested, enginehistory.CancelRequestedPayload{}),
		historyRow(t, 4, enginehistory.EventWorkflowCompleted, enginehistory.WorkflowCompletedPayload{
			Result: result,
		}),
	}

	definition := publicworkflow.Definition{
		Name:    "cancel-order",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			early := ctx.CancellationRequested()
			if err := ctx.SetCustomStatus(map[string]string{"step": "before-cancel"}); err != nil {
				return err
			}
			late := ctx.CancellationRequested()
			return ctx.SetResult(map[string]bool{
				"early": early,
				"late":  late,
			})
		},
	}

	decision, err := replayDefinition(definition, historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected no new history events during replay, got %+v", decision.Events)
	}
	if !equalJSON(decision.Result, result) {
		t.Fatalf("expected replayed result %s, got %s", result, decision.Result)
	}
}

func TestReplayDefinitionCancellationRequestedFoldsPendingCancelAtFrontier(t *testing.T) {
	cancelInboxID := uuid.Must(uuid.NewV7())
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "cancel-pending",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-cancel-pending",
		}),
	}
	inboxRows := []enginedb.EngineInbox{{
		ID:          cancelInboxID,
		RunID:       pgtype.UUID{Bytes: uuid.Nil, Valid: false},
		Kind:        "cancel",
		Payload:     mustRawJSON(t, enginehistory.CancelRequestedPayload{}),
		Status:      enginedb.EngineInboxStatusPending,
		AvailableAt: time.Unix(1, 0).UTC(),
	}}
	result := mustRawJSON(t, map[string]bool{"cancelled": true})

	definition := publicworkflow.Definition{
		Name:    "cancel-pending",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			return ctx.SetResult(map[string]bool{
				"cancelled": ctx.CancellationRequested(),
			})
		},
	}

	decision, err := replayDefinition(definition, historyRows, nil, inboxRows)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}
	if len(decision.Events) != 2 {
		t.Fatalf("expected cancel.requested + workflow.completed, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventCancelRequested {
		t.Fatalf("expected first event to be cancel.requested, got %+v", decision.Events)
	}
	if decision.Events[1].EventType != enginehistory.EventWorkflowCompleted {
		t.Fatalf("expected second event to be workflow.completed, got %+v", decision.Events)
	}
	if len(decision.ConsumedInboxIDs) != 1 || decision.ConsumedInboxIDs[0] != cancelInboxID {
		t.Fatalf("expected pending cancel inbox to be consumed, got %+v", decision.ConsumedInboxIDs)
	}
	if !equalJSON(decision.Result, result) {
		t.Fatalf("expected completed result %s, got %s", result, decision.Result)
	}
}

func historyRow(t *testing.T, sequenceNo int32, eventType string, payload any) enginedb.EngineHistory {
	t.Helper()
	raw, err := enginehistory.MarshalPayload(payload)
	if err != nil {
		t.Fatalf("MarshalPayload() error = %v", err)
	}

	return enginedb.EngineHistory{
		ID:         int64(sequenceNo),
		ProjectID:  uuid.Nil,
		InstanceID: uuid.Nil,
		RunID:      uuid.Nil,
		SequenceNo: sequenceNo,
		EventType:  eventType,
		Payload:    raw,
		CreatedAt:  time.Unix(int64(sequenceNo), 0).UTC(),
	}
}

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func testDefinition(timerAt time.Time) publicworkflow.Definition {
	return publicworkflow.Definition{
		Name:    "demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input map[string]string
			if err := ctx.Input(&input); err != nil {
				return err
			}
			var output map[string]string
			if err := ctx.Activity("fetch", "demo.activity", input, &output); err != nil {
				return err
			}
			if err := ctx.SleepUntil("deadline", timerAt); err != nil {
				return err
			}
			var signal map[string]string
			if err := ctx.ReceiveSignal("approval", &signal); err != nil {
				return err
			}
			return ctx.SetResult(map[string]string{
				"greeting": output["greeting"],
				"approval": signal["approval"],
			})
		},
	}
}
