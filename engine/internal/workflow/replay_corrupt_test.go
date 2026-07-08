package workflow

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

var corruptRecordedJSON = json.RawMessage("{corrupt")

func TestCorruptRecordedCustomStatusQuarantinesAsHistoryCorrupt(t *testing.T) {
	decision, err := newCorruptReplayRunner(decodedEvent{
		EventType: enginehistory.EventCustomStatusUpdated,
		Payload: &enginehistory.CustomStatusUpdatedPayload{
			Status: corruptRecordedJSON,
		},
	}).execute(testCorruptReplayDefinition(func(ctx publicworkflow.Context) error {
		if err := ctx.SetCustomStatus(map[string]string{"phase": "x"}); err != nil {
			return err
		}
		return nil
	}))

	assertHistoryCorruptQuarantine(t, decision, err)
}

func TestCorruptRecordedContinueAsNewInputQuarantinesAsHistoryCorrupt(t *testing.T) {
	decision, err := newCorruptReplayRunner(decodedEvent{
		EventType: enginehistory.EventWorkflowContinuedAsNew,
		Payload: &enginehistory.WorkflowContinuedAsNewPayload{
			Input: corruptRecordedJSON,
		},
	}).execute(testCorruptReplayDefinition(func(publicworkflow.Context) error {
		return publicworkflow.ContinueAsNew(map[string]int{"cursor": 1})
	}))

	assertHistoryCorruptQuarantine(t, decision, err)
}

func TestCorruptRecordedWorkflowResultQuarantinesAsHistoryCorrupt(t *testing.T) {
	decision, err := newCorruptReplayRunner(decodedEvent{
		EventType: enginehistory.EventWorkflowCompleted,
		Payload: &enginehistory.WorkflowCompletedPayload{
			Result: corruptRecordedJSON,
		},
	}).execute(testCorruptReplayDefinition(func(ctx publicworkflow.Context) error {
		return ctx.SetResult(map[string]string{"greeting": "hi"})
	}))

	assertHistoryCorruptQuarantine(t, decision, err)
}

func TestCorruptRecordedActivityInputQuarantinesAsHistoryCorrupt(t *testing.T) {
	decision, err := newCorruptReplayRunner(decodedEvent{
		EventType: enginehistory.EventActivityScheduled,
		Payload: &enginehistory.ActivityScheduledPayload{
			ActivityKey:  "step-1",
			ActivityType: "demo.activity",
			Input:        corruptRecordedJSON,
		},
	}).execute(testCorruptReplayDefinition(func(ctx publicworkflow.Context) error {
		return ctx.Activity("step-1", "demo.activity", map[string]string{"name": "Ada"}, nil)
	}))

	assertHistoryCorruptQuarantine(t, decision, err)
}

func TestCorruptRecordedChildWorkflowInputQuarantinesAsHistoryCorrupt(t *testing.T) {
	childInstanceKey := defaultChildInstanceKey(uuid.Nil, uuid.Nil, "child-1")

	decision, err := newCorruptReplayRunner(decodedEvent{
		EventType: enginehistory.EventChildWorkflowScheduled,
		Payload: &enginehistory.ChildWorkflowScheduledPayload{
			ChildKey:          "child-1",
			DefinitionName:    "child-def",
			DefinitionVersion: "v1",
			ChildInstanceKey:  childInstanceKey,
			Input:             corruptRecordedJSON,
		},
	}).execute(testCorruptReplayDefinition(func(ctx publicworkflow.Context) error {
		return ctx.ChildWorkflowWithOptions(
			"child-1",
			"child-def",
			"v1",
			map[string]string{"name": "Ada"},
			nil,
			publicworkflow.ChildWorkflowOptions{InstanceKey: childInstanceKey},
		)
	}))

	assertHistoryCorruptQuarantine(t, decision, err)
}

func TestDifferentCustomStatusStillQuarantinesAsReplayMismatch(t *testing.T) {
	decision, err := newCorruptReplayRunner(decodedEvent{
		EventType: enginehistory.EventCustomStatusUpdated,
		Payload: &enginehistory.CustomStatusUpdatedPayload{
			Status: json.RawMessage(`{"phase":"other"}`),
		},
	}).execute(testCorruptReplayDefinition(func(ctx publicworkflow.Context) error {
		if err := ctx.SetCustomStatus(map[string]string{"phase": "x"}); err != nil {
			return err
		}
		return nil
	}))

	assertReplayMismatchQuarantine(t, decision, err)
}

func TestDifferentContinueAsNewInputStillQuarantinesAsReplayMismatch(t *testing.T) {
	decision, err := newCorruptReplayRunner(decodedEvent{
		EventType: enginehistory.EventWorkflowContinuedAsNew,
		Payload: &enginehistory.WorkflowContinuedAsNewPayload{
			Input: json.RawMessage(`{"cursor":999}`),
		},
	}).execute(testCorruptReplayDefinition(func(publicworkflow.Context) error {
		return publicworkflow.ContinueAsNew(map[string]int{"cursor": 1})
	}))

	assertReplayMismatchQuarantine(t, decision, err)
}

func TestEmptyRecordedResultVsPresentResultIsReplayMismatchNotCorrupt(t *testing.T) {
	decision, err := newCorruptReplayRunner(decodedEvent{
		EventType: enginehistory.EventWorkflowCompleted,
		Payload: &enginehistory.WorkflowCompletedPayload{
			Result: nil,
		},
	}).execute(testCorruptReplayDefinition(func(ctx publicworkflow.Context) error {
		return ctx.SetResult(map[string]string{"greeting": "hi"})
	}))

	assertReplayMismatchQuarantine(t, decision, err)
	if decision.FailureCode == "history_corrupt" {
		t.Fatalf("FailureCode = %q, want replay_mismatch for absent recorded result", decision.FailureCode)
	}
}

func newCorruptReplayRunner(events ...decodedEvent) *workflowRunner {
	return &workflowRunner{
		input:             json.RawMessage(`{}`),
		nextSequence:      int32(len(events) + 2),
		pendingActivities: make(map[string]activityOutcome),
		pendingChildren:   make(map[string]childWorkflowOutcome),
		pendingSignals:    make(map[string][]pendingSignal),
		pendingTimers:     make(map[string]pendingTimer),
		replayEvents:      events,
	}
}

func testCorruptReplayDefinition(run func(publicworkflow.Context) error) publicworkflow.Definition {
	return publicworkflow.Definition{
		Name:    "demo",
		Version: "v1",
		Run:     run,
	}
}

func assertHistoryCorruptQuarantine(t *testing.T, decision activationDecision, err error) {
	t.Helper()
	assertQuarantineDecision(t, decision, err)
	if decision.FailureCode != "history_corrupt" {
		t.Fatalf("FailureCode = %q, want history_corrupt", decision.FailureCode)
	}
}

func assertReplayMismatchQuarantine(t *testing.T, decision activationDecision, err error) {
	t.Helper()
	assertQuarantineDecision(t, decision, err)
	if decision.FailureCode != "replay_mismatch" {
		t.Fatalf("FailureCode = %q, want replay_mismatch", decision.FailureCode)
	}
}

func assertQuarantineDecision(t *testing.T, decision activationDecision, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if decision.Kind != decisionQuarantined {
		t.Fatalf("decision.Kind = %q, want %q", decision.Kind, decisionQuarantined)
	}
	if decision.FailureMessage == "" {
		t.Fatal("FailureMessage is empty")
	}
	if len(decision.Events) != 0 {
		t.Fatalf("len(decision.Events) = %d, want 0", len(decision.Events))
	}

	var wait enginehistory.ReplayMismatchWait
	if err := json.Unmarshal(decision.WaitingFor, &wait); err != nil {
		t.Fatalf("WaitingFor did not decode as ReplayMismatchWait: %v", err)
	}
	if wait.Kind != enginehistory.WaitKindReplayMismatch {
		t.Fatalf("WaitingFor.kind = %q, want %q", wait.Kind, enginehistory.WaitKindReplayMismatch)
	}
	if wait.Detail == "" {
		t.Fatal("WaitingFor.detail is empty")
	}
}
