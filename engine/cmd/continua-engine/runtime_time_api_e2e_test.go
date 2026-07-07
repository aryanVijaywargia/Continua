package main

import (
	"encoding/json"
	"testing"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"

	"github.com/continua-ai/continua/engine/cmd/continua-engine/internal/darklaunch"
)

// A workflow that sleeps for a duration (not an absolute input-provided
// deadline) must survive a mid-sleep binary restart: the reclaimed run replays
// against the recorded deadline instead of recomputing it from wall time.
func TestEngineRuntimeSleepDurationSurvivesRestartWithoutReplayMismatch(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)

	serve := startServe(t)

	instanceKey := "runtime-restart-sleep-duration"
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.SleepDemoDefinitionName,
		"--version", darklaunch.SleepDemoDefinitionVersion,
		"--request-key", "req-sleep-duration-restart",
		"--input", `{"name":"Sleepy","sleep_ms":1500}`,
	); err != nil {
		t.Fatalf("start sleep demo workflow: %v", err)
	}

	waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "timer")
	})
	serve.stop(t)

	time.Sleep(time.Second)

	serve = startServe(t)
	defer serve.stop(t)

	waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})

	if _, _, err := executeCommand(t,
		"signal",
		"--instance-key", instanceKey,
		"--signal-name", "approval",
		"--payload", `{"approval":"restart-approved"}`,
	); err != nil {
		t.Fatalf("signal sleep demo workflow: %v", err)
	}

	state := waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusCompleted)
	})

	var result darklaunch.WorkflowResult
	if err := json.Unmarshal(state.Result, &result); err != nil {
		t.Fatalf("Unmarshal(result) error = %v", err)
	}
	if result.Greeting != "hello, Sleepy" || result.Approval != "restart-approved" {
		t.Fatalf("unexpected sleep demo result: %+v", result)
	}

	for _, event := range state.History {
		if event.EventType == enginehistory.EventWorkflowReplayMismatch {
			t.Fatalf("run hit replay_mismatch after restart: %+v", event)
		}
	}
}
