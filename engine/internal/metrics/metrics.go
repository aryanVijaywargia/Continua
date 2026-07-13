// Package metrics defines the engine's Prometheus instrumentation surface.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is the engine metrics recorder.
type Metrics struct {
	workerIterationDuration *prometheus.HistogramVec
	workerIterationErrors   *prometheus.CounterVec
	claims                  *prometheus.CounterVec
	activityAttempts        *prometheus.CounterVec
	projectorRowsProjected  prometheus.Counter
	projectorLagRows        prometheus.Gauge
	queueDepth              *prometheus.GaugeVec
	dedupeClaims            *prometheus.CounterVec
}

// New registers and returns the engine metrics recorder.
func New(registerer prometheus.Registerer) *Metrics {
	if registerer == nil {
		return &Metrics{}
	}

	metrics := &Metrics{
		workerIterationDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "continua",
			Subsystem: "engine",
			Name:      "worker_iteration_duration_seconds",
			Help:      "Duration of engine worker polling iterations in seconds.",
			Buckets: []float64{
				0.0001, 0.00025, 0.0005,
				0.001, 0.0025, 0.005, 0.01, 0.025, 0.05,
				0.1, 0.25, 0.5, 1, 2.5, 5, 10,
			},
		}, []string{"worker"}),
		workerIterationErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "continua",
			Subsystem: "engine",
			Name:      "worker_iteration_errors_total",
			Help:      "Total engine worker polling iterations that returned an error.",
		}, []string{"worker"}),
		claims: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "continua",
			Subsystem: "engine",
			Name:      "claims_total",
			Help:      "Total engine resource claim outcomes.",
		}, []string{"resource", "outcome"}),
		activityAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "continua",
			Subsystem: "engine",
			Name:      "activity_attempts_total",
			Help:      "Total activity attempt outcomes.",
		}, []string{"result"}),
		projectorRowsProjected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "continua",
			Subsystem: "engine",
			Name:      "projector_rows_projected_total",
			Help:      "Total engine history rows successfully projected.",
		}),
		projectorLagRows: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "continua",
			Subsystem: "engine",
			Name:      "projector_lag_rows",
			Help:      "Current engine history rows awaiting projection.",
		}),
		queueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "continua",
			Subsystem: "engine",
			Name:      "queue_depth",
			Help:      "Current claimable engine work by queue.",
		}, []string{"queue"}),
		dedupeClaims: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "continua",
			Subsystem: "engine",
			Name:      "dedupe_claims_total",
			Help:      "Total start-request deduplication claim outcomes.",
		}, []string{"outcome"}),
	}
	registerer.MustRegister(
		metrics.workerIterationDuration,
		metrics.workerIterationErrors,
		metrics.claims,
		metrics.activityAttempts,
		metrics.projectorRowsProjected,
		metrics.projectorLagRows,
		metrics.queueDepth,
		metrics.dedupeClaims,
	)
	return metrics
}

// ObserveWorkerIteration records worker iteration latency.
func (m *Metrics) ObserveWorkerIteration(worker string, duration time.Duration) {
	if m == nil || m.workerIterationDuration == nil {
		return
	}
	m.workerIterationDuration.WithLabelValues(worker).Observe(duration.Seconds())
}

// IncWorkerIterationError records a worker iteration error.
func (m *Metrics) IncWorkerIterationError(worker string) {
	if m == nil || m.workerIterationErrors == nil {
		return
	}
	m.workerIterationErrors.WithLabelValues(worker).Inc()
}

// IncClaim records a resource claim outcome.
func (m *Metrics) IncClaim(resource, outcome string) {
	if m == nil || m.claims == nil {
		return
	}
	m.claims.WithLabelValues(resource, outcome).Inc()
}

// IncActivityAttempt records an activity attempt outcome.
func (m *Metrics) IncActivityAttempt(result string) {
	if m == nil || m.activityAttempts == nil {
		return
	}
	m.activityAttempts.WithLabelValues(result).Inc()
}

// AddProjectorRowsProjected records successfully projected history rows.
func (m *Metrics) AddProjectorRowsProjected(rows int) {
	if m == nil || m.projectorRowsProjected == nil {
		return
	}
	m.projectorRowsProjected.Add(float64(rows))
}

// SetProjectorLagRows records the current projector lag.
func (m *Metrics) SetProjectorLagRows(rows int64) {
	if m == nil || m.projectorLagRows == nil {
		return
	}
	m.projectorLagRows.Set(float64(rows))
}

// SetQueueDepth records the current depth of an engine work queue.
func (m *Metrics) SetQueueDepth(queue string, depth int64) {
	if m == nil || m.queueDepth == nil {
		return
	}
	m.queueDepth.WithLabelValues(queue).Set(float64(depth))
}

// IncDedupeClaim records a start-request deduplication claim outcome.
func (m *Metrics) IncDedupeClaim(outcome string) {
	if m == nil || m.dedupeClaims == nil {
		return
	}
	m.dedupeClaims.WithLabelValues(outcome).Inc()
}

// Handler exposes metrics from gatherer using the Prometheus text format.
func Handler(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}
