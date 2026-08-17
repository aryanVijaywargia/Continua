package api

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/otlp"
)

// OTLPTracesPath is the standard OTLP/HTTP trace ingestion path
// (https://opentelemetry.io/docs/specs/otlp/#otlphttp-request).
const OTLPTracesPath = "/v1/traces"

// OTLPTracesContentType is the protobuf content type OTLP/HTTP exporters send.
const OTLPTracesContentType = "application/x-protobuf"

// OTLPTracesJSONContentType is the spec's second encoding, OTLP/JSON.
const OTLPTracesJSONContentType = "application/json"

// google.rpc.Code values carried by an OTLP error Status. Spelled out rather than
// pulled from google.golang.org/grpc/codes, which this module does not otherwise use.
const (
	otlpCodeInvalidArgument   = 3
	otlpCodeResourceExhausted = 8
	otlpCodeInternal          = 13
)

// Body-decoding failures the handler maps onto a status code.
var (
	errOTLPBodyTooLarge    = errors.New("otlp export exceeds 5MB limit")
	errOTLPUnknownEncoding = errors.New("unsupported Content-Encoding")
)

// otlpRouteAvailabilityMiddleware gates the preview OTLP ingestion surface the same way
// engineRouteAvailabilityMiddleware gates /v1/engine: the route 404s while the flag is
// off. Gating cannot rely on leaving the route unmounted, because the SPA fallback would
// otherwise answer it with 200.
func otlpRouteAvailabilityMiddleware(server *Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == OTLPTracesPath && !server.otlpIngestEnabled {
				http.NotFound(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OTLPTraces ingests an OTLP/HTTP protobuf ExportTraceServiceRequest and normalizes it
// into Continua's canonical session -> trace -> span -> span_event model. The export is
// converted to a native ingest batch and written through the shared ingest path, so OTLP
// producers and native clients share one validation and persistence surface.
func (s *Server) OTLPTraces(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}

	if r.ContentLength > MaxBodySize {
		writeOTLPError(w, http.StatusRequestEntityTooLarge, otlpCodeResourceExhausted, errOTLPBodyTooLarge.Error())
		return
	}

	body, err := readOTLPExportBody(w, r)
	switch {
	case errors.Is(err, errOTLPBodyTooLarge):
		writeOTLPError(w, http.StatusRequestEntityTooLarge, otlpCodeResourceExhausted, errOTLPBodyTooLarge.Error())
		return
	case errors.Is(err, errOTLPUnknownEncoding):
		writeOTLPError(w, http.StatusUnsupportedMediaType, otlpCodeInvalidArgument, err.Error())
		return
	case err != nil:
		writeOTLPError(w, http.StatusBadRequest, otlpCodeInvalidArgument, "Failed to read request body: "+err.Error())
		return
	}

	var export coltracepb.ExportTraceServiceRequest
	switch mediaType := otlpMediaType(r); mediaType {
	case "", OTLPTracesContentType:
		err = proto.Unmarshal(body, &export)
	case OTLPTracesJSONContentType:
		// OTLP/JSON is spec-defined and a documented Collector option (`encoding: json`).
		// Unknown fields are discarded so a newer exporter is not rejected wholesale.
		err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(body, &export)
	default:
		writeOTLPError(w, http.StatusUnsupportedMediaType, otlpCodeInvalidArgument,
			"Unsupported Content-Type "+mediaType+"; expected "+OTLPTracesContentType+" or "+OTLPTracesJSONContentType)
		return
	}
	if err != nil {
		writeOTLPError(w, http.StatusBadRequest, otlpCodeInvalidArgument, "Failed to decode OTLP export: "+err.Error())
		return
	}

	req, err := otlp.Normalize(&export, otlpBatchKey(body))
	if err != nil {
		writeOTLPError(w, http.StatusBadRequest, otlpCodeInvalidArgument, err.Error())
		return
	}

	if _, err := s.ingestService.Ingest(r.Context(), projectID, req); err != nil {
		if ingest.IsValidationError(err) {
			writeOTLPError(w, http.StatusBadRequest, otlpCodeInvalidArgument, err.Error())
			return
		}
		log.Printf("otlp trace ingest failed: %v", err)
		writeOTLPError(w, http.StatusInternalServerError, otlpCodeInternal, "An internal error occurred")
		return
	}

	response, _ := proto.Marshal(&coltracepb.ExportTraceServiceResponse{})
	w.Header().Set("Content-Type", OTLPTracesContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}

// readOTLPExportBody reads the export bytes, decompressing the request body when the
// exporter compressed it. The OTLP spec requires servers to support `none` and `gzip`,
// and the Collector's otlphttp exporter gzips by default, so an undecompressed body is
// total span loss: the resulting 400 is non-retryable per the spec.
//
// Decompression is a gzip-bomb vector, so the raw body and the decompressed stream are
// bounded separately: MaxBytesReader only ever sees the compressed bytes, which a bomb
// keeps tiny.
func readOTLPExportBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

	reader := io.Reader(r.Body)
	switch encoding := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); encoding {
	case "", "identity":
	case "gzip":
		decompressed, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		defer func() { _ = decompressed.Close() }()

		// Read one byte past the limit so an over-long stream is detectable without
		// expanding the rest of it.
		reader = io.LimitReader(decompressed, MaxBodySize+1)
	default:
		return nil, fmt.Errorf("%w: %q", errOTLPUnknownEncoding, encoding)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		if isMaxBytesError(err) {
			return nil, errOTLPBodyTooLarge
		}
		return nil, err
	}
	if len(body) > MaxBodySize {
		return nil, errOTLPBodyTooLarge
	}

	return body, nil
}

// otlpMediaType returns the request's Content-Type without its parameters. An absent
// header is reported as empty and read as protobuf, which is what every OTLP/HTTP
// exporter sends by default.
func otlpMediaType(r *http.Request) string {
	header := r.Header.Get("Content-Type")
	if header == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return header
	}
	return mediaType
}

// writeOTLPError writes a protobuf-encoded google.rpc.Status, which the OTLP spec
// requires of every 4xx/5xx on this endpoint. The Collector parses the body as a Status
// and discards it when it cannot, so a JSON error body reaches the operator as a bare
// "permanent error: 400" with the explanation thrown away. Continua's JSON error shape
// stays in place everywhere else.
func writeOTLPError(w http.ResponseWriter, httpStatus int, code int32, message string) {
	body, _ := proto.Marshal(&spb.Status{Code: code, Message: message})

	w.Header().Set("Content-Type", OTLPTracesContentType)
	w.WriteHeader(httpStatus)
	_, _ = w.Write(body)
}

// otlpBatchKey derives the ingest idempotency key from the export bytes, so an OTLP
// exporter retrying the same payload is deduplicated instead of reprocessed.
func otlpBatchKey(body []byte) string {
	sum := sha256.Sum256(body)
	return "otlp-" + hex.EncodeToString(sum[:])
}
