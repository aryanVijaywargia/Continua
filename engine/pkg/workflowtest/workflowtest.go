// Package workflowtest provides an in-memory workflow unit-test kit.
package workflowtest

import (
	"encoding/json"

	"github.com/continua-ai/continua/engine/pkg/workflow"
)

// ActivityHandler is a scripted activity implementation, keyed by activity type.
type ActivityHandler func(input json.RawMessage) (any, error)

// NewActivityError returns an error that fails the activity with a specific code.
// A plain (non-ActivityError) handler error fails with code "activity_failed"
// (same as the real activity worker).
func NewActivityError(code, message string) error {
	panic("workflowtest: not implemented")
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

type Environment struct{}

func NewEnvironment() *Environment {
	return &Environment{}
}

// RegisterActivity scripts the outcome of every activity of the given type. One
// attempt per activity key; retry policies are not simulated.
func (e *Environment) RegisterActivity(activityType string, handler ActivityHandler) {
	panic("workflowtest: not implemented")
}

// RegisterDefinition makes a definition available for ChildWorkflow calls,
// looked up by (Name, Version). Children share the environment's activity
// handlers and child registry but not its signal queue or cancellation.
func (e *Environment) RegisterDefinition(def workflow.Definition) {
	panic("workflowtest: not implemented")
}

// QueueSignal enqueues a signal delivered to the workflow inbox in call order
// (before Execute). Payload is JSON-marshaled.
func (e *Environment) QueueSignal(name string, payload any) error {
	panic("workflowtest: not implemented")
}

// RequestCancellation enqueues a cancellation request in the inbox after any
// already-queued signals (real inbox arrival-order semantics).
func (e *Environment) RequestCancellation() {
	panic("workflowtest: not implemented")
}

// Execute runs def from input to a terminal state, auto-firing timers and
// resolving scripted activities/children between activations. Returns an error
// only for kit-level problems (marshal failure, activation loop cap exceeded).
func (e *Environment) Execute(def workflow.Definition, input any) (*Result, error) {
	panic("workflowtest: not implemented")
}

type Result struct {
	Status            Status
	ErrorCode         string
	ErrorMessage      string
	CustomStatus      json.RawMessage
	ContinuationInput json.RawMessage
	WaitKind          string
	WaitKey           string
}

// DecodeResult unmarshals the workflow result (SetResult value) into out.
func (r *Result) DecodeResult(out any) error {
	panic("workflowtest: not implemented")
}

// HistoryEventTypes returns the full recorded history event types in order,
// starting with history.EventWorkflowStarted.
func (r *Result) HistoryEventTypes() []string {
	panic("workflowtest: not implemented")
}
