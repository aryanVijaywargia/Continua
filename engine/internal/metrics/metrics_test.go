package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsNilReceiverSafe(t *testing.T) {
	var recorder *Metrics

	recorder.ObserveWorkerIteration("workflow", time.Millisecond)
	recorder.IncWorkerIterationError("workflow")
	recorder.IncClaim("run", "claimed")
	recorder.IncActivityAttempt("completed")
	recorder.AddProjectorRowsProjected(1)
	recorder.SetProjectorLagRows(0)
	recorder.SetQueueDepth("runs_ready", 0)
	recorder.IncDedupeClaim("new")
}

func TestHandlerServesRecordedSeries(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := New(registry)

	for _, worker := range []string{"workflow", "activity", "maintenance", "projector", "catalog-heartbeat"} {
		recorder.ObserveWorkerIteration(worker, 10*time.Millisecond)
		recorder.IncWorkerIterationError(worker)
	}
	for _, resource := range []string{"run", "activity_task"} {
		for _, outcome := range []string{"claimed", "empty", "stale"} {
			recorder.IncClaim(resource, outcome)
		}
	}
	for _, result := range []string{"completed", "retried", "failed"} {
		recorder.IncActivityAttempt(result)
	}
	recorder.AddProjectorRowsProjected(2)
	recorder.SetProjectorLagRows(3)
	for _, queue := range []string{"runs_ready", "activity_tasks_pending", "inbox_pending"} {
		recorder.SetQueueDepth(queue, 4)
	}
	for _, outcome := range []string{"new", "existing_finalized", "existing_in_progress"} {
		recorder.IncDedupeClaim(outcome)
	}

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	Handler(registry).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	wantSeries := []string{
		`continua_engine_worker_iteration_duration_seconds_count{worker="workflow"}`,
		`continua_engine_worker_iteration_duration_seconds_count{worker="activity"}`,
		`continua_engine_worker_iteration_duration_seconds_count{worker="maintenance"}`,
		`continua_engine_worker_iteration_duration_seconds_count{worker="projector"}`,
		`continua_engine_worker_iteration_duration_seconds_count{worker="catalog-heartbeat"}`,
		`continua_engine_worker_iteration_errors_total{worker="workflow"}`,
		`continua_engine_worker_iteration_errors_total{worker="activity"}`,
		`continua_engine_worker_iteration_errors_total{worker="maintenance"}`,
		`continua_engine_worker_iteration_errors_total{worker="projector"}`,
		`continua_engine_worker_iteration_errors_total{worker="catalog-heartbeat"}`,
		`continua_engine_claims_total{outcome="claimed",resource="run"}`,
		`continua_engine_claims_total{outcome="claimed",resource="activity_task"}`,
		`continua_engine_claims_total{outcome="empty",resource="run"}`,
		`continua_engine_claims_total{outcome="empty",resource="activity_task"}`,
		`continua_engine_claims_total{outcome="stale",resource="run"}`,
		`continua_engine_claims_total{outcome="stale",resource="activity_task"}`,
		`continua_engine_activity_attempts_total{result="completed"}`,
		`continua_engine_activity_attempts_total{result="retried"}`,
		`continua_engine_activity_attempts_total{result="failed"}`,
		`continua_engine_projector_rows_projected_total 2`,
		`continua_engine_projector_lag_rows 3`,
		`continua_engine_queue_depth{queue="runs_ready"}`,
		`continua_engine_queue_depth{queue="activity_tasks_pending"}`,
		`continua_engine_queue_depth{queue="inbox_pending"}`,
		`continua_engine_dedupe_claims_total{outcome="new"}`,
		`continua_engine_dedupe_claims_total{outcome="existing_finalized"}`,
		`continua_engine_dedupe_claims_total{outcome="existing_in_progress"}`,
	}
	for _, want := range wantSeries {
		if !strings.Contains(body, want) {
			t.Errorf("GET /metrics body missing %q\nbody:\n%s", want, body)
		}
	}
}
