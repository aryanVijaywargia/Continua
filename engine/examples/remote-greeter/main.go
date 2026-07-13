package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

const (
	remoteGreeterWorkflowName    = "examples.remote-greeter"
	remoteGreeterWorkflowVersion = "v1"
	composeGreetingActivityType  = "examples.compose-greeting"
)

func remoteGreeterDefinition() workflow.Definition {
	return workflow.Definition{
		Name:    remoteGreeterWorkflowName,
		Version: remoteGreeterWorkflowVersion,
		Run: func(workflow.Context) error {
			return fmt.Errorf("examples.remote-greeter: workflow not implemented yet")
		},
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Workflows:   []workflow.Definition{remoteGreeterDefinition()},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := rt.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
