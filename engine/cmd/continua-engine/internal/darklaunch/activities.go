package darklaunch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/continua-ai/continua/engine/internal/activity"
)

const (
	DemoActivityType   = "darklaunch.compose-greeting"
	RemoteActivityType = "darklaunch.remote-compose-greeting"
)

type ActivityInput struct {
	Name string `json:"name"`
}

type ActivityOutput struct {
	Greeting string `json:"greeting"`
}

func Handlers() map[string]activity.Handler {
	return map[string]activity.Handler{
		DemoActivityType: composeGreetingActivity,
	}
}

func composeGreetingActivity(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	var input ActivityInput
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode activity input: %w", err)
		}
	}
	if input.Name == "" {
		input.Name = "world"
	}
	if err := applyTestActivityHooks(ctx, input.Name); err != nil {
		return nil, err
	}

	output, err := json.Marshal(ActivityOutput{
		Greeting: "hello, " + input.Name,
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}
