package otlp

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

func TestUnmarshalJSONDecodesHexIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "lower and upper case hex, parent and link IDs",
			body: `{"resourceSpans":[{"scopeSpans":[{"spans":[{
				"traceId":"0123456789abcdef0123456789ABCDEF","spanId":"0123456789ABCDEF",
				"parentSpanId":"fedcba9876543210",
				"links":[{"traceId":"0123456789ABCDEF0123456789abcdef","spanId":"FEDCBA9876543210"}]}]}]}]}`,
		},
		{
			name:    "base64 ID",
			body:    `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"ASNFZ4mrze8BI0VniavN7w=="}]}]}]}`,
			wantErr: true,
		},
		{
			name:    "trailing data",
			body:    `{} {}`,
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
