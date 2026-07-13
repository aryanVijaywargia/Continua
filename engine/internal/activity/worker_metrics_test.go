package activity

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginemetrics "github.com/continua-ai/continua/engine/internal/metrics"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestWorkerRecordsRetryAndFailureAttempts(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	promRegistry := prometheus.NewRegistry()
	store := enginestore.New(db.Pool).WithMetrics(enginemetrics.New(promRegistry))
	ctx := context.Background()

	initial := int64(1)
	maxBackoff := int64(1)
	multiplier := 1.0
	_, _, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-activity-attempt-metrics",
		activityKey:       "always-errors",
		maxAttempts:       2,
		initialBackoffMS:  &initial,
		maxBackoffMS:      &maxBackoff,
		backoffMultiplier: &multiplier,
	})

	activityRegistry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("boom")
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, activityRegistry, time.Minute)
	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() first attempt error = %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() second attempt error = %v", err)
	}

	failedTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if failedTask.Status != enginedb.EngineActivityTaskStatusFailed || failedTask.AttemptCount != 2 {
		t.Fatalf("activity task = %+v, want terminal failure after two attempts", failedTask)
	}

	for _, tc := range []struct {
		result string
		want   float64
	}{
		{result: "retried", want: 1},
		{result: "failed", want: 1},
	} {
		t.Run(tc.result, func(t *testing.T) {
			collector, found, err := gatheredCounterCollector(
				promRegistry,
				"continua_engine_activity_attempts_total",
				map[string]string{"result": tc.result},
			)
			if err != nil {
				t.Fatalf("gather activity attempts metric: %v", err)
			}
			if !found {
				t.Fatalf("continua_engine_activity_attempts_total{result=%q} series is missing", tc.result)
			}
			if got := promtest.ToFloat64(collector); got != tc.want {
				t.Fatalf("continua_engine_activity_attempts_total{result=%q} = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}

type singleGatheredMetric struct {
	prometheus.Metric
}

func (m singleGatheredMetric) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.Desc()
}

func (m singleGatheredMetric) Collect(ch chan<- prometheus.Metric) {
	ch <- m.Metric
}

func gatheredCounterCollector(
	gatherer prometheus.Gatherer,
	metricName string,
	wantLabels map[string]string,
) (prometheus.Collector, bool, error) {
	families, err := gatherer.Gather()
	if err != nil {
		return nil, false, err
	}
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			if !hasExactLabels(metric.GetLabel(), wantLabels) {
				continue
			}

			labelNames := make([]string, 0, len(wantLabels))
			for name := range wantLabels {
				labelNames = append(labelNames, name)
			}
			sort.Strings(labelNames)
			labelValues := make([]string, 0, len(labelNames))
			for _, name := range labelNames {
				labelValues = append(labelValues, wantLabels[name])
			}
			metricValue, err := prometheus.NewConstMetric(
				prometheus.NewDesc(metricName, "gathered counter value", labelNames, nil),
				prometheus.CounterValue,
				metric.GetCounter().GetValue(),
				labelValues...,
			)
			if err != nil {
				return nil, false, err
			}
			return singleGatheredMetric{Metric: metricValue}, true, nil
		}
	}
	return nil, false, nil
}

func hasExactLabels(labelPairs []*dto.LabelPair, want map[string]string) bool {
	if len(labelPairs) != len(want) {
		return false
	}
	for _, pair := range labelPairs {
		if want[pair.GetName()] != pair.GetValue() {
			return false
		}
	}
	return true
}
