package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoV1CharmImports guards the stack decision: fleet pins the
// charm.land v2 module paths, and the v1 github.com/charmbracelet paths
// (with their v1 idioms) must never creep in. AI-written TUI code
// reaching for the v1 tutorials is exactly the regression this catches.
func TestNoV1CharmImports(t *testing.T) {
	root := moduleRoot(t)
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, `"github.com/charmbracelet/`) {
				continue
			}
			offenders = append(offenders, rel(t, root, path)+": "+trimmed)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("v1 charmbracelet imports found (want charm.land/*/v2):\n%s",
			strings.Join(offenders, "\n"))
	}
}

// moduleRoot walks up from the package directory to the go.mod file.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the package directory")
		}
		dir = parent
	}
}

func rel(t *testing.T, root, path string) string {
	t.Helper()
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return relPath
}
