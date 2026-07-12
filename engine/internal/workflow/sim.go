package workflow

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

type SimEvent struct {
	EventType string
	Payload   json.RawMessage
}

type SimActivity struct {
	Key   string
	Type  string
	Input json.RawMessage
}

type SimTimer struct {
	TimerKey string
	DueAt    time.Time
	Payload  json.RawMessage
}

type SimChild struct {
	ChildKey          string
	DefinitionName    string
	DefinitionVersion string
	ChildInstanceKey  string
	Input             json.RawMessage
}

type SimDecision struct {
	Kind              string
	Events            []SimEvent
	NextSequence      int32
	WaitingFor        json.RawMessage
	CustomStatus      json.RawMessage
	Result            json.RawMessage
	ContinuationInput json.RawMessage
	NewActivity       *SimActivity
	NewTimer          *SimTimer
	NewChildWorkflow  *SimChild
	ConsumedInboxIDs  []uuid.UUID
	FailureCode       string
	FailureMessage    string
}

func SimulateActivation(
	def publicworkflow.Definition,
	run *enginedb.EngineRun,
	history []enginedb.EngineHistory,
	tasks []enginedb.EngineActivityTask,
	children []enginedb.ListChildWorkflowOutcomesByParentRunRow,
	inbox []enginedb.EngineInbox,
) (SimDecision, error) {
	decision, err := replayDefinitionForRun(def, run, history, tasks, children, inbox)
	if err != nil {
		return SimDecision{}, err
	}

	out := SimDecision{
		Kind:              string(decision.Kind),
		NextSequence:      decision.NextSequence,
		WaitingFor:        cloneRaw(decision.WaitingFor),
		CustomStatus:      cloneRaw(decision.CustomStatus),
		Result:            cloneRaw(decision.Result),
		ContinuationInput: cloneRaw(decision.ContinuationInput),
		ConsumedInboxIDs:  append([]uuid.UUID(nil), decision.ConsumedInboxIDs...),
		FailureCode:       decision.FailureCode,
		FailureMessage:    decision.FailureMessage,
	}
	for _, event := range decision.Events {
		out.Events = append(out.Events, SimEvent{EventType: event.EventType, Payload: cloneRaw(event.Payload)})
	}
	if decision.NewActivity != nil {
		out.NewActivity = &SimActivity{
			Key:   decision.NewActivity.Scheduled.ActivityKey,
			Type:  decision.NewActivity.Scheduled.ActivityType,
			Input: cloneRaw(decision.NewActivity.Scheduled.Input),
		}
	}
	if decision.NewTimer != nil {
		out.NewTimer = &SimTimer{
			TimerKey: decision.NewTimer.TimerKey,
			DueAt:    decision.NewTimer.DueAt,
			Payload:  mustMarshalPayload(*decision.NewTimer),
		}
	}
	if decision.NewChildWorkflow != nil {
		out.NewChildWorkflow = &SimChild{
			ChildKey:          decision.NewChildWorkflow.Scheduled.ChildKey,
			DefinitionName:    decision.NewChildWorkflow.Scheduled.DefinitionName,
			DefinitionVersion: decision.NewChildWorkflow.Scheduled.DefinitionVersion,
			ChildInstanceKey:  decision.NewChildWorkflow.Scheduled.ChildInstanceKey,
			Input:             cloneRaw(decision.NewChildWorkflow.Scheduled.Input),
		}
	}
	return out, nil
}
