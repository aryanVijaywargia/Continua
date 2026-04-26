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

func TestReplayDefinitionDemoProgression(t *testing.T) {
	t.Run("beginner schedules first activity", func(t *testing.T) {
		input := mustRawJSON(t, map[string]string{"name": "Ada"})
		historyRows := []enginedb.EngineHistory{
			historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
				DefinitionName:    "demo-beginner",
				DefinitionVersion: "v1",
				InstanceKey:       "instance-demo-beginner",
				Input:             input,
			}),
		}

		decision, err := replayDefinition(publicworkflow.Definition{
			Name:    "demo-beginner",
			Version: "v1",
			Run: func(ctx publicworkflow.Context) error {
				var workflowInput map[string]string
				if err := ctx.Input(&workflowInput); err != nil {
					return err
				}
				var activityOutput map[string]string
				if err := ctx.Activity("greet", "demo.greet", workflowInput, &activityOutput); err != nil {
					return err
				}
				return ctx.SetResult(activityOutput)
			},
		}, historyRows, nil, nil)
		if err != nil {
			t.Fatalf("replayDefinition() error = %v", err)
		}
		if decision.Kind != decisionWaiting {
			t.Fatalf("expected waiting decision, got %+v", decision)
		}
		if decision.NewActivity == nil {
			t.Fatalf("expected a new activity to be scheduled, got %+v", decision)
		}
		if len(decision.Events) != 1 || decision.Events[0].EventType != enginehistory.EventActivityScheduled {
			t.Fatalf("expected one activity.scheduled event, got %+v", decision.Events)
		}

		var wait enginehistory.ActivityWait
		if err := json.Unmarshal(decision.WaitingFor, &wait); err != nil {
			t.Fatalf("json.Unmarshal(waiting_for) error = %v", err)
		}
		if wait.Kind != enginehistory.WaitKindActivity || wait.ActivityKey != "greet" {
			t.Fatalf("expected activity wait for greet, got %+v", wait)
		}
	})

	t.Run("intermediate consumes queued signal and completes", func(t *testing.T) {
		input := mustRawJSON(t, map[string]string{"ticket": "case-1"})
		signalPayload := mustRawJSON(t, map[string]string{"approval": "granted"})
		historyRows := []enginedb.EngineHistory{
			historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
				DefinitionName:    "demo-intermediate",
				DefinitionVersion: "v1",
				InstanceKey:       "instance-demo-intermediate",
				Input:             input,
			}),
		}
		inboxRows := []enginedb.EngineInbox{
			{
				ID:          uuid.New(),
				ProjectID:   uuid.Nil,
				InstanceID:  uuid.Nil,
				RunID:       pgtype.UUID{},
				Kind:        "signal",
				Payload:     mustRawJSON(t, enginehistory.SignalReceivedPayload{SignalName: "approval", Payload: signalPayload}),
				Status:      enginedb.EngineInboxStatusPending,
				AvailableAt: time.Unix(0, 0).UTC(),
				CreatedAt:   time.Unix(0, 0).UTC(),
				UpdatedAt:   time.Unix(0, 0).UTC(),
			},
		}

		decision, err := replayDefinition(publicworkflow.Definition{
			Name:    "demo-intermediate",
			Version: "v1",
			Run: func(ctx publicworkflow.Context) error {
				var approval map[string]string
				if err := ctx.ReceiveSignal("approval", &approval); err != nil {
					return err
				}
				return ctx.SetResult(map[string]string{"approval": approval["approval"]})
			},
		}, historyRows, nil, inboxRows)
		if err != nil {
			t.Fatalf("replayDefinition() error = %v", err)
		}
		if decision.Kind != decisionCompleted {
			t.Fatalf("expected completed decision, got %+v", decision)
		}
		if len(decision.Events) != 2 {
			t.Fatalf("expected signal.received and workflow.completed events, got %+v", decision.Events)
		}
		if decision.Events[0].EventType != enginehistory.EventSignalReceived {
			t.Fatalf("expected first event to be signal.received, got %+v", decision.Events)
		}
		if decision.Events[1].EventType != enginehistory.EventWorkflowCompleted {
			t.Fatalf("expected second event to be workflow.completed, got %+v", decision.Events)
		}
		if len(decision.ConsumedInboxIDs) != 1 || decision.ConsumedInboxIDs[0] != inboxRows[0].ID {
			t.Fatalf("expected queued signal inbox item to be consumed, got %+v", decision.ConsumedInboxIDs)
		}
		if !equalJSON(decision.Result, mustRawJSON(t, map[string]string{"approval": "granted"})) {
			t.Fatalf("expected completed result to reflect signal payload, got %s", decision.Result)
		}
	})

	t.Run("advanced child workflow round trip completes", func(t *testing.T) {
		projectID := uuid.New()
		runID := uuid.New()
		childInstanceID := uuid.New()
		childRunID := uuid.New()
		input := mustRawJSON(t, map[string]string{"order_id": "ord-demo-advanced"})
		childOutput := mustRawJSON(t, map[string]string{"status": "authorized"})
		result := mustRawJSON(t, map[string]string{"status": "authorized"})
		childInstanceKey := defaultChildInstanceKey(projectID, runID, "charge-card")

		historyRows := []enginedb.EngineHistory{
			historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
				DefinitionName:    "demo-advanced",
				DefinitionVersion: "v1",
				InstanceKey:       "instance-demo-advanced",
				Input:             input,
			}),
			historyRow(t, 2, enginehistory.EventChildWorkflowScheduled, enginehistory.ChildWorkflowScheduledPayload{
				ChildKey:          "charge-card",
				DefinitionName:    "billing",
				DefinitionVersion: "v1",
				Input:             input,
				ChildInstanceKey:  childInstanceKey,
			}),
			historyRow(t, 3, enginehistory.EventChildWorkflowStarted, enginehistory.ChildWorkflowStartedPayload{
				ChildKey:         "charge-card",
				ChildInstanceID:  childInstanceID.String(),
				ChildInstanceKey: childInstanceKey,
				ChildRunID:       childRunID.String(),
				RootRunID:        runID.String(),
				ChildDepth:       1,
			}),
		}

		decision, err := replayDefinitionForRun(publicworkflow.Definition{
			Name:    "demo-advanced",
			Version: "v1",
			Run: func(ctx publicworkflow.Context) error {
				var workflowInput map[string]string
				if err := ctx.Input(&workflowInput); err != nil {
					return err
				}
				var childResult map[string]string
				if err := ctx.ChildWorkflow("charge-card", "billing", "v1", workflowInput, &childResult); err != nil {
					return err
				}
				return ctx.SetResult(map[string]string{"status": childResult["status"]})
			},
		}, &enginedb.EngineRun{
			ID:        runID,
			ProjectID: projectID,
			RootRunID: runID,
		}, historyRows, nil, []enginedb.ListChildWorkflowOutcomesByParentRunRow{
			{
				ChildKey:                   "charge-card",
				RequestedDefinitionName:    "billing",
				RequestedDefinitionVersion: "v1",
				ChildInstanceID:            childInstanceID,
				ChildInstanceKey:           childInstanceKey,
				CurrentChildRunID:          childRunID,
				TerminalChildRunID:         pgtype.UUID{Bytes: childRunID, Valid: true},
				RootRunID:                  runID,
				ChildDepth:                 1,
				Status:                     enginedb.EngineChildWorkflowStatusCompleted,
				TerminalResult:             childOutput,
			},
		}, nil)
		if err != nil {
			t.Fatalf("replayDefinitionForRun() error = %v", err)
		}
		if decision.Kind != decisionCompleted {
			t.Fatalf("expected completed decision, got %+v", decision)
		}
		if len(decision.Events) != 2 {
			t.Fatalf("expected child_workflow.completed and workflow.completed events, got %+v", decision.Events)
		}
		if decision.Events[0].EventType != enginehistory.EventChildWorkflowCompleted {
			t.Fatalf("expected first event to be child_workflow.completed, got %+v", decision.Events)
		}
		if decision.Events[1].EventType != enginehistory.EventWorkflowCompleted {
			t.Fatalf("expected second event to be workflow.completed, got %+v", decision.Events)
		}
		if !equalJSON(decision.Result, result) {
			t.Fatalf("expected completed result %s, got %s", result, decision.Result)
		}
	})
}
