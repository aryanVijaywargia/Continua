// Package jsonraw holds the one shared helper for copying json.RawMessage
// values across the platform module and the engine module.
package jsonraw

import "encoding/json"

// Clone returns an independent copy of raw so callers cannot mutate a buffer
// someone else may still hold. Empty input maps to nil whether it is nil or a
// non-nil zero-length message, because nil marshals as null while an empty
// non-nil json.RawMessage fails MarshalJSON with unexpected end of JSON input.
func Clone(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
