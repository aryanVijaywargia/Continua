package health_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/continua-ai/continua/engine/internal/health"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestTrackerNotReadyBeforeFirstIteration(t *testing.T) {
	tracker := health.NewTracker()
	tracker.Register("workflow", time.Minute)

	ready, failing := tracker.Check(time.Now())
	if ready {
		t.Fatal("Check() ready = true, want false before the first iteration")
	}
	if !slices.Contains(failing, "workflow") {
		t.Fatalf("Check() failing = %v, want it to contain %q", failing, "workflow")
	}
}

func TestTrackerReadyAfterRecentIteration(t *testing.T) {
	tracker := health.NewTracker()
	tracker.Register("workflow", time.Minute)
	tracker.MarkIteration("workflow")

	ready, failing := tracker.Check(time.Now())
	if !ready {
		t.Fatalf("Check() ready = false, want true; failing = %v", failing)
	}
	if len(failing) != 0 {
		t.Fatalf("Check() failing = %v, want empty", failing)
	}
}

func TestTrackerNotReadyWhenIterationStale(t *testing.T) {
	tracker := health.NewTracker()
	tracker.Register("workflow", 50*time.Millisecond)
	tracker.MarkIteration("workflow")

	ready, failing := tracker.Check(time.Now().Add(time.Second))
	if ready {
		t.Fatal("Check() ready = true, want false for a stale worker")
	}
	if !slices.Contains(failing, "workflow") {
		t.Fatalf("Check() failing = %v, want it to contain %q", failing, "workflow")
	}
}

func TestLivenessHandlerAlwaysOK(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	health.LivenessHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestReadinessHandlerOKWhenChecksPass(t *testing.T) {
	tracker := health.NewTracker()
	tracker.Register("workflow", time.Minute)
	tracker.MarkIteration("workflow")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	health.ReadinessHandler(stubPinger{}, tracker, time.Second).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /readyz status = %d, want %d; body = %q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /readyz body is not a JSON object: %v; body = %q", err, recorder.Body.String())
	}
	if body == nil {
		t.Fatalf("GET /readyz body = %q, want a JSON object", recorder.Body.String())
	}
}

func TestReadinessHandlerReportsStaleWorker(t *testing.T) {
	tracker := health.NewTracker()
	tracker.Register("workflow", time.Minute)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	health.ReadinessHandler(stubPinger{}, tracker, time.Second).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz status = %d, want %d; body = %q", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "workflow") {
		t.Fatalf("GET /readyz body = %q, want it to mention %q", recorder.Body.String(), "workflow")
	}
}

func TestReadinessHandlerReportsDBFailureOnClosedPool(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	pool, err := pgxpool.New(context.Background(), db.DatabaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("pool.Ping() before Close() error = %v", err)
	}
	pool.Close()

	tracker := health.NewTracker()
	tracker.Register("workflow", time.Minute)
	tracker.MarkIteration("workflow")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	health.ReadinessHandler(pool, tracker, time.Second).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz status = %d, want %d; body = %q", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "db") {
		t.Fatalf("GET /readyz body = %q, want it to mention %q", recorder.Body.String(), "db")
	}
}

type stubPinger struct {
	err error
}

func (p stubPinger) Ping(context.Context) error {
	return p.err
}

var _ health.Pinger = stubPinger{}
