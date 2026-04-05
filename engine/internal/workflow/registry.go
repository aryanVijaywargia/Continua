package workflow

import (
	"fmt"

	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

type definitionKey struct {
	name    string
	version string
}

type Registry struct {
	definitions map[definitionKey]publicworkflow.Definition
}

func NewRegistry(definitions ...publicworkflow.Definition) (*Registry, error) {
	registry := &Registry{
		definitions: make(map[definitionKey]publicworkflow.Definition, len(definitions)),
	}

	for _, definition := range definitions {
		if definition.Name == "" || definition.Version == "" || definition.Run == nil {
			return nil, fmt.Errorf("workflow definition must include name, version, and run function")
		}

		key := definitionKey{name: definition.Name, version: definition.Version}
		if _, exists := registry.definitions[key]; exists {
			return nil, fmt.Errorf("duplicate workflow definition %s@%s", definition.Name, definition.Version)
		}
		registry.definitions[key] = definition
	}

	return registry, nil
}

func (r *Registry) Get(name, version string) (publicworkflow.Definition, bool) {
	if r == nil {
		return publicworkflow.Definition{}, false
	}

	definition, ok := r.definitions[definitionKey{name: name, version: version}]
	return definition, ok
}

func (r *Registry) List() []publicworkflow.Definition {
	if r == nil {
		return nil
	}

	definitions := make([]publicworkflow.Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		definitions = append(definitions, definition)
	}

	return definitions
}
