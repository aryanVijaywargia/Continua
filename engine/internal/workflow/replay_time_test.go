package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

// historyFromDecision materializes the events queued by a prior activation into
// history rows, simulating the durable write that precedes a crash-recovery
// replay. Appended rows continue the base rows' sequence numbering.
func historyFromDecision(t *testing.T, base []enginedb.EngineHistory, events []queuedHistoryEvent) []enginedb.EngineHistory {
	t.Helper()

	rows := append([]enginedb.EngineHistory(nil), base...)
	nextSequence := base[len(base)-1].SequenceNo + 1
	for i, event := range events {
		rows = append(rows, historyRow(t, nextSequence+int32(i), event.EventType, event.Payload))
	}
	return rows
}

func startedHistory(t *testing.T) []enginedb.EngineHistory {
	t.Helper()

	return []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "time-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-time",
		}),
	}
}

func TestReplayNowRecordsOnFirstExecutionAndReplaysRecordedValue(t *testing.T) {
	definition := publicworkflow.Definition{
		Name:    "time-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			first := ctx.Now()
			second := ctx.Now()
			return ctx.SetResult(map[string]string{
				"first":  first.UTC().Format(time.RFC3339Nano),
				"second": second.UTC().Format(time.RFC3339Nano),
			})
		},
	}

	base := startedHistory(t)
	before := time.Now().UTC()
	decision, err := replayDefinition(definition, base, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	after := time.Now().UTC()
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}

	var recordedTimes []time.Time
	for _, event := range decision.Events {
		if event.EventType != enginehistory.EventWorkflowTimeRecorded {
			continue
		}
		decoded, err := enginehistory.DecodePayload(event.EventType, event.Payload)
		if err != nil {
			t.Fatalf("DecodePayload(%s) error = %v", event.EventType, err)
		}
		payload, ok := decoded.(*enginehistory.WorkflowTimeRecordedPayload)
		if !ok {
			t.Fatalf("expected WorkflowTimeRecordedPayload, got %T", decoded)
		}
		recordedTimes = append(recordedTimes, payload.Now)
	}
	if len(recordedTimes) != 2 {
		t.Fatalf("expected two workflow.time_recorded events, got %d in %+v", len(recordedTimes), decision.Events)
	}
	for i, recorded := range recordedTimes {
		if recorded.Before(before.Add(-time.Second)) || recorded.After(after.Add(time.Second)) {
			t.Fatalf("recorded time %d = %v outside execution window [%v, %v]", i, recorded, before, after)
		}
	}

	var result map[string]string
	if err := json.Unmarshal(decision.Result, &result); err != nil {
		t.Fatalf("Unmarshal(result) error = %v", err)
	}
	if result["first"] != recordedTimes[0].UTC().Format(time.RFC3339Nano) {
		t.Fatalf("Now() returned %s but recorded %v", result["first"], recordedTimes[0])
	}
	if result["second"] != recordedTimes[1].UTC().Format(time.RFC3339Nano) {
		t.Fatalf("second Now() returned %s but recorded %v", result["second"], recordedTimes[1])
	}

	// Crash-recovery replay: the recorded events are durable history now.
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

func TestReplaySideEffectRecordsOnceAndDoesNotReexecuteOnReplay(t *testing.T) {
	calls := 0
	definition := publicworkflow.Definition{
		Name:    "time-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var token string
			if err := ctx.SideEffect("token", func() (any, error) {
				calls++
				return fmt.Sprintf("token-%d", calls), nil
			}, &token); err != nil {
				return err
			}
			return ctx.SetResult(map[string]string{"token": token})
		},
	}

	base := startedHistory(t)
	decision, err := replayDefinition(definition, base, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}
	if calls != 1 {
		t.Fatalf("expected side effect fn to run exactly once on first execution, ran %d times", calls)
	}

	var sideEffects []*enginehistory.WorkflowSideEffectRecordedPayload
	for _, event := range decision.Events {
		if event.EventType != enginehistory.EventWorkflowSideEffectRecorded {
			continue
		}
		decoded, err := enginehistory.DecodePayload(event.EventType, event.Payload)
		if err != nil {
			t.Fatalf("DecodePayload(%s) error = %v", event.EventType, err)
		}
		payload, ok := decoded.(*enginehistory.WorkflowSideEffectRecordedPayload)
		if !ok {
			t.Fatalf("expected WorkflowSideEffectRecordedPayload, got %T", decoded)
		}
		sideEffects = append(sideEffects, payload)
	}
	if len(sideEffects) != 1 {
		t.Fatalf("expected one workflow.side_effect_recorded event, got %d in %+v", len(sideEffects), decision.Events)
	}
	if sideEffects[0].SideEffectKey != "token" {
		t.Fatalf("expected side effect key %q, got %q", "token", sideEffects[0].SideEffectKey)
	}
	if !equalJSONForTest(t, sideEffects[0].Value, mustRawJSON(t, "token-1")) {
		t.Fatalf("expected recorded side effect value %q, got %s", "token-1", sideEffects[0].Value)
	}

	replayRows := historyFromDecision(t, base, decision.Events)
	replayDecision, err := replayDefinition(definition, replayRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() replay error = %v", err)
	}
	if replayDecision.Kind != decisionCompleted {
		t.Fatalf("expected completed replay decision, got %+v", replayDecision)
	}
	if calls != 1 {
		t.Fatalf("side effect fn must not re-execute on replay, ran %d times total", calls)
	}
	if len(replayDecision.Events) != 0 {
		t.Fatalf("expected no new events on replay, got %+v", replayDecision.Events)
	}
	if !equalJSONForTest(t, replayDecision.Result, mustRawJSON(t, map[string]string{"token": "token-1"})) {
		t.Fatalf("expected replay to return recorded token-1, got %s", replayDecision.Result)
	}
}

func TestReplaySleepDurationSchedulesTimerAndSurvivesReplay(t *testing.T) {
	const sleepFor = 5 * time.Minute

	definition := publicworkflow.Definition{
		Name:    "time-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			if err := ctx.Sleep("wait", sleepFor); err != nil {
				return err
			}
			return ctx.SetResult(map[string]string{"status": "woke"})
		},
	}

	base := startedHistory(t)
	before := time.Now().UTC()
	decision, err := replayDefinition(definition, base, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	after := time.Now().UTC()
	if decision.Kind != decisionWaiting {
		t.Fatalf("expected waiting decision, got %+v", decision)
	}
	if decision.NewTimer == nil {
		t.Fatalf("expected Sleep to schedule a new timer, got %+v", decision)
	}
	if decision.NewTimer.TimerKey != "wait" {
		t.Fatalf("expected timer key %q, got %q", "wait", decision.NewTimer.TimerKey)
	}
	dueAt := decision.NewTimer.DueAt
	if dueAt.Before(before.Add(sleepFor-time.Second)) || dueAt.After(after.Add(sleepFor+time.Second)) {
		t.Fatalf("expected due-at ~now+%v, got %v (executed between %v and %v)", sleepFor, dueAt, before, after)
	}
	var wait enginehistory.TimerWait
	if err := json.Unmarshal(decision.WaitingFor, &wait); err != nil {
		t.Fatalf("decode waiting state: %v", err)
	}
	if wait.Kind != enginehistory.WaitKindTimer || wait.TimerKey != "wait" {
		t.Fatalf("expected timer wait, got %+v", wait)
	}

	// Restart mid-sleep: the recorded events replay without recomputing the
	// deadline, so no mismatch and no duplicate timer.
	midSleepRows := historyFromDecision(t, base, decision.Events)
	midSleepDecision, err := replayDefinition(definition, midSleepRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() mid-sleep replay error = %v", err)
	}
	if midSleepDecision.Kind != decisionWaiting {
		t.Fatalf("expected waiting decision on mid-sleep replay, got %+v", midSleepDecision)
	}
	if midSleepDecision.NewTimer != nil {
		t.Fatalf("mid-sleep replay must not schedule a duplicate timer, got %+v", midSleepDecision.NewTimer)
	}
	if len(midSleepDecision.Events) != 0 {
		t.Fatalf("expected no new events on mid-sleep replay, got %+v", midSleepDecision.Events)
	}

	// Timer fires after the restart: the run completes without mismatch.
	firedRows := append(midSleepRows, historyRow(
		t,
		midSleepRows[len(midSleepRows)-1].SequenceNo+1,
		enginehistory.EventTimerFired,
		enginehistory.TimerFiredPayload{TimerKey: "wait"},
	))
	firedDecision, err := replayDefinition(definition, firedRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() post-fire replay error = %v", err)
	}
	if firedDecision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision after timer fired, got %+v", firedDecision)
	}
	if !equalJSONForTest(t, firedDecision.Result, mustRawJSON(t, map[string]string{"status": "woke"})) {
		t.Fatalf("unexpected post-sleep result %s", firedDecision.Result)
	}
}

func TestReplaySideEffectKeyChangeQuarantinesAsReplayMismatch(t *testing.T) {
	recordingDefinition := publicworkflow.Definition{
		Name:    "time-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var token string
			if err := ctx.SideEffect("token", func() (any, error) {
				return "token-1", nil
			}, &token); err != nil {
				return err
			}
			return ctx.SetResult(map[string]string{"token": token})
		},
	}

	base := startedHistory(t)
	decision, err := replayDefinition(recordingDefinition, base, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}

	changedDefinition := publicworkflow.Definition{
		Name:    "time-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var token string
			if err := ctx.SideEffect("renamed", func() (any, error) {
				return "token-1", nil
			}, &token); err != nil {
				return err
			}
			return ctx.SetResult(map[string]string{"token": token})
		},
	}

	replayRows := historyFromDecision(t, base, decision.Events)
	mismatchDecision, err := replayDefinition(changedDefinition, replayRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() replay error = %v", err)
	}
	if mismatchDecision.Kind != decisionQuarantined {
		t.Fatalf("expected quarantined decision for side effect key change, got %+v", mismatchDecision)
	}
	if len(mismatchDecision.Events) != 0 {
		t.Fatalf("expected replay mismatch to queue no history events, got %+v", mismatchDecision.Events)
	}
	if mismatchDecision.FailureCode != "replay_mismatch" {
		t.Fatalf("expected replay_mismatch failure code, got %q", mismatchDecision.FailureCode)
	}
	var wait struct {
		Kind   string `json:"kind"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(mismatchDecision.WaitingFor, &wait); err != nil {
		t.Fatalf("decode waiting_for: %v", err)
	}
	if wait.Kind != "replay_mismatch" || wait.Detail == "" {
		t.Fatalf("expected replay_mismatch waiting_for detail, got %+v", wait)
	}
}

func TestReplayNowAgainstDifferentRecordedEventQuarantinesAsReplayMismatch(t *testing.T) {
	input := mustRawJSON(t, map[string]string{"name": "Ada"})
	historyRows := []enginedb.EngineHistory{
		historyRow(t, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
			DefinitionName:    "time-demo",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-time",
			Input:             input,
		}),
		historyRow(t, 2, enginehistory.EventActivityScheduled, enginehistory.ActivityScheduledPayload{
			ActivityKey:  "fetch",
			ActivityType: "demo.activity",
			Input:        input,
		}),
	}

	definition := publicworkflow.Definition{
		Name:    "time-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			now := ctx.Now()
			return ctx.SetResult(map[string]string{"now": now.UTC().Format(time.RFC3339Nano)})
		},
	}

	decision, err := replayDefinition(definition, historyRows, nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionQuarantined {
		t.Fatalf("expected quarantined decision when Now() faces a foreign recorded event, got %+v", decision)
	}
	if len(decision.Events) != 0 {
		t.Fatalf("expected replay mismatch to queue no history events, got %+v", decision.Events)
	}
	if decision.FailureCode != "replay_mismatch" {
		t.Fatalf("expected replay_mismatch failure code, got %q", decision.FailureCode)
	}
	var wait struct {
		Kind   string `json:"kind"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(decision.WaitingFor, &wait); err != nil {
		t.Fatalf("decode waiting_for: %v", err)
	}
	if wait.Kind != "replay_mismatch" || wait.Detail == "" {
		t.Fatalf("expected replay_mismatch waiting_for detail, got %+v", wait)
	}
}

func TestReplayTimeAPIValidation(t *testing.T) {
	var sleepErr error
	var sideEffectKeyErr error
	var sideEffectNilErr error
	var sideEffectFnErr error
	fnFailure := errors.New("side effect exploded")

	definition := publicworkflow.Definition{
		Name:    "time-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			sleepErr = ctx.Sleep("", time.Second)

			var out string
			sideEffectKeyErr = ctx.SideEffect("", func() (any, error) {
				return "never", nil
			}, &out)

			sideEffectNilErr = ctx.SideEffect("nil-fn", nil, &out)

			sideEffectFnErr = ctx.SideEffect("failing", func() (any, error) {
				return nil, fnFailure
			}, &out)

			return ctx.SetResult(map[string]string{"status": "done"})
		},
	}

	decision, err := replayDefinition(definition, startedHistory(t), nil, nil)
	if err != nil {
		t.Fatalf("replayDefinition() error = %v", err)
	}
	if decision.Kind != decisionCompleted {
		t.Fatalf("expected completed decision, got %+v", decision)
	}

	if !errors.Is(sleepErr, publicworkflow.ErrEmptyKey) {
		t.Fatalf("Sleep with empty key: want ErrEmptyKey, got %v", sleepErr)
	}
	if !errors.Is(sideEffectKeyErr, publicworkflow.ErrEmptyKey) {
		t.Fatalf("SideEffect with empty key: want ErrEmptyKey, got %v", sideEffectKeyErr)
	}
	if sideEffectNilErr == nil || sideEffectNilErr.Error() != "workflow: SideEffect fn must not be nil" {
		t.Fatalf("SideEffect with nil fn: want nil fn error, got %v", sideEffectNilErr)
	}
	if !errors.Is(sideEffectFnErr, fnFailure) {
		t.Fatalf("SideEffect fn error must propagate, got %v", sideEffectFnErr)
	}

	for _, event := range decision.Events {
		if event.EventType == enginehistory.EventWorkflowSideEffectRecorded {
			t.Fatalf("failed or invalid side effects must not be recorded, got %+v", decision.Events)
		}
		if event.EventType == enginehistory.EventTimerScheduled {
			t.Fatalf("empty-key sleep must not schedule a timer, got %+v", decision.Events)
		}
	}
}
