package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

const (
	remoteGreeterWorkflowName    = "examples.remote-greeter"
	remoteGreeterWorkflowVersion = "v1"
	composeGreetingActivityType  = "examples.compose-greeting"
)

type remoteGreeterInput struct {
	Name string `json:"name"`
}

type remoteGreeterOutput struct {
	Greeting string `json:"greeting"`
}

func remoteGreeterDefinition() workflow.Definition {
	return workflow.Definition{
		Name:    remoteGreeterWorkflowName,
		Version: remoteGreeterWorkflowVersion,
		Run:     runRemoteGreeterWorkflow,
	}
}

func runRemoteGreeterWorkflow(ctx workflow.Context) error {
	var input remoteGreeterInput
	if err := ctx.Input(&input); err != nil {
		return err
	}
	if input.Name == "" {
		input.Name = "world"
	}

	var output remoteGreeterOutput
	if err := ctx.ActivityWithOptions(
		"compose-greeting",
		composeGreetingActivityType,
		input,
		&output,
		workflow.ActivityOptions{
			ExecutionTarget: workflow.ActivityExecutionTargetRemote,
			RetryPolicy: &workflow.RetryPolicy{
				MaxAttempts:       3,
				InitialBackoff:    500 * time.Millisecond,
				MaxBackoff:        5 * time.Second,
				BackoffMultiplier: 2.0,
			},
		},
	); err != nil {
		return err
	}

	return ctx.SetResult(output)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	options := engineruntime.Options{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Workflows:   []workflow.Definition{remoteGreeterDefinition()},
	}
	if value := os.Getenv("ENGINE_PROJECT_ID"); value != "" {
		projectID, err := uuid.Parse(value)
		if err != nil {
			return fmt.Errorf("parse ENGINE_PROJECT_ID: %w", err)
		}
		options.ProjectID = &projectID
	}

	rt, err := engineruntime.New(options)
	if err != nil {
		return err
	}

	return rt.Run(ctx)
}
