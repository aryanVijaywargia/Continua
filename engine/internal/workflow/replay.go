package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

type decisionKind string

const (
	decisionWaiting        decisionKind = "waiting"
	decisionCompleted      decisionKind = "completed"
	decisionFailed         decisionKind = "failed"
	decisionCancelled      decisionKind = "cancelled"
	decisionContinuedAsNew decisionKind = "continued_as_new"
)

type queuedHistoryEvent struct {
	EventType string
	Payload   json.RawMessage
}

type activationDecision struct {
	Kind              decisionKind
	Events            []queuedHistoryEvent
	NextSequence      int32
	WaitingFor        json.RawMessage
	CustomStatus      json.RawMessage
	Result            json.RawMessage
	NewActivity       *newActivityTask
	NewTimer          *enginehistory.TimerScheduledPayload
	ContinuationInput json.RawMessage
	ConsumedInboxIDs  []uuid.UUID
	FailureCode       string
	FailureMessage    string
}

type newActivityTask struct {
	Scheduled enginehistory.ActivityScheduledPayload
	Options   publicworkflow.NormalizedActivityOptions
}

type decodedEvent struct {
	EventType string
	Payload   any
}

type activityOutcome struct {
	completed *enginehistory.ActivityCompletedPayload
	failed    *enginehistory.ActivityFailedPayload
}

type pendingSignal struct {
	inboxID uuid.UUID
	payload enginehistory.SignalReceivedPayload
}

type pendingTimer struct {
	inboxID uuid.UUID
	payload enginehistory.TimerScheduledPayload
}

type pendingInboxItem struct {
	inboxID uuid.UUID
	kind    string
}

type blockedPanic struct{}

type replayMismatchPanic struct {
	payload enginehistory.WorkflowReplayMismatchPayload
}

type codedWorkflowError struct {
	code    string
	message string
}

func (e codedWorkflowError) Error() string {
	return e.message
}

type workflowRunner struct {
	input             json.RawMessage
	replayEvents      []decodedEvent
	cursor            int
	nextSequence      int32
	customStatus      json.RawMessage
	result            json.RawMessage
	cancelRequested   bool
	pendingActivities map[string]activityOutcome
	pendingSignals    map[string][]pendingSignal
	pendingTimers     map[string]pendingTimer
	pendingFrontier   []pendingInboxItem

	queuedEvents     []queuedHistoryEvent
	waitingFor       json.RawMessage
	newActivity      *newActivityTask
	newTimer         *enginehistory.TimerScheduledPayload
	consumedInboxIDs []uuid.UUID
}

func replayDefinition(
	definition publicworkflow.Definition,
	historyRows []enginedb.EngineHistory,
	activityTasks []enginedb.EngineActivityTask,
	inboxRows []enginedb.EngineInbox,
) (activationDecision, error) {
	runner, err := newWorkflowRunner(historyRows, activityTasks, inboxRows)
	if err != nil {
		return activationDecision{}, err
	}

	return runner.execute(definition)
}

func newWorkflowRunner(
	historyRows []enginedb.EngineHistory,
	activityTasks []enginedb.EngineActivityTask,
	inboxRows []enginedb.EngineInbox,
) (*workflowRunner, error) {
	if len(historyRows) == 0 {
		return nil, fmt.Errorf("workflow replay requires at least one history event")
	}
	if historyRows[0].EventType != enginehistory.EventWorkflowStarted {
		return nil, fmt.Errorf("first history event must be %s", enginehistory.EventWorkflowStarted)
	}

	startedPayload := enginehistory.WorkflowStartedPayload{}
	if err := enginehistory.UnmarshalPayload(historyRows[0].Payload, &startedPayload); err != nil {
		return nil, fmt.Errorf("decode workflow.started payload: %w", err)
	}

	runner := &workflowRunner{
		input:             cloneRaw(startedPayload.Input),
		nextSequence:      historyRows[len(historyRows)-1].SequenceNo + 1,
		pendingActivities: make(map[string]activityOutcome),
		pendingSignals:    make(map[string][]pendingSignal),
		pendingTimers:     make(map[string]pendingTimer),
	}

	for i := 1; i < len(historyRows); i++ {
		row := historyRows[i]
		payload, err := enginehistory.DecodePayload(row.EventType, row.Payload)
		if err != nil {
			return nil, fmt.Errorf("decode history event %s: %w", row.EventType, err)
		}
		switch row.EventType {
		case enginehistory.EventWorkflowSuspended, enginehistory.EventWorkflowResumed:
			continue
		}
		if statusPayload, ok := payload.(*enginehistory.CustomStatusUpdatedPayload); ok {
			runner.customStatus = cloneRaw(statusPayload.Status)
		}

		runner.replayEvents = append(runner.replayEvents, decodedEvent{
			EventType: row.EventType,
			Payload:   payload,
		})
	}

	for i := range activityTasks {
		task := activityTasks[i]
		switch task.Status {
		case enginedb.EngineActivityTaskStatusCompleted:
			runner.pendingActivities[task.ActivityKey] = activityOutcome{
				completed: &enginehistory.ActivityCompletedPayload{
					ActivityKey:  task.ActivityKey,
					ActivityType: task.ActivityType,
					Output:       cloneRaw(task.Output),
				},
			}
		case enginedb.EngineActivityTaskStatusFailed:
			runner.pendingActivities[task.ActivityKey] = activityOutcome{
				failed: &enginehistory.ActivityFailedPayload{
					ActivityKey:  task.ActivityKey,
					ActivityType: task.ActivityType,
					ErrorCode:    stringValue(task.LastErrorCode),
					ErrorMessage: stringValue(task.LastErrorMessage),
				},
			}
		}
	}

	for i := range inboxRows {
		inboxRow := inboxRows[i]
		switch inboxRow.Kind {
		case "timer":
			timerPayload := enginehistory.TimerScheduledPayload{}
			if err := enginehistory.UnmarshalPayload(inboxRow.Payload, &timerPayload); err != nil {
				return nil, fmt.Errorf("decode timer inbox payload: %w", err)
			}
			runner.pendingTimers[timerPayload.TimerKey] = pendingTimer{
				inboxID: inboxRow.ID,
				payload: timerPayload,
			}
		case "signal":
			signalPayload := enginehistory.SignalReceivedPayload{}
			if err := enginehistory.UnmarshalPayload(inboxRow.Payload, &signalPayload); err != nil {
				return nil, fmt.Errorf("decode signal inbox payload: %w", err)
			}
			runner.pendingSignals[signalPayload.SignalName] = append(
				runner.pendingSignals[signalPayload.SignalName],
				pendingSignal{inboxID: inboxRow.ID, payload: signalPayload},
			)
		case "cancel":
		default:
			return nil, fmt.Errorf("unsupported inbox kind %q", inboxRow.Kind)
		}

		runner.pendingFrontier = append(runner.pendingFrontier, pendingInboxItem{
			inboxID: inboxRow.ID,
			kind:    inboxRow.Kind,
		})
	}

	return runner, nil
}

func (r *workflowRunner) execute(definition publicworkflow.Definition) (decision activationDecision, err error) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		switch value := recovered.(type) {
		case blockedPanic:
			decision = r.waitingDecision()
			err = nil
		case replayMismatchPanic:
			r.queueEvent(enginehistory.EventWorkflowReplayMismatch, value.payload)
			failure := enginehistory.WorkflowFailedPayload{
				ErrorCode:    "replay_mismatch",
				ErrorMessage: value.payload.Detail,
			}
			r.queueEvent(enginehistory.EventWorkflowFailed, failure)
			decision = r.failedDecision(failure)
			err = nil
		default:
			panicMessage := fmt.Sprintf("workflow panic: %v", value)
			failure := enginehistory.WorkflowFailedPayload{
				ErrorCode:    "workflow_panic",
				ErrorMessage: panicMessage,
			}
			r.advanceState()
			r.queueEvent(enginehistory.EventWorkflowFailed, failure)
			decision = r.failedDecision(failure)
			err = nil
		}
	}()

	runErr := definition.Run(r)
	r.advanceState()

	if runErr != nil {
		if errors.Is(runErr, publicworkflow.ErrCancelled) {
			cancelled := enginehistory.WorkflowCancelledPayload{}
			if next, ok := r.peek(); ok {
				if _, ok := next.Payload.(*enginehistory.WorkflowCancelledPayload); !ok {
					r.replayMismatch(enginehistory.EventWorkflowCancelled, "", next, "workflow cancellation did not match recorded history")
				}
				r.cursor++
			} else {
				r.queueEvent(enginehistory.EventWorkflowCancelled, cancelled)
			}
			if next, ok := r.peek(); ok {
				r.replayMismatch("", "", next, "recorded history contains events after workflow cancellation")
			}
			return r.cancelledDecision(), nil
		}
		if errors.Is(runErr, publicworkflow.ErrContinueAsNew) {
			continuationInput, _ := publicworkflow.ContinueAsNewInput(runErr)
			continuedAsNew := enginehistory.WorkflowContinuedAsNewPayload{Input: cloneRaw(continuationInput)}
			if next, ok := r.peek(); ok {
				recorded, ok := next.Payload.(*enginehistory.WorkflowContinuedAsNewPayload)
				if !ok || !equalJSON(recorded.Input, continuedAsNew.Input) {
					r.replayMismatch(enginehistory.EventWorkflowContinuedAsNew, "", next, "workflow continuation did not match recorded history")
				}
				r.cursor++
			} else {
				r.queueEvent(enginehistory.EventWorkflowContinuedAsNew, continuedAsNew)
			}
			if next, ok := r.peek(); ok {
				r.replayMismatch("", "", next, "recorded history contains events after workflow continuation")
			}
			return r.continuedAsNewDecision(continuedAsNew.Input), nil
		}

		code := "workflow_failed"
		message := runErr.Error()
		var coded codedWorkflowError
		if errors.As(runErr, &coded) {
			code = coded.code
			message = coded.message
		}
		failure := enginehistory.WorkflowFailedPayload{
			ErrorCode:    code,
			ErrorMessage: message,
		}
		if next, ok := r.peek(); ok {
			recorded, ok := next.Payload.(*enginehistory.WorkflowFailedPayload)
			if !ok || recorded.ErrorCode != failure.ErrorCode || recorded.ErrorMessage != failure.ErrorMessage {
				r.replayMismatch(enginehistory.EventWorkflowFailed, "", next, "workflow failure did not match recorded history")
			}
			r.cursor++
		} else {
			r.queueEvent(enginehistory.EventWorkflowFailed, failure)
		}
		if next, ok := r.peek(); ok {
			r.replayMismatch("", "", next, "recorded history contains events after workflow failure")
		}
		return r.failedDecision(failure), nil
	}

	completed := enginehistory.WorkflowCompletedPayload{Result: cloneRaw(r.result)}
	if next, ok := r.peek(); ok {
		recorded, ok := next.Payload.(*enginehistory.WorkflowCompletedPayload)
		if !ok || !equalJSON(recorded.Result, completed.Result) {
			r.replayMismatch(enginehistory.EventWorkflowCompleted, "", next, "workflow completion did not match recorded history")
		}
		r.cursor++
	} else {
		r.queueEvent(enginehistory.EventWorkflowCompleted, completed)
	}
	if next, ok := r.peek(); ok {
		r.replayMismatch("", "", next, "recorded history contains events after workflow completion")
	}

	return r.completedDecision(), nil
}

func (r *workflowRunner) Input(out any) error {
	r.advanceState()
	if out == nil || len(r.input) == 0 {
		return nil
	}
	return json.Unmarshal(r.input, out)
}

func (r *workflowRunner) Activity(key, activityType string, input, out any) error {
	return r.ActivityWithOptions(key, activityType, input, out, publicworkflow.ActivityOptions{})
}

func (r *workflowRunner) ActivityWithOptions(
	key, activityType string,
	input, out any,
	opts publicworkflow.ActivityOptions,
) error {
	if key == "" {
		return publicworkflow.ErrEmptyKey
	}
	r.advanceState()

	normalizedOpts, err := publicworkflow.NormalizeActivityOptions(opts)
	if err != nil {
		return err
	}

	inputRaw, err := enginehistory.MarshalPayload(input)
	if err != nil {
		return err
	}

	if next, ok := r.peek(); ok {
		scheduled, ok := next.Payload.(*enginehistory.ActivityScheduledPayload)
		if !ok || !matchActivityScheduled(scheduled, key, activityType, inputRaw) {
			r.replayMismatch(enginehistory.EventActivityScheduled, key, next, "activity scheduling did not match recorded history")
		}
		r.cursor++
		r.advanceState()
		r.skipRecordedActivityRetryEvents(key)

		if outcomeEvent, ok := r.peek(); ok {
			switch payload := outcomeEvent.Payload.(type) {
			case *enginehistory.ActivityCompletedPayload:
				if payload.ActivityKey == key && payload.ActivityType == activityType {
					r.cursor++
					return unmarshalOptional(payload.Output, out)
				}
			case *enginehistory.ActivityFailedPayload:
				if payload.ActivityKey == key && payload.ActivityType == activityType {
					r.cursor++
					return codedWorkflowError{
						code:    payload.ErrorCode,
						message: payload.ErrorMessage,
					}
				}
			}
		}

		if outcome, ok := r.pendingActivities[key]; ok {
			if outcome.completed != nil && outcome.completed.ActivityType == activityType {
				r.queueEvent(enginehistory.EventActivityCompleted, *outcome.completed)
				return unmarshalOptional(outcome.completed.Output, out)
			}
			if outcome.failed != nil && outcome.failed.ActivityType == activityType {
				r.queueEvent(enginehistory.EventActivityFailed, *outcome.failed)
				return codedWorkflowError{
					code:    outcome.failed.ErrorCode,
					message: outcome.failed.ErrorMessage,
				}
			}
		}

		if nextAfter, ok := r.peek(); ok {
			r.replayMismatch(enginehistory.EventActivityCompleted, key, nextAfter, "activity history is missing a terminal outcome")
		}

		r.blockOnWait(enginehistory.ActivityWait{
			Kind:         enginehistory.WaitKindActivity,
			ActivityKey:  key,
			ActivityType: activityType,
		}, nil, nil)
		return nil
	}

	r.advanceState()
	scheduled := enginehistory.ActivityScheduledPayload{
		ActivityKey:  key,
		ActivityType: activityType,
		Input:        cloneRaw(inputRaw),
	}
	r.newActivity = &newActivityTask{
		Scheduled: scheduled,
		Options:   normalizedOpts,
	}
	r.blockOnWait(
		enginehistory.ActivityWait{
			Kind:         enginehistory.WaitKindActivity,
			ActivityKey:  key,
			ActivityType: activityType,
		},
		&queuedHistoryEvent{EventType: enginehistory.EventActivityScheduled, Payload: mustMarshalPayload(scheduled)},
		nil,
	)
	return nil
}

func (r *workflowRunner) skipRecordedActivityRetryEvents(activityKey string) {
	for {
		next, ok := r.peek()
		if !ok {
			return
		}

		payload, ok := next.Payload.(*enginehistory.ActivityRetryScheduledPayload)
		if !ok || payload.ActivityKey != activityKey {
			return
		}
		r.cursor++
	}
}

func (r *workflowRunner) SleepUntil(key string, at time.Time) error {
	if key == "" {
		return publicworkflow.ErrEmptyKey
	}
	r.advanceState()

	if next, ok := r.peek(); ok {
		scheduled, ok := next.Payload.(*enginehistory.TimerScheduledPayload)
		if !ok || scheduled.TimerKey != key || !scheduled.DueAt.Equal(at) {
			r.replayMismatch(enginehistory.EventTimerScheduled, key, next, "timer scheduling did not match recorded history")
		}
		r.cursor++
		r.advanceState()

		if firedEvent, ok := r.peek(); ok {
			fired, ok := firedEvent.Payload.(*enginehistory.TimerFiredPayload)
			if ok && fired.TimerKey == key {
				r.cursor++
				return nil
			}
		}

		if pending, ok := r.pendingTimers[key]; ok && pending.payload.DueAt.Equal(at) {
			r.removePendingFrontier(pending.inboxID)
			r.queueEvent(enginehistory.EventTimerFired, enginehistory.TimerFiredPayload{TimerKey: key})
			r.consumedInboxIDs = append(r.consumedInboxIDs, pending.inboxID)
			delete(r.pendingTimers, key)
			return nil
		}

		if nextAfter, ok := r.peek(); ok {
			r.replayMismatch(enginehistory.EventTimerFired, key, nextAfter, "timer history is missing a fired event")
		}

		r.blockOnWait(enginehistory.TimerWait{
			Kind:     enginehistory.WaitKindTimer,
			TimerKey: key,
			DueAt:    at,
		}, nil, nil)
		return nil
	}

	r.advanceState()
	scheduled := enginehistory.TimerScheduledPayload{
		TimerKey: key,
		DueAt:    at,
	}
	r.newTimer = &scheduled
	r.blockOnWait(
		enginehistory.TimerWait{
			Kind:     enginehistory.WaitKindTimer,
			TimerKey: key,
			DueAt:    at,
		},
		&queuedHistoryEvent{EventType: enginehistory.EventTimerScheduled, Payload: mustMarshalPayload(scheduled)},
		nil,
	)
	return nil
}

func (r *workflowRunner) ReceiveSignal(name string, out any) error {
	r.advanceState()
	if next, ok := r.peek(); ok {
		recorded, ok := next.Payload.(*enginehistory.SignalReceivedPayload)
		if !ok || recorded.SignalName != name {
			r.replayMismatch(enginehistory.EventSignalReceived, name, next, "signal receive did not match recorded history")
		}
		r.cursor++
		return unmarshalOptional(recorded.Payload, out)
	}

	if signals := r.pendingSignals[name]; len(signals) > 0 {
		nextSignal := signals[0]
		r.pendingSignals[name] = signals[1:]
		r.removePendingFrontier(nextSignal.inboxID)
		r.queueEvent(enginehistory.EventSignalReceived, nextSignal.payload)
		r.consumedInboxIDs = append(r.consumedInboxIDs, nextSignal.inboxID)
		return unmarshalOptional(nextSignal.payload.Payload, out)
	}

	r.blockOnWait(enginehistory.SignalWait{
		Kind:       enginehistory.WaitKindSignal,
		SignalName: name,
	}, nil, nil)
	return nil
}

func (r *workflowRunner) CancellationRequested() bool {
	r.advanceState()
	return r.cancelRequested
}

func (r *workflowRunner) SetCustomStatus(value any) error {
	r.advanceState()

	statusRaw, err := enginehistory.MarshalPayload(value)
	if err != nil {
		return err
	}

	if next, ok := r.peek(); ok {
		recorded, ok := next.Payload.(*enginehistory.CustomStatusUpdatedPayload)
		if !ok || !equalJSON(recorded.Status, statusRaw) {
			r.replayMismatch(enginehistory.EventCustomStatusUpdated, "", next, "custom status update did not match recorded history")
		}
		r.customStatus = cloneRaw(statusRaw)
		r.cursor++
		return nil
	}

	r.customStatus = cloneRaw(statusRaw)
	r.queueEvent(enginehistory.EventCustomStatusUpdated, enginehistory.CustomStatusUpdatedPayload{
		Status: cloneRaw(statusRaw),
	})
	return nil
}

func (r *workflowRunner) SetResult(value any) error {
	r.advanceState()
	resultRaw, err := enginehistory.MarshalPayload(value)
	if err != nil {
		return err
	}
	r.result = cloneRaw(resultRaw)
	return nil
}

func (r *workflowRunner) advanceState() {
	r.consumeRecordedCancels()
	r.consumeFrontierCancels()
}

func (r *workflowRunner) consumeRecordedCancels() {
	for {
		next, ok := r.peek()
		if !ok || next.EventType != enginehistory.EventCancelRequested {
			return
		}
		r.cancelRequested = true
		r.cursor++
	}
}

func (r *workflowRunner) consumeFrontierCancels() {
	if r.cursor != len(r.replayEvents) {
		return
	}

	for len(r.pendingFrontier) > 0 && r.pendingFrontier[0].kind == "cancel" {
		nextCancel := r.pendingFrontier[0]
		r.pendingFrontier = r.pendingFrontier[1:]
		r.cancelRequested = true
		r.queueEvent(enginehistory.EventCancelRequested, enginehistory.CancelRequestedPayload{})
		r.consumedInboxIDs = append(r.consumedInboxIDs, nextCancel.inboxID)
	}
}

func (r *workflowRunner) peek() (decodedEvent, bool) {
	if r.cursor >= len(r.replayEvents) {
		return decodedEvent{}, false
	}
	return r.replayEvents[r.cursor], true
}

func (r *workflowRunner) queueEvent(eventType string, payload any) {
	r.queuedEvents = append(r.queuedEvents, queuedHistoryEvent{
		EventType: eventType,
		Payload:   mustMarshalPayload(payload),
	})
}

func (r *workflowRunner) removePendingFrontier(inboxID uuid.UUID) {
	for idx, item := range r.pendingFrontier {
		if item.inboxID != inboxID {
			continue
		}

		r.pendingFrontier = append(r.pendingFrontier[:idx], r.pendingFrontier[idx+1:]...)
		return
	}

	panic(fmt.Sprintf("frontier inbox %s missing from pending frontier", inboxID))
}

func (r *workflowRunner) blockOnWait(wait any, scheduledEvent *queuedHistoryEvent, _ any) {
	if scheduledEvent != nil {
		r.queuedEvents = append(r.queuedEvents, *scheduledEvent)
	}
	waitingFor, err := enginehistory.MarshalPayload(wait)
	if err != nil {
		panic(fmt.Sprintf("marshal waiting state: %v", err))
	}
	r.waitingFor = waitingFor
	panic(blockedPanic{})
}

func (r *workflowRunner) replayMismatch(expectedType, expectedKey string, actual decodedEvent, detail string) {
	panic(replayMismatchPanic{
		payload: enginehistory.WorkflowReplayMismatchPayload{
			ExpectedType: expectedType,
			ExpectedKey:  expectedKey,
			ActualType:   actual.EventType,
			ActualKey:    enginehistory.EventKey(actual.EventType, actual.Payload),
			Detail:       detail,
		},
	})
}

func (r *workflowRunner) waitingDecision() activationDecision {
	return activationDecision{
		Kind:             decisionWaiting,
		Events:           append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:     r.nextSequence,
		WaitingFor:       cloneRaw(r.waitingFor),
		CustomStatus:     cloneRaw(r.customStatus),
		NewActivity:      r.newActivity,
		NewTimer:         r.newTimer,
		ConsumedInboxIDs: append([]uuid.UUID(nil), r.consumedInboxIDs...),
	}
}

func (r *workflowRunner) completedDecision() activationDecision {
	return activationDecision{
		Kind:             decisionCompleted,
		Events:           append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:     r.nextSequence,
		CustomStatus:     cloneRaw(r.customStatus),
		Result:           cloneRaw(r.result),
		ConsumedInboxIDs: append([]uuid.UUID(nil), r.consumedInboxIDs...),
	}
}

func (r *workflowRunner) failedDecision(failure enginehistory.WorkflowFailedPayload) activationDecision {
	return activationDecision{
		Kind:             decisionFailed,
		Events:           append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:     r.nextSequence,
		CustomStatus:     cloneRaw(r.customStatus),
		ConsumedInboxIDs: append([]uuid.UUID(nil), r.consumedInboxIDs...),
		FailureCode:      failure.ErrorCode,
		FailureMessage:   failure.ErrorMessage,
	}
}

func (r *workflowRunner) cancelledDecision() activationDecision {
	return activationDecision{
		Kind:             decisionCancelled,
		Events:           append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:     r.nextSequence,
		CustomStatus:     cloneRaw(r.customStatus),
		ConsumedInboxIDs: append([]uuid.UUID(nil), r.consumedInboxIDs...),
		FailureCode:      "cancelled",
		FailureMessage:   "workflow cancelled",
	}
}

func (r *workflowRunner) continuedAsNewDecision(input json.RawMessage) activationDecision {
	return activationDecision{
		Kind:              decisionContinuedAsNew,
		Events:            append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:      r.nextSequence,
		CustomStatus:      cloneRaw(r.customStatus),
		ContinuationInput: cloneRaw(input),
		ConsumedInboxIDs:  append([]uuid.UUID(nil), r.consumedInboxIDs...),
	}
}

func matchActivityScheduled(
	recorded *enginehistory.ActivityScheduledPayload,
	key, activityType string,
	input json.RawMessage,
) bool {
	return recorded.ActivityKey == key &&
		recorded.ActivityType == activityType &&
		equalJSON(recorded.Input, input)
}

func mustMarshalPayload(payload any) json.RawMessage {
	raw, err := enginehistory.MarshalPayload(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal history payload: %v", err))
	}
	return raw
}

func unmarshalOptional(raw []byte, out any) error {
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func equalJSON(left, right json.RawMessage) bool {
	if bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right)) {
		return true
	}
	if len(left) == 0 && len(right) == 0 {
		return true
	}

	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
