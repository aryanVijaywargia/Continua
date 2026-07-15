package workflow

import (
	"encoding/json"
	"errors"
	"strings"
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
	if !equalJSONForTest(t, decision.Result, result) {
		t.Fatalf("expected replayed result %s, got %s", result, decision.Result)
	}
}

func TestReplayDefinitionNilActivityInputRoundTrip(t *testing.T) {
	historyRows := make([]enginedb.EngineHistory, 0, 2)
	historyRows = append(historyRows,
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "nil-input",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-1",
		}),
	)
	definition := publicworkflow.Definition{
		Name:    "nil-input",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var out string
			if err := ctx.Activity("nil-activity", "demo.nil", nil, &out); err != nil {
				return err
			}
			return ctx.SetResult(out)
		},
	}

	firstDecision, err := replayDefinition(definition, historyRows, nil, nil)
	if err != nil {
		t.Fatalf("first replayDefinition() error = %v", err)
	}
	if firstDecision.Kind != decisionWaiting {
		t.Fatalf("expected waiting decision, got %+v", firstDecision)
	}
	if len(firstDecision.Events) != 1 || firstDecision.Events[0].EventType != enginehistory.EventActivityScheduled {
		t.Fatalf("expected activity scheduled event, got %+v", firstDecision.Events)
	}

	historyRows = append(historyRows, enginedb.EngineHistory{
		SequenceNo: 2,
		EventType:  firstDecision.Events[0].EventType,
		Payload:    firstDecision.Events[0].Payload,
	})
	output := mustRawJSON(t, "done")
	activityTasks := []enginedb.EngineActivityTask{{
		ActivityKey:  "nil-activity",
		ActivityType: "demo.nil",
		Output:       output,
		Status:       enginedb.EngineActivityTaskStatusCompleted,
	}}
	secondDecision, err := replayDefinition(definition, historyRows, activityTasks, nil)
	if err != nil {
		t.Fatalf("second replayDefinition() error = %v", err)
	}
	if secondDecision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", secondDecision)
	}
	if len(secondDecision.Events) != 2 {
		t.Fatalf("expected activity completed + workflow completed events, got %+v", secondDecision.Events)
	}
	if secondDecision.Events[0].EventType != enginehistory.EventActivityCompleted {
		t.Fatalf("expected activity completed event, got %+v", secondDecision.Events)
	}
	if secondDecision.Events[1].EventType != enginehistory.EventWorkflowCompleted {
		t.Fatalf("expected workflow completed event, got %+v", secondDecision.Events)
	}
}

func TestReplayDefinitionChildWorkflowRoundTrip(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	childInstanceID := uuid.New()
	childRunID := uuid.New()
	input := mustRawJSON(t, map[string]string{"order_id": "ord-123"})
	childInput := mustRawJSON(t, map[string]string{"order_id": "ord-123"})
	childOutput := mustRawJSON(t, map[string]string{"status": "authorized"})
	result := mustRawJSON(t, map[string]string{"status": "authorized"})
	childInstanceKey := defaultChildInstanceKey(projectID, runID, "charge-card")

	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "checkout",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-checkout",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventChildWorkflowScheduled, enginehistory.ChildWorkflowScheduledPayload{
			ChildKey:          "charge-card",
			DefinitionName:    "billing",
			DefinitionVersion: "v1",
			Input:             childInput,
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
		historyRow(t, 4, enginehistory.EventChildWorkflowCompleted, enginehistory.ChildWorkflowCompletedPayload{
			ChildKey:           "charge-card",
			ChildInstanceID:    childInstanceID.String(),
			TerminalChildRunID: childRunID.String(),
			Result:             childOutput,
		}),
		historyRow(t, 5, enginehistory.EventWorkflowCompleted, enginehistory.WorkflowCompletedPayload{
			Result: result,
		}),
	}

	decision, err := replayDefinitionForRun(publicworkflow.Definition{
		Name:    "checkout",
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
	}, historyRows, nil, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinitionForRun() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected no new history events during child workflow replay, got %+v", decision.Events)
	}
	if !equalJSONForTest(t, decision.Result, result) {
		t.Fatalf("expected replayed result %s, got %s", result, decision.Result)
	}
}

func TestReplayDefinitionConsumesFailedPendingChildOutcomeBeforeReturningError(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	childInstanceID := uuid.New()
	childRunID := uuid.New()
	input := mustRawJSON(t, map[string]string{"order_id": "ord-123"})
	childInput := mustRawJSON(t, map[string]string{"order_id": "ord-123"})
	childInstanceKey := defaultChildInstanceKey(projectID, runID, "charge-card")
	errorCode := "card_declined"
	errorMessage := "card declined"

	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "checkout",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-checkout",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventChildWorkflowScheduled, enginehistory.ChildWorkflowScheduledPayload{
			ChildKey:          "charge-card",
			DefinitionName:    "billing",
			DefinitionVersion: "v1",
			Input:             childInput,
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
		historyRow(t, 4, enginehistory.EventChildWorkflowScheduled, enginehistory.ChildWorkflowScheduledPayload{
			ChildKey:          "charge-card",
			DefinitionName:    "billing",
			DefinitionVersion: "v1",
			Input:             childInput,
			ChildInstanceKey:  childInstanceKey,
		}),
		historyRow(t, 5, enginehistory.EventChildWorkflowStarted, enginehistory.ChildWorkflowStartedPayload{
			ChildKey:         "charge-card",
			ChildInstanceID:  childInstanceID.String(),
			ChildInstanceKey: childInstanceKey,
			ChildRunID:       childRunID.String(),
			RootRunID:        runID.String(),
			ChildDepth:       1,
		}),
	}

	decision, err := replayDefinitionForRun(publicworkflow.Definition{
		Name:    "checkout",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var workflowInput map[string]string
			if err := ctx.Input(&workflowInput); err != nil {
				return err
			}
			for i := 0; i < 2; i++ {
				var childResult map[string]string
				err := ctx.ChildWorkflow("charge-card", "billing", "v1", workflowInput, &childResult)
				if err == nil {
					continue
				}
				var childErr *publicworkflow.ChildWorkflowError
				if !errors.As(err, &childErr) {
					return err
				}
				if childErr.Code() != errorCode {
					return err
				}
			}
			return ctx.SetResult(map[string]string{"status": "handled"})
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
			Status:                     enginedb.EngineChildWorkflowStatusFailed,
			TerminalLastErrorCode:      &errorCode,
			TerminalLastErrorMessage:   &errorMessage,
			TerminalRunStatus: enginedb.NullEngineRunLifecycleStatus{
				EngineRunLifecycleStatus: enginedb.EngineRunLifecycleStatusFailed,
				Valid:                    true,
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("replayDefinitionForRun() error = %v", err)
	}
	if decision.Kind != decisionWaiting {
		t.Fatalf("expected pending child outcome to be consumed before second wait, got %+v", decision)
	}
	if len(decision.Events) != 1 {
		t.Fatalf("expected one child failure event, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventChildWorkflowFailed {
		t.Fatalf("expected child workflow failure event, got %+v", decision.Events[0])
	}
	var wait enginehistory.ChildWorkflowWait
	if err := json.Unmarshal(decision.WaitingFor, &wait); err != nil {
		t.Fatalf("decode waiting state: %v", err)
	}
	if wait.Kind != enginehistory.WaitKindChildWorkflow || wait.ChildKey != "charge-card" {
		t.Fatalf("expected second child workflow wait, got %+v", wait)
	}
}

func TestReplayDefinitionRejectsChildWorkflowAtMaxDepth(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	input := mustRawJSON(t, map[string]string{"order_id": "ord-depth"})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "checkout",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-depth",
			Input:             input,
		}),
	}

	decision, err := replayDefinitionForRun(publicworkflow.Definition{
		Name:    "checkout",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var workflowInput map[string]string
			if err := ctx.Input(&workflowInput); err != nil {
				return err
			}
			var childResult map[string]string
			return ctx.ChildWorkflow("too-deep", "billing", "v1", workflowInput, &childResult)
		},
	}, &enginedb.EngineRun{
		ID:         runID,
		ProjectID:  projectID,
		RootRunID:  runID,
		ChildDepth: maxChildDepth,
	}, historyRows, nil, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinitionForRun() error = %v", err)
	}
	if decision.Kind != decisionFailed {
		t.Fatalf("expected failed decision, got %+v", decision)
	}
	if decision.FailureCode != "max_child_depth_exceeded" {
		t.Fatalf("expected max depth failure code, got %+v", decision)
	}
	if len(decision.Events) != 1 || decision.Events[0].EventType != enginehistory.EventWorkflowFailed {
		t.Fatalf("expected workflow.failed event, got %+v", decision.Events)
	}
}

func TestChildDepthLimitConfigurable(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	input := mustRawJSON(t, map[string]string{"order_id": "ord-configurable-depth"})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "checkout",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-configurable-depth",
			Input:             input,
		}),
	}
	definition := publicworkflow.Definition{
		Name:    "checkout",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var workflowInput map[string]string
			if err := ctx.Input(&workflowInput); err != nil {
				return err
			}
			var childResult map[string]string
			return ctx.ChildWorkflow("too-deep", "billing", "v1", workflowInput, &childResult)
		},
	}
	run := &enginedb.EngineRun{
		ID:         runID,
		ProjectID:  projectID,
		RootRunID:  runID,
		ChildDepth: 3,
	}

	limitedDecision, err := replayDefinitionForRunWithDepthLimits(
		definition,
		run,
		historyRows,
		nil,
		nil,
		nil,
		DepthLimits{MaxChildDepth: 3, MaxContinuationFollowDepth: 32},
	)
	if err != nil {
		t.Fatalf("replayDefinitionForRunWithDepthLimits() error = %v", err)
	}
	if limitedDecision.Kind != decisionFailed {
		t.Errorf("configured-limit decision kind = %q, want %q", limitedDecision.Kind, decisionFailed)
	}
	if limitedDecision.FailureCode != "max_child_depth_exceeded" {
		t.Errorf("configured-limit failure code = %q, want max_child_depth_exceeded", limitedDecision.FailureCode)
	}
	if len(limitedDecision.Events) != 1 || limitedDecision.Events[0].EventType != enginehistory.EventWorkflowFailed {
		t.Errorf("configured-limit events = %+v, want one workflow.failed event", limitedDecision.Events)
	}

	defaultDecision, err := replayDefinitionForRun(definition, run, historyRows, nil, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinitionForRun() error = %v", err)
	}
	if defaultDecision.Kind != decisionWaiting {
		t.Errorf("default-limit decision kind = %q, want %q", defaultDecision.Kind, decisionWaiting)
	}
	if defaultDecision.NewChildWorkflow == nil {
		t.Error("default-limit decision did not schedule a child workflow")
	}
	if len(defaultDecision.Events) != 1 || defaultDecision.Events[0].EventType != enginehistory.EventChildWorkflowScheduled {
		t.Errorf("default-limit events = %+v, want one child_workflow.scheduled event", defaultDecision.Events)
	}
}

func TestReplayDefinitionFailsDeterministicallyWhenNewChildValidatorRejectsBinding(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	input := mustRawJSON(t, map[string]string{"order_id": "ord-conflict"})

	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "checkout",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-conflict",
			Input:             input,
		}),
	}

	decision, err := replayDefinitionForRunWithValidator(
		publicworkflow.Definition{
			Name:    "checkout",
			Version: "v1",
			Run: func(ctx publicworkflow.Context) error {
				var workflowInput map[string]string
				if err := ctx.Input(&workflowInput); err != nil {
					return err
				}
				var childResult map[string]string
				return ctx.ChildWorkflowWithOptions(
					"charge-card",
					"billing",
					"v2",
					workflowInput,
					&childResult,
					publicworkflow.ChildWorkflowOptions{InstanceKey: "custom-child-binding"},
				)
			},
		},
		&enginedb.EngineRun{
			ID:        runID,
			ProjectID: projectID,
			RootRunID: runID,
		},
		historyRows,
		nil,
		nil,
		nil,
		func(enginehistory.ChildWorkflowScheduledPayload) error {
			return codedWorkflowError{
				code:    "instance_conflict",
				message: "child instance key is already attached to a different workflow relationship",
			}
		},
	)
	if err != nil {
		t.Fatalf("replayDefinitionForRunWithValidator() error = %v", err)
	}
	if decision.Kind != decisionFailed {
		t.Fatalf("expected failed decision, got %+v", decision)
	}
	if decision.NewChildWorkflow != nil {
		t.Fatalf("expected rejected child binding not to queue child creation, got %+v", decision.NewChildWorkflow)
	}
	if len(decision.Events) != 1 || decision.Events[0].EventType != enginehistory.EventWorkflowFailed {
		t.Fatalf("expected only workflow.failed event, got %+v", decision.Events)
	}
	var failure enginehistory.WorkflowFailedPayload
	if err := enginehistory.UnmarshalPayload(decision.Events[0].Payload, &failure); err != nil {
		t.Fatalf("UnmarshalPayload(workflow.failed) error = %v", err)
	}
	if failure.ErrorCode != "instance_conflict" {
		t.Fatalf("expected instance_conflict failure, got %+v", failure)
	}
}

func TestReplayDefinitionPendingChildTerminalOutcomesExposeResultAndErrors(t *testing.T) {
	testCases := []struct {
		name          string
		status        enginedb.EngineChildWorkflowStatus
		errorCode     string
		errorMessage  string
		terminalState string
		wantCompleted bool
		wantEvent     string
	}{
		{
			name:          "completed",
			status:        enginedb.EngineChildWorkflowStatusCompleted,
			wantCompleted: true,
			wantEvent:     enginehistory.EventChildWorkflowCompleted,
		},
		{
			name:          "failed",
			status:        enginedb.EngineChildWorkflowStatusFailed,
			errorCode:     "card_declined",
			errorMessage:  "card declined",
			terminalState: "failed",
			wantEvent:     enginehistory.EventChildWorkflowFailed,
		},
		{
			name:          "cancelled",
			status:        enginedb.EngineChildWorkflowStatusCancelled,
			errorCode:     "cancelled",
			errorMessage:  "workflow cancelled",
			terminalState: "cancelled",
			wantEvent:     enginehistory.EventChildWorkflowCancelled,
		},
		{
			name:          "terminated",
			status:        enginedb.EngineChildWorkflowStatusTerminated,
			errorCode:     "terminated",
			errorMessage:  "workflow terminated",
			terminalState: "terminated",
			wantEvent:     enginehistory.EventChildWorkflowTerminated,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			projectID := uuid.New()
			runID := uuid.New()
			childInstanceID := uuid.New()
			childRunID := uuid.New()
			input := mustRawJSON(t, map[string]string{"order_id": "ord-terminal"})
			childOutput := mustRawJSON(t, map[string]string{"status": "authorized"})
			childInstanceKey := defaultChildInstanceKey(projectID, runID, "charge-card")
			errorCode := testCase.errorCode
			errorMessage := testCase.errorMessage
			historyRows := childWorkflowStartedHistory(t, runID, childInstanceID, childRunID, input, childInstanceKey)

			decision, err := replayDefinitionForRun(publicworkflow.Definition{
				Name:    "checkout",
				Version: "v1",
				Run: func(ctx publicworkflow.Context) error {
					var workflowInput map[string]string
					if err := ctx.Input(&workflowInput); err != nil {
						return err
					}
					var childResult map[string]string
					err := ctx.ChildWorkflow("charge-card", "billing", "v1", workflowInput, &childResult)
					if testCase.wantCompleted {
						if err != nil {
							return err
						}
						return ctx.SetResult(map[string]string{"status": childResult["status"]})
					}
					var childErr *publicworkflow.ChildWorkflowError
					if !errors.As(err, &childErr) {
						return err
					}
					if childErr.Code() != testCase.errorCode ||
						childErr.Message() != testCase.errorMessage ||
						childErr.TerminalState() != testCase.terminalState {
						return err
					}
					return ctx.SetResult(map[string]string{"handled": childErr.TerminalState()})
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
					Status:                     testCase.status,
					TerminalResult:             childOutput,
					TerminalLastErrorCode:      &errorCode,
					TerminalLastErrorMessage:   &errorMessage,
				},
			}, nil)
			if err != nil {
				t.Fatalf("replayDefinitionForRun() error = %v", err)
			}
			if decision.Kind != decisionCompleted {
				t.Fatalf("expected completed decision, got %+v", decision)
			}
			if len(decision.Events) != 2 {
				t.Fatalf("expected child terminal + workflow completed events, got %+v", decision.Events)
			}
			if decision.Events[0].EventType != testCase.wantEvent {
				t.Fatalf("expected child terminal event %s, got %+v", testCase.wantEvent, decision.Events)
			}
			if decision.Events[1].EventType != enginehistory.EventWorkflowCompleted {
				t.Fatalf("expected workflow.completed event, got %+v", decision.Events)
			}
		})
	}
}

func TestReplayDefinitionContinuationWaitFailedBeatsLateTerminalOutcome(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	childInstanceID := uuid.New()
	childRunID := uuid.New()
	terminalChildRunID := uuid.New()
	input := mustRawJSON(t, map[string]string{"order_id": "ord-continuation"})
	childInstanceKey := defaultChildInstanceKey(projectID, runID, "charge-card")
	historyRows := childWorkflowStartedHistory(t, runID, childInstanceID, childRunID, input, childInstanceKey)

	decision, err := replayDefinitionForRun(publicworkflow.Definition{
		Name:    "checkout",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var workflowInput map[string]string
			if err := ctx.Input(&workflowInput); err != nil {
				return err
			}
			var childResult map[string]string
			err := ctx.ChildWorkflow("charge-card", "billing", "v1", workflowInput, &childResult)
			var childErr *publicworkflow.ChildWorkflowError
			if !errors.As(err, &childErr) {
				return err
			}
			if childErr.Code() != childWaitFailedContinuationCode ||
				childErr.TerminalState() != "wait_failed" {
				return err
			}
			return ctx.SetResult(map[string]string{"handled": childErr.Code()})
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
			TerminalChildRunID:         pgtype.UUID{Bytes: terminalChildRunID, Valid: true},
			RootRunID:                  runID,
			ChildDepth:                 1,
			ContinuationCount:          maxContinuationFollowDepth,
			Status:                     enginedb.EngineChildWorkflowStatusCompleted,
			TerminalResult:             mustRawJSON(t, map[string]string{"status": "too-late"}),
		},
	}, nil)
	if err != nil {
		t.Fatalf("replayDefinitionForRun() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision after handled wait_failed, got %+v", decision)
	}
	if len(decision.Events) != 2 {
		t.Fatalf("expected wait_failed + workflow.completed events, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventChildWorkflowWaitFailed {
		t.Fatalf("expected child_workflow.wait_failed to beat late terminal outcome, got %+v", decision.Events)
	}
	if decision.Events[1].EventType != enginehistory.EventWorkflowCompleted {
		t.Fatalf("expected workflow.completed event, got %+v", decision.Events)
	}
	if len(decision.ChildWaitFailures) != 1 || decision.ChildWaitFailures[0].ChildKey != "charge-card" {
		t.Fatalf("expected child wait failure marker, got %+v", decision.ChildWaitFailures)
	}
}

func TestReplayDefinitionMismatchProducesQuarantineDecision(t *testing.T) {
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
	if decision.Kind != decisionQuarantined {
		t.Fatalf("expected quarantined decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected replay mismatch to queue no history events, got %+v", decision.Events)
	}
	if decision.FailureCode != "replay_mismatch" {
		t.Fatalf("expected replay_mismatch failure code, got %q", decision.FailureCode)
	}
	if decision.FailureMessage == "" {
		t.Fatal("expected replay mismatch failure message")
	}
	if len(decision.WaitingFor) == 0 {
		t.Fatal("expected replay mismatch waiting_for reason")
	}
	var wait struct {
		Kind   string `json:"kind"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(decision.WaitingFor, &wait); err != nil {
		t.Fatalf("decode waiting_for: %v", err)
	}
	if wait.Kind != "replay_mismatch" || wait.Detail == "" {
		t.Fatalf("expected replay_mismatch waiting_for detail, got %+v", wait)
	}
	if len(decision.ConsumedInboxIDs) != 0 {
		t.Fatalf("expected replay mismatch to consume no inbox rows, got %+v", decision.ConsumedInboxIDs)
	}
}

func TestReplayDefinitionContinueAsNewFirstExecution(t *testing.T) {
	input := mustRawJSON(t, map[string]any{"cursor": 1})
	continuationInput := mustRawJSON(t, map[string]any{"cursor": 2, "phase": "next"})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "continue-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-continue-demo",
			Input:             input,
		}),
	}

	decision, err := replayDefinition(continueAsNewDefinition(continuationInput), historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionContinuedAsNew {
		t.Fatalf("expected continued-as-new decision, got %+v", decision)
	}
	if len(decision.Events) != 1 {
		t.Fatalf("expected workflow.continued_as_new event, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventWorkflowContinuedAsNew {
		t.Fatalf("expected workflow.continued_as_new event, got %+v", decision.Events[0])
	}
	if !equalJSONForTest(t, decision.ContinuationInput, continuationInput) {
		t.Fatalf("expected continuation input %s, got %s", continuationInput, decision.ContinuationInput)
	}
}

func TestReplayDefinitionContinueAsNewMatchesRecordedHistory(t *testing.T) {
	input := mustRawJSON(t, map[string]any{"cursor": 1})
	continuationInput := mustRawJSON(t, map[string]any{"cursor": 2, "phase": "next"})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "continue-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-continue-demo",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventWorkflowContinuedAsNew, enginehistory.WorkflowContinuedAsNewPayload{
			Input: continuationInput,
		}),
	}

	decision, err := replayDefinition(continueAsNewDefinition(continuationInput), historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionContinuedAsNew {
		t.Fatalf("expected continued-as-new decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected replay to avoid new continuation events, got %+v", decision.Events)
	}
}

func TestReplayDefinitionContinueAsNewMatchesSemanticallyEquivalentJSON(t *testing.T) {
	input := mustRawJSON(t, map[string]any{"cursor": 1})
	recordedInput := json.RawMessage(`{"a":1,"b":2}`)
	replayedInput := json.RawMessage(`{"b":2,"a":1}`)
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "continue-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-continue-demo",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventWorkflowContinuedAsNew, enginehistory.WorkflowContinuedAsNewPayload{
			Input: recordedInput,
		}),
	}

	decision, err := replayDefinition(continueAsNewDefinition(replayedInput), historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionContinuedAsNew {
		t.Fatalf("expected continued-as-new decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected semantic JSON replay equality to avoid new events, got %+v", decision.Events)
	}
	if !equalJSONForTest(t, decision.ContinuationInput, recordedInput) {
		t.Fatalf("expected continuation input to match recorded JSON, got %s", decision.ContinuationInput)
	}
}

func TestReplayDefinitionMismatchAfterContinuedAsNewQuarantines(t *testing.T) {
	input := mustRawJSON(t, map[string]any{"cursor": 1})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "continue-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-continue-demo",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventWorkflowContinuedAsNew, enginehistory.WorkflowContinuedAsNewPayload{
			Input: mustRawJSON(t, map[string]any{"cursor": 2}),
		}),
	}

	decision, err := replayDefinition(publicworkflow.Definition{
		Name:    "continue-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			return ctx.SetResult(map[string]bool{"done": true})
		},
	}, historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionQuarantined {
		t.Fatalf("expected quarantined decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected replay mismatch to queue no history events, got %+v", decision.Events)
	}
}

func TestReplayDefinitionSkipsRecordedActivityRetryEvents(t *testing.T) {
	input := mustRawJSON(t, map[string]string{"name": "Ada"})
	output := mustRawJSON(t, map[string]string{"greeting": "hello, Ada"})

	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "retry-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-retry-demo",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventActivityScheduled, enginehistory.ActivityScheduledPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			Input:        input,
		}),
		historyRow(t, 3, enginehistory.EventActivityRetryScheduled, enginehistory.ActivityRetryScheduledPayload{
			ActivityKey:     "fetch",
			ActivityType:    "demo.activity",
			FailedAttempt:   1,
			NextAvailableAt: time.Unix(10, 0).UTC(),
			ErrorCode:       "activity_failed",
			ErrorMessage:    "first failure",
		}),
		historyRow(t, 4, enginehistory.EventActivityRetryScheduled, enginehistory.ActivityRetryScheduledPayload{
			ActivityKey:     "fetch",
			ActivityType:    "demo.activity",
			FailedAttempt:   2,
			NextAvailableAt: time.Unix(20, 0).UTC(),
			ErrorCode:       "activity_failed",
			ErrorMessage:    "second failure",
		}),
		historyRow(t, 5, enginehistory.EventActivityCompleted, enginehistory.ActivityCompletedPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			Output:       output,
		}),
		historyRow(t, 6, enginehistory.EventWorkflowCompleted, enginehistory.WorkflowCompletedPayload{
			Result: output,
		}),
	}

	decision, err := replayDefinition(retryDemoDefinition(), historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected no new events while replaying recorded retries, got %+v", decision.Events)
	}
	if !equalJSONForTest(t, decision.Result, output) {
		t.Fatalf("expected replayed result %s, got %s", output, decision.Result)
	}
}

func TestReplayDefinitionSkipsSingleRecordedActivityRetryEvent(t *testing.T) {
	input := mustRawJSON(t, map[string]string{"name": "Ada"})
	output := mustRawJSON(t, map[string]string{"greeting": "hello, Ada"})

	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "retry-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-retry-single",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventActivityScheduled, enginehistory.ActivityScheduledPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			Input:        input,
		}),
		historyRow(t, 3, enginehistory.EventActivityRetryScheduled, enginehistory.ActivityRetryScheduledPayload{
			ActivityKey:     "fetch",
			ActivityType:    "demo.activity",
			FailedAttempt:   1,
			NextAvailableAt: time.Unix(10, 0).UTC(),
			ErrorCode:       "activity_failed",
			ErrorMessage:    "first failure",
		}),
		historyRow(t, 4, enginehistory.EventActivityCompleted, enginehistory.ActivityCompletedPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			Output:       output,
		}),
		historyRow(t, 5, enginehistory.EventWorkflowCompleted, enginehistory.WorkflowCompletedPayload{
			Result: output,
		}),
	}

	decision, err := replayDefinition(retryDemoDefinition(), historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected single recorded retry replay to avoid new events, got %+v", decision.Events)
	}
	if !equalJSONForTest(t, decision.Result, output) {
		t.Fatalf("expected replayed result %s, got %s", output, decision.Result)
	}
}

func TestReplayDefinitionSkipsRecordedActivityRetryEventsBeforeFailure(t *testing.T) {
	input := mustRawJSON(t, map[string]string{"name": "Ada"})

	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "retry-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-retry-failed",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventActivityScheduled, enginehistory.ActivityScheduledPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			Input:        input,
		}),
		historyRow(t, 3, enginehistory.EventActivityRetryScheduled, enginehistory.ActivityRetryScheduledPayload{
			ActivityKey:     "fetch",
			ActivityType:    "demo.activity",
			FailedAttempt:   1,
			NextAvailableAt: time.Unix(10, 0).UTC(),
			ErrorCode:       "activity_failed",
			ErrorMessage:    "first failure",
		}),
		historyRow(t, 4, enginehistory.EventActivityRetryScheduled, enginehistory.ActivityRetryScheduledPayload{
			ActivityKey:     "fetch",
			ActivityType:    "demo.activity",
			FailedAttempt:   2,
			NextAvailableAt: time.Unix(20, 0).UTC(),
			ErrorCode:       "activity_failed",
			ErrorMessage:    "second failure",
		}),
		historyRow(t, 5, enginehistory.EventActivityFailed, enginehistory.ActivityFailedPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			ErrorCode:    "activity_failed",
			ErrorMessage: "final failure",
		}),
		historyRow(t, 6, enginehistory.EventWorkflowFailed, enginehistory.WorkflowFailedPayload{
			ErrorCode:    "activity_failed",
			ErrorMessage: "final failure",
		}),
	}

	decision, err := replayDefinition(retryDemoDefinition(), historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionFailed {
		t.Fatalf("expected failed decision, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected exhausted retry replay to avoid new events, got %+v", decision.Events)
	}
	if decision.FailureCode != "activity_failed" || decision.FailureMessage != "final failure" {
		t.Fatalf("expected exhausted retry replay failure, got code=%q message=%q", decision.FailureCode, decision.FailureMessage)
	}
}

func TestReplayDefinitionIgnoresSuspendResumeControlEvents(t *testing.T) {
	input := mustRawJSON(t, map[string]string{"name": "Ada"})
	output := mustRawJSON(t, map[string]string{"greeting": "hello, Ada"})

	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "resume-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-resume-demo",
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
		historyRow(t, 4, enginehistory.EventWorkflowSuspended, enginehistory.WorkflowSuspendedPayload{}),
		historyRow(t, 5, enginehistory.EventWorkflowResumed, enginehistory.WorkflowResumedPayload{}),
		historyRow(t, 6, enginehistory.EventWorkflowCompleted, enginehistory.WorkflowCompletedPayload{
			Result: output,
		}),
	}

	definition := publicworkflow.Definition{
		Name:    "resume-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var activityInput map[string]string
			if err := ctx.Input(&activityInput); err != nil {
				return err
			}

			var activityOutput map[string]string
			if err := ctx.Activity("fetch", "demo.activity", activityInput, &activityOutput); err != nil {
				return err
			}

			return ctx.SetResult(activityOutput)
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
		t.Fatalf("expected suspend/resume control events to be ignored during replay, got %+v", decision.Events)
	}
	if !equalJSONForTest(t, decision.Result, output) {
		t.Fatalf("expected replayed result %s, got %s", output, decision.Result)
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
	if !equalJSONForTest(t, decision.Result, result) {
		t.Fatalf("expected replayed result %s, got %s", result, decision.Result)
	}
}

func retryDemoDefinition() publicworkflow.Definition {
	return publicworkflow.Definition{
		Name:    "retry-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var activityInput map[string]string
			if err := ctx.Input(&activityInput); err != nil {
				return err
			}

			var activityOutput map[string]string
			if err := ctx.ActivityWithOptions("fetch", "demo.activity", activityInput, &activityOutput, publicworkflow.ActivityOptions{
				RetryPolicy: &publicworkflow.RetryPolicy{
					MaxAttempts:       3,
					InitialBackoff:    time.Second,
					MaxBackoff:        5 * time.Second,
					BackoffMultiplier: 2,
				},
			}); err != nil {
				return err
			}

			return ctx.SetResult(activityOutput)
		},
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
	if !equalJSONForTest(t, decision.Result, result) {
		t.Fatalf("expected completed result %s, got %s", result, decision.Result)
	}
}

func TestReplayDefinitionCancelledWorkflowReturnsCancelledDecision(t *testing.T) {
	cancelInboxID := uuid.Must(uuid.NewV7())
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "cancel-terminal",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-cancel-terminal",
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

	definition := publicworkflow.Definition{
		Name:    "cancel-terminal",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			if ctx.CancellationRequested() {
				return publicworkflow.ErrCancelled
			}
			return ctx.SetResult(map[string]bool{"ok": true})
		},
	}

	decision, err := replayDefinition(definition, historyRows, nil, inboxRows)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCancelled {
		t.Fatalf("expected cancelled decision, got %+v", decision)
	}
	if len(decision.Events) != 2 {
		t.Fatalf("expected cancel.requested + workflow.cancelled events, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventCancelRequested {
		t.Fatalf("expected first event to be cancel.requested, got %+v", decision.Events)
	}
	if decision.Events[1].EventType != enginehistory.EventWorkflowCancelled {
		t.Fatalf("expected second event to be workflow.cancelled, got %+v", decision.Events)
	}
	if decision.FailureCode != "cancelled" || decision.FailureMessage != "workflow cancelled" {
		t.Fatalf("expected cancelled failure summary, got %+v", decision)
	}
	if len(decision.ConsumedInboxIDs) != 1 || decision.ConsumedInboxIDs[0] != cancelInboxID {
		t.Fatalf("expected cancel inbox to be consumed, got %+v", decision.ConsumedInboxIDs)
	}
}

func TestReplayDefinitionPendingSignalBeforeCancelPreservesFrontierOrder(t *testing.T) {
	signalInboxID := uuid.Must(uuid.NewV7())
	cancelInboxID := uuid.Must(uuid.NewV7())
	signalPayload := mustRawJSON(t, map[string]string{"approval": "yes"})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "signal-before-cancel",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-signal-before-cancel",
		}),
	}
	inboxRows := []enginedb.EngineInbox{
		{
			ID:          signalInboxID,
			RunID:       pgtype.UUID{Bytes: uuid.Nil, Valid: false},
			Kind:        "signal",
			Payload:     mustRawJSON(t, enginehistory.SignalReceivedPayload{SignalName: "approval", Payload: signalPayload}),
			Status:      enginedb.EngineInboxStatusPending,
			AvailableAt: time.Unix(1, 0).UTC(),
		},
		{
			ID:          cancelInboxID,
			RunID:       pgtype.UUID{Bytes: uuid.Nil, Valid: false},
			Kind:        "cancel",
			Payload:     mustRawJSON(t, enginehistory.CancelRequestedPayload{}),
			Status:      enginedb.EngineInboxStatusPending,
			AvailableAt: time.Unix(2, 0).UTC(),
		},
	}
	result := mustRawJSON(t, map[string]any{
		"early_cancelled": false,
		"signal_approval": "yes",
		"late_cancelled":  true,
	})

	definition := publicworkflow.Definition{
		Name:    "signal-before-cancel",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			earlyCancelled := ctx.CancellationRequested()

			var signal map[string]string
			if err := ctx.ReceiveSignal("approval", &signal); err != nil {
				return err
			}

			lateCancelled := ctx.CancellationRequested()
			return ctx.SetResult(map[string]any{
				"early_cancelled": earlyCancelled,
				"signal_approval": signal["approval"],
				"late_cancelled":  lateCancelled,
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
	if len(decision.Events) != 3 {
		t.Fatalf("expected signal.received + cancel.requested + workflow.completed, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventSignalReceived {
		t.Fatalf("expected first event to be signal.received, got %+v", decision.Events)
	}
	if decision.Events[1].EventType != enginehistory.EventCancelRequested {
		t.Fatalf("expected second event to be cancel.requested, got %+v", decision.Events)
	}
	if decision.Events[2].EventType != enginehistory.EventWorkflowCompleted {
		t.Fatalf("expected third event to be workflow.completed, got %+v", decision.Events)
	}
	if len(decision.ConsumedInboxIDs) != 2 || decision.ConsumedInboxIDs[0] != signalInboxID || decision.ConsumedInboxIDs[1] != cancelInboxID {
		t.Fatalf("expected signal then cancel inbox consumption, got %+v", decision.ConsumedInboxIDs)
	}
	if !equalJSONForTest(t, decision.Result, result) {
		t.Fatalf("expected completed result %s, got %s", result, decision.Result)
	}
}

func TestReplayDefinitionPendingTimerBeforeCancelPreservesFrontierOrder(t *testing.T) {
	timerInboxID := uuid.Must(uuid.NewV7())
	cancelInboxID := uuid.Must(uuid.NewV7())
	timerAt := time.Unix(10, 0).UTC()
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "timer-before-cancel",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-timer-before-cancel",
		}),
		historyRow(t, 2, enginehistory.EventTimerScheduled, enginehistory.TimerScheduledPayload{
			TimerKey: "deadline",
			DueAt:    timerAt,
		}),
	}
	inboxRows := []enginedb.EngineInbox{
		{
			ID:          timerInboxID,
			RunID:       pgtype.UUID{Bytes: uuid.Nil, Valid: false},
			Kind:        "timer",
			Payload:     mustRawJSON(t, enginehistory.TimerScheduledPayload{TimerKey: "deadline", DueAt: timerAt}),
			Status:      enginedb.EngineInboxStatusPending,
			AvailableAt: time.Unix(1, 0).UTC(),
		},
		{
			ID:          cancelInboxID,
			RunID:       pgtype.UUID{Bytes: uuid.Nil, Valid: false},
			Kind:        "cancel",
			Payload:     mustRawJSON(t, enginehistory.CancelRequestedPayload{}),
			Status:      enginedb.EngineInboxStatusPending,
			AvailableAt: time.Unix(2, 0).UTC(),
		},
	}
	result := mustRawJSON(t, map[string]bool{
		"early_cancelled": false,
		"late_cancelled":  true,
	})

	definition := publicworkflow.Definition{
		Name:    "timer-before-cancel",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			earlyCancelled := ctx.CancellationRequested()
			if err := ctx.SleepUntil("deadline", timerAt); err != nil {
				return err
			}

			lateCancelled := ctx.CancellationRequested()
			return ctx.SetResult(map[string]bool{
				"early_cancelled": earlyCancelled,
				"late_cancelled":  lateCancelled,
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
	if len(decision.Events) != 3 {
		t.Fatalf("expected timer.fired + cancel.requested + workflow.completed, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventTimerFired {
		t.Fatalf("expected first event to be timer.fired, got %+v", decision.Events)
	}
	if decision.Events[1].EventType != enginehistory.EventCancelRequested {
		t.Fatalf("expected second event to be cancel.requested, got %+v", decision.Events)
	}
	if decision.Events[2].EventType != enginehistory.EventWorkflowCompleted {
		t.Fatalf("expected third event to be workflow.completed, got %+v", decision.Events)
	}
	if len(decision.ConsumedInboxIDs) != 2 || decision.ConsumedInboxIDs[0] != timerInboxID || decision.ConsumedInboxIDs[1] != cancelInboxID {
		t.Fatalf("expected timer then cancel inbox consumption, got %+v", decision.ConsumedInboxIDs)
	}
	if !equalJSONForTest(t, decision.Result, result) {
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

func childWorkflowStartedHistory(
	t *testing.T,
	runID uuid.UUID,
	childInstanceID uuid.UUID,
	childRunID uuid.UUID,
	input json.RawMessage,
	childInstanceKey string,
) []enginedb.EngineHistory {
	t.Helper()

	return []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "checkout",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-checkout",
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
}

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func equalJSONForTest(t *testing.T, left, right json.RawMessage) bool {
	t.Helper()

	equal, err := equalJSON(left, right)
	if err != nil {
		t.Fatalf("equalJSON() error = %v", err)
	}
	return equal
}

func TestReplayDefinitionEngineInvariantViolationQuarantines(t *testing.T) {
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-1",
		}),
	}
	definition := publicworkflow.Definition{
		Name:    "demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			ctx.(*workflowRunner).removePendingFrontier(uuid.New())
			return nil
		},
	}

	decision, err := replayDefinition(definition, historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionQuarantined {
		t.Fatalf("expected quarantined decision, got %+v", decision)
	}
	if decision.FailureCode != "engine_invariant" {
		t.Fatalf("expected engine_invariant failure code, got %q", decision.FailureCode)
	}
	if !strings.Contains(decision.FailureMessage, "missing from pending frontier") {
		t.Fatalf("expected invariant detail in failure message, got %q", decision.FailureMessage)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected no events for quarantined invariant violation, got %+v", decision.Events)
	}
	if len(decision.ConsumedInboxIDs) != 0 {
		t.Fatalf("expected no consumed inbox IDs, got %+v", decision.ConsumedInboxIDs)
	}
	if len(decision.WaitingFor) == 0 {
		t.Fatalf("expected waiting state for quarantined invariant violation")
	}
	var waitingFor struct {
		Kind   string `json:"kind"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(decision.WaitingFor, &waitingFor); err != nil {
		t.Fatalf("decode waiting state: %v", err)
	}
	if waitingFor.Kind != "engine_invariant" {
		t.Fatalf("expected engine_invariant waiting kind, got %q", waitingFor.Kind)
	}
	if waitingFor.Detail == "" {
		t.Fatalf("expected non-empty invariant waiting detail")
	}
}

func TestReplayDefinitionUserPanicKeepsWorkflowPanicCode(t *testing.T) {
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-1",
		}),
	}
	definition := publicworkflow.Definition{
		Name:    "demo",
		Version: "v1",
		Run: func(publicworkflow.Context) error {
			panic("user boom")
		},
	}

	decision, err := replayDefinition(definition, historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionFailed {
		t.Fatalf("expected failed decision, got %+v", decision)
	}
	if decision.FailureCode != "workflow_panic" {
		t.Fatalf("expected workflow_panic failure code, got %q", decision.FailureCode)
	}
	if !strings.Contains(decision.FailureMessage, "user boom") {
		t.Fatalf("expected user panic detail in failure message, got %q", decision.FailureMessage)
	}
	if len(decision.Events) != 1 {
		t.Fatalf("expected one workflow failed event, got %+v", decision.Events)
	}
	if decision.Events[0].EventType != enginehistory.EventWorkflowFailed {
		t.Fatalf("expected workflow failed event, got %+v", decision.Events[0])
	}
}

func TestRemovePendingFrontierMissingInboxPanicsTyped(t *testing.T) {
	r := &workflowRunner{}
	id := uuid.New()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected removePendingFrontier to panic")
		}
		invariant, ok := recovered.(internalInvariantPanic)
		if !ok {
			t.Fatalf("expected internalInvariantPanic, got %T: %[1]v", recovered)
		}
		if !strings.Contains(invariant.detail, id.String()) {
			t.Fatalf("expected invariant detail to contain inbox ID %s, got %q", id, invariant.detail)
		}
	}()

	r.removePendingFrontier(id)
}

func TestEqualJSONPreservesNumberPrecision(t *testing.T) {
	equal, err := equalJSON(
		json.RawMessage(`{"value":9007199254740992}`),
		json.RawMessage(`{"value":9007199254740993}`),
	)
	if err != nil {
		t.Fatalf("equalJSON() error = %v", err)
	}
	if equal {
		t.Fatal("equalJSON() = true, want false for distinct large integers")
	}
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

func continueAsNewDefinition(nextInput any) publicworkflow.Definition {
	return publicworkflow.Definition{
		Name:    "continue-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input map[string]any
			if err := ctx.Input(&input); err != nil {
				return err
			}
			return publicworkflow.ContinueAsNew(nextInput)
		},
	}
}
