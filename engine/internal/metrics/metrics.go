// Package metrics defines the engine's Prometheus instrumentation surface.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is the engine metrics recorder.
//
// The acceptance-test phase intentionally leaves this recorder empty.
type Metrics struct{}

// New returns an empty metrics recorder.
func New(_ prometheus.Registerer) *Metrics {
	return &Metrics{}
}

// ObserveWorkerIteration is a no-op placeholder for worker iteration latency.
func (*Metrics) ObserveWorkerIteration(_ string, _ time.Duration) {}

// IncWorkerIterationError is a no-op placeholder for worker iteration errors.
func (*Metrics) IncWorkerIterationError(_ string) {}

// IncClaim is a no-op placeholder for resource claim outcomes.
func (*Metrics) IncClaim(_, _ string) {}

// IncActivityAttempt is a no-op placeholder for activity attempt outcomes.
func (*Metrics) IncActivityAttempt(_ string) {}

// AddProjectorRowsProjected is a no-op placeholder for projected row counts.
func (*Metrics) AddProjectorRowsProjected(_ int) {}

// SetProjectorLagRows is a no-op placeholder for projector lag sampling.
func (*Metrics) SetProjectorLagRows(_ int64) {}

// SetQueueDepth is a no-op placeholder for queue depth sampling.
func (*Metrics) SetQueueDepth(_ string, _ int64) {}

// IncDedupeClaim is a no-op placeholder for request-dedupe claim outcomes.
func (*Metrics) IncDedupeClaim(_ string) {}

// Handler exposes metrics from gatherer using the Prometheus text format.
func Handler(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}
