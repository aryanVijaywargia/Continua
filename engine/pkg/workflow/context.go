package workflow

import (
	"errors"
	"time"
)

var ErrEmptyKey = errors.New("workflow: stable key is required")

// ErrCancelled is the explicit replay-aware cancellation sentinel.
// Returning it records a terminal workflow.cancelled event and transitions the
// run to CANCELLED; returning nil after observing cancellation still produces
// COMPLETED. Replay consults this sentinel through errors.Is.
var ErrCancelled = errors.New("workflow: cancelled")

type Context interface {
	Input(out any) error
	Activity(key, activityType string, input any, out any) error
	SleepUntil(key string, at time.Time) error
	ReceiveSignal(name string, out any) error
	CancellationRequested() bool
	SetCustomStatus(value any) error
	SetResult(value any) error
}
