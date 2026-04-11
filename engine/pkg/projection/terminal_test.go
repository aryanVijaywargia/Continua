package projection

import (
	"encoding/json"
	"testing"
)

func TestTerminalStatuses(t *testing.T) {
	testCases := []struct {
		name      string
		runStatus string
		wantTrace string
		wantSpan  string
	}{
		{name: "completed", runStatus: "completed", wantTrace: "completed", wantSpan: "completed"},
		{name: "continued_as_new", runStatus: "continued_as_new", wantTrace: "completed", wantSpan: "completed"},
		{name: "cancelled", runStatus: "cancelled", wantTrace: "cancelled", wantSpan: "failed"},
		{name: "terminated", runStatus: "terminated", wantTrace: "failed", wantSpan: "failed"},
		{name: "failed", runStatus: "failed", wantTrace: "failed", wantSpan: "failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotTrace, gotSpan := TerminalStatuses(tc.runStatus)
			if gotTrace != tc.wantTrace || gotSpan != tc.wantSpan {
				t.Fatalf("TerminalStatuses(%q) = (%q, %q), want (%q, %q)", tc.runStatus, gotTrace, gotSpan, tc.wantTrace, tc.wantSpan)
			}
		})
	}
}

func TestTerminalOutputPayload(t *testing.T) {
	t.Run("completed returns result", func(t *testing.T) {
		result := json.RawMessage(`{"ok":true}`)
		got, err := TerminalOutputPayload("completed", result, nil, nil)
		if err != nil {
			t.Fatalf("TerminalOutputPayload() error = %v", err)
		}
		if string(got) != string(result) {
			t.Fatalf("TerminalOutputPayload() = %s, want %s", got, result)
		}
	})

	t.Run("terminal failure returns structured payload", func(t *testing.T) {
		errorCode := "terminated"
		errorMessage := "run terminated by operator"
		got, err := TerminalOutputPayload("terminated", nil, &errorCode, &errorMessage)
		if err != nil {
			t.Fatalf("TerminalOutputPayload() error = %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload["error_code"] != errorCode || payload["error_message"] != errorMessage || payload["status"] != "terminated" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	})

	t.Run("continued_as_new keeps terminal output empty", func(t *testing.T) {
		got, err := TerminalOutputPayload("continued_as_new", nil, nil, nil)
		if err != nil {
			t.Fatalf("TerminalOutputPayload() error = %v", err)
		}
		if got != nil {
			t.Fatalf("TerminalOutputPayload() = %s, want nil", got)
		}
	})
}
