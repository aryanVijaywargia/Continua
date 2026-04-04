package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"

	"github.com/continua-ai/continua/engine/cmd/continua-engine/internal/darklaunch"
)

func TestEngineRuntimeHappyPathAndTerminalRejection(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)

	serve := startServe(t)
	defer serve.stop(t)

	instanceKey := "runtime-happy-path"
	input := map[string]any{
		"name":     "Ada",
		"timer_at": time.Now().Add(300 * time.Millisecond).UTC().Format(time.RFC3339),
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal(input) error = %v", err)
	}

	stdout, stderr, err := executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-happy-path",
		"--input", string(inputJSON),
	)
	if err != nil {
		t.Fatalf("start command error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var started startResponse
	if err := json.Unmarshal([]byte(stdout), &started); err != nil {
		t.Fatalf("Unmarshal(start stdout) error = %v", err)
	}

	state := waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})
	if state.InstanceID != started.InstanceID || state.RunID != started.RunID {
		t.Fatalf("inspect returned unexpected identity: %+v vs %+v", state, started)
	}

	stdout, stderr, err = executeCommand(t,
		"signal",
		"--instance-key", instanceKey,
		"--signal-name", "approval",
		"--payload", `{"approval":"yes"}`,
	)
	if err != nil {
		t.Fatalf("signal command error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var signaled controlResponse
	if err := json.Unmarshal([]byte(stdout), &signaled); err != nil {
		t.Fatalf("Unmarshal(signal stdout) error = %v", err)
	}
	if !signaled.Accepted {
		t.Fatalf("expected signal accepted response, got %+v", signaled)
	}

	state = waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusCompleted)
	})

	var result darklaunch.WorkflowResult
	if err := json.Unmarshal(state.Result, &result); err != nil {
		t.Fatalf("Unmarshal(result) error = %v", err)
	}
	if result.Greeting != "hello, Ada" || result.Approval != "yes" {
		t.Fatalf("unexpected workflow result: %+v", result)
	}

	var startedPayload map[string]any
	if err := json.Unmarshal(state.History[0].Payload, &startedPayload); err != nil {
		t.Fatalf("Unmarshal(started payload) error = %v", err)
	}
	if startedPayload["instance_key"] != instanceKey {
		t.Fatalf("expected workflow.started instance_key %q, got %+v", instanceKey, startedPayload)
	}

	eventTypes := filterEventTypes(state.History)
	expectedOrder := []string{
		"workflow.started",
		"activity.scheduled",
		"activity.completed",
		"timer.scheduled",
		"timer.fired",
		"signal.received",
		"workflow.completed",
	}
	if !equalStrings(eventTypes, expectedOrder) {
		t.Fatalf("unexpected history order: got %v want %v", eventTypes, expectedOrder)
	}

	stdout, _, err = executeCommand(t,
		"signal",
		"--instance-key", instanceKey,
		"--signal-name", "approval",
	)
	if err == nil {
		t.Fatal("expected signal on terminal run to fail")
	}
	assertJSONErrorCode(t, stdout, "run_terminal")

	stdout, _, err = executeCommand(t, "cancel", "--instance-key", instanceKey)
	if err == nil {
		t.Fatal("expected cancel on terminal run to fail")
	}
	assertJSONErrorCode(t, stdout, "run_terminal")
}

func TestStartCommandDedupeAndRegistrationErrors(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)

	inputJSON := `{"name":"Grace"}`
	instanceKey := "runtime-start-dedupe"

	stdout, _, err := executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-1",
		"--input", inputJSON,
	)
	if err != nil {
		t.Fatalf("initial start error = %v", err)
	}
	var firstResponse startResponse
	if err := json.Unmarshal([]byte(stdout), &firstResponse); err != nil {
		t.Fatalf("Unmarshal(initial start stdout) error = %v", err)
	}

	stdout, _, err = executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-1",
		"--input", `{"name":"Ignored"}`,
	)
	if err != nil {
		t.Fatalf("duplicate request key start error = %v", err)
	}
	var duplicateResponse startResponse
	if err := json.Unmarshal([]byte(stdout), &duplicateResponse); err != nil {
		t.Fatalf("Unmarshal(duplicate start stdout) error = %v", err)
	}
	if duplicateResponse != firstResponse {
		t.Fatalf("expected duplicate request key to replay cached response, got %+v want %+v", duplicateResponse, firstResponse)
	}

	stdout, _, err = executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-2",
	)
	if err == nil {
		t.Fatal("expected instance conflict start to fail")
	}
	assertJSONErrorCode(t, stdout, "instance_conflict")

	stdout, _, err = executeCommand(t,
		"start",
		"--instance-key", "runtime-unknown-definition",
		"--definition", "missing",
		"--version", "v0",
		"--request-key", "req-unknown",
	)
	if err == nil {
		t.Fatal("expected unknown definition start to fail")
	}
	assertJSONErrorCode(t, stdout, "definition_not_registered")
}

func TestEngineRuntimeTimerAndSignalPersistenceAcrossRestart(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)

	serve := startServe(t)

	timerInstanceKey := "runtime-restart-timer"
	timerInput, _ := json.Marshal(map[string]any{
		"name":     "Timer",
		"timer_at": time.Now().Add(750 * time.Millisecond).UTC().Format(time.RFC3339),
	})
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", timerInstanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-timer-restart",
		"--input", string(timerInput),
	); err != nil {
		t.Fatalf("start timer restart workflow: %v", err)
	}

	waitForInspect(t, timerInstanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "timer")
	})
	serve.stop(t)

	time.Sleep(time.Second)

	serve = startServe(t)
	waitForInspect(t, timerInstanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})
	serve.stop(t)

	serve = startServe(t)
	signalInstanceKey := "runtime-restart-signal"
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", signalInstanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-signal-restart",
		"--input", `{"name":"Signal","timer_at":"1970-01-01T00:00:00Z"}`,
	); err != nil {
		t.Fatalf("start signal restart workflow: %v", err)
	}

	waitForInspect(t, signalInstanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})
	serve.stop(t)

	if _, _, err := executeCommand(t,
		"signal",
		"--instance-key", signalInstanceKey,
		"--signal-name", "approval",
		"--payload", `{"approval":"restarted"}`,
	); err != nil {
		t.Fatalf("signal while serve stopped: %v", err)
	}

	serve = startServe(t)
	defer serve.stop(t)

	state := waitForInspect(t, signalInstanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusCompleted)
	})

	var result darklaunch.WorkflowResult
	if err := json.Unmarshal(state.Result, &result); err != nil {
		t.Fatalf("Unmarshal(result) error = %v", err)
	}
	if result.Approval != "restarted" {
		t.Fatalf("expected persisted signal payload after restart, got %+v", result)
	}
}

func TestEngineRuntimeActivityPickupAfterRestart(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)
	t.Setenv("ENGINE_ACTIVITY_POLL_INTERVAL", "5s")

	serve := startServe(t)

	instanceKey := "runtime-restart-activity-pickup"
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-activity-pickup",
		"--input", `{"name":"Pickup","timer_at":"1970-01-01T00:00:00Z"}`,
	); err != nil {
		t.Fatalf("start activity pickup workflow: %v", err)
	}

	waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "activity")
	})
	serve.stop(t)

	t.Setenv("ENGINE_ACTIVITY_POLL_INTERVAL", "50ms")
	serve = startServe(t)
	defer serve.stop(t)

	waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})
}

func TestSignalAndCancelRejectConcurrentTerminalization(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)
	store := enginestore.New(db.Pool)

	assertConcurrentTerminalizationRejectsControlCommand(t, store, db, "signal-terminal-race", "signal",
		"--signal-name", "approval", "--payload", `{"approval":"late"}`,
	)
	assertConcurrentTerminalizationRejectsControlCommand(t, store, db, "cancel-terminal-race", "cancel")
}

func TestEngineRuntimeRunReclaimedAfterRestartBeforeActivationCompletes(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)
	t.Setenv("ENGINE_RUN_LEASE_TTL", "250ms")

	tempDir := t.TempDir()
	claimMarker := filepath.Join(tempDir, "workflow-claim.marker")
	claimRelease := filepath.Join(tempDir, "workflow-claim.release")

	serve := startServeProcess(t,
		darklaunchTestEnv(engineworkflow.TestWorkflowClaimMarkerEnv, claimMarker),
		darklaunchTestEnv(engineworkflow.TestWorkflowClaimReleaseEnv, claimRelease),
	)

	instanceKey := "runtime-restart-before-activation-completes"
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-before-activation-completes",
		"--input", `{"name":"ClaimRestart","timer_at":"1970-01-01T00:00:00Z"}`,
	); err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	waitForFileExists(t, claimMarker)
	serve.kill(t)

	serve = startServeProcess(t)
	defer serve.kill(t)

	waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})

	if _, _, err := executeCommand(t,
		"signal",
		"--instance-key", instanceKey,
		"--signal-name", "approval",
		"--payload", `{"approval":"claimed"}`,
	); err != nil {
		t.Fatalf("signal reclaimed workflow: %v", err)
	}

	waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusCompleted)
	})
}

func TestEngineRuntimeMidActivityRestartRecovery(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)
	t.Setenv("ENGINE_ACTIVITY_LEASE_TTL", "250ms")

	tempDir := t.TempDir()
	attemptsFile := filepath.Join(tempDir, "activity-attempts.log")
	releaseFile := filepath.Join(tempDir, "activity-release")

	serve := startServeProcess(t,
		darklaunchTestEnv(darklaunch.TestActivityAttemptsFileEnv, attemptsFile),
		darklaunchTestEnv(darklaunch.TestActivityReleaseFileEnv, releaseFile),
	)

	instanceKey := "runtime-restart-activity-mid-flight"
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-activity-mid-flight",
		"--input", `{"name":"MidFlight","timer_at":"1970-01-01T00:00:00Z"}`,
	); err != nil {
		t.Fatalf("start activity recovery workflow: %v", err)
	}

	waitForFileLineCount(t, attemptsFile, 1)
	serve.kill(t)

	serve = startServeProcess(t,
		darklaunchTestEnv(darklaunch.TestActivityAttemptsFileEnv, attemptsFile),
		darklaunchTestEnv(darklaunch.TestActivityReleaseFileEnv, releaseFile),
	)
	defer serve.kill(t)

	waitForFileLineCount(t, attemptsFile, 2)
	if err := os.WriteFile(releaseFile, []byte("release"), 0o644); err != nil {
		t.Fatalf("WriteFile(release) error = %v", err)
	}

	waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})

	if count := fileLineCount(t, attemptsFile); count < 2 {
		t.Fatalf("expected duplicate activity execution after restart, got %d attempt(s)", count)
	}

	if _, _, err := executeCommand(t,
		"signal",
		"--instance-key", instanceKey,
		"--signal-name", "approval",
		"--payload", `{"approval":"recovered"}`,
	); err != nil {
		t.Fatalf("signal recovered workflow: %v", err)
	}

	state := waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusCompleted)
	})

	var result darklaunch.WorkflowResult
	if err := json.Unmarshal(state.Result, &result); err != nil {
		t.Fatalf("Unmarshal(result) error = %v", err)
	}
	if result.Greeting != "hello, MidFlight" || result.Approval != "recovered" {
		t.Fatalf("unexpected recovered workflow result: %+v", result)
	}
}

func TestEngineRuntimeRunReclaimedBeforeFinalCommit(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	configureRuntimeEnv(t, db.DatabaseURL)
	t.Setenv("ENGINE_RUN_LEASE_TTL", "250ms")

	tempDir := t.TempDir()
	finalMarker := filepath.Join(tempDir, "workflow-final.marker")
	finalRelease := filepath.Join(tempDir, "workflow-final.release")

	serve := startServeProcess(t,
		darklaunchTestEnv(engineworkflow.TestWorkflowFinalMarkerEnv, finalMarker),
		darklaunchTestEnv(engineworkflow.TestWorkflowFinalReleaseEnv, finalRelease),
	)

	instanceKey := "runtime-restart-before-final-commit"
	if _, _, err := executeCommand(t,
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-before-final-commit",
		"--input", `{"name":"FinalRestart","timer_at":"1970-01-01T00:00:00Z"}`,
	); err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusWaiting) &&
			jsonContainsKind(state.WaitingFor, "signal")
	})

	if _, _, err := executeCommand(t,
		"signal",
		"--instance-key", instanceKey,
		"--signal-name", "approval",
		"--payload", `{"approval":"final"}`,
	); err != nil {
		t.Fatalf("signal workflow: %v", err)
	}

	waitForFileExists(t, finalMarker)
	serve.kill(t)

	serve = startServeProcess(t)
	defer serve.kill(t)

	state := waitForInspect(t, instanceKey, func(state inspectResponse) bool {
		return state.Status == string(enginedb.EngineRunLifecycleStatusCompleted)
	})

	var result darklaunch.WorkflowResult
	if err := json.Unmarshal(state.Result, &result); err != nil {
		t.Fatalf("Unmarshal(result) error = %v", err)
	}
	if result.Greeting != "hello, FinalRestart" || result.Approval != "final" {
		t.Fatalf("unexpected final restart result: %+v", result)
	}
}

type serveHandle struct {
	cancel context.CancelFunc
	done   chan error
}

type serveProcess struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
	done   chan error
}

type commandResult struct {
	stdout string
	stderr string
	err    error
}

func startServe(t *testing.T) serveHandle {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := newRootCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"serve"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	return serveHandle{
		cancel: cancel,
		done:   done,
	}
}

func (s serveHandle) stop(t *testing.T) {
	t.Helper()

	s.cancel()
	select {
	case err := <-s.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("serve command returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve command did not stop")
	}
}

func startServeProcess(t *testing.T, extraEnv ...string) *serveProcess {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessServe$")
	cmd.Env = append(os.Environ(), "GO_WANT_ENGINE_SERVE_HELPER=1")
	cmd.Env = append(cmd.Env, extraEnv...)

	process := &serveProcess{
		cmd:  cmd,
		done: make(chan error, 1),
	}
	cmd.Stdout = &process.stdout
	cmd.Stderr = &process.stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper serve process: %v", err)
	}
	go func() {
		process.done <- cmd.Wait()
	}()

	return process
}

func (s *serveProcess) kill(t *testing.T) {
	t.Helper()

	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return
	}

	if err := s.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("kill helper serve process: %v", err)
	}

	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatalf("helper serve process did not exit\nstdout=%s\nstderr=%s", s.stdout.String(), s.stderr.String())
	}
}

func configureRuntimeEnv(t *testing.T, databaseURL string) {
	t.Helper()

	t.Setenv("ENGINE_DATABASE_URL", databaseURL)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENGINE_WORKFLOW_POLL_INTERVAL", "50ms")
	t.Setenv("ENGINE_ACTIVITY_POLL_INTERVAL", "50ms")
	t.Setenv("ENGINE_MAINTENANCE_POLL_INTERVAL", "50ms")
	t.Setenv("ENGINE_RUN_LEASE_TTL", "500ms")
	t.Setenv("ENGINE_ACTIVITY_LEASE_TTL", "500ms")
	t.Setenv("ENGINE_REQUEST_DEDUPE_TTL", "2s")
}

func inspectInstance(t *testing.T, instanceKey string) inspectResponse {
	t.Helper()

	stdout, stderr, err := executeCommand(t, "inspect", "--instance-key", instanceKey)
	if err != nil {
		t.Fatalf("inspect command error = %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var state inspectResponse
	if err := json.Unmarshal([]byte(stdout), &state); err != nil {
		t.Fatalf("Unmarshal(inspect stdout) error = %v", err)
	}
	return state
}

func waitForInspect(t *testing.T, instanceKey string, predicate func(inspectResponse) bool) inspectResponse {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	var last inspectResponse
	for time.Now().Before(deadline) {
		last = inspectInstance(t, instanceKey)
		if predicate(last) {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for inspect predicate, last state = %+v", last)
	return inspectResponse{}
}

func assertConcurrentTerminalizationRejectsControlCommand(
	t *testing.T,
	store *enginestore.Store,
	db *enginetest.TestDatabase,
	instanceKey string,
	command string,
	extraArgs ...string,
) {
	t.Helper()

	startArgs := []string{
		"start",
		"--instance-key", instanceKey,
		"--definition", darklaunch.DemoDefinitionName,
		"--version", darklaunch.DemoDefinitionVersion,
		"--request-key", "req-" + instanceKey,
		"--input", `{"name":"TerminalRace","timer_at":"1970-01-01T00:00:00Z"}`,
	}
	stdout, stderr, err := executeCommand(t, startArgs...)
	if err != nil {
		t.Fatalf("start command error = %v stdout=%q stderr=%q", err, stdout, stderr)
	}

	ctx := context.Background()
	_, run, err := loadLatestRun(ctx, store, instanceKey)
	if err != nil {
		t.Fatalf("loadLatestRun() error = %v", err)
	}
	claimed, err := store.ClaimNextRun(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	if claimed.ID != run.ID {
		t.Fatalf("claimed wrong run: got %s want %s", claimed.ID, run.ID)
	}

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lockedRun, err := tx.GetRunForUpdate(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunForUpdate() error = %v", err)
	}

	commandDone := make(chan commandResult, 1)
	go func() {
		args := append([]string{command, "--instance-key", instanceKey}, extraArgs...)
		stdout, stderr, err := executeCommand(t, args...)
		commandDone <- commandResult{stdout: stdout, stderr: stderr, err: err}
	}()

	waitForRunLockWaiter(t, db)

	if _, err := tx.TransitionRunToCompleted(ctx, enginedb.TransitionRunToCompletedParams{
		ID:           lockedRun.ID,
		ClaimedBy:    lockedRun.ClaimedBy,
		Result:       []byte(`{"done":true}`),
		CustomStatus: nil,
	}); err != nil {
		t.Fatalf("TransitionRunToCompleted() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	var result commandResult
	select {
	case result = <-commandDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s command did not finish", command)
	}
	if result.err == nil {
		t.Fatalf("expected %s command to fail once run terminalized, stdout=%q stderr=%q", command, result.stdout, result.stderr)
	}
	assertJSONErrorCode(t, result.stdout, "run_terminal")

	inboxRows, err := store.ListPendingInboxByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListPendingInboxByRun() error = %v", err)
	}
	if len(inboxRows) != 0 {
		t.Fatalf("expected no pending inbox rows after rejected %s, got %+v", command, inboxRows)
	}
}

func waitForRunLockWaiter(t *testing.T, db *enginetest.TestDatabase) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := db.Pool.QueryRow(context.Background(), `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE datname = current_database()
				  AND wait_event_type = 'Lock'
				  AND query LIKE '%FROM engine.runs%'
				  AND query LIKE '%FOR UPDATE%'
			)
		`).Scan(&waiting); err != nil {
			t.Fatalf("check pg_stat_activity: %v", err)
		}
		if waiting {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for command transaction to block on run row lock")
}

func waitForFileLineCount(t *testing.T, path string, want int) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if fileLineCount(t, path) >= want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d activity attempts in %s", want, path)
}

func waitForFileExists(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) error = %v", path, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for file %s", path)
}

func fileLineCount(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return len(strings.FieldsFunc(string(data), func(r rune) bool { return r == '\n' }))
}

func darklaunchTestEnv(key, value string) string { return key + "=" + value }

func TestHelperProcessServe(_ *testing.T) {
	if os.Getenv("GO_WANT_ENGINE_SERVE_HELPER") != "1" {
		return
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"serve"})
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func filterEventTypes(history []inspectHistoryEvent) []string {
	result := make([]string, 0, len(history))
	for i := range history {
		event := history[i]
		switch event.EventType {
		case "custom_status.updated", "cancel.requested":
			continue
		default:
			result = append(result, event.EventType)
		}
	}
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertJSONErrorCode(t *testing.T, stdout string, code string) {
	t.Helper()

	var response jsonCommandError
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("Unmarshal(error stdout) error = %v", err)
	}
	if response.Error.Code != code {
		t.Fatalf("expected error code %q, got %+v", code, response)
	}
}

func jsonContainsKind(raw json.RawMessage, kind string) bool {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return payload["kind"] == kind
}
