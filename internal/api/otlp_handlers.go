package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"

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
		write413Error(w, "otlp export exceeds 5MB limit")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if isMaxBytesError(err) {
			write413Error(w, "otlp export exceeds 5MB limit")
			return
		}
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

// otlpBatchKey derives the ingest idempotency key from the export bytes, so an OTLP
// exporter retrying the same payload is deduplicated instead of reprocessed.
func otlpBatchKey(body []byte) string {
	sum := sha256.Sum256(body)
	return "otlp-" + hex.EncodeToString(sum[:])
}
