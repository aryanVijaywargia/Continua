package main

import (
	"encoding/json"
	"testing"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"

	"github.com/continua-ai/continua/engine/cmd/continua-engine/internal/darklaunch"
)

// A definition swap between restarts must not fail in-flight runs: the run
// started under the old code completes its old branch under the new
// GetVersion-guarded code, while a fresh run takes the new branch and records
// a workflow.version_marker in history.
func TestEngineRuntimeDefinitionSwapKeepsInFlightRunOnOldBranch(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)
	t.Setenv(darklaunch.TestVersionDemoCodeEnv, darklaunch.VersionDemoCodeOld)

	serve := startServe(t)

	inFlightKey := "runtime-version-swap-inflight"
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", inFlightKey,
		"--definition", darklaunch.VersionDemoDefinitionName,
		"--version", darklaunch.VersionDemoDefinitionVersion,
		"--request-key", "req-version-swap-inflight",
		"--input", `{"name":"Ada"}`,
	); err != nil {
		t.Fatalf("start version demo workflow under old code: %v", err)
	}

	waitForInspect(t, inFlightKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})
	serve.stop(t)

	// Deploy the new code while the run is waiting on its approval signal.
	t.Setenv(darklaunch.TestVersionDemoCodeEnv, darklaunch.VersionDemoCodeNew)
	serve = startServe(t)
	defer serve.stop(t)

	if _, _, err := executeCommand(t,
		"signal",
		"--instance-key", inFlightKey,
		"--signal-name", "approval",
		"--payload", `{"approval":"swap-approved"}`,
	); err != nil {
		t.Fatalf("signal in-flight run after definition swap: %v", err)
	}

	inFlightState := waitForInspect(t, inFlightKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusCompleted)
	})

	var inFlightResult map[string]string
	if err := json.Unmarshal(inFlightState.Result, &inFlightResult); err != nil {
		t.Fatalf("Unmarshal(in-flight result) error = %v", err)
	}
	if inFlightResult["branch"] != "old" {
		t.Fatalf("in-flight run must complete its old branch under new code, got %+v", inFlightResult)
	}
	if inFlightResult["greeting"] != "hello, Ada" || inFlightResult["approval"] != "swap-approved" {
		t.Fatalf("unexpected in-flight old-branch result: %+v", inFlightResult)
	}
	for _, event := range inFlightState.History {
		if event.EventType == enginehistory.EventWorkflowReplayMismatch {
			t.Fatalf("in-flight run hit replay_mismatch after definition swap: %+v", event)
		}
		if event.EventType == enginehistory.EventWorkflowVersionMarker {
			t.Fatalf("marker-less old run must not gain a version marker, got %+v", event)
		}
	}

	// A fresh run under the new code takes the new branch and records the
	// version marker.
	newRunKey := "runtime-version-swap-new"
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", newRunKey,
		"--definition", darklaunch.VersionDemoDefinitionName,
		"--version", darklaunch.VersionDemoDefinitionVersion,
		"--request-key", "req-version-swap-new",
		"--input", `{"name":"Grace"}`,
	); err != nil {
		t.Fatalf("start version demo workflow under new code: %v", err)
	}

	newRunState := waitForInspect(t, newRunKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusCompleted)
	})

	var newRunResult map[string]string
	if err := json.Unmarshal(newRunState.Result, &newRunResult); err != nil {
		t.Fatalf("Unmarshal(new run result) error = %v", err)
	}
	if newRunResult["branch"] != "new" {
		t.Fatalf("new run must take the new branch, got %+v", newRunResult)
	}
	if newRunResult["greeting"] != "hello, Grace" {
		t.Fatalf("unexpected new-branch result: %+v", newRunResult)
	}

	var markers []enginehistory.WorkflowVersionMarkerPayload
	for _, event := range newRunState.History {
		if event.EventType == enginehistory.EventWorkflowReplayMismatch {
			t.Fatalf("new run hit replay_mismatch: %+v", event)
		}
		if event.EventType != enginehistory.EventWorkflowVersionMarker {
			continue
		}
		var payload enginehistory.WorkflowVersionMarkerPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("Unmarshal(version marker payload) error = %v", err)
		}
		markers = append(markers, payload)
	}
	if len(markers) != 1 {
		t.Fatalf("expected exactly one workflow.version_marker in new run history, got %d in %+v", len(markers), newRunState.History)
	}
	if markers[0].ChangeID != darklaunch.VersionDemoChangeID || markers[0].Version != 2 {
		t.Fatalf("expected version marker {%s, 2}, got %+v", darklaunch.VersionDemoChangeID, markers[0])
	}
}
