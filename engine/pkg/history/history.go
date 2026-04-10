package history

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	EventWorkflowStarted        = "workflow.started"
	EventWorkflowCompleted      = "workflow.completed"
	EventWorkflowFailed         = "workflow.failed"
	EventWorkflowCancelled      = "workflow.cancelled"
	EventWorkflowSuspended      = "workflow.suspended"
	EventWorkflowResumed        = "workflow.resumed"
	EventWorkflowTerminated     = "workflow.terminated"
	EventWorkflowReplayMismatch = "workflow.replay_mismatch"
	EventActivityScheduled      = "activity.scheduled"
	EventActivityCompleted      = "activity.completed"
	EventActivityFailed         = "activity.failed"
	EventActivityRetryScheduled = "activity.retry_scheduled"
	EventTimerScheduled         = "timer.scheduled"
	EventTimerFired             = "timer.fired"
	EventSignalReceived         = "signal.received"
	EventCancelRequested        = "cancel.requested"
	EventCustomStatusUpdated    = "custom_status.updated"
)

const (
	WaitKindActivity = "activity"
	WaitKindTimer    = "timer"
	WaitKindSignal   = "signal"
)

type WorkflowStartedPayload struct {
	DefinitionName    string          `json:"definition_name"`
	DefinitionVersion string          `json:"definition_version"`
	InstanceKey       string          `json:"instance_key"`
	Input             json.RawMessage `json:"input,omitempty"`
}

type WorkflowCompletedPayload struct {
	Result json.RawMessage `json:"result"`
}

type WorkflowFailedPayload struct {
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type WorkflowCancelledPayload struct{}

type WorkflowSuspendedPayload struct{}

type WorkflowResumedPayload struct{}

type WorkflowTerminatedPayload struct {
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type WorkflowReplayMismatchPayload struct {
	ExpectedType string `json:"expected_type"`
	ExpectedKey  string `json:"expected_key"`
	ActualType   string `json:"actual_type"`
	ActualKey    string `json:"actual_key"`
	Detail       string `json:"detail"`
}

type ActivityScheduledPayload struct {
	ActivityKey  string          `json:"activity_key"`
	ActivityType string          `json:"activity_type"`
	Input        json.RawMessage `json:"input"`
}

type ActivityCompletedPayload struct {
	ActivityKey  string          `json:"activity_key"`
	ActivityType string          `json:"activity_type"`
	Output       json.RawMessage `json:"output"`
}

type ActivityFailedPayload struct {
	ActivityKey  string `json:"activity_key"`
	ActivityType string `json:"activity_type"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type ActivityRetryScheduledPayload struct {
	ActivityKey     string    `json:"activity_key"`
	ActivityType    string    `json:"activity_type"`
	FailedAttempt   int32     `json:"failed_attempt"`
	NextAvailableAt time.Time `json:"next_available_at"`
	ErrorCode       string    `json:"error_code"`
	ErrorMessage    string    `json:"error_message"`
}

type TimerScheduledPayload struct {
	TimerKey string    `json:"timer_key"`
	DueAt    time.Time `json:"due_at"`
}

type TimerFiredPayload struct {
	TimerKey string `json:"timer_key"`
}

type SignalReceivedPayload struct {
	SignalName string          `json:"signal_name"`
	Payload    json.RawMessage `json:"payload"`
}

type CancelRequestedPayload struct{}

type CustomStatusUpdatedPayload struct {
	Status json.RawMessage `json:"status"`
}

type ActivityWait struct {
	Kind         string `json:"kind"`
	ActivityKey  string `json:"activity_key"`
	ActivityType string `json:"activity_type"`
}

type TimerWait struct {
	Kind     string    `json:"kind"`
	TimerKey string    `json:"timer_key"`
	DueAt    time.Time `json:"due_at"`
}

type SignalWait struct {
	Kind       string `json:"kind"`
	SignalName string `json:"signal_name"`
}

func MarshalPayload(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func UnmarshalPayload(raw []byte, out any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func DecodePayload(eventType string, raw []byte) (any, error) {
	if len(raw) == 0 {
		if eventType == EventCancelRequested {
			return CancelRequestedPayload{}, nil
		}
		if eventType == EventWorkflowCancelled {
			return WorkflowCancelledPayload{}, nil
		}
		if eventType == EventWorkflowSuspended {
			return WorkflowSuspendedPayload{}, nil
		}
		if eventType == EventWorkflowResumed {
			return WorkflowResumedPayload{}, nil
		}
		return nil, nil
	}

	payload, err := payloadTarget(eventType)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func EventKey(eventType string, payload any) string {
	switch value := payload.(type) {
	case ActivityScheduledPayload:
		return value.ActivityKey
	case *ActivityScheduledPayload:
		return value.ActivityKey
	case ActivityCompletedPayload:
		return value.ActivityKey
	case *ActivityCompletedPayload:
		return value.ActivityKey
	case ActivityFailedPayload:
		return value.ActivityKey
	case *ActivityFailedPayload:
		return value.ActivityKey
	case ActivityRetryScheduledPayload:
		return value.ActivityKey
	case *ActivityRetryScheduledPayload:
		return value.ActivityKey
	case TimerScheduledPayload:
		return value.TimerKey
	case *TimerScheduledPayload:
		return value.TimerKey
	case TimerFiredPayload:
		return value.TimerKey
	case *TimerFiredPayload:
		return value.TimerKey
	case SignalReceivedPayload:
		return value.SignalName
	case *SignalReceivedPayload:
		return value.SignalName
	default:
		return ""
	}
}

func payloadTarget(eventType string) (any, error) {
	switch eventType {
	case EventWorkflowStarted:
		return &WorkflowStartedPayload{}, nil
	case EventWorkflowCompleted:
		return &WorkflowCompletedPayload{}, nil
	case EventWorkflowFailed:
		return &WorkflowFailedPayload{}, nil
	case EventWorkflowCancelled:
		return &WorkflowCancelledPayload{}, nil
	case EventWorkflowSuspended:
		return &WorkflowSuspendedPayload{}, nil
	case EventWorkflowResumed:
		return &WorkflowResumedPayload{}, nil
	case EventWorkflowTerminated:
		return &WorkflowTerminatedPayload{}, nil
	case EventWorkflowReplayMismatch:
		return &WorkflowReplayMismatchPayload{}, nil
	case EventActivityScheduled:
		return &ActivityScheduledPayload{}, nil
	case EventActivityCompleted:
		return &ActivityCompletedPayload{}, nil
	case EventActivityFailed:
		return &ActivityFailedPayload{}, nil
	case EventActivityRetryScheduled:
		return &ActivityRetryScheduledPayload{}, nil
	case EventTimerScheduled:
		return &TimerScheduledPayload{}, nil
	case EventTimerFired:
		return &TimerFiredPayload{}, nil
	case EventSignalReceived:
		return &SignalReceivedPayload{}, nil
	case EventCancelRequested:
		return &CancelRequestedPayload{}, nil
	case EventCustomStatusUpdated:
		return &CustomStatusUpdatedPayload{}, nil
	default:
		return nil, fmt.Errorf("unknown event type %q", eventType)
	}
}
