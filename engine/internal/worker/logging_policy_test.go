package worker

import (
	"bufio"
	"bytes"
	"fmt"
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
		if _, err := os.Stat(root); err != nil {
			t.Errorf("stat %s: %v", root, err)
			continue
		}

		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			contents, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}

			scanner := bufio.NewScanner(bytes.NewReader(contents))
			for lineNumber := 1; scanner.Scan(); lineNumber++ {
				line := scanner.Text()
				if bareLogCall.MatchString(line) {
					t.Errorf("%s:%d: %s", path, lineNumber, strings.TrimSpace(line))
				}
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("scan %s: %w", path, err)
			}
			return nil
		})
		if err != nil {
			t.Errorf("scan %s: %v", root, err)
		}
	}
}
