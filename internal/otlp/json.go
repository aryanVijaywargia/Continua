package otlp

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// jsonIDFields are the trace export fields that OTLP/JSON encodes as hex: a span's
// traceId, spanId, and parentSpanId, and a link's traceId and spanId. No other message
// in an ExportTraceServiceRequest has a field with these names. The original proto
// names are accepted too, as protojson and the Collector accept them.
var jsonIDFields = map[string]bool{
	"traceId": true, "spanId": true, "parentSpanId": true,
	"trace_id": true, "span_id": true, "parent_span_id": true,
}

// UnmarshalJSON decodes an OTLP/JSON ExportTraceServiceRequest. OTLP/JSON is the
// standard protobuf JSON mapping except that trace and span IDs are case-insensitive
// hex, not base64. The IDs are rewritten to base64 so protojson decodes the rest.
// Unknown fields are discarded so a newer exporter is not rejected wholesale.
func UnmarshalJSON(body []byte, export *coltracepb.ExportTraceServiceRequest) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("parse otlp json: %w", err)
	}
	if decoder.More() {
		return errors.New("parse otlp json: unexpected data after the export")
	}
	if err := hexIDsToBase64(document); err != nil {
		return err
	}

	rewritten, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode otlp json: %w", err)
	}
	return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(rewritten, export)
}

func hexIDsToBase64(node any) error {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			id, isString := child.(string)
			if !isString || !jsonIDFields[key] {
				if err := hexIDsToBase64(child); err != nil {
					return err
				}
				continue
			}
			raw, err := hex.DecodeString(id)
			if err != nil {
				return fmt.Errorf("%w: %s %q is not hex", ErrInvalidExport, key, id)
			}
			value[key] = base64.StdEncoding.EncodeToString(raw)
		}
	case []any:
		for _, child := range value {
			if err := hexIDsToBase64(child); err != nil {
				return err
			}
		}
	}
	return nil
}
