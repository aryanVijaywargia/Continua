package activity

import (
	"context"
	"encoding/json"
	"fmt"
)

type Handler func(context.Context, json.RawMessage) (json.RawMessage, error)

type Registry struct {
	handlers map[string]Handler
}

func NewRegistry(handlers map[string]Handler) (*Registry, error) {
	registry := &Registry{
		handlers: make(map[string]Handler, len(handlers)),
	}

	for activityType, handler := range handlers {
		if activityType == "" || handler == nil {
			return nil, fmt.Errorf("activity registry entries require type and handler")
		}
		if _, exists := registry.handlers[activityType]; exists {
			return nil, fmt.Errorf("duplicate activity handler for %q", activityType)
		}
		registry.handlers[activityType] = handler
	}

	return registry, nil
}

func (r *Registry) Get(activityType string) (Handler, bool) {
	if r == nil {
		return nil, false
	}
	handler, ok := r.handlers[activityType]
	return handler, ok
}
