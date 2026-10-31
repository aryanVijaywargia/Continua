package otlp

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// UnmarshalJSON decodes an OTLP/JSON ExportTraceServiceRequest. OTLP/JSON is the
// standard protobuf JSON mapping except that a span's traceId, spanId, and
// parentSpanId, and a link's traceId and spanId, are case-insensitive hex, not base64.
// Unknown fields are discarded so a newer exporter is not rejected wholesale.
//
// Every hex digit is also a base64 character, so protojson decodes a hex ID as base64
// instead of rejecting it. That decoding is not one-to-one: it drops CR and LF and
// accepts padding, so checkIDText first rejects any ID string that is not plain hex.
// For plain hex of the lengths OTLP uses, a multiple of 4, the base64 decoding is exact,
// so encoding the bytes back to base64 recovers the hex text the exporter sent. Only
// those typed ID fields are converted; unknown subtrees never reach them.
func UnmarshalJSON(body []byte, export *coltracepb.ExportTraceServiceRequest) error {
	if err := checkIDText(json.NewDecoder(bytes.NewReader(body)), exportIDFields); err != nil {
		return err
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, export); err != nil {
		return fmt.Errorf("parse otlp json: %w", err)
	}
	for _, resourceSpans := range export.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				if err := hexIDs("span", &span.TraceId, &span.SpanId, &span.ParentSpanId); err != nil {
					return err
				}
				for _, link := range span.GetLinks() {
					if err := hexIDs("link", &link.TraceId, &link.SpanId); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// idFields is the part of the ExportTraceServiceRequest schema that leads to a span or
// link ID, under both names protojson accepts for a field. A nil entry is an ID string;
// any other entry is a repeated message with those fields.
type idFields map[string]idFields

var (
	linkIDFields = idFields{"traceId": nil, "trace_id": nil, "spanId": nil, "span_id": nil}
	spanIDFields = idFields{
		"traceId": nil, "trace_id": nil, "spanId": nil, "span_id": nil,
		"parentSpanId": nil, "parent_span_id": nil, "links": linkIDFields,
	}
	scopeSpansIDFields    = idFields{"spans": spanIDFields}
	resourceSpansIDFields = idFields{"scopeSpans": scopeSpansIDFields, "scope_spans": scopeSpansIDFields}
	exportIDFields        = idFields{"resourceSpans": resourceSpansIDFields, "resource_spans": resourceSpansIDFields}
)

// checkIDText reads the next JSON value, an object with fields or null, and rejects
// any ID string under fields that is not plain hex. Unknown fields are skipped
// unchecked.
func checkIDText(dec *json.Decoder, fields idFields) error {
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("parse otlp json: %w", err)
	}
	if token == nil {
		return nil
	}
	if token != json.Delim('{') {
		return fmt.Errorf("%w: expected a JSON object, got %v", ErrInvalidExport, token)
	}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return fmt.Errorf("parse otlp json: %w", err)
		}
		name, _ := token.(string)
		child, known := fields[name]
		switch {
		case !known:
			var skipped json.RawMessage
			if err = dec.Decode(&skipped); err != nil {
				err = fmt.Errorf("parse otlp json: %w", err)
			}
		case child == nil:
			err = checkHexString(dec, name)
		default:
			err = checkIDTextList(dec, name, child)
		}
		if err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("parse otlp json: %w", err)
	}
	return nil
}

// checkIDTextList runs checkIDText over each element of a JSON array, or null.
func checkIDTextList(dec *json.Decoder, name string, fields idFields) error {
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("parse otlp json: %w", err)
	}
	if token == nil {
		return nil
	}
	if token != json.Delim('[') {
		return fmt.Errorf("%w: %s is not a JSON array", ErrInvalidExport, name)
	}
	for dec.More() {
		if err := checkIDText(dec, fields); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("parse otlp json: %w", err)
	}
	return nil
}

// checkHexString reads an ID, a JSON string or null, and rejects it unless it is hex.
func checkHexString(dec *json.Decoder, name string) error {
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("parse otlp json: %w", err)
	}
	switch id := token.(type) {
	case nil:
		return nil
	case string:
		if _, err := hex.DecodeString(id); err != nil {
			return fmt.Errorf("%w: %s %q is not hex: %w", ErrInvalidExport, name, id, err)
		}
		return nil
	default:
		return fmt.Errorf("%w: %s is not a JSON string", ErrInvalidExport, name)
	}
}

// hexIDs replaces each of owner's IDs, which protojson decoded from plain hex text as
// base64, with the bytes that hex text names. Hex text whose length is not a multiple
// of 4 does not survive the base64 round trip and is rejected.
func hexIDs(owner string, ids ...*[]byte) error {
	for _, id := range ids {
		text := base64.StdEncoding.EncodeToString(*id)
		decoded, err := hex.DecodeString(text)
		if err != nil {
			return fmt.Errorf("%w: %s ID %q is not hex", ErrInvalidExport, owner, text)
		}
		*id = decoded
	}
	return nil
}
