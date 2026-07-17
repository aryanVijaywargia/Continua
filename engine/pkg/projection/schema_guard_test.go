package projection_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	"github.com/continua-ai/continua/engine/pkg/projection"
)

func TestTraceEngineColumnManifestMatchesDatabase(t *testing.T) {
	db := enginetest.NewTestDatabase(t)

	rows, err := db.Pool.Query(context.Background(), `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'traces'
		  AND column_name LIKE 'engine\_%'
	`)
	if err != nil {
		t.Fatalf("query public.traces engine columns: %v", err)
	}
	defer rows.Close()

	databaseColumns := make(map[string]string)
	for rows.Next() {
		var columnName string
		var dataType string
		if err := rows.Scan(&columnName, &dataType); err != nil {
			t.Fatalf("scan public.traces engine column: %v", err)
		}
		databaseColumns[columnName] = dataType
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read public.traces engine columns: %v", err)
	}

	if len(projection.TraceEngineColumns) == 0 {
		t.Error("the projection schema contract must declare the engine_* columns of public.traces")
	}

	for columnName, wantType := range projection.TraceEngineColumns {
		gotType, ok := databaseColumns[columnName]
		if !ok {
			t.Errorf("manifest column %q does not exist in public.traces (want data_type %q)", columnName, wantType)
			continue
		}
		if gotType != wantType {
			t.Errorf("public.traces column %q data_type = %q, want %q", columnName, gotType, wantType)
		}
	}

	for columnName, gotType := range databaseColumns {
		_, ok := projection.TraceEngineColumns[columnName]
		if !ok {
			t.Errorf("public.traces column %q with data_type %q is not declared in projection.TraceEngineColumns", columnName, gotType)
		}
	}
}

func TestProjectionSQLReferencesOnlyManifestColumns(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	packageDir := filepath.Dir(testFile)

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read projection package directory: %v", err)
	}

	engineColumnPattern := regexp.MustCompile(`(^|[^a-z0-9_])(engine_[a-z0-9_]+)`)
	undeclared := make(map[string]struct{})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(packageDir, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}

		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}

			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("unquote string literal in %s: %v", entry.Name(), err)
			}
			for _, match := range engineColumnPattern.FindAllStringSubmatch(value, -1) {
				columnName := match[2]
				if _, declared := projection.TraceEngineColumns[columnName]; !declared {
					undeclared[entry.Name()+": "+columnName] = struct{}{}
				}
			}
			return true
		})
	}

	if len(undeclared) > 0 {
		found := make([]string, 0, len(undeclared))
		for occurrence := range undeclared {
			found = append(found, occurrence)
		}
		sort.Strings(found)
		t.Fatalf("projection SQL references engine_* columns missing from projection.TraceEngineColumns:\n%s", strings.Join(found, "\n"))
	}
}
