package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// ---------------------------------------------------------------------------
// Canonical attribute names used by the OTLP normalizer.
//
// Precedence (issue #239): explicit Continua-native context beats the
// OpenTelemetry / OpenInference standard attributes, and anything the producer
// did not emit is left unknown rather than guessed.
// ---------------------------------------------------------------------------
const (
	// OpenTelemetry / OpenInference standard attributes.
	otelAttrSessionID         = "session.id"
	otelAttrUserID            = "user.id"
	otelAttrUserName          = "user.name"
	otelAttrUserFullName      = "user.full_name"
	otelAttrWorkflowName      = "gen_ai.workflow.name"
	openInferenceSpanKindAttr = "openinference.span.kind"

	// Continua-native context attributes (highest precedence).
	continuaAttrSessionID   = "continua.session.id"
	continuaAttrSessionName = "continua.session.name"
	continuaAttrUserID      = "continua.user.id"
	continuaAttrUserName    = "continua.user.name"
)

// Canonical persistence contract asserted by the OTLP acceptance tests.
//
//	spans.metadata -> {"otel": {"attributes": {...}, "resource": {"attributes": {...},
//	                   "schema_url": "..."}, "scope": {"name","version","schema_url"}}}
//	sessions.metadata -> {"user_name": "..."} for the resolved display name.
const (
	spanMetaOTelKey         = "otel"
	spanMetaAttributesKey   = "attributes"
	spanMetaResourceKey     = "resource"
	spanMetaScopeKey        = "scope"
	spanMetaSchemaURLKey    = "schema_url"
	spanMetaScopeNameKey    = "name"
	spanMetaScopeVersionKey = "version"
	sessionMetaUserNameKey  = "user_name"
)

// otlpSpanFixture describes one OTLP span to encode into an export request.
type otlpSpanFixture struct {
	Name        string
	SpanIDHex   string
	ParentIDHex string
	Kind        tracepb.Span_SpanKind
	Attributes  map[string]any
	Start       time.Time
	End         time.Time
}

// otlpExportFixture describes a full OTLP/HTTP ExportTraceServiceRequest.
type otlpExportFixture struct {
	TraceIDHex        string
	ResourceAttrs     map[string]any
	ResourceSchemaURL string
	ScopeName         string
	ScopeVersion      string
	ScopeSchemaURL    string
	Spans             []otlpSpanFixture
}

func newOTLPServer(t *testing.T) (*Server, *store.Store, *platform.Queries, uuid.UUID) {
	t.Helper()

	pool := testutil.TestDB(t)
	s := store.New(pool)
	server := NewServer(s, ingest.NewService(s, nil, ingest.NewProcessor(s, nil), nil))
	server.otlpIngestEnabled = true
	projectID := testutil.CreateTestProject(t, context.Background(), s.Queries())

	return server, s, s.Queries(), projectID
}

// invokeOTLPTraces posts a raw OTLP/HTTP protobuf body at the handler, matching the
// direct-handler-invocation convention used by the existing ingest handler tests.
func invokeOTLPTraces(t *testing.T, server *Server, projectID uuid.UUID, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, OTLPTracesPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", OTLPTracesContentType)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.OTLPTraces(rec, req.WithContext(ctx))
	return rec
}

func newTraceIDHex() string {
	id := uuid.New()
	return hex.EncodeToString(id[:])
}

func newSpanIDHex() string {
	id := uuid.New()
	return hex.EncodeToString(id[:8])
}

func encodeOTLPExport(t *testing.T, fixture otlpExportFixture) []byte {
	t.Helper()

	traceID, err := hex.DecodeString(fixture.TraceIDHex)
	require.NoError(t, err)
	require.Len(t, traceID, 16, "OTLP trace ids are 16 bytes")

	spans := make([]*tracepb.Span, 0, len(fixture.Spans))
	for _, spanFixture := range fixture.Spans {
		spanID, decodeErr := hex.DecodeString(spanFixture.SpanIDHex)
		require.NoError(t, decodeErr)
		require.Len(t, spanID, 8, "OTLP span ids are 8 bytes")

		var parentID []byte
		if spanFixture.ParentIDHex != "" {
			parentID, decodeErr = hex.DecodeString(spanFixture.ParentIDHex)
			require.NoError(t, decodeErr)
			require.Len(t, parentID, 8, "OTLP span ids are 8 bytes")
		}

		kind := spanFixture.Kind
		if kind == tracepb.Span_SPAN_KIND_UNSPECIFIED {
			kind = tracepb.Span_SPAN_KIND_INTERNAL
		}

		start := spanFixture.Start
		if start.IsZero() {
			start = time.Now().UTC().Add(-time.Second).Truncate(time.Microsecond)
		}
		end := spanFixture.End
		if end.IsZero() {
			end = start.Add(250 * time.Millisecond)
		}

		spans = append(spans, &tracepb.Span{
			TraceId:           traceID,
			SpanId:            spanID,
			ParentSpanId:      parentID,
			Name:              spanFixture.Name,
			Kind:              kind,
			StartTimeUnixNano: uint64(start.UnixNano()), //nolint:gosec // test timestamps are positive
			EndTimeUnixNano:   uint64(end.UnixNano()),   //nolint:gosec // test timestamps are positive
			Attributes:        keyValues(t, spanFixture.Attributes),
			Status:            &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK},
		})
	}

	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{
				Attributes: keyValues(t, fixture.ResourceAttrs),
			},
			SchemaUrl: fixture.ResourceSchemaURL,
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{
					Name:    fixture.ScopeName,
					Version: fixture.ScopeVersion,
				},
				SchemaUrl: fixture.ScopeSchemaURL,
				Spans:     spans,
			}},
		}},
	}

	body, err := proto.Marshal(req)
	require.NoError(t, err)
	return body
}

func keyValues(t *testing.T, attrs map[string]any) []*commonpb.KeyValue {
	t.Helper()

	if len(attrs) == 0 {
		return nil
	}

	out := make([]*commonpb.KeyValue, 0, len(attrs))
	for key, value := range attrs {
		out = append(out, &commonpb.KeyValue{Key: key, Value: anyValue(t, value)})
	}
	return out
}

func anyValue(t *testing.T, value any) *commonpb.AnyValue {
	t.Helper()

	switch typed := value.(type) {
	case string:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: typed}}
	case bool:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: typed}}
	case int:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: int64(typed)}}
	case int64:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: typed}}
	case float64:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: typed}}
	default:
		t.Fatalf("unsupported OTLP attribute value type %T", value)
		return nil
	}
}

// ---------------------------------------------------------------------------
// Persistence assertions
// ---------------------------------------------------------------------------

func requireTraceByExternalID(t *testing.T, q *platform.Queries, projectID uuid.UUID, traceIDHex string) platform.Trace {
	t.Helper()

	trace, err := q.GetTraceByExternalID(context.Background(), platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceIDHex,
	})
	require.NoError(t, err, "OTLP trace %s should be persisted", traceIDHex)
	return trace
}

func requireSessionForTrace(t *testing.T, q *platform.Queries, trace platform.Trace) platform.Session {
	t.Helper()

	require.True(t, trace.SessionID.Valid, "trace should be linked to a materialized session")
	session, err := q.GetSession(context.Background(), uuid.UUID(trace.SessionID.Bytes))
	require.NoError(t, err)
	return session
}

func spansByExternalID(t *testing.T, q *platform.Queries, trace platform.Trace) map[string]platform.Span {
	t.Helper()

	spans, err := q.ListSpansByTrace(context.Background(), platform.ListSpansByTraceParams{
		TraceID: trace.ID,
	})
	require.NoError(t, err)

	out := make(map[string]platform.Span, len(spans))
	for _, span := range spans {
		out[span.SpanID] = span
	}
	return out
}

func jsonObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	if len(raw) == 0 {
		return map[string]any{}
	}

	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func nestedObject(t *testing.T, parent map[string]any, keys ...string) map[string]any {
	t.Helper()

	current := parent
	for _, key := range keys {
		value, ok := current[key]
		require.Truef(t, ok, "expected metadata key %q in %v", key, current)
		typed, ok := value.(map[string]any)
		require.Truef(t, ok, "expected metadata key %q to be an object, got %T", key, value)
		current = typed
	}
	return current
}

// spanOTelAttributes returns the verbatim OTLP span attributes retained on a persisted span.
func spanOTelAttributes(t *testing.T, span platform.Span) map[string]any {
	t.Helper()

	return nestedObject(t, jsonObject(t, span.Metadata), spanMetaOTelKey, spanMetaAttributesKey)
}
