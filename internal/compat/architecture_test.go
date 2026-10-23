package compat

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/AkatukiSora/vrc-vrpoker-ststs/"

// TestCollectionPackagesAreFormatAgnostic keeps line collection (logsource) and
// the tail watcher independent of format, parsing, event, persistence and stats.
func TestCollectionPackagesAreFormatAgnostic(t *testing.T) {
	t.Parallel()

	forbidden := []string{"logformat", "parser", "logevent", "persistence", "stats"}
	for _, pkg := range []string{"logsource", "watcher"} {
		dir := filepath.Join("..", pkg)
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no Go files in %s (err=%v)", dir, err)
		}
		for _, file := range files {
			if _, err := os.Stat(file); err != nil {
				t.Fatal(err)
			}
			f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			for _, imp := range f.Imports {
				path, _ := strconv.Unquote(imp.Path.Value)
				rel, ok := strings.CutPrefix(path, modulePath+"internal/")
				if !ok {
					continue
				}
				for _, bad := range forbidden {
					if rel == bad || strings.HasPrefix(rel, bad+"/") {
						t.Errorf("%s imports internal/%s; %s must not depend on it", file, rel, pkg)
					}
				}
			}
		}
	}
}
