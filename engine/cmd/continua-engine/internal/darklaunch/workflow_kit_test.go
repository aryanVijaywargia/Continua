package darklaunch_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/continua-ai/continua/engine/cmd/continua-engine/internal/darklaunch"
	"github.com/continua-ai/continua/engine/pkg/workflow"
	"github.com/continua-ai/continua/engine/pkg/workflowtest"
)

func TestSleepDemoWorkflowKit(t *testing.T) {
	env := workflowtest.NewEnvironment()
	if err := env.QueueSignal("approval", darklaunch.SignalPayload{Approval: "granted"}); err != nil {
		t.Fatalf("QueueSignal returned error: %v", err)
	}
	def := lookupDefinition(t, darklaunch.SleepDemoDefinitionName, darklaunch.SleepDemoDefinitionVersion)

	start := time.Now()
	result, err := env.Execute(def, darklaunch.SleepDemoInput{Name: "Ada", SleepMS: 60000})
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
	var decoded darklaunch.WorkflowResult
	if err := result.DecodeResult(&decoded); err != nil {
		t.Fatalf("DecodeResult returned error: %v", err)
	}
	want := darklaunch.WorkflowResult{Greeting: "hello, Ada", Approval: "granted"}
	if decoded != want {
		t.Fatalf("result = %#v, want %#v", decoded, want)
	}
}

func TestDemoWorkflowKit(t *testing.T) {
	env := workflowtest.NewEnvironment()
	env.RegisterActivity(darklaunch.DemoActivityType, func(input json.RawMessage) (any, error) {
		var decoded darklaunch.ActivityInput
		if err := json.Unmarshal(input, &decoded); err != nil {
			return nil, err
		}
		return darklaunch.ActivityOutput{Greeting: "hello, " + decoded.Name}, nil
	})
	if err := env.QueueSignal("approval", darklaunch.SignalPayload{Approval: "granted"}); err != nil {
		t.Fatalf("QueueSignal returned error: %v", err)
	}

	result, err := env.Execute(
		lookupDefinition(t, darklaunch.DemoDefinitionName, darklaunch.DemoDefinitionVersion),
		darklaunch.WorkflowInput{Name: "Ada"},
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	var decoded darklaunch.WorkflowResult
	if err := result.DecodeResult(&decoded); err != nil {
		t.Fatalf("DecodeResult returned error: %v", err)
	}
	want := darklaunch.WorkflowResult{Greeting: "hello, Ada", Approval: "granted"}
	if decoded != want {
		t.Fatalf("result = %#v, want %#v", decoded, want)
	}
	var status map[string]string
	if err := json.Unmarshal(result.CustomStatus, &status); err != nil {
		t.Fatalf("CustomStatus unmarshal returned error: %v", err)
	}
	if status["phase"] != "completed" {
		t.Fatalf("custom status phase = %q, want %q", status["phase"], "completed")
	}
	if status["approval"] != "granted" {
		t.Fatalf("custom status approval = %q, want %q", status["approval"], "granted")
	}
}

func TestDemoWorkflowCancellationKit(t *testing.T) {
	t.Run("demo cancels", func(t *testing.T) {
		env := workflowtest.NewEnvironment()
		env.RequestCancellation()
		result, err := env.Execute(
			lookupDefinition(t, darklaunch.DemoDefinitionName, darklaunch.DemoDefinitionVersion),
			darklaunch.WorkflowInput{Name: "Ada"},
		)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusCancelled {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCancelled)
		}
	})

	t.Run("cancel-completes completes", func(t *testing.T) {
		env := workflowtest.NewEnvironment()
		env.RequestCancellation()
		result, err := env.Execute(
			lookupDefinition(t, darklaunch.CancelCompletesDefinitionName, darklaunch.CancelCompletesDefinitionVersion),
			darklaunch.WorkflowInput{Name: "Ada"},
		)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Status != workflowtest.StatusCompleted {
			t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
		}
	})
}

func TestRetryHandledDemoKit(t *testing.T) {
	env := workflowtest.NewEnvironment()
	env.RegisterActivity(darklaunch.DemoActivityType, func(input json.RawMessage) (any, error) {
		return nil, errors.New("boom")
	})

	result, err := env.Execute(
		lookupDefinition(t, darklaunch.RetryHandledDefinitionName, darklaunch.RetryHandledDefinitionVersion),
		darklaunch.WorkflowInput{Name: "Ada"},
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	var decoded map[string]any
	if err := result.DecodeResult(&decoded); err != nil {
		t.Fatalf("DecodeResult returned error: %v", err)
	}
	if got, want := decoded["handled"], true; !reflect.DeepEqual(got, want) {
		t.Fatalf("handled = %#v, want %#v", got, want)
	}
}

func lookupDefinition(t *testing.T, name, version string) workflow.Definition {
	t.Helper()
	for _, def := range darklaunch.Definitions() {
		if def.Name == name && def.Version == version {
			return def
		}
	}
	t.Fatalf("definition %s@%s not found", name, version)
	return workflow.Definition{}
}
