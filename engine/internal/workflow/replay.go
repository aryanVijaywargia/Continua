package workflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	maxChildDepth                      int32 = 32
	maxContinuationFollowDepth         int32 = 32
	childWaitFailedContinuationCode          = "max_continuation_follow_depth_exceeded"
	childWaitFailedContinuationMessage       = "child workflow continuation follow depth exceeded"
)

const (
	decisionWaiting        decisionKind = "waiting"
	decisionCompleted      decisionKind = "completed"
	decisionFailed         decisionKind = "failed"
	decisionQuarantined    decisionKind = "quarantined"
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
	NewChildWorkflow  *newChildWorkflow
	ContinuationInput json.RawMessage
	ConsumedInboxIDs  []uuid.UUID
	ChildWaitFailures []childWaitFailure
	FailureCode       string
	FailureMessage    string
}

type newActivityTask struct {
	Scheduled enginehistory.ActivityScheduledPayload
	Options   publicworkflow.NormalizedActivityOptions
}

type newChildWorkflow struct {
	Scheduled enginehistory.ChildWorkflowScheduledPayload
}

type childWaitFailure struct {
	ChildKey     string
	ErrorCode    string
	ErrorMessage string
}

type decodedEvent struct {
	EventType string
	Payload   any
}

type activityOutcome struct {
	completed *enginehistory.ActivityCompletedPayload
	failed    *enginehistory.ActivityFailedPayload
}

type childWorkflowOutcome struct {
	childKey                   string
	requestedDefinitionName    string
	requestedDefinitionVersion string
	childInstanceID            uuid.UUID
	childInstanceKey           string
	currentChildRunID          uuid.UUID
	terminalChildRunID         uuid.UUID
	terminalChildRunIDValid    bool
	rootRunID                  uuid.UUID
	childDepth                 int32
	continuationCount          int32
	status                     enginedb.EngineChildWorkflowStatus
	parentWaitFailed           bool
	parentWaitErrorCode        string
	parentWaitErrorMessage     string
	terminalResult             json.RawMessage
	terminalLastErrorCode      string
	terminalLastErrorMessage   string
	terminalRunStatus          enginedb.EngineRunLifecycleStatus
	terminalRunStatusValid     bool
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

type controlledFailurePanic struct {
	failure enginehistory.WorkflowFailedPayload
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
	projectID         uuid.UUID
	runID             uuid.UUID
	rootRunID         uuid.UUID
	childDepth        int32
	validateNewChild  func(enginehistory.ChildWorkflowScheduledPayload) error
	replayEvents      []decodedEvent
	cursor            int
	nextSequence      int32
	customStatus      json.RawMessage
	result            json.RawMessage
	cancelRequested   bool
	pendingActivities map[string]activityOutcome
	pendingChildren   map[string]childWorkflowOutcome
	pendingSignals    map[string][]pendingSignal
	pendingTimers     map[string]pendingTimer
	pendingFrontier   []pendingInboxItem

	queuedEvents      []queuedHistoryEvent
	waitingFor        json.RawMessage
	newActivity       *newActivityTask
	newTimer          *enginehistory.TimerScheduledPayload
	newChildWorkflow  *newChildWorkflow
	consumedInboxIDs  []uuid.UUID
	childWaitFailures []childWaitFailure
}

//nolint:unparam // Test helper wrapper keeps the optional pending activity input explicit.
func replayDefinition(
	definition publicworkflow.Definition,
	historyRows []enginedb.EngineHistory,
	activityTasks []enginedb.EngineActivityTask,
	inboxRows []enginedb.EngineInbox,
) (activationDecision, error) {
	runner, err := newWorkflowRunner(nil, historyRows, activityTasks, nil, inboxRows, nil)
	if err != nil {
		return activationDecision{}, err
	}

	return runner.execute(definition)
}

//nolint:unparam // Test helper wrapper keeps the full runner inputs explicit.
func replayDefinitionForRun(
	definition publicworkflow.Definition,
	run *enginedb.EngineRun,
	historyRows []enginedb.EngineHistory,
	activityTasks []enginedb.EngineActivityTask,
	childWorkflows []enginedb.ListChildWorkflowOutcomesByParentRunRow,
	inboxRows []enginedb.EngineInbox,
) (activationDecision, error) {
	return replayDefinitionForRunWithValidator(
		definition,
		run,
		historyRows,
		activityTasks,
		childWorkflows,
		inboxRows,
		nil,
	)
}

func replayDefinitionForRunWithValidator(
	definition publicworkflow.Definition,
	run *enginedb.EngineRun,
	historyRows []enginedb.EngineHistory,
	activityTasks []enginedb.EngineActivityTask,
	childWorkflows []enginedb.ListChildWorkflowOutcomesByParentRunRow,
	inboxRows []enginedb.EngineInbox,
	validateNewChild func(enginehistory.ChildWorkflowScheduledPayload) error,
) (activationDecision, error) {
	runner, err := newWorkflowRunner(run, historyRows, activityTasks, childWorkflows, inboxRows, validateNewChild)
	if err != nil {
		return activationDecision{}, err
	}

	return runner.execute(definition)
}

func newWorkflowRunner(
	run *enginedb.EngineRun,
	historyRows []enginedb.EngineHistory,
	activityTasks []enginedb.EngineActivityTask,
	childWorkflows []enginedb.ListChildWorkflowOutcomesByParentRunRow,
	inboxRows []enginedb.EngineInbox,
	validateNewChild func(enginehistory.ChildWorkflowScheduledPayload) error,
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
		pendingChildren:   make(map[string]childWorkflowOutcome),
		pendingSignals:    make(map[string][]pendingSignal),
		pendingTimers:     make(map[string]pendingTimer),
		validateNewChild:  validateNewChild,
	}
	if run != nil {
		runner.projectID = run.ProjectID
		runner.runID = run.ID
		runner.rootRunID = run.RootRunID
		runner.childDepth = run.ChildDepth
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

	for i := range childWorkflows {
		child := childWorkflowOutcomeFromRow(&childWorkflows[i])
		runner.pendingChildren[child.childKey] = child
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
			decision = r.quarantinedDecision(&value.payload)
			err = nil
		case controlledFailurePanic:
			decision = r.recordWorkflowFailure(value.failure)
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
		var childErr *publicworkflow.ChildWorkflowError
		if errors.As(runErr, &childErr) {
			code = childErr.Code()
			message = childErr.Message()
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

func (r *workflowRunner) ChildWorkflow(
	childKey, definitionName, definitionVersion string,
	input, out any,
) error {
	return r.ChildWorkflowWithOptions(
		childKey,
		definitionName,
		definitionVersion,
		input,
		out,
		publicworkflow.ChildWorkflowOptions{},
	)
}

func (r *workflowRunner) ChildWorkflowWithOptions(
	childKey, definitionName, definitionVersion string,
	input, out any,
	opts publicworkflow.ChildWorkflowOptions,
) error {
	if childKey == "" {
		return publicworkflow.ErrEmptyKey
	}
	r.advanceState()

	inputRaw, err := enginehistory.MarshalPayload(input)
	if err != nil {
		return err
	}
	instanceKey := opts.InstanceKey
	if instanceKey == "" {
		if r.projectID == uuid.Nil || r.runID == uuid.Nil {
			return codedWorkflowError{code: "missing_run_context", message: "child workflow requires durable run context"}
		}
		instanceKey = defaultChildInstanceKey(r.projectID, r.runID, childKey)
	}

	if next, ok := r.peek(); ok {
		scheduled, ok := next.Payload.(*enginehistory.ChildWorkflowScheduledPayload)
		if !ok || !matchChildWorkflowScheduled(scheduled, childKey, definitionName, definitionVersion, inputRaw, instanceKey) {
			r.replayMismatch(enginehistory.EventChildWorkflowScheduled, childKey, next, "child workflow scheduling did not match recorded history")
		}
		r.cursor++
		r.advanceState()

		startedEvent, ok := r.peek()
		if !ok {
			r.replayMismatch(enginehistory.EventChildWorkflowStarted, childKey, decodedEvent{}, "child workflow history is missing started event")
		}
		started, ok := startedEvent.Payload.(*enginehistory.ChildWorkflowStartedPayload)
		if !ok || started.ChildKey != childKey || started.ChildInstanceKey != instanceKey {
			r.replayMismatch(enginehistory.EventChildWorkflowStarted, childKey, startedEvent, "child workflow started event did not match recorded history")
		}
		r.cursor++
		r.advanceState()

		if outcomeEvent, ok := r.peek(); ok {
			if result, handled, err := r.consumeRecordedChildWorkflowOutcome(childKey, outcomeEvent, out); handled {
				if result {
					r.cursor++
				}
				if err != nil {
					return err
				}
				return nil
			}
		}

		if outcome, ok := r.pendingChildren[childKey]; ok {
			if outcome.requestedDefinitionName != definitionName || outcome.requestedDefinitionVersion != definitionVersion {
				return codedWorkflowError{
					code:    "instance_conflict",
					message: "child workflow definition binding does not match recorded child key",
				}
			}
			consumesPendingOutcome := childWorkflowIsTerminal(outcome.status) || childWorkflowWaitFailed(&outcome)
			if consumesPendingOutcome {
				delete(r.pendingChildren, childKey)
			}
			if err := r.applyPendingChildWorkflowOutcome(&outcome, out); err != nil {
				return err
			}
			if consumesPendingOutcome {
				return nil
			}
		}

		if nextAfter, ok := r.peek(); ok {
			r.replayMismatch(enginehistory.EventChildWorkflowCompleted, childKey, nextAfter, "child workflow history is missing a terminal outcome")
		}

		r.blockOnWait(enginehistory.ChildWorkflowWait{
			Kind:     enginehistory.WaitKindChildWorkflow,
			ChildKey: childKey,
		}, nil, nil)
		return nil
	}

	r.advanceState()
	if r.childDepth >= maxChildDepth {
		return codedWorkflowError{
			code:    "max_child_depth_exceeded",
			message: "child workflow depth limit exceeded",
		}
	}
	scheduled := enginehistory.ChildWorkflowScheduledPayload{
		ChildKey:          childKey,
		DefinitionName:    definitionName,
		DefinitionVersion: definitionVersion,
		Input:             cloneRaw(inputRaw),
		ChildInstanceKey:  instanceKey,
	}
	if r.validateNewChild != nil {
		if err := r.validateNewChild(scheduled); err != nil {
			return err
		}
	}
	r.newChildWorkflow = &newChildWorkflow{Scheduled: scheduled}
	r.blockOnWait(
		enginehistory.ChildWorkflowWait{
			Kind:     enginehistory.WaitKindChildWorkflow,
			ChildKey: childKey,
		},
		&queuedHistoryEvent{EventType: enginehistory.EventChildWorkflowScheduled, Payload: mustMarshalPayload(scheduled)},
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

func (r *workflowRunner) Now() time.Time {
	r.advanceState()

	if next, ok := r.peek(); ok {
		recorded, ok := next.Payload.(*enginehistory.WorkflowTimeRecordedPayload)
		if !ok {
			r.replayMismatch(enginehistory.EventWorkflowTimeRecorded, "", next, "workflow time read did not match recorded history")
		}
		r.cursor++
		return recorded.Now
	}

	recorded := time.Now().UTC()
	r.queueEvent(enginehistory.EventWorkflowTimeRecorded, enginehistory.WorkflowTimeRecordedPayload{
		Now: recorded,
	})
	return recorded
}

func (r *workflowRunner) Sleep(key string, d time.Duration) error {
	if key == "" {
		return publicworkflow.ErrEmptyKey
	}

	base := r.Now()
	return r.SleepUntil(key, base.Add(d))
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

func (r *workflowRunner) SideEffect(key string, fn func() (any, error), out any) error {
	if key == "" {
		return publicworkflow.ErrEmptyKey
	}
	if fn == nil {
		return errors.New("workflow: SideEffect fn must not be nil")
	}
	r.advanceState()

	if next, ok := r.peek(); ok {
		recorded, ok := next.Payload.(*enginehistory.WorkflowSideEffectRecordedPayload)
		if !ok || recorded.SideEffectKey != key {
			r.replayMismatch(enginehistory.EventWorkflowSideEffectRecorded, key, next, "workflow side effect did not match recorded history")
		}
		r.cursor++
		return unmarshalOptional(recorded.Value, out)
	}

	value, err := fn()
	if err != nil {
		return err
	}
	valueRaw, err := enginehistory.MarshalPayload(value)
	if err != nil {
		return err
	}
	recorded := enginehistory.WorkflowSideEffectRecordedPayload{
		SideEffectKey: key,
		Value:         cloneRaw(valueRaw),
	}
	r.queueEvent(enginehistory.EventWorkflowSideEffectRecorded, recorded)
	return unmarshalOptional(recorded.Value, out)
}

func (r *workflowRunner) GetVersion(changeID string, minSupported, maxSupported int) int {
	r.advanceState()
	if changeID == "" {
		r.failControlled("version_invalid", "workflow version change_id is required")
	}
	if minSupported < 1 {
		r.failControlled("version_invalid", "workflow version min_supported must be at least 1")
	}
	if minSupported > maxSupported {
		r.failControlled("version_invalid", "workflow version min_supported must be less than or equal to max_supported")
	}
	if maxSupported > int(^uint32(0)>>1) {
		r.failControlled("version_invalid", "workflow version max_supported exceeds int32")
	}

	if next, ok := r.peek(); ok {
		recorded, ok := next.Payload.(*enginehistory.WorkflowVersionMarkerPayload)
		if !ok {
			return minSupported
		}
		if recorded.ChangeID != changeID {
			r.replayMismatch(enginehistory.EventWorkflowVersionMarker, changeID, next, "workflow version marker did not match recorded history")
		}
		r.cursor++
		if recorded.Version < int32(minSupported) {
			r.failControlled("version_unsupported", fmt.Sprintf("workflow version %d for change %q is below min_supported %d", recorded.Version, changeID, minSupported))
		}
		return int(recorded.Version)
	}

	recorded := enginehistory.WorkflowVersionMarkerPayload{
		ChangeID: changeID,
		Version:  int32(maxSupported),
	}
	r.queueEvent(enginehistory.EventWorkflowVersionMarker, recorded)
	return maxSupported
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
		payload: replayMismatchPayload(expectedType, expectedKey, actual, detail),
	})
}

func replayMismatchPayload(
	expectedType, expectedKey string,
	actual decodedEvent,
	detail string,
) enginehistory.WorkflowReplayMismatchPayload {
	return enginehistory.WorkflowReplayMismatchPayload{
		ExpectedType: expectedType,
		ExpectedKey:  expectedKey,
		ActualType:   actual.EventType,
		ActualKey:    enginehistory.EventKey(actual.EventType, actual.Payload),
		Detail:       detail,
	}
}

func (r *workflowRunner) failControlled(code, message string) {
	panic(controlledFailurePanic{
		failure: enginehistory.WorkflowFailedPayload{
			ErrorCode:    code,
			ErrorMessage: message,
		},
	})
}

func (r *workflowRunner) recordWorkflowFailure(failure enginehistory.WorkflowFailedPayload) activationDecision {
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
	return r.failedDecision(failure)
}

func (r *workflowRunner) waitingDecision() activationDecision {
	return activationDecision{
		Kind:              decisionWaiting,
		Events:            append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:      r.nextSequence,
		WaitingFor:        cloneRaw(r.waitingFor),
		CustomStatus:      cloneRaw(r.customStatus),
		NewActivity:       r.newActivity,
		NewTimer:          r.newTimer,
		NewChildWorkflow:  r.newChildWorkflow,
		ConsumedInboxIDs:  append([]uuid.UUID(nil), r.consumedInboxIDs...),
		ChildWaitFailures: append([]childWaitFailure(nil), r.childWaitFailures...),
	}
}

func (r *workflowRunner) completedDecision() activationDecision {
	return activationDecision{
		Kind:              decisionCompleted,
		Events:            append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:      r.nextSequence,
		CustomStatus:      cloneRaw(r.customStatus),
		Result:            cloneRaw(r.result),
		ConsumedInboxIDs:  append([]uuid.UUID(nil), r.consumedInboxIDs...),
		ChildWaitFailures: append([]childWaitFailure(nil), r.childWaitFailures...),
	}
}

func (r *workflowRunner) failedDecision(failure enginehistory.WorkflowFailedPayload) activationDecision {
	return activationDecision{
		Kind:              decisionFailed,
		Events:            append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:      r.nextSequence,
		CustomStatus:      cloneRaw(r.customStatus),
		ConsumedInboxIDs:  append([]uuid.UUID(nil), r.consumedInboxIDs...),
		FailureCode:       failure.ErrorCode,
		FailureMessage:    failure.ErrorMessage,
		ChildWaitFailures: append([]childWaitFailure(nil), r.childWaitFailures...),
	}
}

func (r *workflowRunner) quarantinedDecision(mismatch *enginehistory.WorkflowReplayMismatchPayload) activationDecision {
	waitingFor := mustMarshalPayload(enginehistory.ReplayMismatchWait{
		Kind:         enginehistory.WaitKindReplayMismatch,
		ExpectedType: mismatch.ExpectedType,
		ExpectedKey:  mismatch.ExpectedKey,
		ActualType:   mismatch.ActualType,
		ActualKey:    mismatch.ActualKey,
		Detail:       mismatch.Detail,
	})
	return activationDecision{
		Kind:           decisionQuarantined,
		NextSequence:   r.nextSequence,
		WaitingFor:     cloneRaw(waitingFor),
		CustomStatus:   cloneRaw(r.customStatus),
		FailureCode:    "replay_mismatch",
		FailureMessage: mismatch.Detail,
	}
}

func (r *workflowRunner) cancelledDecision() activationDecision {
	return activationDecision{
		Kind:              decisionCancelled,
		Events:            append([]queuedHistoryEvent(nil), r.queuedEvents...),
		NextSequence:      r.nextSequence,
		CustomStatus:      cloneRaw(r.customStatus),
		ConsumedInboxIDs:  append([]uuid.UUID(nil), r.consumedInboxIDs...),
		FailureCode:       "cancelled",
		FailureMessage:    "workflow cancelled",
		ChildWaitFailures: append([]childWaitFailure(nil), r.childWaitFailures...),
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
		ChildWaitFailures: append([]childWaitFailure(nil), r.childWaitFailures...),
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

func matchChildWorkflowScheduled(
	recorded *enginehistory.ChildWorkflowScheduledPayload,
	childKey, definitionName, definitionVersion string,
	input json.RawMessage,
	instanceKey string,
) bool {
	return recorded.ChildKey == childKey &&
		recorded.DefinitionName == definitionName &&
		recorded.DefinitionVersion == definitionVersion &&
		recorded.ChildInstanceKey == instanceKey &&
		equalJSON(recorded.Input, input)
}

func defaultChildInstanceKey(projectID, parentRunID uuid.UUID, childKey string) string {
	input := projectID.String() + "\x00" + parentRunID.String() + "\x00" + childKey
	sum := sha256.Sum256([]byte(input))
	return "child:v1:" + hex.EncodeToString(sum[:])
}

func childWorkflowOutcomeFromRow(row *enginedb.ListChildWorkflowOutcomesByParentRunRow) childWorkflowOutcome {
	if row == nil {
		return childWorkflowOutcome{}
	}

	outcome := childWorkflowOutcome{
		childKey:                   row.ChildKey,
		requestedDefinitionName:    row.RequestedDefinitionName,
		requestedDefinitionVersion: row.RequestedDefinitionVersion,
		childInstanceID:            row.ChildInstanceID,
		childInstanceKey:           row.ChildInstanceKey,
		currentChildRunID:          row.CurrentChildRunID,
		rootRunID:                  row.RootRunID,
		childDepth:                 row.ChildDepth,
		continuationCount:          row.ContinuationCount,
		status:                     row.Status,
		parentWaitFailed:           row.ParentWaitFailedAt.Valid,
		parentWaitErrorCode:        stringValue(row.ParentWaitErrorCode),
		parentWaitErrorMessage:     stringValue(row.ParentWaitErrorMessage),
		terminalResult:             cloneRaw(row.TerminalResult),
		terminalLastErrorCode:      stringValue(row.TerminalLastErrorCode),
		terminalLastErrorMessage:   stringValue(row.TerminalLastErrorMessage),
	}
	if row.TerminalChildRunID.Valid {
		outcome.terminalChildRunID = row.TerminalChildRunID.Bytes
		outcome.terminalChildRunIDValid = true
	}
	if row.TerminalRunStatus.Valid {
		outcome.terminalRunStatus = row.TerminalRunStatus.EngineRunLifecycleStatus
		outcome.terminalRunStatusValid = true
	}
	return outcome
}

func (r *workflowRunner) consumeRecordedChildWorkflowOutcome(
	childKey string,
	event decodedEvent,
	out any,
) (consume, handled bool, err error) {
	switch payload := event.Payload.(type) {
	case *enginehistory.ChildWorkflowCompletedPayload:
		if payload.ChildKey != childKey {
			return false, false, nil
		}
		return true, true, unmarshalOptional(payload.Result, out)
	case *enginehistory.ChildWorkflowFailedPayload:
		if payload.ChildKey != childKey {
			return false, false, nil
		}
		return true, true, publicworkflow.NewChildWorkflowError(payload.ErrorCode, payload.ErrorMessage, "failed")
	case *enginehistory.ChildWorkflowCancelledPayload:
		if payload.ChildKey != childKey {
			return false, false, nil
		}
		code, message := normalizedChildCancelledError(payload.ErrorCode, payload.ErrorMessage)
		return true, true, publicworkflow.NewChildWorkflowError(code, message, "cancelled")
	case *enginehistory.ChildWorkflowTerminatedPayload:
		if payload.ChildKey != childKey {
			return false, false, nil
		}
		code, message := normalizedChildTerminatedError(payload.ErrorCode, payload.ErrorMessage)
		return true, true, publicworkflow.NewChildWorkflowError(code, message, "terminated")
	case *enginehistory.ChildWorkflowWaitFailedPayload:
		if payload.ChildKey != childKey {
			return false, false, nil
		}
		code, message := normalizedChildWaitFailure(payload.ErrorCode, payload.ErrorMessage)
		return true, true, publicworkflow.NewChildWorkflowError(code, message, "wait_failed")
	default:
		return false, false, nil
	}
}

func (r *workflowRunner) applyPendingChildWorkflowOutcome(outcome *childWorkflowOutcome, out any) error {
	if childWorkflowWaitFailed(outcome) {
		code, message := normalizedChildWaitFailure(outcome.parentWaitErrorCode, outcome.parentWaitErrorMessage)
		payload := enginehistory.ChildWorkflowWaitFailedPayload{
			ChildKey:          outcome.childKey,
			ChildInstanceID:   outcome.childInstanceID.String(),
			CurrentChildRunID: outcome.currentChildRunID.String(),
			ErrorCode:         code,
			ErrorMessage:      message,
		}
		r.queueEvent(enginehistory.EventChildWorkflowWaitFailed, payload)
		r.childWaitFailures = append(r.childWaitFailures, childWaitFailure{
			ChildKey:     outcome.childKey,
			ErrorCode:    code,
			ErrorMessage: message,
		})
		return publicworkflow.NewChildWorkflowError(code, message, "wait_failed")
	}

	switch outcome.status {
	case enginedb.EngineChildWorkflowStatusCompleted:
		r.queueEvent(enginehistory.EventChildWorkflowCompleted, enginehistory.ChildWorkflowCompletedPayload{
			ChildKey:           outcome.childKey,
			ChildInstanceID:    outcome.childInstanceID.String(),
			TerminalChildRunID: outcome.terminalChildRunID.String(),
			Result:             cloneRaw(outcome.terminalResult),
		})
		return unmarshalOptional(outcome.terminalResult, out)
	case enginedb.EngineChildWorkflowStatusFailed:
		r.queueEvent(enginehistory.EventChildWorkflowFailed, enginehistory.ChildWorkflowFailedPayload{
			ChildKey:           outcome.childKey,
			ChildInstanceID:    outcome.childInstanceID.String(),
			TerminalChildRunID: outcome.terminalChildRunID.String(),
			ErrorCode:          outcome.terminalLastErrorCode,
			ErrorMessage:       outcome.terminalLastErrorMessage,
		})
		return publicworkflow.NewChildWorkflowError(
			outcome.terminalLastErrorCode,
			outcome.terminalLastErrorMessage,
			"failed",
		)
	case enginedb.EngineChildWorkflowStatusCancelled:
		code, message := normalizedChildCancelledError(outcome.terminalLastErrorCode, outcome.terminalLastErrorMessage)
		r.queueEvent(enginehistory.EventChildWorkflowCancelled, enginehistory.ChildWorkflowCancelledPayload{
			ChildKey:           outcome.childKey,
			ChildInstanceID:    outcome.childInstanceID.String(),
			TerminalChildRunID: outcome.terminalChildRunID.String(),
			ErrorCode:          code,
			ErrorMessage:       message,
		})
		return publicworkflow.NewChildWorkflowError(code, message, "cancelled")
	case enginedb.EngineChildWorkflowStatusTerminated:
		code, message := normalizedChildTerminatedError(outcome.terminalLastErrorCode, outcome.terminalLastErrorMessage)
		r.queueEvent(enginehistory.EventChildWorkflowTerminated, enginehistory.ChildWorkflowTerminatedPayload{
			ChildKey:           outcome.childKey,
			ChildInstanceID:    outcome.childInstanceID.String(),
			TerminalChildRunID: outcome.terminalChildRunID.String(),
			ErrorCode:          code,
			ErrorMessage:       message,
		})
		return publicworkflow.NewChildWorkflowError(code, message, "terminated")
	default:
		return nil
	}
}

func childWorkflowIsTerminal(status enginedb.EngineChildWorkflowStatus) bool {
	switch status {
	case enginedb.EngineChildWorkflowStatusCompleted,
		enginedb.EngineChildWorkflowStatusFailed,
		enginedb.EngineChildWorkflowStatusCancelled,
		enginedb.EngineChildWorkflowStatusTerminated:
		return true
	default:
		return false
	}
}

func childWorkflowWaitFailed(outcome *childWorkflowOutcome) bool {
	return outcome.parentWaitFailed || outcome.continuationCount >= maxContinuationFollowDepth
}

func normalizedChildWaitFailure(code, message string) (normalizedCode, normalizedMessage string) {
	if code == "" {
		code = childWaitFailedContinuationCode
	}
	if message == "" {
		message = childWaitFailedContinuationMessage
	}
	return code, message
}

func normalizedChildCancelledError(code, message string) (normalizedCode, normalizedMessage string) {
	if code == "" {
		code = "cancelled"
	}
	if message == "" {
		message = "workflow cancelled"
	}
	return code, message
}

func normalizedChildTerminatedError(code, message string) (normalizedCode, normalizedMessage string) {
	if code == "" {
		code = "terminated"
	}
	if message == "" {
		message = "workflow terminated"
	}
	return code, message
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
