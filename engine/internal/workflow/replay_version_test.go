package workflow

import (
	"testing"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

const (
	versionDemoChangeID     = "greeting-upgrade"
	versionDemoActivityType = "demo.compose"
)

func versionStartedHistory(t *testing.T) []enginedb.EngineHistory {
	t.Helper()

	return []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "version-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-version",
		}),
	}
}

// oldVersionDemoDefinition is the pre-GetVersion code: it never calls
// GetVersion, so runs it produces carry no version marker in history.
func oldVersionDemoDefinition() publicworkflow.Definition {
	return publicworkflow.Definition{
		Name:    "version-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var output struct {
				Greeting string `json:"greeting"`
			}
			if err := ctx.Activity("greet", versionDemoActivityType, map[string]string{"name": "Ada"}, &output); err != nil {
				return err
			}
			return ctx.SetResult(map[string]string{"branch": "old", "greeting": output.Greeting})
		},
	}
}

// newVersionDemoDefinition is the same workflow after a code change guarded by
// GetVersion: version 1 preserves the old branch byte-for-byte, version 2 skips
// the activity and returns a different result.
func newVersionDemoDefinition() publicworkflow.Definition {
	return publicworkflow.Definition{
		Name:    "version-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			version := ctx.GetVersion(versionDemoChangeID, 1, 2)
			if version >= 2 {
				return ctx.SetResult(map[string]string{"branch": "new"})
			}
			var output struct {
				Greeting string `json:"greeting"`
			}
			if err := ctx.Activity("greet", versionDemoActivityType, map[string]string{"name": "Ada"}, &output); err != nil {
				return err
			}
			return ctx.SetResult(map[string]string{"branch": "old", "greeting": output.Greeting})
		},
	}
}

func collectVersionMarkers(t *testing.T, events []queuedHistoryEvent) []*enginehistory.WorkflowVersionMarkerPayload {
	t.Helper()

	var markers []*enginehistory.WorkflowVersionMarkerPayload
	for _, event := range events {
		if event.EventType != enginehistory.EventWorkflowVersionMarker {
			continue
		}
		decoded, err := enginehistory.DecodePayload(event.EventType, event.Payload)
		if err != nil {
			t.Fatalf("DecodePayload(%s) error = %v", event.EventType, err)
		}
		payload, ok := decoded.(*enginehistory.WorkflowVersionMarkerPayload)
		if !ok {
			t.Fatalf("expected WorkflowVersionMarkerPayload, got %T", decoded)
		}
		markers = append(markers, payload)
	}
	return markers
}

func TestReplayGetVersionRecordsMaxSupportedOnFirstExecutionAndReplaysRecordedValue(t *testing.T) {
	definition := publicworkflow.Definition{
		Name:    "version-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			version := ctx.GetVersion(versionDemoChangeID, 1, 2)
			return ctx.SetResult(map[string]int{"version": version})
		},
	}

	base := versionStartedHistory(t)
	decision, err := replayDefinition(definition, base, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}

	markers := collectVersionMarkers(t, decision.Events)
	if len(markers) != 1 {
		t.Fatalf("expected exactly one workflow.version_marker event, got %d in %+v", len(markers), decision.Events)
	}
	if markers[0].ChangeID != versionDemoChangeID {
		t.Fatalf("expected marker change_id %q, got %q", versionDemoChangeID, markers[0].ChangeID)
	}
	if markers[0].Version != 2 {
		t.Fatalf("first execution must record maxSupported (2), got %d", markers[0].Version)
	}
	if !equalJSONForTest(t, decision.Result, mustRawJSON(t, map[string]int{"version": 2})) {
		t.Fatalf("expected GetVersion to return maxSupported on first execution, result = %s", decision.Result)
	}

	// Crash-recovery replay: the marker is durable history now; GetVersion must
	// return the recorded value without recording again.
	replayRows := historyFromDecision(t, base, decision.Events)
	replayDecision, err := replayDefinition(definition, replayRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() replay error = %v", err)
	}
	if replayDecision.Kind != decisionCompleted {
		t.Fatalf("expected completed replay decision, got %+v", replayDecision)
	}
	if len(replayDecision.Events) != 0 {
		t.Fatalf("expected no new events on replay, got %+v", replayDecision.Events)
	}
	if !equalJSONForTest(t, replayDecision.Result, decision.Result) {
		t.Fatalf("replayed result %s differs from first execution %s", replayDecision.Result, decision.Result)
	}
}

func TestReplayGetVersionMarkerlessOldRunTakesOldBranchUnderNewCode(t *testing.T) {
	oldDefinition := oldVersionDemoDefinition()
	newDefinition := newVersionDemoDefinition()

	// Execute the old code up to its activity wait, then record the activity
	// outcome, simulating a run that lived under the old deployment.
	base := versionStartedHistory(t)
	scheduleDecision, err := replayDefinition(oldDefinition, base, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() old schedule error = %v", err)
	}
	if scheduleDecision.Kind != decisionWaiting || scheduleDecision.NewActivity == nil {
		t.Fatalf("expected old code to wait on a new activity, got %+v", scheduleDecision)
	}
	scheduledRows := historyFromDecision(t, base, scheduleDecision.Events)
	inFlightRows := append(scheduledRows, historyRow(
		t,
		scheduledRows[len(scheduledRows)-1].SequenceNo+1,
		enginehistory.EventActivityCompleted,
		enginehistory.ActivityCompletedPayload{
			ActivityKey:  "greet",
			ActivityType: versionDemoActivityType,
			Output:       mustRawJSON(t, map[string]string{"greeting": "hello, Ada"}),
		},
	))

	// The deployment swaps to the new code while the run is in flight. The
	// remainder of the run must complete on the old branch: no version marker
	// exists in history, so GetVersion returns minSupported.
	inFlightDecision, err := replayDefinition(newDefinition, inFlightRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() in-flight under new code error = %v", err)
	}
	if inFlightDecision.Kind != decisionCompleted {
		t.Fatalf("expected in-flight run to complete under new code, got %+v", inFlightDecision)
	}
	if inFlightDecision.FailureCode != "" {
		t.Fatalf("expected no failure completing the in-flight run, got %q: %q", inFlightDecision.FailureCode, inFlightDecision.FailureMessage)
	}
	wantOldResult := mustRawJSON(t, map[string]string{"branch": "old", "greeting": "hello, Ada"})
	if !equalJSONForTest(t, inFlightDecision.Result, wantOldResult) {
		t.Fatalf("expected old-branch result %s, got %s", wantOldResult, inFlightDecision.Result)
	}
	if markers := collectVersionMarkers(t, inFlightDecision.Events); len(markers) != 0 {
		t.Fatalf("a marker-less old run must not gain version markers under new code, got %+v", markers)
	}
	for _, event := range inFlightDecision.Events {
		if event.EventType == enginehistory.EventWorkflowReplayMismatch {
			t.Fatalf("in-flight run hit replay mismatch under new code: %+v", inFlightDecision.Events)
		}
	}

	// Full-history crash-recovery replay under the new code: the terminal
	// events are durable now and must replay cleanly with zero new events.
	fullRows := historyFromDecision(t, inFlightRows, inFlightDecision.Events)
	replayDecision, err := replayDefinition(newDefinition, fullRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() full replay under new code error = %v", err)
	}
	if replayDecision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision on full replay, got %+v", replayDecision)
	}
	if len(replayDecision.Events) != 0 {
		t.Fatalf("expected no new events on full replay, got %+v", replayDecision.Events)
	}
	if !equalJSONForTest(t, replayDecision.Result, wantOldResult) {
		t.Fatalf("full replay under new code returned %s, want %s", replayDecision.Result, wantOldResult)
	}
}

func TestReplayGetVersionNewRunTakesNewBranch(t *testing.T) {
	newDefinition := newVersionDemoDefinition()

	decision, err := replayDefinition(newDefinition, versionStartedHistory(t), nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected new run to complete on the new branch, got %+v", decision)
	}
	if decision.NewActivity != nil {
		t.Fatalf("new branch must not schedule the old activity, got %+v", decision.NewActivity)
	}
	if !equalJSONForTest(t, decision.Result, mustRawJSON(t, map[string]string{"branch": "new"})) {
		t.Fatalf("expected new-branch result, got %s", decision.Result)
	}

	markers := collectVersionMarkers(t, decision.Events)
	if len(markers) != 1 {
		t.Fatalf("expected exactly one version marker for the new run, got %d in %+v", len(markers), decision.Events)
	}
	if markers[0].ChangeID != versionDemoChangeID || markers[0].Version != 2 {
		t.Fatalf("expected marker {%s, 2}, got %+v", versionDemoChangeID, markers[0])
	}
}

func TestReplayGetVersionRecordedBelowMinSupportedFailsControlled(t *testing.T) {
	definition := publicworkflow.Definition{
		Name:    "version-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			version := ctx.GetVersion(versionDemoChangeID, 2, 3)
			return ctx.SetResult(map[string]int{"version": version})
		},
	}

	rows := append(versionStartedHistory(t), historyRow(
		t,
		2,
		enginehistory.EventWorkflowVersionMarker,
		enginehistory.WorkflowVersionMarkerPayload{
			ChangeID: versionDemoChangeID,
			Version:  1,
		},
	))

	decision, err := replayDefinition(definition, rows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionFailed {
		t.Fatalf("expected controlled failure when recorded version is below minSupported, got %+v", decision)
	}
	if decision.FailureCode == "replay_mismatch" {
		t.Fatalf("dropping support for an old version must be a controlled failure, not replay_mismatch: %+v", decision)
	}
	if decision.FailureCode != "version_unsupported" {
		t.Fatalf("expected failure code %q, got %q (%q)", "version_unsupported", decision.FailureCode, decision.FailureMessage)
	}
}
