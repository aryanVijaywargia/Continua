package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

const (
	greeterWorkflowName    = "examples.greeter"
	greeterWorkflowVersion = "v1"
	greetActivityType      = "examples.greet"
)

type greetInput struct {
	Name string `json:"name"`
}

type greetOutput struct {
	Greeting string `json:"greeting"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Workflows: []workflow.Definition{
			{
				Name:    greeterWorkflowName,
				Version: greeterWorkflowVersion,
				Run:     runGreeterWorkflow,
			},
		},
		Activities: map[string]engineruntime.ActivityHandler{
			greetActivityType: greetActivity,
		},
	})
	if err != nil {
		return err
	}

	// Run the engine worker loop. To create runs, use the continua-engine CLI
	// or the preview engine control plane.
	return rt.Run(ctx)
}

func runGreeterWorkflow(ctx workflow.Context) error {
	var input greetInput
	if err := ctx.Input(&input); err != nil {
		return err
	}

	var output greetOutput
	if err := ctx.Activity("greet", greetActivityType, input, &output); err != nil {
		return err
	}

	return ctx.SetResult(output)
}

func greetActivity(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var input greetInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
	}
	if input.Name == "" {
		input.Name = "world"
	}

	return json.Marshal(greetOutput{
		Greeting: "hello, " + input.Name,
	})
}
