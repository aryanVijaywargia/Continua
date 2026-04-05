package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRootSideEngineStartPath_DoesNotImportEngineInternals(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	dir := filepath.Dir(currentFile)

	for _, file := range []string{"engine_control.go", "engine_handlers.go", "engine_mapper.go"} {
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("os.ReadFile(%s) error = %v", file, err)
		}
		source := string(data)
		if strings.Contains(source, "engine/internal/") {
			t.Fatalf("expected %s to avoid engine/internal imports", file)
		}
	}
}
