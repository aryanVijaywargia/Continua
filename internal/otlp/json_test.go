package otlp

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// validSpanFields is a span's ID fields and one link, in mixed-case hex.
const validSpanFields = `"traceId":"0123456789abcdef0123456789ABCDEF","spanId":"0123456789ABCDEF",
	"parentSpanId":"fedcba9876543210",
	"links":[{"traceId":"0123456789ABCDEF0123456789abcdef","spanId":"FEDCBA9876543210"}]`

func TestUnmarshalJSONDecodesHexIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "lower and upper case hex, parent and link IDs",
			body: `{"resourceSpans":[{"scopeSpans":[{"spans":[{` + validSpanFields + `}]}]}]}`,
		},
		{
			name: "original proto field names",
			body: `{"resource_spans":[{"scope_spans":[{"spans":[{
				"trace_id":"0123456789abcdef0123456789ABCDEF","span_id":"0123456789ABCDEF",
				"parent_span_id":"fedcba9876543210",
				"links":[{"trace_id":"0123456789ABCDEF0123456789abcdef","span_id":"FEDCBA9876543210"}]}]}]}]}`,
		},
		{
			// OTLP receivers must ignore unknown fields, including ones that reuse an ID name.
			name: "unknown fields at the export, resource, span, and event levels",
			body: `{"future":{"traceId":"unrelated-value"},"resourceSpans":[{
				"resource":{"future":{"spanId":"unrelated-value"}},"futureList":[{"spanId":"x"}],
				"scopeSpans":[{"spans":[{` + validSpanFields + `,
					"future":{"parentSpanId":"unrelated-value"},
					"events":[{"name":"event","spanId":"unrelated-value","future":{"traceId":"x"}}]}]}]}]}`,
		},
		{
			name: "trailing whitespace",
			body: `{"resourceSpans":[{"scopeSpans":[{"spans":[{` + validSpanFields + `}]}]}]}` + "\n\t ",
		},
		{
			name:    "base64 ID",
			body:    `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"ASNFZ4mrze8BI0VniavN7w=="}]}]}]}`,
			wantErr: true,
		},
		{
			name:    "base64 link ID",
			body:    `{"resourceSpans":[{"scopeSpans":[{"spans":[{"links":[{"spanId":"ASNFZ4mrze8="}]}]}]}]}`,
			wantErr: true,
		},
		{
			// 30 hex digits are not a whole number of base64 groups.
			name:    "hex ID of a length that is not a multiple of 4",
			body:    `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"0123456789abcdef0123456789abcd"}]}]}]}`,
			wantErr: true,
		},
		{
			name:    "non-hex ID",
			body:    `{"resourceSpans":[{"scopeSpans":[{"spans":[{"spanId":"zzzzzzzzzzzzzzzz"}]}]}]}`,
			wantErr: true,
		},
		{
			name:    "trailing data",
			body:    `{} {}`,
			wantErr: true,
		},
		{
			name:    "trailing bracket",
			body:    `{"resourceSpans":[{"scopeSpans":[{"spans":[{` + validSpanFields + `}]}]}]}]`,
			wantErr: true,
		},
		{
			name:    "trailing brace",
			body:    `{"resourceSpans":[{"scopeSpans":[{"spans":[{` + validSpanFields + `}]}]}]}}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var export coltracepb.ExportTraceServiceRequest
			err := UnmarshalJSON([]byte(tt.body), &export)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			span := export.GetResourceSpans()[0].GetScopeSpans()[0].GetSpans()[0]
			assert.Equal(t, "0123456789abcdef0123456789abcdef", hex.EncodeToString(span.GetTraceId()))
			assert.Equal(t, "0123456789abcdef", hex.EncodeToString(span.GetSpanId()))
			assert.Equal(t, "fedcba9876543210", hex.EncodeToString(span.GetParentSpanId()))
			link := span.GetLinks()[0]
			assert.Equal(t, "0123456789abcdef0123456789abcdef", hex.EncodeToString(link.GetTraceId()))
			assert.Equal(t, "fedcba9876543210", hex.EncodeToString(link.GetSpanId()))
		})
	}
}
