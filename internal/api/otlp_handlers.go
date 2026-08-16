package api

import "net/http"

// OTLPTracesPath is the standard OTLP/HTTP trace ingestion path
// (https://opentelemetry.io/docs/specs/otlp/#otlphttp-request).
const OTLPTracesPath = "/v1/traces"

// OTLPTracesContentType is the protobuf content type OTLP/HTTP exporters send.
const OTLPTracesContentType = "application/x-protobuf"

// OTLPTraces ingests an OTLP/HTTP protobuf ExportTraceServiceRequest and normalizes it
// into Continua's canonical session -> trace -> span -> span_event model.
//
// Scaffolding only: the normalization, span-kind mapping, and session materialization
// implementation land in a follow-up commit. The handler deliberately reports 501 so the
// acceptance tests in otlp_handlers_test.go fail RED until it is implemented.
func (s *Server) OTLPTraces(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "otlp trace ingestion is not implemented", http.StatusNotImplemented)
}
