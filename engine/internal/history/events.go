package history

import (
	"encoding/json"

	publichistory "github.com/continua-ai/continua/engine/pkg/history"
)

const (
	EventWorkflowStarted        = publichistory.EventWorkflowStarted
	EventWorkflowCompleted      = publichistory.EventWorkflowCompleted
	EventWorkflowFailed         = publichistory.EventWorkflowFailed
	EventWorkflowReplayMismatch = publichistory.EventWorkflowReplayMismatch
	EventActivityScheduled      = publichistory.EventActivityScheduled
	EventActivityCompleted      = publichistory.EventActivityCompleted
	EventActivityFailed         = publichistory.EventActivityFailed
	EventTimerScheduled         = publichistory.EventTimerScheduled
	EventTimerFired             = publichistory.EventTimerFired
	EventSignalReceived         = publichistory.EventSignalReceived
	EventCancelRequested        = publichistory.EventCancelRequested
	EventCustomStatusUpdated    = publichistory.EventCustomStatusUpdated
)

const (
	WaitKindActivity = publichistory.WaitKindActivity
	WaitKindTimer    = publichistory.WaitKindTimer
	WaitKindSignal   = publichistory.WaitKindSignal
)

type WorkflowStartedPayload = publichistory.WorkflowStartedPayload
type WorkflowCompletedPayload = publichistory.WorkflowCompletedPayload
type WorkflowFailedPayload = publichistory.WorkflowFailedPayload
type WorkflowReplayMismatchPayload = publichistory.WorkflowReplayMismatchPayload
type ActivityScheduledPayload = publichistory.ActivityScheduledPayload
type ActivityCompletedPayload = publichistory.ActivityCompletedPayload
type ActivityFailedPayload = publichistory.ActivityFailedPayload
type TimerScheduledPayload = publichistory.TimerScheduledPayload
type TimerFiredPayload = publichistory.TimerFiredPayload
type SignalReceivedPayload = publichistory.SignalReceivedPayload
type CancelRequestedPayload = publichistory.CancelRequestedPayload
type CustomStatusUpdatedPayload = publichistory.CustomStatusUpdatedPayload
type ActivityWait = publichistory.ActivityWait
type TimerWait = publichistory.TimerWait
type SignalWait = publichistory.SignalWait

func MarshalPayload(value any) (json.RawMessage, error) {
	return publichistory.MarshalPayload(value)
}

func UnmarshalPayload(raw []byte, out any) error {
	return publichistory.UnmarshalPayload(raw, out)
}

func DecodePayload(eventType string, raw []byte) (any, error) {
	return publichistory.DecodePayload(eventType, raw)
}

func EventKey(eventType string, payload any) string {
	return publichistory.EventKey(eventType, payload)
}
