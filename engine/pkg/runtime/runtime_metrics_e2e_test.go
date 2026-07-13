package runtime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestRuntimeRecordsMetricsForDemoRun(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	registry := prometheus.NewRegistry()

	rt := newMetricsTestRuntime(t, db.DatabaseURL, projectID, registry, nil)
	store := enginestore.New(db.Pool)
	instanceID := seedMetricsGreeterRun(t, store, projectID, "runtime-metrics-e2e")

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	stopped := false
	go func() {
		done <- rt.Run(runCtx)
	}()
	defer func() {
		if stopped {
			return
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runtime did not stop within 5s after test cleanup cancellation")
		}
	}()

	waitForCompletedRun(t, store, instanceID)
	waitForRuntimeMetrics(t, registry, done)

	cancel()
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatalf("Runtime.Run() after cancellation error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime.Run() did not stop within 5s after cancellation")
	}
}

func TestRuntimeServesMetricsEndpointOnListener(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	registry := prometheus.NewRegistry()
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = lst.Close() }()

	rt := newMetricsTestRuntime(t, db.DatabaseURL, projectID, registry, lst)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	stopped := false
	go func() {
		done <- rt.Run(runCtx)
	}()
	defer func() {
		if stopped {
			return
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runtime did not stop within 5s after test cleanup cancellation")
		}
	}()

	client := &http.Client{
		Timeout: 250 * time.Millisecond,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	endpoint := "http://" + lst.Addr().String() + "/metrics"
	deadline := time.Now().Add(10 * time.Second)
	var lastStatus int
	var lastBody string
	var lastErr error
	served := false
	for time.Now().Before(deadline) {
		select {
		case runErr := <-done:
			stopped = true
			t.Fatalf("Runtime.Run() returned before cancellation: %v", runErr)
		default:
		}

		response, requestErr := client.Get(endpoint)
		lastErr = requestErr
		if requestErr == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else {
				lastStatus = response.StatusCode
				lastBody = string(body)
				if response.StatusCode == http.StatusOK && strings.Contains(lastBody, "continua_engine_worker_iteration_duration_seconds") {
					served = true
					break
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !served {
		t.Fatalf("GET %s did not serve worker metrics within 10s; last status=%d last error=%v last body=%q", endpoint, lastStatus, lastErr, lastBody)
	}

	cancel()
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatalf("Runtime.Run() after cancellation error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime.Run() did not stop within 5s after cancellation")
	}

	response, err := client.Get(endpoint)
	if err == nil {
		_ = response.Body.Close()
		t.Fatalf("GET %s after runtime shutdown unexpectedly succeeded with status %d", endpoint, response.StatusCode)
	}
}

func newMetricsTestRuntime(
	t *testing.T,
	databaseURL string,
	projectID uuid.UUID,
	registry *prometheus.Registry,
	listener net.Listener,
) *engineruntime.Runtime {
	t.Helper()

	workflowDefinition := workflow.Definition{
		Name:    "usertest.metrics-greeter",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			var in struct {
				Name string `json:"name"`
			}
			if err := ctx.Input(&in); err != nil {
				return err
			}

			var out struct {
				Greeting string `json:"greeting"`
			}
			if err := ctx.Activity("greet", "usertest.metrics-greet", in, &out); err != nil {
				return err
			}
			return ctx.SetResult(out)
		},
	}
	activities := map[string]engineruntime.ActivityHandler{
		"usertest.metrics-greet": func(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
			var in struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			return json.Marshal(struct {
				Greeting string `json:"greeting"`
			}{Greeting: "hello, " + in.Name})
		},
	}

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             databaseURL,
		Workflows:               []workflow.Definition{workflowDefinition},
		Activities:              activities,
		ProjectID:               &projectID,
		WorkflowPollInterval:    25 * time.Millisecond,
		ActivityPollInterval:    25 * time.Millisecond,
		MaintenancePollInterval: 100 * time.Millisecond,
		MetricsRegistry:         registry,
		MetricsListener:         listener,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}
	return rt
}

func seedMetricsGreeterRun(t *testing.T, store *enginestore.Store, projectID uuid.UUID, instanceKey string) uuid.UUID {
	t.Helper()

	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    instanceKey,
		DefinitionName: "usertest.metrics-greeter",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	input := mustJSON(t, map[string]string{"name": "Ada"})
	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    "usertest.metrics-greeter",
		DefinitionVersion: "v1",
		InstanceKey:       instanceKey,
		Input:             input,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(workflow started) error = %v", err)
	}
	startedEvent, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    startedPayload,
	})
	if err != nil {
		t.Fatalf("AppendHistory(workflow started) error = %v", err)
	}

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	traceName := "usertest.metrics-greeter"
	if err := publicprojection.NewWriter(tx.Tx()).CreateTraceShell(ctx, &instance, &run, &publicprojection.TraceShellSeed{}, &startedEvent, input, &traceName); err != nil {
		t.Fatalf("CreateTraceShell() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	return instance.ID
}

type runtimeMetricExpectation struct {
	name       string
	labels     map[string]string
	kind       string
	minimum    float64
	existsOnly bool
}

func waitForRuntimeMetrics(t *testing.T, registry *prometheus.Registry, runtimeDone <-chan error) {
	t.Helper()

	expectations := []runtimeMetricExpectation{
		{name: "continua_engine_claims_total", labels: map[string]string{"resource": "run", "outcome": "claimed"}, kind: "counter", minimum: 1},
		{name: "continua_engine_claims_total", labels: map[string]string{"resource": "activity_task", "outcome": "claimed"}, kind: "counter", minimum: 1},
		{name: "continua_engine_claims_total", labels: map[string]string{"resource": "run", "outcome": "empty"}, kind: "counter", minimum: 1},
		{name: "continua_engine_activity_attempts_total", labels: map[string]string{"result": "completed"}, kind: "counter", minimum: 1},
		{name: "continua_engine_projector_rows_projected_total", kind: "counter", minimum: 1},
		{name: "continua_engine_worker_iteration_duration_seconds", labels: map[string]string{"worker": "workflow"}, kind: "histogram", minimum: 1},
		{name: "continua_engine_worker_iteration_duration_seconds", labels: map[string]string{"worker": "activity"}, kind: "histogram", minimum: 1},
		{name: "continua_engine_worker_iteration_duration_seconds", labels: map[string]string{"worker": "projector"}, kind: "histogram", minimum: 1},
		{name: "continua_engine_queue_depth", labels: map[string]string{"queue": "runs_ready"}, kind: "gauge", existsOnly: true},
		{name: "continua_engine_queue_depth", labels: map[string]string{"queue": "activity_tasks_pending"}, kind: "gauge", existsOnly: true},
		{name: "continua_engine_queue_depth", labels: map[string]string{"queue": "inbox_pending"}, kind: "gauge", existsOnly: true},
		{name: "continua_engine_projector_lag_rows", kind: "gauge", minimum: 0},
	}

	deadline := time.Now().Add(10 * time.Second)
	var lastFamilies []*dto.MetricFamily
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case runErr := <-runtimeDone:
			t.Fatalf("Runtime.Run() returned before metrics were observed: %v", runErr)
		default:
		}

		lastFamilies, lastErr = registry.Gather()
		if lastErr == nil && allRuntimeMetricExpectationsMet(lastFamilies, expectations) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("runtime metrics were not all observed within 10s; last gather error=%v\nlast gathered families:\n%s", lastErr, fmt.Sprint(lastFamilies))
}

func allRuntimeMetricExpectationsMet(families []*dto.MetricFamily, expectations []runtimeMetricExpectation) bool {
	for _, expectation := range expectations {
		if !runtimeMetricExpectationMet(families, expectation) {
			return false
		}
	}
	return true
}

func runtimeMetricExpectationMet(families []*dto.MetricFamily, expectation runtimeMetricExpectation) bool {
	for _, family := range families {
		if family.GetName() != expectation.name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if !runtimeMetricHasLabels(metric, expectation.labels) {
				continue
			}
			if expectation.existsOnly {
				return true
			}
			switch expectation.kind {
			case "counter":
				return metric.GetCounter().GetValue() >= expectation.minimum
			case "gauge":
				return metric.GetGauge().GetValue() >= expectation.minimum
			case "histogram":
				return float64(metric.GetHistogram().GetSampleCount()) >= expectation.minimum
			}
		}
	}
	return false
}

func runtimeMetricHasLabels(metric *dto.Metric, want map[string]string) bool {
	if len(metric.GetLabel()) != len(want) {
		return false
	}
	for _, pair := range metric.GetLabel() {
		if want[pair.GetName()] != pair.GetValue() {
			return false
		}
	}
	return true
}
