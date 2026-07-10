// Package workflowtest provides an in-memory workflow unit-test kit.
package workflowtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	internalworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	"github.com/continua-ai/continua/engine/pkg/history"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

const activationCap = 1024

// ActivityHandler is a scripted activity implementation, keyed by activity type.
type ActivityHandler func(input json.RawMessage) (any, error)

type activityError struct {
	code    string
	message string
}

func (e *activityError) Error() string {
	return e.message
}

// NewActivityError returns an error that fails the activity with a specific code.
// A plain (non-ActivityError) handler error fails with code "activity_failed"
// (same as the real activity worker).
func NewActivityError(code, message string) error {
	return &activityError{code: code, message: message}
}

type Status string

const (
	StatusCompleted      Status = "completed"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
	StatusContinuedAsNew Status = "continued_as_new"
	StatusQuarantined    Status = "quarantined"
	StatusBlocked        Status = "blocked"
)

type queuedSignal struct {
	name    string
	payload json.RawMessage
}

type Environment struct {
	activities  map[string]ActivityHandler
	definitions map[string]workflow.Definition
	signals     []queuedSignal
	cancel      bool
}

func NewEnvironment() *Environment {
	return &Environment{
		activities:  make(map[string]ActivityHandler),
		definitions: make(map[string]workflow.Definition),
	}
}

// RegisterActivity scripts the outcome of every activity of the given type. One
// attempt per activity key; retry policies are not simulated.
func (e *Environment) RegisterActivity(activityType string, handler ActivityHandler) {
	e.activities[activityType] = handler
}

// RegisterDefinition makes a definition available for ChildWorkflow calls,
// looked up by (Name, Version). Children share the environment's activity
// handlers and child registry but not its signal queue or cancellation.
func (e *Environment) RegisterDefinition(def workflow.Definition) {
	e.definitions[definitionKey(def.Name, def.Version)] = def
}

// QueueSignal enqueues a signal delivered to the workflow inbox in call order
// (before Execute). Payload is JSON-marshaled.
func (e *Environment) QueueSignal(name string, payload any) error {
	raw, err := history.MarshalPayload(payload)
	if err != nil {
		return err
	}
	e.signals = append(e.signals, queuedSignal{name: name, payload: raw})
	return nil
}

// RequestCancellation enqueues a cancellation request in the inbox after any
// already-queued signals (real inbox arrival-order semantics).
func (e *Environment) RequestCancellation() {
	e.cancel = true
}

// Execute runs def from input to a terminal state, auto-firing timers and
// resolving scripted activities/children between activations. Returns an error
// only for kit-level problems (marshal failure, activation loop cap exceeded).
func (e *Environment) Execute(def workflow.Definition, input any) (*Result, error) {
	inputRaw, err := history.MarshalPayload(input)
	if err != nil {
		return nil, err
	}
	sim := newSimulation(e, def, inputRaw)
	sim.inbox = e.materializeInbox(sim)
	return sim.run(activationCap)
}

type Result struct {
	Status            Status
	ErrorCode         string
	ErrorMessage      string
	CustomStatus      json.RawMessage
	ContinuationInput json.RawMessage
	WaitKind          string
	WaitKey           string

	result  json.RawMessage
	history []enginedb.EngineHistory
}

// DecodeResult unmarshals the workflow result (SetResult value) into out.
func (r *Result) DecodeResult(out any) error {
	return history.UnmarshalPayload(r.result, out)
}

// HistoryEventTypes returns the full recorded history event types in order,
// starting with history.EventWorkflowStarted.
func (r *Result) HistoryEventTypes() []string {
	events := make([]string, 0, len(r.history))
	for i := range r.history {
		events = append(events, r.history[i].EventType)
	}
	return events
}

type simulation struct {
	env       *Environment
	def       workflow.Definition
	engineRun enginedb.EngineRun
	history   []enginedb.EngineHistory
	tasks     []enginedb.EngineActivityTask
	children  []enginedb.ListChildWorkflowOutcomesByParentRunRow
	inbox     []enginedb.EngineInbox
}

func newSimulation(env *Environment, def workflow.Definition, input json.RawMessage) *simulation {
	now := time.Now().UTC()
	projectID := uuid.New()
	instanceID := uuid.New()
	runID := uuid.New()
	engineRun := enginedb.EngineRun{
		ID:                runID,
		ProjectID:         projectID,
		InstanceID:        instanceID,
		DefinitionVersion: def.Version,
		Status:            enginedb.EngineRunLifecycleStatusRunning,
		ReadyAt:           now,
		CreatedAt:         now,
		UpdatedAt:         now,
		RootRunID:         runID,
	}
	started, _ := history.MarshalPayload(history.WorkflowStartedPayload{
		DefinitionName:    def.Name,
		DefinitionVersion: def.Version,
		InstanceKey:       uuid.NewString(),
		Input:             cloneRaw(input),
	})
	return &simulation{
		env:       env,
		def:       def,
		engineRun: engineRun,
		history: []enginedb.EngineHistory{{
			ID:         1,
			ProjectID:  projectID,
			InstanceID: instanceID,
			RunID:      runID,
			SequenceNo: 1,
			EventType:  history.EventWorkflowStarted,
			Payload:    started,
			CreatedAt:  now,
		}},
	}
}

func (s *simulation) run(remaining int) (*Result, error) {
	for remaining > 0 {
		remaining--
		decision, err := internalworkflow.SimulateActivation(s.def, &s.engineRun, s.history, s.tasks, s.children, s.inbox)
		if err != nil {
			return nil, err
		}
		s.appendEvents(decision.NextSequence, decision.Events)
		s.removeInbox(decision.ConsumedInboxIDs)
		result := s.resultFromDecision(&decision)

		switch decision.Kind {
		case "completed", "failed", "cancelled", "continued_as_new", "quarantined":
			return result, nil
		case "waiting":
			if decision.NewActivity != nil {
				if handled, err := s.resolveActivity(decision.NewActivity); err != nil || !handled {
					return result, err
				}
				continue
			}
			if decision.NewTimer != nil {
				s.inbox = append(s.inbox, s.inboxRow("timer", decision.NewTimer.Payload))
				continue
			}
			if decision.NewChildWorkflow != nil {
				if handled, err := s.resolveChild(decision.NewChildWorkflow, remaining); err != nil || !handled {
					return result, err
				}
				continue
			}
			return result, nil
		default:
			return nil, fmt.Errorf("workflowtest: unsupported decision kind %q", decision.Kind)
		}
	}
	return nil, fmt.Errorf("workflowtest: activation cap %d exceeded", activationCap)
}

func (s *simulation) resultFromDecision(decision *internalworkflow.SimDecision) *Result {
	r := &Result{
		CustomStatus:      cloneRaw(decision.CustomStatus),
		ContinuationInput: cloneRaw(decision.ContinuationInput),
		ErrorCode:         decision.FailureCode,
		ErrorMessage:      decision.FailureMessage,
		result:            cloneRaw(decision.Result),
		history:           append([]enginedb.EngineHistory(nil), s.history...),
	}
	switch decision.Kind {
	case "completed":
		r.Status = StatusCompleted
	case "failed":
		r.Status = StatusFailed
	case "cancelled":
		r.Status = StatusCancelled
		if r.ErrorCode == "" {
			r.ErrorCode = "cancelled"
		}
	case "continued_as_new":
		r.Status = StatusContinuedAsNew
	case "quarantined":
		r.Status = StatusQuarantined
	case "waiting":
		r.Status = StatusBlocked
		r.WaitKind, r.WaitKey = decodeWait(decision.WaitingFor)
	}
	return r
}

func (s *simulation) resolveActivity(activity *internalworkflow.SimActivity) (bool, error) {
	handler := s.env.activities[activity.Type]
	if handler == nil {
		return false, nil
	}
	output, err := handler(cloneRaw(activity.Input))
	now := time.Now().UTC()
	task := enginedb.EngineActivityTask{
		ID:           uuid.New(),
		ProjectID:    s.engineRun.ProjectID,
		InstanceID:   s.engineRun.InstanceID,
		RunID:        s.engineRun.ID,
		ActivityKey:  activity.Key,
		ActivityType: activity.Type,
		Input:        cloneRaw(activity.Input),
		AttemptCount: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err != nil {
		code := "activity_failed"
		var coded *activityError
		if errors.As(err, &coded) {
			code = coded.code
		}
		msg := err.Error()
		task.Status = enginedb.EngineActivityTaskStatusFailed
		task.LastErrorCode = &code
		task.LastErrorMessage = &msg
	} else {
		raw, marshalErr := history.MarshalPayload(output)
		if marshalErr != nil {
			return false, marshalErr
		}
		task.Status = enginedb.EngineActivityTaskStatusCompleted
		task.Output = raw
	}
	s.tasks = append(s.tasks, task)
	return true, nil
}

func (s *simulation) resolveChild(child *internalworkflow.SimChild, remaining int) (bool, error) {
	def, ok := s.env.definitions[definitionKey(child.DefinitionName, child.DefinitionVersion)]
	if !ok {
		return false, nil
	}
	childRunID := uuid.New()
	childInstanceID := uuid.New()
	startedPayload, err := history.MarshalPayload(history.ChildWorkflowStartedPayload{
		ChildKey:         child.ChildKey,
		ChildInstanceID:  childInstanceID.String(),
		ChildInstanceKey: child.ChildInstanceKey,
		ChildRunID:       childRunID.String(),
		RootRunID:        s.engineRun.RootRunID.String(),
		ChildDepth:       s.engineRun.ChildDepth + 1,
	})
	if err != nil {
		return false, err
	}
	s.history = append(s.history, s.historyRow(nextSequence(s.history), history.EventChildWorkflowStarted, startedPayload))

	childSim := newSimulation(s.env, def, cloneRaw(child.Input))
	childSim.engineRun.ProjectID = s.engineRun.ProjectID
	childSim.engineRun.RootRunID = s.engineRun.RootRunID
	childSim.engineRun.ChildDepth = s.engineRun.ChildDepth + 1
	childSim.engineRun.ID = childRunID
	childSim.engineRun.InstanceID = childInstanceID
	childSim.history[0].ProjectID = s.engineRun.ProjectID
	childSim.history[0].InstanceID = childInstanceID
	childSim.history[0].RunID = childRunID

	childResult, err := childSim.run(remaining)
	if err != nil {
		return false, err
	}
	status, runStatus := childStatuses(childResult.Status)
	code := childResult.ErrorCode
	message := childResult.ErrorMessage
	if childResult.Status == StatusFailed && code == "" {
		code = "workflow_failed"
	}
	s.children = append(s.children, enginedb.ListChildWorkflowOutcomesByParentRunRow{
		ID:                         uuid.New(),
		ProjectID:                  s.engineRun.ProjectID,
		ParentInstanceID:           s.engineRun.InstanceID,
		ParentRunID:                s.engineRun.ID,
		ChildKey:                   child.ChildKey,
		RequestedDefinitionName:    child.DefinitionName,
		RequestedDefinitionVersion: child.DefinitionVersion,
		ChildInstanceID:            childInstanceID,
		ChildInstanceKey:           child.ChildInstanceKey,
		CurrentChildRunID:          childRunID,
		TerminalChildRunID:         pgtype.UUID{Bytes: childRunID, Valid: true},
		RootRunID:                  s.engineRun.RootRunID,
		ChildDepth:                 s.engineRun.ChildDepth + 1,
		Status:                     status,
		TerminalResult:             cloneRaw(childResult.result),
		TerminalLastErrorCode:      stringPtr(code),
		TerminalLastErrorMessage:   stringPtr(message),
		TerminalRunStatus:          enginedb.NullEngineRunLifecycleStatus{EngineRunLifecycleStatus: runStatus, Valid: true},
	})
	return true, nil
}

func (s *simulation) appendEvents(nextSequence int32, events []internalworkflow.SimEvent) {
	seq := nextSequence
	for _, event := range events {
		s.history = append(s.history, s.historyRow(seq, event.EventType, normalizeEventPayload(event.EventType, event.Payload)))
		seq++
	}
}

func (s *simulation) historyRow(sequence int32, eventType string, payload json.RawMessage) enginedb.EngineHistory {
	return enginedb.EngineHistory{
		ID:         int64(len(s.history) + 1),
		ProjectID:  s.engineRun.ProjectID,
		InstanceID: s.engineRun.InstanceID,
		RunID:      s.engineRun.ID,
		SequenceNo: sequence,
		EventType:  eventType,
		Payload:    cloneRaw(payload),
		CreatedAt:  time.Now().UTC(),
	}
}

func (s *simulation) inboxRow(kind string, payload json.RawMessage) enginedb.EngineInbox {
	return enginedb.EngineInbox{
		ID:          uuid.New(),
		ProjectID:   s.engineRun.ProjectID,
		InstanceID:  s.engineRun.InstanceID,
		RunID:       pgtype.UUID{Bytes: s.engineRun.ID, Valid: true},
		Kind:        kind,
		Payload:     cloneRaw(payload),
		Status:      enginedb.EngineInboxStatusPending,
		AvailableAt: time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func (e *Environment) materializeInbox(sim *simulation) []enginedb.EngineInbox {
	rows := make([]enginedb.EngineInbox, 0, len(e.signals)+1)
	for _, signal := range e.signals {
		payload, _ := history.MarshalPayload(history.SignalReceivedPayload{
			SignalName: signal.name,
			Payload:    cloneRaw(signal.payload),
		})
		rows = append(rows, sim.inboxRow("signal", payload))
	}
	if e.cancel {
		payload, _ := history.MarshalPayload(history.CancelRequestedPayload{})
		rows = append(rows, sim.inboxRow("cancel", payload))
	}
	return rows
}

func (s *simulation) removeInbox(ids []uuid.UUID) {
	if len(ids) == 0 {
		return
	}
	consumed := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		consumed[id] = struct{}{}
	}
	filtered := s.inbox[:0]
	for i := range s.inbox {
		if _, ok := consumed[s.inbox[i].ID]; !ok {
			filtered = append(filtered, s.inbox[i])
		}
	}
	s.inbox = filtered
}

func decodeWait(raw json.RawMessage) (kind, key string) {
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", ""
	}
	switch probe.Kind {
	case history.WaitKindActivity:
		var wait history.ActivityWait
		_ = json.Unmarshal(raw, &wait)
		return wait.Kind, wait.ActivityKey
	case history.WaitKindTimer:
		var wait history.TimerWait
		_ = json.Unmarshal(raw, &wait)
		return wait.Kind, wait.TimerKey
	case history.WaitKindSignal:
		var wait history.SignalWait
		_ = json.Unmarshal(raw, &wait)
		return wait.Kind, wait.SignalName
	case history.WaitKindChildWorkflow:
		var wait history.ChildWorkflowWait
		_ = json.Unmarshal(raw, &wait)
		return wait.Kind, wait.ChildKey
	default:
		return probe.Kind, ""
	}
}

func childStatuses(status Status) (enginedb.EngineChildWorkflowStatus, enginedb.EngineRunLifecycleStatus) {
	switch status {
	case StatusCompleted:
		return enginedb.EngineChildWorkflowStatusCompleted, enginedb.EngineRunLifecycleStatusCompleted
	case StatusCancelled:
		return enginedb.EngineChildWorkflowStatusCancelled, enginedb.EngineRunLifecycleStatusCancelled
	default:
		return enginedb.EngineChildWorkflowStatusFailed, enginedb.EngineRunLifecycleStatusFailed
	}
}

func definitionKey(name, version string) string {
	return name + "\x00" + version
}

func nextSequence(rows []enginedb.EngineHistory) int32 {
	if len(rows) == 0 {
		return 1
	}
	return rows[len(rows)-1].SequenceNo + 1
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func normalizeEventPayload(eventType string, payload json.RawMessage) json.RawMessage {
	if eventType != history.EventActivityScheduled && eventType != history.EventChildWorkflowScheduled {
		return payload
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(payload, &value); err != nil {
		return payload
	}
	if string(value["input"]) != "null" {
		return payload
	}
	delete(value, "input")
	normalized, err := json.Marshal(value)
	if err != nil {
		return payload
	}
	return normalized
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
