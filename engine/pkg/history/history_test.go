package history

import (
	"testing"
	"time"
)

func TestDecodePayloadRoundTripsCancelledAndTerminated(t *testing.T) {
	t.Run("workflow.cancelled", func(t *testing.T) {
		raw, err := MarshalPayload(WorkflowCancelledPayload{})
		if err != nil {
			t.Fatalf("MarshalPayload() error = %v", err)
		}

		payload, err := DecodePayload(EventWorkflowCancelled, raw)
		if err != nil {
			t.Fatalf("DecodePayload() error = %v", err)
		}

		if _, ok := payload.(*WorkflowCancelledPayload); !ok {
			t.Fatalf("expected *WorkflowCancelledPayload, got %T", payload)
		}
	})

	t.Run("workflow.continued_as_new", func(t *testing.T) {
		expected := WorkflowContinuedAsNewPayload{
			Input: []byte(`{"cursor":2,"batch":"next"}`),
		}
		raw, err := MarshalPayload(expected)
		if err != nil {
			t.Fatalf("MarshalPayload() error = %v", err)
		}

		payload, err := DecodePayload(EventWorkflowContinuedAsNew, raw)
		if err != nil {
			t.Fatalf("DecodePayload() error = %v", err)
		}

		typed, ok := payload.(*WorkflowContinuedAsNewPayload)
		if !ok {
			t.Fatalf("expected *WorkflowContinuedAsNewPayload, got %T", payload)
		}
		if string(typed.Input) != string(expected.Input) {
			t.Fatalf("unexpected continued-as-new payload: %s", typed.Input)
		}
	})

	t.Run("workflow.suspended", func(t *testing.T) {
		raw, err := MarshalPayload(WorkflowSuspendedPayload{})
		if err != nil {
			t.Fatalf("MarshalPayload() error = %v", err)
		}

		payload, err := DecodePayload(EventWorkflowSuspended, raw)
		if err != nil {
			t.Fatalf("DecodePayload() error = %v", err)
		}

		if _, ok := payload.(*WorkflowSuspendedPayload); !ok {
			t.Fatalf("expected *WorkflowSuspendedPayload, got %T", payload)
		}
	})

	t.Run("workflow.resumed", func(t *testing.T) {
		raw, err := MarshalPayload(WorkflowResumedPayload{})
		if err != nil {
			t.Fatalf("MarshalPayload() error = %v", err)
		}

		payload, err := DecodePayload(EventWorkflowResumed, raw)
		if err != nil {
			t.Fatalf("DecodePayload() error = %v", err)
		}

		if _, ok := payload.(*WorkflowResumedPayload); !ok {
			t.Fatalf("expected *WorkflowResumedPayload, got %T", payload)
		}
	})

	t.Run("workflow.terminated", func(t *testing.T) {
		expected := WorkflowTerminatedPayload{
			ErrorCode:    "terminated",
			ErrorMessage: "run terminated by operator",
		}
		raw, err := MarshalPayload(expected)
		if err != nil {
			t.Fatalf("MarshalPayload() error = %v", err)
		}

		payload, err := DecodePayload(EventWorkflowTerminated, raw)
		if err != nil {
			t.Fatalf("DecodePayload() error = %v", err)
		}

		typed, ok := payload.(*WorkflowTerminatedPayload)
		if !ok {
			t.Fatalf("expected *WorkflowTerminatedPayload, got %T", payload)
		}
		if typed.ErrorCode != expected.ErrorCode || typed.ErrorMessage != expected.ErrorMessage {
			t.Fatalf("unexpected terminated payload: %+v", typed)
		}
	})

	t.Run("activity.retry_scheduled", func(t *testing.T) {
		expected := ActivityRetryScheduledPayload{
			ActivityKey:     "fetch",
			ActivityType:    "demo.activity",
			FailedAttempt:   2,
			NextAvailableAt: time.Unix(1710000000, 0).UTC(),
			ErrorCode:       "activity_failed",
			ErrorMessage:    "boom",
		}
		raw, err := MarshalPayload(expected)
		if err != nil {
			t.Fatalf("MarshalPayload() error = %v", err)
		}

		payload, err := DecodePayload(EventActivityRetryScheduled, raw)
		if err != nil {
			t.Fatalf("DecodePayload() error = %v", err)
		}

		typed, ok := payload.(*ActivityRetryScheduledPayload)
		if !ok {
			t.Fatalf("expected *ActivityRetryScheduledPayload, got %T", payload)
		}
		if typed.ActivityKey != expected.ActivityKey ||
			typed.ActivityType != expected.ActivityType ||
			typed.FailedAttempt != expected.FailedAttempt ||
			!typed.NextAvailableAt.Equal(expected.NextAvailableAt) ||
			typed.ErrorCode != expected.ErrorCode ||
			typed.ErrorMessage != expected.ErrorMessage {
			t.Fatalf("unexpected retry scheduled payload: %+v", typed)
		}
	})
}
