package jsonraw

import (
	"encoding/json"
	"testing"
)

func TestClone(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		raw      json.RawMessage
		wantNil  bool
		wantJSON string
	}{
		{name: "nil", raw: nil, wantNil: true, wantJSON: "null"},
		{name: "empty non-nil", raw: json.RawMessage{}, wantNil: true, wantJSON: "null"},
		{name: "empty with capacity", raw: make(json.RawMessage, 0, 8), wantNil: true, wantJSON: "null"},
		{name: "real JSON", raw: json.RawMessage(`{"ok":true}`), wantJSON: `{"ok":true}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Clone(tc.raw)
			if tc.wantNil && got != nil {
				t.Fatalf("Clone(%#v) = %#v, want nil", tc.raw, got)
			}
			marshalled, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("json.Marshal(Clone(%#v)) error = %v", tc.raw, err)
			}
			if string(marshalled) != tc.wantJSON {
				t.Fatalf("json.Marshal(Clone(%#v)) = %s, want %s", tc.raw, marshalled, tc.wantJSON)
			}
			if !tc.wantNil {
				got[0] = ' '
				if tc.raw[0] != '{' {
					t.Fatalf("Clone(%s) shares memory with its input; got[0]=%q changed raw[0] to %q", tc.wantJSON, got[0], tc.raw[0])
				}
			}
		})
	}
}
