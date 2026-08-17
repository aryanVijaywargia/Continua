package api

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/otlp"
)

// OTLPTracesPath is the standard OTLP/HTTP trace ingestion path
// (https://opentelemetry.io/docs/specs/otlp/#otlphttp-request).
const OTLPTracesPath = "/v1/traces"

// OTLPTracesContentType is the protobuf content type OTLP/HTTP exporters send.
const OTLPTracesContentType = "application/x-protobuf"

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
		write413Error(w, errOTLPBodyTooLarge.Error())
		return
	}

	body, err := readOTLPExportBody(w, r)
	switch {
	case errors.Is(err, errOTLPBodyTooLarge):
		write413Error(w, errOTLPBodyTooLarge.Error())
		return
	case errors.Is(err, errOTLPUnknownEncoding):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_content_encoding", err.Error())
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "invalid_otlp_export", "Failed to read request body: "+err.Error())
		return
	}

	var export coltracepb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &export); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_otlp_export", "Failed to decode OTLP protobuf export: "+err.Error())
		return
	}

	req, err := otlp.Normalize(&export, otlpBatchKey(body))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_otlp_export", err.Error())
		return
	}

	if _, err := s.ingestService.Ingest(r.Context(), projectID, req); err != nil {
		if ingest.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		log.Printf("otlp trace ingest failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "An internal error occurred")
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

// otlpBatchKey derives the ingest idempotency key from the export bytes, so an OTLP
// exporter retrying the same payload is deduplicated instead of reprocessed.
func otlpBatchKey(body []byte) string {
	sum := sha256.Sum256(body)
	return "otlp-" + hex.EncodeToString(sum[:])
}
