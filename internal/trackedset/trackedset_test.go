package trackedset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

func testPaths(t *testing.T) *paths.Paths {
	t.Helper()
	return paths.New(filepath.Join(t.TempDir(), "home"))
}

func writeTrackedConfig(t *testing.T, p *paths.Paths, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p.FleetConfigFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.FleetConfigFile(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func equalLists(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestListEmptyWhenNothingSet(t *testing.T) {
	p := testPaths(t)
	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List() = %q, want empty", got)
	}
}

func TestListIgnoresRetiredFileKeys(t *testing.T) {
	p := testPaths(t)
	// A config carrying only retired keys (the repo-root list and the older
	// single pointer) behaves as unset: neither is read.
	writeTrackedConfig(t, p, `{"future": 123, "skillsRepos": ["/repo"], "skillsRepo": "/old/pointer"}`)
	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List() = %q, want empty (retired keys behave as unset)", got)
	}
}

func TestListReturnsSkillsDirsInOrder(t *testing.T) {
	p := testPaths(t)
	b := filepath.Join(t.TempDir(), "b")
	a := filepath.Join(t.TempDir(), "a")
	writeTrackedConfig(t, p, `{"skillsDirs": ["`+b+`", "`+a+`"]}`)
	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !equalLists(got, []string{b, a}) {
		t.Errorf("List() = %q, want [%q %q] in order", got, b, a)
	}
}

func TestListPreservesDuplicatesForDoctor(t *testing.T) {
	p := testPaths(t)
	dir := filepath.Join(t.TempDir(), "dir")
	// Trailing separator and a dot segment clean to the same path: the
	// duplicate is preserved so doctor can report a hand-edited config.
	writeTrackedConfig(t, p, `{"skillsDirs": ["`+dir+string(filepath.Separator)+`", "`+dir+`"]}`)
	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !equalLists(got, []string{filepath.Clean(dir), filepath.Clean(dir)}) {
		t.Errorf("List() = %q, want the duplicate preserved cleaned", got)
	}
}
