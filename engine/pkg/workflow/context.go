package workflow

import (
	"encoding/json"
	"errors"
	"time"

	publichistory "github.com/continua-ai/continua/engine/pkg/history"
)

var ErrEmptyKey = errors.New("workflow: stable key is required")

// ErrCancelled is the explicit replay-aware cancellation sentinel.
// Returning it records a terminal workflow.cancelled event and transitions the
// run to CANCELLED; returning nil after observing cancellation still produces
// COMPLETED. Replay consults this sentinel through errors.Is.
var ErrCancelled = errors.New("workflow: cancelled")

// ErrContinueAsNew is the explicit replay-aware continuation sentinel.
// Returning it records a terminal workflow.continued_as_new event and causes
// the engine to atomically create the next run for the same instance. Replay
// consults this sentinel through errors.Is.
var ErrContinueAsNew = errors.New("workflow: continue as new")

type continueAsNewError struct {
	input json.RawMessage
}

func (e *continueAsNewError) Error() string {
	return ErrContinueAsNew.Error()
}

func (e *continueAsNewError) Unwrap() error {
	return ErrContinueAsNew
}

func (e *continueAsNewError) Input() json.RawMessage {
	if len(e.input) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), e.input...)
}

// ContinueAsNew requests that the current run terminate and immediately start a
// fresh run of the same instance with the provided input.
func ContinueAsNew(input any) error {
	raw, err := publichistory.MarshalPayload(input)
	if err != nil {
		return err
	}

	return &continueAsNewError{input: raw}
}

// ContinueAsNewInput extracts the marshaled continuation input from an error
// wrapping ErrContinueAsNew.
func ContinueAsNewInput(err error) (json.RawMessage, bool) {
	var continuation *continueAsNewError
	if errors.As(err, &continuation) {
		return continuation.Input(), true
	}
	return nil, errors.Is(err, ErrContinueAsNew)
}

type Context interface {
	Input(out any) error
	Activity(key, activityType string, input any, out any) error
	ActivityWithOptions(key, activityType string, input any, out any, opts ActivityOptions) error
	SleepUntil(key string, at time.Time) error
	ReceiveSignal(name string, out any) error
	CancellationRequested() bool
	SetCustomStatus(value any) error
	SetResult(value any) error
}
