package jsonraw

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCloneHasSingleOwner guards ownership of the RawMessage copy helper so a
// local clone cannot quietly reappear in either module and drift away from
// jsonraw.Clone, which is the only place allowed to define it.
//
// Detection is signature-first: outside package jsonraw, any function of
// signature func(json.RawMessage) json.RawMessage is a finding, because that is
// exactly what a re-drifted copy looks like no matter how its body is spelled.
//
// One evasion is deliberately accepted: naming the file *_gen.go skips the
// scan, because nobody re-drifts a helper by disguising it as generated output.
func TestCloneHasSingleOwner(t *testing.T) {
	t.Parallel()

	// Positive control, independent of the walk: parse a known-bad snippet with
	// the same parser and require the same predicate to flag it. If this fails,
	// the predicate is dead and the walk below proves nothing.
	controlViolations := scanGoSource("positive_control.go", controlSource)
	foundControl := false
	for _, violation := range controlViolations {
		if strings.Contains(violation, "duplicateRawPayload") {
			foundControl = true
		}
	}
	if !foundControl {
		t.Fatalf("positive control failed: known-bad duplicateRawPayload was not flagged; got %d violations from control source", len(controlViolations))
	}

	root := filepath.Join("..", "..", "..")
	guardFileRel := filepath.ToSlash(filepath.Join("engine", "pkg", "jsonraw", "clone_guard_test.go"))
	var violations []string
	parsed := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "testdata":
				return filepath.SkipDir
			}
			if strings.Contains(filepath.ToSlash(path), "db/gen/") {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_gen.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		switch filepath.ToSlash(rel) {
		case guardFileRel, "engine/pkg/jsonraw/clone.go":
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		parsed++
		for _, violation := range scanGoSource(rel, source) {
			violations = append(violations, fmt.Sprintf("%s:%s", rel, violation))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan sources under %s: %v", root, err)
	}

	// Floor: a wrong relative path or a broken filter must fail loudly instead
	// of walking nothing and reporting a green ok. The tree holds ~260 parsed
	// Go files today; 150 leaves generous headroom without passing on a stub.
	const minParsedGoFiles = 150
	if parsed < minParsedGoFiles {
		t.Fatalf("scanned only %d Go files under %s, want at least %d; the walk found no sources, so this run guarded nothing", parsed, root, minParsedGoFiles)
	}

	for _, violation := range violations {
		t.Errorf("%s", violation)
	}
}

const controlSource = `package control

import "encoding/json"

func duplicateRawPayload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
`

// scanGoSource parses one Go source and returns one violation string per
// function that either is named cloneRaw or has the raw-to-raw signature. The
// violation text starts with the function name.
func scanGoSource(filename string, source interface{}) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, source, 0)
	if err != nil {
		return []string{fmt.Sprintf("%s does not parse: %v", filename, err)}
	}
	var violations []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		switch {
		case fn.Name.Name == "cloneRaw":
			violations = append(violations, fmt.Sprintf(
				"%s redefines the shared clone helper; call jsonraw.Clone instead",
				fn.Name.Name,
			))
		case file.Name.Name != "jsonraw" && isRawToRawSignature(fn):
			violations = append(violations, fmt.Sprintf(
				"%s takes and returns json.RawMessage, which is the shape of a re-drifted clone; call jsonraw.Clone instead",
				fn.Name.Name,
			))
		}
	}
	return violations
}

func isRawToRawSignature(fn *ast.FuncDecl) bool {
	params := fn.Type.Params
	results := fn.Type.Results
	if params == nil || len(params.List) != 1 || len(params.List[0].Names) > 1 {
		return false
	}
	if results == nil || len(results.List) != 1 || len(results.List[0].Names) != 0 {
		return false
	}
	return isRawMessageSelector(params.List[0].Type) && isRawMessageSelector(results.List[0].Type)
}

func isRawMessageSelector(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "RawMessage"
}
