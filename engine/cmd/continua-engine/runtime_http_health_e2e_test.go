package main

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestServeExposesHealthEndpoints(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)

	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := lst.Addr().String()
	if err := lst.Close(); err != nil {
		t.Fatalf("listener.Close() error = %v", err)
	}
	t.Setenv("ENGINE_HTTP_ADDR", addr)

	serve := startServe(t)
	defer serve.stop(t)

	client := &http.Client{
		Timeout: 250 * time.Millisecond,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	baseURL := "http://" + addr
	waitForServeHTTPStatus(t, client, baseURL+"/healthz", http.StatusOK, 10*time.Second)
	waitForServeHTTPStatus(t, client, baseURL+"/readyz", http.StatusOK, 15*time.Second)

	response, err := client.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics error = %v", err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatalf("read GET /metrics response: %v", readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func waitForServeHTTPStatus(t *testing.T, client *http.Client, endpoint string, wantStatus int, timeout time.Duration) {
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
