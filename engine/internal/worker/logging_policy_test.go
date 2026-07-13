package worker

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNoBareLogPrintfUnderEngineInternalAndPkg(t *testing.T) {
	engineRoot := filepath.Join("..", "..")
	bareLogCall := regexp.MustCompile(`(^|[^\w.])log\.(Printf|Print|Println)`)

	for _, directory := range []string{"internal", "pkg"} {
		root := filepath.Join(engineRoot, directory)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			t.Logf("skipping %s: %v", root, err)
			continue
		}

		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				t.Logf("skipping %s: %v", path, walkErr)
				return nil
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			contents, err := os.ReadFile(path)
			if err != nil {
				t.Logf("skipping %s: %v", path, err)
				return nil
			}

			scanner := bufio.NewScanner(bytes.NewReader(contents))
			for lineNumber := 1; scanner.Scan(); lineNumber++ {
				line := scanner.Text()
				if bareLogCall.MatchString(line) {
					t.Errorf("%s:%d: %s", path, lineNumber, strings.TrimSpace(line))
				}
			}
			return nil
		})
	}
}
