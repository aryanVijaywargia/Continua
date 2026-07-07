package workflow

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

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

// Latest returns the highest registered version for name. Version comparison is
// numeric-aware: if both versions end in digits, the trailing integers are
// compared first so v10 sorts after v2; ties and non-numeric versions fall back
// to lexicographic ordering.
func (r *Registry) Latest(name string) (publicworkflow.Definition, bool) {
	if r == nil {
		return publicworkflow.Definition{}, false
	}

	var latest publicworkflow.Definition
	found := false
	for key, definition := range r.definitions {
		if key.name != name {
			continue
		}
		if !found || compareDefinitionVersions(definition.Version, latest.Version) > 0 {
			latest = definition
			found = true
		}
	}
	return latest, found
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

func compareDefinitionVersions(left, right string) int {
	leftNumber, leftOK := trailingVersionNumber(left)
	rightNumber, rightOK := trailingVersionNumber(right)
	if leftOK && rightOK && leftNumber != rightNumber {
		if leftNumber > rightNumber {
			return 1
		}
		return -1
	}

	return strings.Compare(left, right)
}

func trailingVersionNumber(version string) (int64, bool) {
	end := len(version)
	start := end
	for start > 0 {
		r, size := utf8LastRuneInString(version[:start])
		if !unicode.IsDigit(r) {
			break
		}
		start -= size
	}
	if start == end {
		return 0, false
	}

	number, err := strconv.ParseInt(version[start:end], 10, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func utf8LastRuneInString(value string) (rune, int) {
	return utf8.DecodeLastRuneInString(value)
}
