// Package otlp adapts OTLP/HTTP trace exports into Continua's canonical
// session -> trace -> span ingest model. It is an adapter only: every export is
// normalized into an ingest.IngestRequest and written through the shared ingest
// write path, so OTLP producers land in exactly the same tables as native clients.
package otlp

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/pkg/truncation"
)

// Attributes carrying session/user context.
//
// Precedence (issue #239): explicit Continua-native context beats the
// OpenTelemetry / OpenInference standard attributes, and anything the producer did
// not emit is left unknown rather than guessed from arbitrary keys.
const (
	attrSessionID    = "session.id"
	attrUserID       = "user.id"
	attrUserName     = "user.name"
	attrUserFullName = "user.full_name"
	attrWorkflowName = "gen_ai.workflow.name"
	attrSpanKind     = "openinference.span.kind"

	attrContinuaSessionID   = "continua.session.id"
	attrContinuaSessionName = "continua.session.name"
	attrContinuaUserID      = "continua.user.id"
	attrContinuaUserName    = "continua.user.name"
)

// Keys used to retain the raw OTLP envelope on spans.metadata.
const (
	metaOTelKey       = "otel"
	metaAttributesKey = "attributes"
	metaResourceKey   = "resource"
	metaScopeKey      = "scope"
	metaSchemaURLKey  = "schema_url"
	metaNameKey       = "name"
	metaVersionKey    = "version"
	metaTruncatedKey  = "_truncated"
)

const (
	statusCompleted = "completed"
	statusFailed    = "failed"
)

// Limits on one export. OTLP is far denser than native JSON ingest — a span costs
// roughly 50 bytes on the wire — so the shared 5MB body ceiling admits an order of
// magnitude more work than it was calibrated for, and the resource attribute block
// is copied onto every span of its ResourceSpans. Both have to be bounded here.
const (
	// MaxSpansPerExport bounds the sequential span upserts one export can force into
	// a single open transaction.
	MaxSpansPerExport = 10000

	// MaxResourceAttributesBytes bounds the resource attribute block. It is copied
	// onto every span, so its cost is multiplied by the span count.
	MaxResourceAttributesBytes = 8 * 1024

	// maxIdentityBytes bounds attacker-controlled identity values that land in indexed
	// session columns (Postgres btree tuples cap out at 2704 bytes).
	maxIdentityBytes = 512

	// maxDisplayNameBytes bounds the free-text display fields materialized onto a session.
	maxDisplayNameBytes = 1024
)

// ErrInvalidExport marks an export the adapter cannot normalize; callers map it to 4xx.
var ErrInvalidExport = errors.New("invalid otlp export")

// spanTypeByOpenInferenceKind maps OpenInference span kinds onto Continua span types.
var spanTypeByOpenInferenceKind = map[string]string{
	"AGENT":     "agent",
	"CHAIN":     "chain",
	"EMBEDDING": "embedding",
	"EVALUATOR": "evaluator",
	"GUARDRAIL": "guardrail",
	"LLM":       "llm",
	"RETRIEVER": "retrieval",
	"TOOL":      "tool",
}

// Normalize converts an OTLP/HTTP export into a native ingest batch.
func Normalize(export *coltracepb.ExportTraceServiceRequest, batchKey string) (*ingest.IngestRequest, error) {
	// OTLP packs a span into roughly 50 bytes, an order of magnitude denser than the
	// native JSON the shared 5MB body ceiling was calibrated for, so the ceiling alone
	// admits six figures of sequential span upserts inside one open transaction.
	if total := exportSpanCount(export); total > MaxSpansPerExport {
		return nil, fmt.Errorf("%w: export carries %d spans, the limit is %d", ErrInvalidExport, total, MaxSpansPerExport)
	}

	req := &ingest.IngestRequest{BatchKey: batchKey}
	traces := make(map[string]*traceContext)

	for _, resourceSpans := range export.GetResourceSpans() {
		resourceAttrs, err := attributeMap(resourceSpans.GetResource().GetAttributes())
		if err != nil {
			return nil, err
		}
		if err := checkStorableText("resource schema url", resourceSpans.GetSchemaUrl()); err != nil {
			return nil, err
		}

		resource := map[string]any{
			metaAttributesKey: boundedResourceAttributes(resourceAttrs),
			metaSchemaURLKey:  resourceSpans.GetSchemaUrl(),
		}

		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			scope, err := scopeMetadata(scopeSpans)
			if err != nil {
				return nil, err
			}

			for _, span := range scopeSpans.GetSpans() {
				traceIDHex, err := encodeID(span.GetTraceId(), 16, "trace id")
				if err != nil {
					return nil, err
				}

				attrs, err := attributeMap(span.GetAttributes())
				if err != nil {
					return nil, err
				}

				spanInput, err := normalizeSpan(span, traceIDHex, attrs, resource, scope)
				if err != nil {
					return nil, err
				}
				req.Spans = append(req.Spans, spanInput)

				trace, ok := traces[traceIDHex]
				if !ok {
					trace = &traceContext{traceID: traceIDHex}
					traces[traceIDHex] = trace
					req.Traces = append(req.Traces, ingest.TraceInput{})
					trace.index = len(req.Traces) - 1
				}
				trace.observe(span, attrs)
			}
		}
	}

	if len(req.Spans) == 0 {
		return nil, fmt.Errorf("%w: export contains no spans", ErrInvalidExport)
	}

	for _, trace := range traces {
		input, err := trace.traceInput()
		if err != nil {
			return nil, err
		}
		req.Traces[trace.index] = input
	}

	// Traces are upserted in slice order, so the order decides which rows a transaction
	// locks first. Ordering by first-span appearance means two concurrent exports over
	// an overlapping set of traces can take those locks in opposite orders and deadlock;
	// OTLP ingest is synchronous, so nothing retries the loser. Sort by trace id, which
	// every concurrent export agrees on.
	sort.Slice(req.Traces, func(i, j int) bool { return req.Traces[i].TraceID < req.Traces[j].TraceID })

	return req, nil
}

func exportSpanCount(export *coltracepb.ExportTraceServiceRequest) int {
	total := 0
	for _, resourceSpans := range export.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			total += len(scopeSpans.GetSpans())
		}
	}
	return total
}

func scopeMetadata(scopeSpans *tracepb.ScopeSpans) (map[string]any, error) {
	name := scopeSpans.GetScope().GetName()
	version := scopeSpans.GetScope().GetVersion()
	schemaURL := scopeSpans.GetSchemaUrl()

	if err := checkStorableText("scope metadata", name, version, schemaURL); err != nil {
		return nil, err
	}

	return map[string]any{
		metaNameKey:      name,
		metaVersionKey:   version,
		metaSchemaURLKey: schemaURL,
	}, nil
}

// checkStorableText rejects text Postgres cannot store. A NUL byte comes back from the
// driver as `unsupported Unicode escape sequence: ... cannot be converted to text`,
// which is not an ingest validation error, so it used to roll the whole batch back
// behind a 500. Invalid UTF-8 is already rejected during protobuf decoding, which
// leaves NUL as the only unstorable byte an export can carry.
func checkStorableText(label string, values ...string) error {
	for _, value := range values {
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("%w: %s contains a NUL byte", ErrInvalidExport, label)
		}
	}
	return nil
}

// traceContext accumulates the session/user context contributed by every span of one trace.
type traceContext struct {
	traceID  string
	index    int
	native   sessionContext
	standard sessionContext
	workflow string
	rootName string
}

type sessionContext struct {
	sessionID   string
	sessionName string
	userID      string
	userName    string
}

func (t *traceContext) observe(span *tracepb.Span, attrs map[string]any) {
	t.native.sessionID = firstNonEmpty(t.native.sessionID, stringAttr(attrs, attrContinuaSessionID))
	t.native.sessionName = firstNonEmpty(t.native.sessionName, stringAttr(attrs, attrContinuaSessionName))
	t.native.userID = firstNonEmpty(t.native.userID, stringAttr(attrs, attrContinuaUserID))
	t.native.userName = firstNonEmpty(t.native.userName, stringAttr(attrs, attrContinuaUserName))

	t.standard.sessionID = firstNonEmpty(t.standard.sessionID, stringAttr(attrs, attrSessionID))
	t.standard.userID = firstNonEmpty(t.standard.userID, stringAttr(attrs, attrUserID))
	t.standard.userName = firstNonEmpty(
		t.standard.userName,
		firstNonEmpty(stringAttr(attrs, attrUserFullName), stringAttr(attrs, attrUserName)),
	)

	t.workflow = firstNonEmpty(t.workflow, stringAttr(attrs, attrWorkflowName))
	if len(span.GetParentSpanId()) == 0 {
		t.rootName = firstNonEmpty(t.rootName, span.GetName())
	}
}

func (t *traceContext) traceInput() (ingest.TraceInput, error) {
	sessionID := firstNonEmpty(t.native.sessionID, t.standard.sessionID)
	userID := firstNonEmpty(t.native.userID, t.standard.userID)
	userName := firstNonEmpty(t.native.userName, t.standard.userName)

	// These are the only attacker-controlled values materialized onto a session row.
	// sessions.user_id and sessions.external_id are both btree-indexed, and a Postgres
	// btree tuple caps out at 2704 bytes, so an unbounded identity failed the whole
	// batch with `index row size ... exceeds btree version 4 maximum`.
	for _, bound := range []struct {
		attribute string
		value     string
		limit     int
	}{
		{attrSessionID, sessionID, maxIdentityBytes},
		{attrUserID, userID, maxIdentityBytes},
		{attrContinuaSessionName, t.native.sessionName, maxDisplayNameBytes},
		{attrUserName, userName, maxDisplayNameBytes},
	} {
		if len(bound.value) > bound.limit {
			return ingest.TraceInput{}, fmt.Errorf("%w: %s is %d bytes, the limit is %d",
				ErrInvalidExport, bound.attribute, len(bound.value), bound.limit)
		}
	}

	return ingest.TraceInput{
		TraceID:   t.traceID,
		Name:      nonEmptyPtr(firstNonEmpty(t.workflow, t.rootName)),
		SessionID: nonEmptyPtr(sessionID),
		UserID:    nonEmptyPtr(userID),
		SessionContext: ingest.SessionContextInput{
			Name:     nonEmptyPtr(t.native.sessionName),
			UserID:   nonEmptyPtr(userID),
			UserName: nonEmptyPtr(userName),
		},
	}, nil
}

func normalizeSpan(
	span *tracepb.Span,
	traceIDHex string,
	attrs map[string]any,
	resource, scope map[string]any,
) (ingest.SpanInput, error) {
	if err := checkStorableText("span name", span.GetName()); err != nil {
		return ingest.SpanInput{}, err
	}
	if err := checkStorableText("span status message", span.GetStatus().GetMessage()); err != nil {
		return ingest.SpanInput{}, err
	}

	spanIDHex, err := encodeID(span.GetSpanId(), 8, "span id")
	if err != nil {
		return ingest.SpanInput{}, err
	}

	parentSpanID, err := parentSpanIDHex(span.GetParentSpanId())
	if err != nil {
		return ingest.SpanInput{}, err
	}

	input := ingest.SpanInput{
		TraceID:      traceIDHex,
		SpanID:       spanIDHex,
		ParentSpanID: parentSpanID,
		Name:         span.GetName(),
		Type:         spanType(attrs),
		Status:       spanStatus(span),
		StartTime:    spanTimestamp(span.GetStartTimeUnixNano()),
		Metadata: map[string]any{
			metaOTelKey: map[string]any{
				metaAttributesKey: attrs,
				metaResourceKey:   resource,
				metaScopeKey:      scope,
			},
		},
	}

	if endTime := spanTimestamp(span.GetEndTimeUnixNano()); !endTime.IsZero() {
		input.EndTime = &endTime
	}
	input.StatusMessage = nonEmptyPtr(span.GetStatus().GetMessage())

	return input, nil
}

func spanType(attrs map[string]any) *string {
	kind := strings.ToUpper(strings.TrimSpace(stringAttr(attrs, attrSpanKind)))
	spanType, ok := spanTypeByOpenInferenceKind[kind]
	if !ok {
		return nil
	}
	return &spanType
}

func spanStatus(span *tracepb.Span) *string {
	if span.GetStatus().GetCode() == tracepb.Status_STATUS_CODE_ERROR {
		status := statusFailed
		return &status
	}
	if span.GetEndTimeUnixNano() == 0 {
		return nil
	}
	status := statusCompleted
	return &status
}

// spanTimestamp converts an OTLP nanosecond timestamp. An absent (0) or out-of-range
// value yields the zero time, which the ingest validator rejects for start_time and the
// adapter reads as "still running" for end_time.
func spanTimestamp(unixNano uint64) time.Time {
	if unixNano == 0 || unixNano > math.MaxInt64 {
		return time.Time{}
	}
	return time.Unix(0, int64(unixNano)).UTC()
}

func encodeID(id []byte, size int, label string) (string, error) {
	if len(id) != size {
		return "", fmt.Errorf("%w: %s must be %d bytes, got %d", ErrInvalidExport, label, size, len(id))
	}
	return hex.EncodeToString(id), nil
}

// parentSpanIDHex resolves the parent link. Root spans carry either an empty parent
// (per the OTLP spec) or an all-zero span id (emitted by some collector/SDK bridges);
// both mean "no parent" and must not be persisted as a dangling parent reference.
func parentSpanIDHex(id []byte) (*string, error) {
	if len(id) == 0 || isZeroID(id) {
		return nil, nil
	}
	parentSpanID, err := encodeID(id, 8, "parent span id")
	if err != nil {
		return nil, err
	}
	return &parentSpanID, nil
}

func isZeroID(id []byte) bool {
	for _, b := range id {
		if b != 0 {
			return false
		}
	}
	return true
}

// boundedResourceAttributes caps the resource attribute block before it fans out.
// Unlike span attributes, which are written once, the resource block is copied onto
// every span of its ResourceSpans, so its size is multiplied by the span count: a 4MB
// resource attribute in a single 5MB export used to amplify into tens of gigabytes of
// jsonb written inside one transaction.
func boundedResourceAttributes(attrs map[string]any) map[string]any {
	// Values come from attributeValue, which is JSON-safe by construction.
	raw, _ := json.Marshal(attrs)
	if len(raw) <= MaxResourceAttributesBytes {
		return attrs
	}

	bounded := map[string]any{}
	if err := json.Unmarshal(truncation.TruncateWithLimit(raw, MaxResourceAttributesBytes).Data, &bounded); err != nil {
		return map[string]any{metaTruncatedKey: true}
	}
	return bounded
}

func attributeMap(attributes []*commonpb.KeyValue) (map[string]any, error) {
	out := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		if err := checkStorableText("attribute key", attribute.GetKey()); err != nil {
			return nil, err
		}

		value, err := attributeValue(attribute.GetValue())
		if err != nil {
			return nil, err
		}
		out[attribute.GetKey()] = value
	}
	return out, nil
}

// attributeValue converts an OTLP AnyValue into a JSON-serializable value, retaining
// unrecognized attributes verbatim instead of assigning them semantics.
func attributeValue(value *commonpb.AnyValue) (any, error) {
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		if err := checkStorableText("attribute value", typed.StringValue); err != nil {
			return nil, err
		}
		return typed.StringValue, nil
	case *commonpb.AnyValue_BoolValue:
		return typed.BoolValue, nil
	case *commonpb.AnyValue_IntValue:
		return typed.IntValue, nil
	case *commonpb.AnyValue_DoubleValue:
		// json.Marshal rejects NaN and ±Inf, and metadata is marshaled as one document,
		// so a single non-finite double would fail the whole span's metadata. OTLP
		// doubles are attacker-controlled, so they are retained in string form instead.
		if math.IsNaN(typed.DoubleValue) || math.IsInf(typed.DoubleValue, 0) {
			return strconv.FormatFloat(typed.DoubleValue, 'g', -1, 64), nil
		}
		return typed.DoubleValue, nil
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(typed.BytesValue), nil
	case *commonpb.AnyValue_ArrayValue:
		values := typed.ArrayValue.GetValues()
		out := make([]any, len(values))
		for i, element := range values {
			converted, err := attributeValue(element)
			if err != nil {
				return nil, err
			}
			out[i] = converted
		}
		return out, nil
	case *commonpb.AnyValue_KvlistValue:
		return attributeMap(typed.KvlistValue.GetValues())
	default:
		// A KeyValue with an unset or unknown value type; keep the key, drop the value.
		return nil, nil
	}
}

func stringAttr(attrs map[string]any, key string) string {
	value, _ := attrs[key].(string)
	return value
}

func firstNonEmpty(current, candidate string) string {
	if current != "" {
		return current
	}
	return candidate
}

func nonEmptyPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
