package workflow

import (
	"errors"
	"time"
)

var ErrEmptyKey = errors.New("workflow: stable key is required")
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
