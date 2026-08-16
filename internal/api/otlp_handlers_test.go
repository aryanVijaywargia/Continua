package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// Acceptance criterion: "Continua exposes a documented OTLP trace ingestion endpoint",
// gated the same way the other preview ingest surfaces are gated (404 while disabled).
func TestOTLPTraces_RouteIsPreviewGated(t *testing.T) {
	ctx := context.Background()
	s := store.New(testutil.TestDB(t))

	apiKey := "otlp-preview-gate-" + uuid.NewString()
	_, err := s.Queries().CreateProject(ctx, platform.CreateProjectParams{
		Name:       "otlp-preview-gate-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)

	body := encodeOTLPExport(t, otlpExportFixture{
		TraceIDHex: newTraceIDHex(),
		Spans: []otlpSpanFixture{{
			Name:      "root",
			SpanIDHex: newSpanIDHex(),
		}},
	})

	call := func(server *Server) int {
		router := newAuthenticatedRouter(t, server, s)
		req := httptest.NewRequest(http.MethodPost, OTLPTracesPath, bytes.NewReader(body))
		req.Header.Set("Content-Type", OTLPTracesContentType)
		req.Header.Set("X-API-Key", apiKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	disabled := NewServer(s, ingest.NewService(s, nil, ingest.NewProcessor(s, nil), nil))
	disabled.otlpIngestEnabled = false
	assert.Equal(t, http.StatusNotFound, call(disabled), "OTLP route must 404 while the preview flag is off")

	enabled := NewServer(s, ingest.NewService(s, nil, ingest.NewProcessor(s, nil), nil))
	enabled.otlpIngestEnabled = true
	enabledCode := call(enabled)
	assert.NotEqual(t, http.StatusNotFound, enabledCode, "OTLP route must be mounted while the preview flag is on")
	assert.Less(t, enabledCode, http.StatusInternalServerError, "enabled OTLP route must not 5xx on a valid export")
}

// Acceptance criterion: "An OTLP trace containing session.id and user.id populates the
// corresponding Continua session."
func TestOTLPTraces_SessionAndUserFromStandardAttributes(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	traceIDHex := newTraceIDHex()
	externalSessionID := "conversation-" + uuid.NewString()[:8]

	body := encodeOTLPExport(t, otlpExportFixture{
		TraceIDHex: traceIDHex,
		Spans: []otlpSpanFixture{{
			Name:      "arithmetic-agent",
			SpanIDHex: newSpanIDHex(),
			Attributes: map[string]any{
				otelAttrSessionID: externalSessionID,
				otelAttrUserID:    "user-456",
			},
		}},
	})

	rec := invokeOTLPTraces(t, server, projectID, body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var exportResp coltracepb.ExportTraceServiceResponse
	require.NoError(t, proto.Unmarshal(rec.Body.Bytes(), &exportResp), "OTLP responses are ExportTraceServiceResponse protobufs")
	if exportResp.GetPartialSuccess() != nil {
		assert.Zero(t, exportResp.GetPartialSuccess().GetRejectedSpans(), "no spans should be rejected")
	}

	trace := requireTraceByExternalID(t, q, projectID, traceIDHex)
	assert.Equal(t, "user-456", derefString(trace.UserID), "trace.user_id comes from the OTel user.id attribute")

	session := requireSessionForTrace(t, q, trace)
	assert.Equal(t, externalSessionID, session.ExternalID, "session is resolved by (project_id, external_id)")
	assert.Equal(t, "user-456", derefString(session.UserID), "session.user_id is materialized from user.id")
}

// Acceptance criteria: session display name and user name normalization, plus the documented
// precedence "explicit Continua native context > OTel/OpenInference standard attributes".
func TestOTLPTraces_SessionNamePrecedence(t *testing.T) {
	t.Run("standard_attributes_populate_name_and_user_name", func(t *testing.T) {
		server, _, q, projectID := newOTLPServer(t)

		traceIDHex := newTraceIDHex()
		externalSessionID := "conversation-" + uuid.NewString()[:8]

		body := encodeOTLPExport(t, otlpExportFixture{
			TraceIDHex: traceIDHex,
			Spans: []otlpSpanFixture{{
				Name:      "arithmetic-agent",
				SpanIDHex: newSpanIDHex(),
				Attributes: map[string]any{
					otelAttrSessionID:       externalSessionID,
					continuaAttrSessionName: "Arithmetic discussion",
					otelAttrUserID:          "user-456",
					otelAttrUserFullName:    "Aryan",
					otelAttrWorkflowName:    "arithmetic-agent",
				},
			}},
		})

		rec := invokeOTLPTraces(t, server, projectID, body)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		trace := requireTraceByExternalID(t, q, projectID, traceIDHex)
		session := requireSessionForTrace(t, q, trace)

		assert.Equal(t, externalSessionID, session.ExternalID)
		assert.Equal(t, "Arithmetic discussion", derefString(session.Name), "continua.session.name is the session display title")
		assert.Equal(t, "user-456", derefString(session.UserID))
		assert.Equal(t, "Aryan", jsonObject(t, session.Metadata)[sessionMetaUserNameKey],
			"user.full_name materializes the canonical session user name")
	})

	t.Run("native_context_beats_standard_attributes", func(t *testing.T) {
		server, _, q, projectID := newOTLPServer(t)

		traceIDHex := newTraceIDHex()
		nativeSessionID := "native-" + uuid.NewString()[:8]

		body := encodeOTLPExport(t, otlpExportFixture{
			TraceIDHex: traceIDHex,
			Spans: []otlpSpanFixture{{
				Name:      "arithmetic-agent",
				SpanIDHex: newSpanIDHex(),
				Attributes: map[string]any{
					// Standard attributes (lower precedence).
					otelAttrSessionID:    "otel-session-should-lose",
					otelAttrUserID:       "otel-user-should-lose",
					otelAttrUserFullName: "Otel Full Name",
					otelAttrUserName:     "otel-user-name",
					// Explicit Continua-native context (highest precedence).
					continuaAttrSessionID:   nativeSessionID,
					continuaAttrSessionName: "Native session title",
					continuaAttrUserID:      "native-user",
					continuaAttrUserName:    "Native User",
				},
			}},
		})

		rec := invokeOTLPTraces(t, server, projectID, body)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		trace := requireTraceByExternalID(t, q, projectID, traceIDHex)
		session := requireSessionForTrace(t, q, trace)

		assert.Equal(t, nativeSessionID, session.ExternalID, "continua.session.id wins over session.id")
		assert.Equal(t, "Native session title", derefString(session.Name))
		assert.Equal(t, "native-user", derefString(session.UserID), "continua.user.id wins over user.id")
		assert.Equal(t, "Native User", jsonObject(t, session.Metadata)[sessionMetaUserNameKey],
			"continua.user.name wins over user.full_name and user.name")
	})
}

// Acceptance criterion: "OpenInference agent, LLM, tool, retrieval, evaluator, and guardrail
// spans map to the correct Continua types." The DB column is spans.type (the API/SDK calls it kind).
func TestOTLPTraces_OpenInferenceSpanKindsMapToContinuaSpanTypes(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	cases := []struct {
		openInferenceKind string
		continuaSpanType  string
	}{
		{"AGENT", "agent"},
		{"LLM", "llm"},
		{"TOOL", "tool"},
		{"RETRIEVER", "retrieval"},
		{"EVALUATOR", "evaluator"},
		{"GUARDRAIL", "guardrail"},
	}

	traceIDHex := newTraceIDHex()
	fixture := otlpExportFixture{TraceIDHex: traceIDHex}
	expectedTypes := make(map[string]string, len(cases))
	for _, testCase := range cases {
		spanIDHex := newSpanIDHex()
		expectedTypes[spanIDHex] = testCase.continuaSpanType
		fixture.Spans = append(fixture.Spans, otlpSpanFixture{
			Name:      "span-" + testCase.openInferenceKind,
			SpanIDHex: spanIDHex,
			Attributes: map[string]any{
				openInferenceSpanKindAttr: testCase.openInferenceKind,
			},
		})
	}

	rec := invokeOTLPTraces(t, server, projectID, encodeOTLPExport(t, fixture))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, traceIDHex)
	spans := spansByExternalID(t, q, trace)
	require.Len(t, spans, len(cases))

	for spanIDHex, expectedType := range expectedTypes {
		span, ok := spans[spanIDHex]
		require.Truef(t, ok, "span %s should be persisted", spanIDHex)
		assert.Equalf(t, expectedType, span.Type, "span %s (%s) should map to Continua type %s", spanIDHex, span.Name, expectedType)
	}
}

// Acceptance criterion: "Session upserts preserve existing non-empty metadata and fill missing
// metadata", with deterministic conflict behavior.
//
// Encoded rule: FIRST NON-EMPTY WINS. A stored non-empty session field is never replaced by a
// later trace, even when the later value is also non-empty; only missing/empty fields get filled,
// and session metadata is merged additively. This keeps session identity stable and makes
// repeated/out-of-order ingestion of the same conversation idempotent, which the issue's
// "never replace a non-empty value with an empty value" rule implies but does not fully specify.
func TestOTLPTraces_SessionUpsertKeepsFirstNonEmptyValuesAndFillsGaps(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)
	ctx := context.Background()

	externalSessionID := "conversation-" + uuid.NewString()[:8]
	seeded, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: externalSessionID,
		Name:       testutil.StrPtr("Original session title"),
		UserID:     testutil.StrPtr("user-original"),
		Metadata:   []byte(`{"tenant":"acme"}`),
	})
	require.NoError(t, err)

	traceIDHex := newTraceIDHex()
	body := encodeOTLPExport(t, otlpExportFixture{
		TraceIDHex: traceIDHex,
		Spans: []otlpSpanFixture{{
			Name:      "arithmetic-agent",
			SpanIDHex: newSpanIDHex(),
			Attributes: map[string]any{
				otelAttrSessionID:       externalSessionID,
				continuaAttrSessionName: "Conflicting new title",
				otelAttrUserID:          "user-conflicting",
				otelAttrUserFullName:    "Aryan",
			},
		}},
	})

	rec := invokeOTLPTraces(t, server, projectID, body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, traceIDHex)
	session := requireSessionForTrace(t, q, trace)
	require.Equal(t, seeded.ID, session.ID, "the existing session must be reused, not duplicated")

	assert.Equal(t, "Original session title", derefString(session.Name), "existing non-empty name is preserved")
	assert.Equal(t, "user-original", derefString(session.UserID), "existing non-empty user id is preserved")

	metadata := jsonObject(t, session.Metadata)
	assert.Equal(t, "acme", metadata["tenant"], "pre-existing session metadata survives the upsert")
	assert.Equal(t, "Aryan", metadata[sessionMetaUserNameKey], "the previously missing user name is filled in")
}

// Acceptance criterion: "Never replace a non-empty value with an empty value."
func TestOTLPTraces_EmptyAttributesNeverOverwriteStoredValues(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)
	ctx := context.Background()

	externalSessionID := "conversation-" + uuid.NewString()[:8]
	seeded, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: externalSessionID,
		Name:       testutil.StrPtr("Stored session title"),
		UserID:     testutil.StrPtr("user-stored"),
		Metadata:   []byte(`{"user_name":"Stored User"}`),
	})
	require.NoError(t, err)

	traceIDHex := newTraceIDHex()
	body := encodeOTLPExport(t, otlpExportFixture{
		TraceIDHex: traceIDHex,
		Spans: []otlpSpanFixture{{
			Name:      "arithmetic-agent",
			SpanIDHex: newSpanIDHex(),
			Attributes: map[string]any{
				otelAttrSessionID: externalSessionID,
				// Emitted but empty, plus user.full_name absent entirely.
				continuaAttrSessionName: "",
				otelAttrUserID:          "",
			},
		}},
	})

	rec := invokeOTLPTraces(t, server, projectID, body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, traceIDHex)
	session := requireSessionForTrace(t, q, trace)
	require.Equal(t, seeded.ID, session.ID)

	assert.Equal(t, "Stored session title", derefString(session.Name), "an empty attribute must not blank the stored name")
	assert.Equal(t, "user-stored", derefString(session.UserID), "an empty attribute must not blank the stored user id")
	assert.Equal(t, "Stored User", jsonObject(t, session.Metadata)[sessionMetaUserNameKey],
		"an absent attribute must not blank the stored user name")
}

// Acceptance criterion: "Unknown attributes are retained but not semantically guessed."
func TestOTLPTraces_UnknownAttributesRetainedButNotGuessed(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	traceIDHex := newTraceIDHex()
	spanIDHex := newSpanIDHex()
	externalSessionID := "conversation-" + uuid.NewString()[:8]

	body := encodeOTLPExport(t, otlpExportFixture{
		TraceIDHex: traceIDHex,
		Spans: []otlpSpanFixture{{
			Name:      "arithmetic-agent",
			SpanIDHex: spanIDHex,
			Attributes: map[string]any{
				otelAttrSessionID: externalSessionID,
				// Deliberately seductive but unmapped keys - none of these may become
				// the session name, the user id, or the user name.
				"name":                "Not a session name",
				"user":                "Not a user id",
				"conversation_name":   "Not a session name either",
				"vendor.custom.flag":  true,
				"vendor.custom.count": int64(7),
			},
		}},
	})

	rec := invokeOTLPTraces(t, server, projectID, body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, traceIDHex)
	session := requireSessionForTrace(t, q, trace)

	assert.Equal(t, externalSessionID, session.ExternalID)
	assert.Empty(t, derefString(session.Name), "an arbitrary `name` attribute must not become the session name")
	assert.Empty(t, derefString(session.UserID), "an arbitrary `user` attribute must not become the user id")
	assert.NotContains(t, jsonObject(t, session.Metadata), sessionMetaUserNameKey,
		"no user name may be guessed from unmapped attributes")

	spans := spansByExternalID(t, q, trace)
	span, ok := spans[spanIDHex]
	require.True(t, ok)

	attrs := spanOTelAttributes(t, span)
	assert.Equal(t, "Not a session name", attrs["name"], "unknown attributes are retained verbatim")
	assert.Equal(t, "Not a user id", attrs["user"])
	assert.Equal(t, "Not a session name either", attrs["conversation_name"])
	assert.Equal(t, true, attrs["vendor.custom.flag"])
	assert.EqualValues(t, 7, attrs["vendor.custom.count"])
}

// Acceptance criterion: "Preserve trace/span IDs, parentage, resource attributes,
// instrumentation scope, and schema URL."
func TestOTLPTraces_PreservesIdentityParentageResourceAndScope(t *testing.T) {
	server, _, q, projectID := newOTLPServer(t)

	traceIDHex := newTraceIDHex()
	rootSpanIDHex := newSpanIDHex()
	childSpanIDHex := newSpanIDHex()
	start := time.Now().UTC().Add(-2 * time.Second).Truncate(time.Millisecond)

	body := encodeOTLPExport(t, otlpExportFixture{
		TraceIDHex: traceIDHex,
		ResourceAttrs: map[string]any{
			"service.name":           "arithmetic-agent",
			"deployment.environment": "staging",
		},
		ResourceSchemaURL: "https://opentelemetry.io/schemas/1.30.0",
		ScopeName:         "openinference.instrumentation.langchain",
		ScopeVersion:      "0.1.29",
		ScopeSchemaURL:    "https://opentelemetry.io/schemas/1.29.0",
		Spans: []otlpSpanFixture{
			{
				Name:      "workflow",
				SpanIDHex: rootSpanIDHex,
				Kind:      tracepb.Span_SPAN_KIND_SERVER,
				Start:     start,
				End:       start.Add(900 * time.Millisecond),
			},
			{
				Name:        "llm-call",
				SpanIDHex:   childSpanIDHex,
				ParentIDHex: rootSpanIDHex,
				Start:       start.Add(100 * time.Millisecond),
				End:         start.Add(500 * time.Millisecond),
				Attributes: map[string]any{
					openInferenceSpanKindAttr: "LLM",
				},
			},
		},
	})

	rec := invokeOTLPTraces(t, server, projectID, body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	trace := requireTraceByExternalID(t, q, projectID, traceIDHex)
	assert.Equal(t, traceIDHex, trace.TraceID, "the 16-byte OTLP trace id is persisted as lowercase hex")

	spans := spansByExternalID(t, q, trace)
	require.Len(t, spans, 2)

	root, ok := spans[rootSpanIDHex]
	require.True(t, ok, "root span %s should be persisted under its OTLP span id", rootSpanIDHex)
	assert.Nil(t, root.ParentSpanID, "a root OTLP span has no parent")
	assert.Equal(t, "workflow", root.Name)
	assert.WithinDuration(t, start, root.StartTime, time.Millisecond)
	require.True(t, root.EndTime.Valid)
	assert.WithinDuration(t, start.Add(900*time.Millisecond), root.EndTime.Time, time.Millisecond)

	child, ok := spans[childSpanIDHex]
	require.True(t, ok, "child span %s should be persisted under its OTLP span id", childSpanIDHex)
	require.NotNil(t, child.ParentSpanID)
	assert.Equal(t, rootSpanIDHex, *child.ParentSpanID, "OTLP parentage survives ingestion")

	metadata := jsonObject(t, child.Metadata)
	resource := nestedObject(t, metadata, spanMetaOTelKey, spanMetaResourceKey)
	resourceAttrs, isObject := resource[spanMetaAttributesKey].(map[string]any)
	require.True(t, isObject, "resource attributes are retained as an object")
	assert.Equal(t, "arithmetic-agent", resourceAttrs["service.name"])
	assert.Equal(t, "staging", resourceAttrs["deployment.environment"])
	assert.Equal(t, "https://opentelemetry.io/schemas/1.30.0", resource[spanMetaSchemaURLKey], "resource schema URL survives ingestion")

	scope := nestedObject(t, metadata, spanMetaOTelKey, spanMetaScopeKey)
	assert.Equal(t, "openinference.instrumentation.langchain", scope[spanMetaScopeNameKey])
	assert.Equal(t, "0.1.29", scope[spanMetaScopeVersionKey])
	assert.Equal(t, "https://opentelemetry.io/schemas/1.29.0", scope[spanMetaSchemaURLKey], "scope schema URL survives ingestion")
}

// Acceptance criterion: "Existing /v1/ingest clients remain backward compatible."
// The native payload is ingested, an OTLP export is ingested into the same project, and the
// native rows must be untouched and still reproducible byte-for-byte by a replayed native batch.
func TestOTLPTraces_NativeIngestRemainsBackwardCompatible(t *testing.T) {
	server, s, q, projectID := newOTLPServer(t)
	ctx := context.Background()

	nativeTraceID := "trace-" + uuid.NewString()[:8]
	nativeSessionID := "session-" + uuid.NewString()[:8]
	nativeBody := `{"batch_key":"native-compat-1","traces":[{"trace_id":"` + nativeTraceID +
		`","name":"native","session_id":"` + nativeSessionID + `","user_id":"native-user"}],` +
		`"spans":[{"trace_id":"` + nativeTraceID + `","span_id":"native-span-1","name":"native span",` +
		`"type":"llm","start_time":"2026-01-01T00:00:00Z"}]}`

	sync := true
	rec := invokeIngest(t, server, projectID, nativeBody, IngestParams{Sync: &sync}, nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	beforeTrace := requireTraceByExternalID(t, q, projectID, nativeTraceID)
	beforeSession := requireSessionForTrace(t, q, beforeTrace)
	beforeSpans := spansByExternalID(t, q, beforeTrace)
	require.Len(t, beforeSpans, 1)
	require.Equal(t, "llm", beforeSpans["native-span-1"].Type)
	require.Equal(t, nativeSessionID, beforeSession.ExternalID)

	otlpTraceIDHex := newTraceIDHex()
	otlpBody := encodeOTLPExport(t, otlpExportFixture{
		TraceIDHex: otlpTraceIDHex,
		Spans: []otlpSpanFixture{{
			Name:      "otlp span",
			SpanIDHex: newSpanIDHex(),
			Attributes: map[string]any{
				otelAttrSessionID: "otlp-" + uuid.NewString()[:8],
				otelAttrUserID:    "otlp-user",
			},
		}},
	})

	otlpRec := invokeOTLPTraces(t, server, projectID, otlpBody)
	require.Equal(t, http.StatusOK, otlpRec.Code, "body: %s", otlpRec.Body.String())

	// The native rows must be entirely unaffected by the OTLP surface.
	afterTrace := requireTraceByExternalID(t, q, projectID, nativeTraceID)
	assert.Equal(t, beforeTrace.ID, afterTrace.ID)
	assert.Equal(t, derefString(beforeTrace.Name), derefString(afterTrace.Name))
	assert.Equal(t, derefString(beforeTrace.UserID), derefString(afterTrace.UserID))
	assert.Equal(t, beforeTrace.SessionID, afterTrace.SessionID)

	afterSession, err := q.GetSession(ctx, beforeSession.ID)
	require.NoError(t, err)
	assert.Equal(t, nativeSessionID, afterSession.ExternalID)
	assert.Equal(t, derefString(beforeSession.Name), derefString(afterSession.Name))
	assert.Equal(t, derefString(beforeSession.UserID), derefString(afterSession.UserID))

	afterSpans := spansByExternalID(t, q, afterTrace)
	require.Len(t, afterSpans, 1)
	assert.Equal(t, beforeSpans["native-span-1"].ID, afterSpans["native-span-1"].ID)
	assert.Equal(t, "llm", afterSpans["native-span-1"].Type)

	// And a fresh native batch on a clean server still ingests identically.
	replayServer := NewServer(s, ingest.NewService(s, nil, ingest.NewProcessor(s, nil), nil))
	replayProjectID := testutil.CreateTestProject(t, ctx, s.Queries())
	replayRec := invokeIngest(t, replayServer, replayProjectID, nativeBody, IngestParams{Sync: &sync}, nil)
	require.Equal(t, http.StatusOK, replayRec.Code, "body: %s", replayRec.Body.String())

	replayTrace := requireTraceByExternalID(t, q, replayProjectID, nativeTraceID)
	replaySpans := spansByExternalID(t, q, replayTrace)
	require.Len(t, replaySpans, 1)
	assert.Equal(t, "llm", replaySpans["native-span-1"].Type)
	assert.Equal(t, "native-user", derefString(replayTrace.UserID))
}

// Acceptance criterion: malformed input is rejected cleanly, not with a panic or a 500.
func TestOTLPTraces_MalformedBodyReturnsClientError(t *testing.T) {
	server, _, _, projectID := newOTLPServer(t)

	cases := map[string][]byte{
		"not_protobuf": []byte("this is definitely not an OTLP protobuf export"),
		"empty_body":   {},
		"truncated_protobuf": func() []byte {
			body := encodeOTLPExport(t, otlpExportFixture{
				TraceIDHex: newTraceIDHex(),
				Spans:      []otlpSpanFixture{{Name: "root", SpanIDHex: newSpanIDHex()}},
			})
			return body[:len(body)/2]
		}(),
		"json_body": func() []byte {
			raw, err := json.Marshal(map[string]any{"resourceSpans": []any{}})
			require.NoError(t, err)
			return raw
		}(),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			require.NotPanics(t, func() {
				rec = invokeOTLPTraces(t, server, projectID, body)
			}, "malformed OTLP bodies must never panic")

			assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest, "malformed OTLP body must be a client error")
			assert.Less(t, rec.Code, http.StatusInternalServerError, "malformed OTLP body must not be a server error")
		})
	}
}
