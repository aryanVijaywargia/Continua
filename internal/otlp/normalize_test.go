package otlp

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestNormalizeValidatesIDs(t *testing.T) {
	t.Parallel()

	traceID := bytes.Repeat([]byte{0x01}, 16)
	spanID := bytes.Repeat([]byte{0x02}, 8)
	validLink := func() *tracepb.Span_Link {
		return &tracepb.Span_Link{TraceId: bytes.Repeat([]byte{0x03}, 16), SpanId: bytes.Repeat([]byte{0x04}, 8)}
	}

	tests := []struct {
		name     string
		mutate   func(span *tracepb.Span)
		noParent bool
		wantErr  bool
	}{
		{name: "valid IDs", mutate: func(*tracepb.Span) {}},
		{name: "all-zero trace ID", mutate: func(s *tracepb.Span) { s.TraceId = make([]byte, 16) }, wantErr: true},
		{name: "all-zero span ID", mutate: func(s *tracepb.Span) { s.SpanId = make([]byte, 8) }, wantErr: true},
		{name: "empty parent span ID means no parent", mutate: func(s *tracepb.Span) { s.ParentSpanId = nil }, noParent: true},
		{name: "all-zero parent span ID means no parent", mutate: func(s *tracepb.Span) { s.ParentSpanId = make([]byte, 8) }, noParent: true},
		{name: "empty link trace ID", mutate: func(s *tracepb.Span) { s.Links[0].TraceId = nil }, wantErr: true},
		{name: "short link trace ID", mutate: func(s *tracepb.Span) { s.Links[0].TraceId = traceID[:8] }, wantErr: true},
		{name: "empty link span ID", mutate: func(s *tracepb.Span) { s.Links[0].SpanId = nil }, wantErr: true},
		{name: "short link span ID", mutate: func(s *tracepb.Span) { s.Links[0].SpanId = spanID[:4] }, wantErr: true},
		{
			// The OTel trace API says SDKs SHOULD record a link to an invalid (all-zero)
			// SpanContext when it carries attributes or a TraceState, so exporters send them.
			name: "all-zero link IDs",
			mutate: func(s *tracepb.Span) {
				s.Links[0].TraceId = make([]byte, 16)
				s.Links[0].SpanId = make([]byte, 8)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			span := &tracepb.Span{
				TraceId:           traceID,
				SpanId:            spanID,
				ParentSpanId:      bytes.Repeat([]byte{0x05}, 8),
				Name:              "span",
				StartTimeUnixNano: 1,
				Links:             []*tracepb.Span_Link{validLink()},
			}
			tt.mutate(span)
			export := &coltracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
				ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}},
			}}}

			req, err := Normalize(export, "batch")
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidExport)
				return
			}
			require.NoError(t, err)
			if tt.noParent {
				assert.Nil(t, req.Spans[0].ParentSpanID)
				return
			}
			require.NotNil(t, req.Spans[0].ParentSpanID)
			assert.Equal(t, "0505050505050505", *req.Spans[0].ParentSpanID)
		})
	}
}
