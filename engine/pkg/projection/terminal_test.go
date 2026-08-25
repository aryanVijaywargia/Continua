package projection

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func TestTerminalStatuses(t *testing.T) {
	testCases := []struct {
		name      string
		runStatus string
		wantTrace string
		wantSpan  string
	}{
		{name: "completed", runStatus: "completed", wantTrace: "completed", wantSpan: "completed"},
		{name: "continued_as_new", runStatus: "continued_as_new", wantTrace: "completed", wantSpan: "completed"},
		{name: "cancelled", runStatus: "cancelled", wantTrace: "cancelled", wantSpan: "failed"},
		{name: "terminated", runStatus: "terminated", wantTrace: "failed", wantSpan: "failed"},
		{name: "failed", runStatus: "failed", wantTrace: "failed", wantSpan: "failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotTrace, gotSpan := TerminalStatuses(tc.runStatus)
			if gotTrace != tc.wantTrace || gotSpan != tc.wantSpan {
				t.Fatalf("TerminalStatuses(%q) = (%q, %q), want (%q, %q)", tc.runStatus, gotTrace, gotSpan, tc.wantTrace, tc.wantSpan)
			}
		})
	}
}

func TestIsTerminalRunStatus(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		constant string
		status   enginedb.EngineRunLifecycleStatus
		want     bool
	}{
		{constant: "EngineRunLifecycleStatusQueued", status: enginedb.EngineRunLifecycleStatusQueued, want: false},
		{constant: "EngineRunLifecycleStatusRunning", status: enginedb.EngineRunLifecycleStatusRunning, want: false},
		{constant: "EngineRunLifecycleStatusCompleted", status: enginedb.EngineRunLifecycleStatusCompleted, want: true},
		{constant: "EngineRunLifecycleStatusFailed", status: enginedb.EngineRunLifecycleStatusFailed, want: true},
		{constant: "EngineRunLifecycleStatusCancelled", status: enginedb.EngineRunLifecycleStatusCancelled, want: true},
		{constant: "EngineRunLifecycleStatusWaiting", status: enginedb.EngineRunLifecycleStatusWaiting, want: false},
		{constant: "EngineRunLifecycleStatusTerminated", status: enginedb.EngineRunLifecycleStatusTerminated, want: true},
		{constant: "EngineRunLifecycleStatusSuspended", status: enginedb.EngineRunLifecycleStatusSuspended, want: false},
		{constant: "EngineRunLifecycleStatusQuarantined", status: enginedb.EngineRunLifecycleStatusQuarantined, want: false},
		{constant: "EngineRunLifecycleStatusContinuedAsNew", status: enginedb.EngineRunLifecycleStatusContinuedAsNew, want: true},
	}

	covered := make(map[string]bool, len(testCases))
	for _, tc := range testCases {
		covered[tc.constant] = true
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := IsTerminalRunStatus(tc.status); got != tc.want {
				t.Fatalf("IsTerminalRunStatus(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}

	generated, err := os.ReadFile(filepath.Join("..", "..", "db", "gen", "go", "models.go"))
	if err != nil {
		t.Fatalf("read generated enum models: %v", err)
	}
	constantRe := regexp.MustCompile(`EngineRunLifecycleStatus([A-Za-z0-9_]+)\s+EngineRunLifecycleStatus\s*=`)
	extracted := map[string]bool{}
	for _, match := range constantRe.FindAllStringSubmatch(string(generated), -1) {
		extracted["EngineRunLifecycleStatus"+match[1]] = true
	}
	declared := declaredRunStatusConstants(t, generated)
	if len(extracted) != len(declared) {
		t.Fatalf("constant extraction mismatch in generated models: regexp captured %d distinct constants, parser found %d; some constants escape the extraction and would silently default to non-terminal, so fix the extraction before trusting this table", len(extracted), len(declared))
	}
	for _, constant := range declared {
		if !covered[constant] {
			t.Errorf("generated enum constant %s is missing from this table; add it with an explicit expectation so a new status cannot silently default", constant)
		}
	}
}

func declaredRunStatusConstants(t *testing.T, source []byte) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "models.go", source, 0)
	if err != nil {
		t.Fatalf("parse generated enum models: %v", err)
	}
	var names []string
	inheritedType := ""
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		inheritedType = ""
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if id, ok := valueSpec.Type.(*ast.Ident); ok {
				inheritedType = id.Name
			}
			if inheritedType == "EngineRunLifecycleStatus" {
				for _, name := range valueSpec.Names {
					names = append(names, name.Name)
				}
			}
		}
	}
	sort.Strings(names)
	return names
}

func TestTerminalOutputPayload(t *testing.T) {
	t.Run("completed returns result", func(t *testing.T) {
		result := json.RawMessage(`{"ok":true}`)
		got, err := TerminalOutputPayload("completed", result, nil, nil)
		if err != nil {
			t.Fatalf("TerminalOutputPayload() error = %v", err)
		}
		if string(got) != string(result) {
			t.Fatalf("TerminalOutputPayload() = %s, want %s", got, result)
		}
	})

	t.Run("terminal failure returns structured payload", func(t *testing.T) {
		errorCode := "terminated"
		errorMessage := "run terminated by operator"
		got, err := TerminalOutputPayload("terminated", nil, &errorCode, &errorMessage)
		if err != nil {
			t.Fatalf("TerminalOutputPayload() error = %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload["error_code"] != errorCode || payload["error_message"] != errorMessage || payload["status"] != "terminated" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	})

	t.Run("continued_as_new keeps terminal output empty", func(t *testing.T) {
		got, err := TerminalOutputPayload("continued_as_new", nil, nil, nil)
		if err != nil {
			t.Fatalf("TerminalOutputPayload() error = %v", err)
		}
		if got != nil {
			t.Fatalf("TerminalOutputPayload() = %s, want nil", got)
		}
	})
}
