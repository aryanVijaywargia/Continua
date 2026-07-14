package runtime_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
)

func TestRuntimeServesHealthEndpointsOnListener(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = lst.Close() }()

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             db.DatabaseURL,
		ProjectID:               &projectID,
		HTTPListener:            lst,
		WorkflowPollInterval:    25 * time.Millisecond,
		ActivityPollInterval:    25 * time.Millisecond,
		MaintenancePollInterval: 50 * time.Millisecond,
		MetricsSampleInterval:   50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

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
	baseURL := "http://" + lst.Addr().String()
	waitForRuntimeHTTPStatus(t, client, baseURL+"/healthz", http.StatusOK, 10*time.Second)
	waitForRuntimeHTTPStatus(t, client, baseURL+"/readyz", http.StatusOK, 15*time.Second)

	response, err := client.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics error = %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatalf("read GET /metrics response: %v", readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d; body = %q", response.StatusCode, http.StatusOK, body)
	}
	if !strings.Contains(string(body), "continua_engine_worker_iteration_duration_seconds") {
		t.Fatalf("GET /metrics body does not contain worker iteration duration metric; body = %q", body)
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
}

func waitForRuntimeHTTPStatus(t *testing.T, client *http.Client, endpoint string, wantStatus int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastStatus int
	var lastBody string
	var lastErr error
	for time.Now().Before(deadline) {
		response, err := client.Get(endpoint)
		lastErr = err
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else {
				lastStatus = response.StatusCode
				lastBody = string(body)
				if response.StatusCode == wantStatus {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("GET %s did not return %d within %s; last status=%d last error=%v last body=%q", endpoint, wantStatus, timeout, lastStatus, lastErr, lastBody)
}
