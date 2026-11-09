package main

import (
	"encoding/json"
	"testing"

	"github.com/continua-ai/continua/engine/pkg/workflowtest"
)

func TestHelloWorkflowReturnsActivityResult(t *testing.T) {
	env := workflowtest.NewEnvironment()
	env.RegisterActivity(sayHelloActivityType, func(input json.RawMessage) (any, error) {
		return sayHello(t.Context(), input)
	})

	result, err := env.Execute(helloDefinition(), helloInput{Name: "Ada"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != workflowtest.StatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, workflowtest.StatusCompleted)
	}
	var output helloOutput
	if err := result.DecodeResult(&output); err != nil {
		t.Fatalf("DecodeResult() error = %v", err)
	}
	if output.Message != "hello, Ada" {
		t.Fatalf("message = %q, want %q", output.Message, "hello, Ada")
	}
}
