package otlp

import (
	"encoding/base64"
	"encoding/hex"
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
// instead of rejecting it. For the hex lengths OTLP uses, a multiple of 4, that
// decoding is exact, so encoding the bytes back to base64 recovers the hex text the
// exporter sent. Only those typed ID fields are converted; unknown subtrees never reach
// them.
func UnmarshalJSON(body []byte, export *coltracepb.ExportTraceServiceRequest) error {
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

// hexIDs replaces each of owner's IDs, which protojson decoded from hex text as base64,
// with the bytes that hex text names. An ID sent as real base64 is not hex and is
// rejected.
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
