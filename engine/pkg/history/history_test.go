package history

import (
	"testing"
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
}
