package projection

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestTerminalRunStatusHasSingleOwner guards ownership of run-terminality so a
// local predicate cannot quietly reappear and drift away from
// projection.IsTerminalRunStatus, which is the only place allowed to decide it.
func TestTerminalRunStatusHasSingleOwner(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "..")
	guardFileRel := filepath.ToSlash(filepath.Join("engine", "pkg", "projection", "terminal_guard_test.go"))
	var violations []string

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
		if filepath.ToSlash(rel) == guardFileRel {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, rel, source, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if file.Name.Name == "projection" && fn.Name.Name == "IsTerminalRunStatus" {
				continue
			}
			if constants := terminalDecisionsIn(fn.Body); len(constants) >= 2 {
				names := make([]string, 0, len(constants))
				for constant := range constants {
					names = append(names, constant)
				}
				sort.Strings(names)
				pos := fset.Position(fn.Pos())
				violations = append(violations, fmt.Sprintf(
					"%s:%d %s decides run terminality from %s; call projection.IsTerminalRunStatus instead of keeping a local predicate",
					rel, pos.Line, fn.Name.Name, strings.Join(names, ", "),
				))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan sources under %s: %v", root, err)
	}

	for _, violation := range violations {
		t.Errorf("%s", violation)
	}
}

var lifecycleConstantRe = regexp.MustCompile(`^EngineRunLifecycleStatus[A-Z]`)

func lifecycleConstant(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || !lifecycleConstantRe.MatchString(sel.Sel.Name) {
		return ""
	}
	return sel.Sel.Name
}

func orChainLeaves(expr ast.Expr) []ast.Expr {
	if bin, ok := expr.(*ast.BinaryExpr); ok && bin.Op == token.LOR {
		return append(orChainLeaves(bin.X), orChainLeaves(bin.Y)...)
	}
	return []ast.Expr{expr}
}

func comparisonConstants(expr ast.Expr, into map[string]bool) {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
		return
	}
	for _, side := range []ast.Expr{bin.X, bin.Y} {
		if constant := lifecycleConstant(side); constant != "" {
			into[constant] = true
		}
	}
}

func returnsBoolLiteral(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		if id, ok := ret.Results[0].(*ast.Ident); ok && (id.Name == "true" || id.Name == "false") {
			return true
		}
	}
	return false
}

func terminalDecisionsIn(body ast.Stmt) map[string]bool {
	constants := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CaseClause:
			clause := map[string]bool{}
			for _, item := range n.List {
				if constant := lifecycleConstant(item); constant != "" {
					clause[constant] = true
				}
			}
			if len(clause) >= 2 && returnsBoolLiteral(n.Body) {
				for constant := range clause {
					constants[constant] = true
				}
			}
		case *ast.SwitchStmt:
			if n.Tag != nil {
				break
			}
			tagless := map[string]bool{}
			boolBody := false
			for _, stmt := range n.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, item := range clause.List {
					comparisonConstants(item, tagless)
				}
				boolBody = boolBody || returnsBoolLiteral(clause.Body)
			}
			if len(tagless) >= 2 && boolBody {
				for constant := range tagless {
					constants[constant] = true
				}
			}
		case *ast.IfStmt:
			condition := map[string]bool{}
			for _, leaf := range orChainLeaves(n.Cond) {
				comparisonConstants(leaf, condition)
			}
			if len(condition) >= 2 && returnsBoolLiteral(n.Body.List) {
				for constant := range condition {
					constants[constant] = true
				}
			}
		case *ast.ReturnStmt:
			if len(n.Results) != 1 {
				break
			}
			results := map[string]bool{}
			for _, leaf := range orChainLeaves(n.Results[0]) {
				comparisonConstants(leaf, results)
			}
			if len(results) >= 2 {
				for constant := range results {
					constants[constant] = true
				}
			}
		}
		return true
	})
	return constants
}
