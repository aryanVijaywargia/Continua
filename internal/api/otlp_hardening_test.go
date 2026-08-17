package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/otlp"
)

// ---------------------------------------------------------------------------
// Helpers. These build raw exports so the hardening tests can reach shapes the
// acceptance fixtures deliberately do not model (NUL bytes, non-finite doubles,
// very large resources, very high span counts).
// ---------------------------------------------------------------------------

// postOTLP invokes the handler with explicit transport headers so the tests can
// exercise Content-Type and Content-Encoding negotiation.
func postOTLP(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	body []byte,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, OTLPTracesPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", OTLPTracesContentType)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.OTLPTraces(rec, req.WithContext(ctx))
	return rec
}

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, err := writer.Write(raw)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

// rawExport builds an ExportTraceServiceRequest without the acceptance fixtures'
// validation, so a test can express a deliberately hostile payload.
func rawExport(traceID []byte, resourceAttrs, spanAttrs []*commonpb.KeyValue, spanCount int, spanName string) *coltracepb.ExportTraceServiceRequest {
	start := time.Now().UTC().Add(-time.Second)

	spans := make([]*tracepb.Span, 0, spanCount)
	for i := range spanCount {
		spanID := make([]byte, 8)
		spanID[0] = byte(i % 251)
		spanID[1] = byte((i / 251) % 251)
		spanID[2] = byte((i / (251 * 251)) % 251)
		spanID[3] = 0x0a
		spans = append(spans, &tracepb.Span{
			TraceId:           traceID,
			SpanId:            spanID,
			Name:              spanName,
			Kind:              tracepb.Span_SPAN_KIND_INTERNAL,
			StartTimeUnixNano: uint64(start.UnixNano()), //nolint:gosec // positive test timestamps
			EndTimeUnixNano:   uint64(start.Add(time.Millisecond).UnixNano()),
			Attributes:        spanAttrs,
			Status:            &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK},
		})
	}

	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource:  &resourcepb.Resource{Attributes: resourceAttrs},
			SchemaUrl: "https://opentelemetry.io/schemas/1.30.0",
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{Name: "hardening", Version: "1.0.0"},
				Spans: spans,
			}},
		}},
	}
}

func stringKV(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func doubleKV(key string, value float64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: value}}}
}

func marshalExport(t *testing.T, export *coltracepb.ExportTraceServiceRequest) []byte {
	t.Helper()

	body, err := proto.Marshal(export)
	require.NoError(t, err)
	return body
}

func newRawTraceID() []byte {
	id := uuid.New()
	return id[:]
}

// ---------------------------------------------------------------------------
// B1: Content-Encoding
// ---------------------------------------------------------------------------

// The OpenTelemetry Collector's otlphttp exporter enables gzip by default, and
// net/http does not decompress request bodies. Without explicit support every
// span from a default-configured Collector is dropped with a non-retryable 400.
func TestOTLPTraces_GzipEncodedExportIsIngested(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	traceID := newRawTraceID()
	export := rawExport(traceID, []*commonpb.KeyValue{stringKV("service.name", "gzip-agent")},
		[]*commonpb.KeyValue{stringKV(otelAttrSessionID, "gzip-session"), stringKV(otelAttrUserID, "gzip-user")},
		1, "gzip-root")

	rec := postOTLP(t, server, projectID, gzipBytes(t, marshalExport(t, export)),
		map[string]string{"Content-Encoding": "gzip"})

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, hex.EncodeToString(traceID))
	session := requireSessionForTrace(t, q, trace)
	require.NotNil(t, session.UserID)
	assert.Equal(t, "gzip-user", *session.UserID)
}

// Decompression introduces a gzip-bomb vector, so the decompressed stream must be
// bounded by the same 5MB limit as the raw body rather than allocated in full.
func TestOTLPTraces_GzipBombIsRejectedWithoutAllocating(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	bomb := gzipBytes(t, bytes.Repeat([]byte{0}, MaxBodySize*4))
	require.Less(t, len(bomb), 64*1024, "the compressed bomb is small; only the decompressed stream is huge")

	rec := postOTLP(t, server, projectID, bomb, map[string]string{"Content-Encoding": "gzip"})

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, "body: %s", rec.Body.String())
}

func TestOTLPTraces_UnknownContentEncodingIsRejected(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	export := rawExport(newRawTraceID(), nil, nil, 1, "root")
	rec := postOTLP(t, server, projectID, marshalExport(t, export),
		map[string]string{"Content-Encoding": "br"})

	assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)
	assert.Less(t, rec.Code, http.StatusInternalServerError, "an unsupported encoding is a client error")
}

// An absent or identity Content-Encoding keeps working exactly as before.
func TestOTLPTraces_IdentityContentEncodingStillWorks(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	export := rawExport(newRawTraceID(), nil, nil, 1, "root")
	rec := postOTLP(t, server, projectID, marshalExport(t, export),
		map[string]string{"Content-Encoding": "identity"})

	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// ---------------------------------------------------------------------------
// B2: resource-attribute fan-out
// ---------------------------------------------------------------------------

// The resource attribute map is copied onto every span in the ResourceSpans, so a
// single large resource attribute multiplies into gigabytes of jsonb written in
// one transaction. The copied block must be bounded.
func TestOTLPTraces_LargeResourceAttributesDoNotFanOutIntoEverySpan(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	traceID := newRawTraceID()
	huge := strings.Repeat("x", 512*1024)
	export := rawExport(traceID,
		[]*commonpb.KeyValue{stringKV("service.name", "fanout-agent"), stringKV("blob", huge)},
		nil, 20, "fanout-span")

	rec := postOTLP(t, server, projectID, marshalExport(t, export), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, hex.EncodeToString(traceID))
	spans := spansByExternalID(t, q, trace)
	require.Len(t, spans, 20)

	total := 0
	for spanID, span := range spans {
		assert.LessOrEqual(t, len(span.Metadata), 2*otlp.MaxResourceAttributesBytes,
			"span %s metadata must stay bounded regardless of resource size", spanID)
		total += len(span.Metadata)
	}
	assert.Less(t, total, 4*1024*1024, "the whole export must not amplify into megabytes of jsonb")
}

// ---------------------------------------------------------------------------
// M1: non-finite doubles
// ---------------------------------------------------------------------------

// json.Marshal rejects NaN/±Inf. The processor discarded that error and wrote an
// explicit SQL NULL, so one NaN attribute destroyed the whole span's metadata.
func TestOTLPTraces_NonFiniteDoubleAttributeKeepsMetadata(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	traceID := newRawTraceID()
	export := rawExport(traceID,
		[]*commonpb.KeyValue{stringKV("service.name", "nan-agent")},
		[]*commonpb.KeyValue{
			doubleKV("score", math.NaN()),
			doubleKV("ratio", math.Inf(1)),
			stringKV("model", "gpt-4o"),
		},
		1, "nan-span")

	rec := postOTLP(t, server, projectID, marshalExport(t, export), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, hex.EncodeToString(traceID))
	spans := spansByExternalID(t, q, trace)
	require.Len(t, spans, 1)

	for _, span := range spans {
		require.NotEmpty(t, span.Metadata, "a NaN attribute must not wipe the span metadata")
		attrs := spanOTelAttributes(t, span)
		assert.Equal(t, "gpt-4o", attrs["model"], "sibling attributes survive a non-finite value")
		assert.Contains(t, attrs, "score", "the non-finite attribute is retained in a JSON-safe form")
	}
}

// ---------------------------------------------------------------------------
// M2: characters Postgres cannot store
// ---------------------------------------------------------------------------

// A NUL byte reaches Postgres as `cannot be converted to text`, which is not an
// ingest.ValidationError, so the handler answered 500 and rolled the batch back.
func TestOTLPTraces_NULByteReturnsClientError(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	cases := map[string]*coltracepb.ExportTraceServiceRequest{
		"attribute_value": rawExport(newRawTraceID(), nil,
			[]*commonpb.KeyValue{stringKV("prompt", "hello\x00world")}, 1, "root"),
		"attribute_key": rawExport(newRawTraceID(), nil,
			[]*commonpb.KeyValue{stringKV("bad\x00key", "value")}, 1, "root"),
		"span_name": rawExport(newRawTraceID(), nil, nil, 1, "root\x00span"),
		"resource_attribute": rawExport(newRawTraceID(),
			[]*commonpb.KeyValue{stringKV("service.name", "svc\x00")}, nil, 1, "root"),
		"session_id": rawExport(newRawTraceID(), nil,
			[]*commonpb.KeyValue{stringKV(otelAttrSessionID, "conv\x001")}, 1, "root"),
	}

	for name, export := range cases {
		t.Run(name, func(t *testing.T) {
			rec := postOTLP(t, server, projectID, marshalExport(t, export), nil)

			assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)
			assert.Less(t, rec.Code, http.StatusInternalServerError,
				"unstorable text must be a client error, never a 500: body %s", rec.Body.String())
		})
	}
}

// ---------------------------------------------------------------------------
// M3: unbounded identity lengths
// ---------------------------------------------------------------------------

// sessions.user_id is covered by a btree index, so a multi-kilobyte user.id fails
// with `index row size ... exceeds btree version 4 maximum` and loses the batch.
func TestOTLPTraces_OversizedIdentityAttributesReturnClientError(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	oversized := strings.Repeat("u", 6*1024)
	cases := map[string]*commonpb.KeyValue{
		"user_id":      stringKV(otelAttrUserID, oversized),
		"session_id":   stringKV(otelAttrSessionID, oversized),
		"session_name": stringKV(continuaAttrSessionName, oversized),
		"user_name":    stringKV(otelAttrUserName, oversized),
	}

	for name, attr := range cases {
		t.Run(name, func(t *testing.T) {
			export := rawExport(newRawTraceID(), nil,
				[]*commonpb.KeyValue{stringKV(otelAttrSessionID, "sized-session-"+name), attr}, 1, "root")

			rec := postOTLP(t, server, projectID, marshalExport(t, export), nil)

			assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)
			assert.Less(t, rec.Code, http.StatusInternalServerError,
				"an oversized identity must be a client error, never a 500: body %s", rec.Body.String())
		})
	}
}

// ---------------------------------------------------------------------------
// M4: per-request span count
// ---------------------------------------------------------------------------

// OTLP packs ~50 bytes per span, so the 5MB body ceiling admits an order of
// magnitude more sequential UpsertSpan round-trips than it was calibrated for.
func TestOTLPTraces_ExcessiveSpanCountIsRejected(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	export := rawExport(newRawTraceID(), nil, nil, otlp.MaxSpansPerExport+1, "s")
	rec := postOTLP(t, server, projectID, marshalExport(t, export), nil)

	assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)
	assert.Less(t, rec.Code, http.StatusInternalServerError)
	assert.Contains(t, strings.ToLower(rec.Body.String()+errorMessageOf(t, rec)), "span")
}

// ---------------------------------------------------------------------------
// M5: terminal trace status
// ---------------------------------------------------------------------------

// OTLP carries no completion signal, so its traces default to `running`. The
// UpsertTrace conflict clause only protected failed/error, so a straggler export
// flipped an already-completed native trace back to running, permanently.
func TestOTLPTraces_DoesNotDowngradeCompletedTraceStatus(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	traceID := newRawTraceID()
	traceIDHex := hex.EncodeToString(traceID)

	sync := true
	nativeBody := fmt.Sprintf(
		`{"batch_key":"otlp-status-guard-1","traces":[{"trace_id":%q,"status":"completed","end_time":%q}]}`,
		traceIDHex, time.Now().UTC().Format(time.RFC3339Nano))
	rec := invokeIngest(t, server, projectID, nativeBody, IngestParams{Sync: &sync}, nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, "completed", requireTraceByExternalID(t, q, projectID, traceIDHex).Status)

	export := rawExport(traceID, nil, nil, 1, "straggler")
	otlpRec := postOTLP(t, server, projectID, marshalExport(t, export), nil)
	require.Equal(t, http.StatusOK, otlpRec.Code, "body: %s", otlpRec.Body.String())

	assert.Equal(t, "completed", requireTraceByExternalID(t, q, projectID, traceIDHex).Status,
		"an OTLP export must never downgrade an already-terminal trace status")
}

// ---------------------------------------------------------------------------
// M6: Content-Type
// ---------------------------------------------------------------------------

// OTLP/JSON is a spec-defined encoding and a documented Collector option.
func TestOTLPTraces_JSONEncodedExportIsIngested(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	traceID := newRawTraceID()
	export := rawExport(traceID, []*commonpb.KeyValue{stringKV("service.name", "json-agent")},
		[]*commonpb.KeyValue{stringKV(otelAttrSessionID, "json-session")}, 1, "json-root")

	body, err := protojson.Marshal(export)
	require.NoError(t, err)

	rec := postOTLP(t, server, projectID, body, map[string]string{"Content-Type": "application/json"})
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, hex.EncodeToString(traceID))
	assert.Equal(t, hex.EncodeToString(traceID), trace.TraceID)
}

func TestOTLPTraces_UnsupportedContentTypeIsRejected(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	export := rawExport(newRawTraceID(), nil, nil, 1, "root")
	rec := postOTLP(t, server, projectID, marshalExport(t, export),
		map[string]string{"Content-Type": "text/plain"})

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code, "body: %s", rec.Body.String())
}

// ---------------------------------------------------------------------------
// M7: protobuf Status error bodies
// ---------------------------------------------------------------------------

// The OTLP spec requires 4xx/5xx bodies to be a protobuf-encoded google.rpc.Status.
// The Collector parses the body as one and discards the details when it cannot,
// which is what makes an encoding mismatch undiagnosable in the field.
func TestOTLPTraces_ErrorResponsesAreProtobufStatus(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	rec := postOTLP(t, server, projectID, []byte("this is definitely not an OTLP protobuf export"), nil)
	require.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)

	assert.Equal(t, OTLPTracesContentType, rec.Header().Get("Content-Type"),
		"OTLP error bodies are protobuf, not Continua JSON")

	var status spb.Status
	require.NoError(t, proto.Unmarshal(rec.Body.Bytes(), &status),
		"OTLP error bodies must be a protobuf google.rpc.Status")
	assert.NotEmpty(t, status.GetMessage(), "the Status carries the explanation the Collector logs")
}

// errorMessageOf reads the human-readable message out of an OTLP error body,
// tolerating both the protobuf Status and the legacy JSON shape.
func errorMessageOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var status spb.Status
	if err := proto.Unmarshal(rec.Body.Bytes(), &status); err == nil && status.GetMessage() != "" {
		return status.GetMessage()
	}
	return rec.Body.String()
}

// ---------------------------------------------------------------------------
// M8: contract coverage
// ---------------------------------------------------------------------------

// CLAUDE.md declares contracts/openapi/openapi.yaml the source of truth for REST.
func TestOTLPTraces_PathIsDeclaredInTheOpenAPIContract(t *testing.T) {
	for _, path := range []string{
		"../../contracts/openapi/openapi.yaml",
		"../../contracts/openapi/openapi.bundle.yaml",
	} {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "\n  "+OTLPTracesPath+":",
			"%s must declare the OTLP ingestion path", path)
	}
}

// ---------------------------------------------------------------------------
// m1: flag documentation
// ---------------------------------------------------------------------------

// INGEST_OTLP_ENABLED gates a preview surface exactly like ENGINE_PUBLIC_API_ENABLED,
// which is documented across the self-hosting and installation guides.
func TestOTLPFlagIsDocumentedAlongsideItsPrecedent(t *testing.T) {
	for _, path := range []string{
		"../../docs-site/guides/self-hosting.mdx",
		"../../docs-site/guides/installation.mdx",
		"../../docs-site/api-reference/auth-and-headers.mdx",
	} {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "INGEST_OTLP_ENABLED",
			"%s documents ENGINE_PUBLIC_API_ENABLED and must document its OTLP counterpart", path)
	}
}

// ---------------------------------------------------------------------------
// m2: session metadata empty-string handling
// ---------------------------------------------------------------------------

// name/user_id use COALESCE(NULLIF(...)), but the metadata merge had no empty
// check, so a stored empty user_name permanently blocked a real one.
func TestOTLPTraces_StoredEmptyUserNameIsFilledByALaterExport(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	externalID := "empty-username-session"
	seeded, err := json.Marshal(map[string]string{sessionMetaUserNameKey: "", "tenant": "acme"})
	require.NoError(t, err)
	_, err = q.UpsertSessionContext(context.Background(), platform.UpsertSessionContextParams{
		ProjectID:  projectID,
		ExternalID: externalID,
		Metadata:   seeded,
	})
	require.NoError(t, err)

	traceID := newRawTraceID()
	export := rawExport(traceID, nil, []*commonpb.KeyValue{
		stringKV(otelAttrSessionID, externalID),
		stringKV(otelAttrUserName, "Aryan"),
	}, 1, "root")

	rec := postOTLP(t, server, projectID, marshalExport(t, export), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, hex.EncodeToString(traceID))
	session := requireSessionForTrace(t, q, trace)
	metadata := jsonObject(t, session.Metadata)
	assert.Equal(t, "Aryan", metadata[sessionMetaUserNameKey],
		"a stored empty user name must not block a real one")
	assert.Equal(t, "acme", metadata["tenant"], "unrelated stored metadata still wins")
}

// ---------------------------------------------------------------------------
// m3: deterministic lock ordering
// ---------------------------------------------------------------------------

// Normalize ordered traces by first-span appearance, so two concurrent exports
// with overlapping trace sets could lock rows in opposite orders and deadlock.
func TestNormalizeOrdersTracesDeterministically(t *testing.T) {
	traceA := make([]byte, 16)
	traceA[0] = 0x01
	traceB := make([]byte, 16)
	traceB[0] = 0xf0

	build := func(first, second []byte) *coltracepb.ExportTraceServiceRequest {
		export := rawExport(first, nil, nil, 1, "first")
		second1 := rawExport(second, nil, nil, 1, "second")
		export.ResourceSpans = append(export.ResourceSpans, second1.GetResourceSpans()...)
		return export
	}

	forward, err := otlp.Normalize(build(traceA, traceB), "batch-forward")
	require.NoError(t, err)
	reverse, err := otlp.Normalize(build(traceB, traceA), "batch-reverse")
	require.NoError(t, err)

	forwardIDs := make([]string, 0, len(forward.Traces))
	for _, trace := range forward.Traces {
		forwardIDs = append(forwardIDs, trace.TraceID)
	}
	reverseIDs := make([]string, 0, len(reverse.Traces))
	for _, trace := range reverse.Traces {
		reverseIDs = append(reverseIDs, trace.TraceID)
	}

	assert.Equal(t, forwardIDs, reverseIDs,
		"traces must be locked in a stable order regardless of span arrival order")
	assert.Equal(t, []string{hex.EncodeToString(traceA), hex.EncodeToString(traceB)}, forwardIDs)
}
