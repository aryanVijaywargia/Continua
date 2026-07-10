package workflowtest_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/continua-ai/continua/engine/pkg/history"
	"github.com/continua-ai/continua/engine/pkg/workflow"
	"github.com/continua-ai/continua/engine/pkg/workflowtest"
)

func TestExecuteCompletesSimpleWorkflow(t *testing.T) {
	def := workflow.Definition{
		Name:    "simple",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			var input map[string]string
			if err := ctx.Input(&input); err != nil {
				return err
			}
			if input["name"] != "Ada" {
				return errors.New("unexpected workflow input")
			}
			return ctx.SetResult(map[string]string{"greeting": "hello, Ada"})
		},
	}

	result, err := workflowtest.NewEnvironment().Execute(def, map[string]string{"name": "Ada"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	var decoded map[string]string
	if err := result.DecodeResult(&decoded); err != nil {
		t.Fatalf("DecodeResult returned error: %v", err)
	}
	if want := map[string]string{"greeting": "hello, Ada"}; !reflect.DeepEqual(decoded, want) {
		t.Fatalf("result = %#v, want %#v", decoded, want)
	}
	events := result.HistoryEventTypes()
	if len(events) == 0 {
		t.Fatal("HistoryEventTypes returned no events")
	}
	if events[0] != history.EventWorkflowStarted {
		t.Fatalf("first event = %q, want %q", events[0], history.EventWorkflowStarted)
	}
	if got, want := events[len(events)-1], history.EventWorkflowCompleted; got != want {
		t.Fatalf("last event = %q, want %q", got, want)
	}
}

func TestExecuteRunsScriptedActivity(t *testing.T) {
	env := workflowtest.NewEnvironment()
	env.RegisterActivity("demo.greet", func(input json.RawMessage) (any, error) {
		var decoded map[string]string
		if err := json.Unmarshal(input, &decoded); err != nil {
			return nil, err
		}
		if want := map[string]string{"name": "Ada"}; !reflect.DeepEqual(decoded, want) {
			return nil, errors.New("unexpected activity input")
		}
		return map[string]string{"greeting": "hello, Ada"}, nil
	})
	def := workflow.Definition{
		Name:    "activity",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			var out map[string]string
			if err := ctx.Activity("greet", "demo.greet", map[string]string{"name": "Ada"}, &out); err != nil {
				return err
			}
			return ctx.SetResult(out)
		},
	}

	result, err := env.Execute(def, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	var decoded map[string]string
	if err := result.DecodeResult(&decoded); err != nil {
		t.Fatalf("DecodeResult returned error: %v", err)
	}
	if want := map[string]string{"greeting": "hello, Ada"}; !reflect.DeepEqual(decoded, want) {
		t.Fatalf("result = %#v, want %#v", decoded, want)
	}
	assertEventOrder(t, result.HistoryEventTypes(), history.EventActivityScheduled, history.EventActivityCompleted, history.EventWorkflowCompleted)
}

func TestExecuteActivityFailureCodes(t *testing.T) {
	tests := []struct {
		name      string
		handler   workflowtest.ActivityHandler
		errorCode string
		message   string
	}{
		{
			name: "coded error",
			handler: func(input json.RawMessage) (any, error) {
				return nil, workflowtest.NewActivityError("quota_exceeded", "boom")
			},
			errorCode: "quota_exceeded",
			message:   "boom",
		},
		{
			name: "plain error",
			handler: func(input json.RawMessage) (any, error) {
				return nil, errors.New("boom")
			},
			errorCode: "activity_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := workflowtest.NewEnvironment()
			env.RegisterActivity("demo.fail", tt.handler)
			def := workflow.Definition{
				Name:    "activity-fails",
				Version: "v1",
				Run: func(ctx workflow.Context) error {
					return ctx.Activity("fail", "demo.fail", nil, nil)
				},
			}

			result, err := env.Execute(def, nil)
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if result.Status != workflowtest.StatusFailed {
				t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusFailed)
			}
			if result.ErrorCode != tt.errorCode {
				t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, tt.errorCode)
			}
			if tt.message != "" && result.ErrorMessage != tt.message {
				t.Fatalf("ErrorMessage = %q, want %q", result.ErrorMessage, tt.message)
			}
			events := result.HistoryEventTypes()
			assertContainsEvent(t, events, history.EventActivityFailed)
			if got, want := events[len(events)-1], history.EventWorkflowFailed; got != want {
				t.Fatalf("last event = %q, want %q", got, want)
			}
		})
	}
}

func TestExecuteBlocksOnUnregisteredActivity(t *testing.T) {
	def := workflow.Definition{
		Name:    "missing-activity",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			return ctx.Activity("greet", "demo.greet", nil, nil)
		},
	}

	result, err := workflowtest.NewEnvironment().Execute(def, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusBlocked)
	}
	if result.WaitKind != history.WaitKindActivity {
		t.Fatalf("WaitKind = %q, want %q", result.WaitKind, history.WaitKindActivity)
	}
	if result.WaitKey != "greet" {
		t.Fatalf("WaitKey = %q, want %q", result.WaitKey, "greet")
	}
	assertContainsEvent(t, result.HistoryEventTypes(), history.EventActivityScheduled)
}

func TestExecuteAutoFiresTimers(t *testing.T) {
	def := workflow.Definition{
		Name:    "timer",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			if err := ctx.Sleep("wait", time.Hour); err != nil {
				return err
			}
			return ctx.SetResult("done")
		},
	}

	start := time.Now()
	result, err := workflowtest.NewEnvironment().Execute(def, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	if elapsed >= 10*time.Second {
		t.Fatalf("Execute elapsed = %s, want less than 10s", elapsed)
	}
	assertEventOrder(t, result.HistoryEventTypes(), history.EventTimerScheduled, history.EventTimerFired)
}

func TestExecuteDeliversQueuedSignals(t *testing.T) {
	def := workflow.Definition{
		Name:    "signal",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			var sig map[string]string
			if err := ctx.ReceiveSignal("approval", &sig); err != nil {
				return err
			}
			return ctx.SetResult(sig)
		},
	}

	t.Run("queued signal delivered", func(t *testing.T) {
		env := workflowtest.NewEnvironment()
		if err := env.QueueSignal("approval", map[string]string{"approval": "granted"}); err != nil {
			t.Fatalf("QueueSignal returned error: %v", err)
		}
		result, err := env.Execute(def, nil)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusCompleted {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
		}
		var decoded map[string]string
		if err := result.DecodeResult(&decoded); err != nil {
			t.Fatalf("DecodeResult returned error: %v", err)
		}
		if want := map[string]string{"approval": "granted"}; !reflect.DeepEqual(decoded, want) {
			t.Fatalf("result = %#v, want %#v", decoded, want)
		}
		assertContainsEvent(t, result.HistoryEventTypes(), history.EventSignalReceived)
	})

	t.Run("missing signal blocks", func(t *testing.T) {
		result, err := workflowtest.NewEnvironment().Execute(def, nil)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusBlocked {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusBlocked)
		}
		if result.WaitKind != history.WaitKindSignal {
			t.Fatalf("WaitKind = %q, want %q", result.WaitKind, history.WaitKindSignal)
		}
		if result.WaitKey != "approval" {
			t.Fatalf("WaitKey = %q, want %q", result.WaitKey, "approval")
		}
	})
}

func TestExecuteCancellation(t *testing.T) {
	t.Run("cancel produces cancelled", func(t *testing.T) {
		env := workflowtest.NewEnvironment()
		env.RequestCancellation()
		def := workflow.Definition{
			Name:    "cancel",
			Version: "v1",
			Run: func(ctx workflow.Context) error {
				if ctx.CancellationRequested() {
					return workflow.ErrCancelled
				}
				return errors.New("expected cancel")
			},
		}

		result, err := env.Execute(def, nil)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusCancelled {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCancelled)
		}
		if result.ErrorCode != "cancelled" {
			t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, "cancelled")
		}
		events := result.HistoryEventTypes()
		assertContainsEvent(t, events, history.EventCancelRequested)
		if got, want := events[len(events)-1], history.EventWorkflowCancelled; got != want {
			t.Fatalf("last event = %q, want %q", got, want)
		}
	})

	t.Run("observed cancel with nil return completes", func(t *testing.T) {
		env := workflowtest.NewEnvironment()
		env.RequestCancellation()
		def := workflow.Definition{
			Name:    "graceful-cancel",
			Version: "v1",
			Run: func(ctx workflow.Context) error {
				if ctx.CancellationRequested() {
					return ctx.SetResult("graceful")
				}
				return errors.New("expected cancel")
			},
		}

		result, err := env.Execute(def, nil)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusCompleted {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
		}
	})
}

func TestExecuteChildWorkflowOutcomes(t *testing.T) {
	t.Run("child result flows to parent", func(t *testing.T) {
		env := workflowtest.NewEnvironment()
		env.RegisterDefinition(workflow.Definition{
			Name:    "child",
			Version: "v1",
			Run: func(ctx workflow.Context) error {
				var input int
				if err := ctx.Input(&input); err != nil {
					return err
				}
				return ctx.SetResult(input * 2)
			},
		})
		parent := workflow.Definition{
			Name:    "parent",
			Version: "v1",
			Run: func(ctx workflow.Context) error {
				var out int
				if err := ctx.ChildWorkflow("c1", "child", "v1", 21, &out); err != nil {
					return err
				}
				return ctx.SetResult(out)
			},
		}

		result, err := env.Execute(parent, nil)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusCompleted {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
		}
		var decoded int
		if err := result.DecodeResult(&decoded); err != nil {
			t.Fatalf("DecodeResult returned error: %v", err)
		}
		if decoded != 42 {
			t.Fatalf("result = %d, want 42", decoded)
		}
		assertEventOrder(t, result.HistoryEventTypes(), history.EventChildWorkflowScheduled, history.EventChildWorkflowStarted, history.EventChildWorkflowCompleted)
	})

	t.Run("child failure surfaces as ChildWorkflowError", func(t *testing.T) {
		env := workflowtest.NewEnvironment()
		env.RegisterDefinition(workflow.Definition{
			Name:    "child",
			Version: "v1",
			Run: func(ctx workflow.Context) error {
				return errors.New("child boom")
			},
		})
		parent := workflow.Definition{
			Name:    "parent",
			Version: "v1",
			Run: func(ctx workflow.Context) error {
				err := ctx.ChildWorkflow("c1", "child", "v1", nil, nil)
				if err == nil {
					return errors.New("expected child workflow error")
				}
				var cwe *workflow.ChildWorkflowError
				if !errors.As(err, &cwe) {
					return err
				}
				return ctx.SetResult(map[string]string{"code": cwe.Code(), "state": cwe.TerminalState()})
			},
		}

		result, err := env.Execute(parent, nil)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusCompleted {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
		}
		var decoded map[string]string
		if err := result.DecodeResult(&decoded); err != nil {
			t.Fatalf("DecodeResult returned error: %v", err)
		}
		if decoded["code"] != "workflow_failed" {
			t.Fatalf("child error code = %q, want %q", decoded["code"], "workflow_failed")
		}
		if decoded["state"] != "failed" {
			t.Fatalf("child terminal state = %q, want %q", decoded["state"], "failed")
		}
	})

	t.Run("unregistered child blocks", func(t *testing.T) {
		parent := workflow.Definition{
			Name:    "parent",
			Version: "v1",
			Run: func(ctx workflow.Context) error {
				return ctx.ChildWorkflow("c1", "nope", "v1", nil, nil)
			},
		}

		result, err := workflowtest.NewEnvironment().Execute(parent, nil)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusBlocked {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusBlocked)
		}
		if result.WaitKind != history.WaitKindChildWorkflow {
			t.Fatalf("WaitKind = %q, want %q", result.WaitKind, history.WaitKindChildWorkflow)
		}
		if result.WaitKey != "c1" {
			t.Fatalf("WaitKey = %q, want %q", result.WaitKey, "c1")
		}
	})
}

func TestExecuteContinueAsNew(t *testing.T) {
	def := workflow.Definition{
		Name:    "continue",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			return workflow.ContinueAsNew(map[string]int{"round": 2})
		},
	}

	result, err := workflowtest.NewEnvironment().Execute(def, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusContinuedAsNew {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusContinuedAsNew)
	}
	var decoded map[string]int
	if err := json.Unmarshal(result.ContinuationInput, &decoded); err != nil {
		t.Fatalf("ContinuationInput unmarshal returned error: %v", err)
	}
	if want := map[string]int{"round": 2}; !reflect.DeepEqual(decoded, want) {
		t.Fatalf("ContinuationInput = %#v, want %#v", decoded, want)
	}
}

func TestExecuteSideEffectRunsOnce(t *testing.T) {
	var calls int
	def := workflow.Definition{
		Name:    "side-effect",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			var out string
			if err := ctx.SideEffect("gen", func() (any, error) {
				calls++
				return "value-1", nil
			}, &out); err != nil {
				return err
			}
			if err := ctx.Sleep("wait", time.Minute); err != nil {
				return err
			}
			return ctx.SetResult(out)
		},
	}

	result, err := workflowtest.NewEnvironment().Execute(def, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	var decoded string
	if err := result.DecodeResult(&decoded); err != nil {
		t.Fatalf("DecodeResult returned error: %v", err)
	}
	if decoded != "value-1" {
		t.Fatalf("result = %q, want %q", decoded, "value-1")
	}
	if calls != 1 {
		t.Fatalf("side effect calls = %d, want 1", calls)
	}
}

func TestExecuteNowStableAcrossActivations(t *testing.T) {
	var seen []time.Time
	def := workflow.Definition{
		Name:    "now",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			t := ctx.Now()
			seen = append(seen, t)
			if err := ctx.Sleep("wait", time.Minute); err != nil {
				return err
			}
			return ctx.SetResult(t)
		},
	}

	result, err := workflowtest.NewEnvironment().Execute(def, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	if len(seen) < 2 {
		t.Fatalf("Now observed %d activation(s), want at least 2", len(seen))
	}
	for i, got := range seen {
		if !got.Equal(seen[0]) {
			t.Fatalf("seen[%d] = %s, want %s", i, got, seen[0])
		}
	}
}

func TestExecuteDetectsNonDeterminism(t *testing.T) {
	var attempts int
	env := workflowtest.NewEnvironment()
	env.RegisterActivity("type.a", func(input json.RawMessage) (any, error) { return nil, nil })
	env.RegisterActivity("type.b", func(input json.RawMessage) (any, error) { return nil, nil })
	def := workflow.Definition{
		Name:    "nondeterministic",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			attempts++
			if attempts == 1 {
				if err := ctx.Activity("a", "type.a", nil, nil); err != nil {
					return err
				}
				return ctx.SetResult("a")
			}
			if err := ctx.Activity("b", "type.b", nil, nil); err != nil {
				return err
			}
			return ctx.SetResult("b")
		},
	}

	result, err := env.Execute(def, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusQuarantined {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusQuarantined)
	}
	if result.ErrorCode != "replay_mismatch" {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, "replay_mismatch")
	}
}

func TestExecuteCustomStatus(t *testing.T) {
	def := workflow.Definition{
		Name:    "custom-status",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			if err := ctx.SetCustomStatus(map[string]string{"phase": "done"}); err != nil {
				return err
			}
			return ctx.SetResult(nil)
		},
	}

	result, err := workflowtest.NewEnvironment().Execute(def, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	var decoded map[string]string
	if err := json.Unmarshal(result.CustomStatus, &decoded); err != nil {
		t.Fatalf("CustomStatus unmarshal returned error: %v", err)
	}
	if want := map[string]string{"phase": "done"}; !reflect.DeepEqual(decoded, want) {
		t.Fatalf("CustomStatus = %#v, want %#v", decoded, want)
	}
}

func assertContainsEvent(t *testing.T, events []string, want string) {
	t.Helper()
	for _, eventType := range events {
		if eventType == want {
			return
		}
	}
	t.Fatalf("events %v do not contain %q", events, want)
}

func assertEventOrder(t *testing.T, events []string, ordered ...string) {
	t.Helper()
	next := 0
	for _, eventType := range events {
		if next < len(ordered) && eventType == ordered[next] {
			next++
		}
	}
	if next != len(ordered) {
		t.Fatalf("events %v do not contain relative order %v", events, ordered)
	}
}
