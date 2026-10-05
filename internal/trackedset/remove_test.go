package trackedset

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveUnlistsPreservingOrder(t *testing.T) {
	p := testPaths(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	c := filepath.Join(dir, "c")
	trackDirs(t, p, a, b, c)

	res, err := Remove(p, b)
	if err != nil {
		t.Fatalf("Remove(b) error = %v", err)
	}
	if res.Dir != filepath.Clean(b) {
		t.Errorf("Remove(b) Dir = %q, want %q", res.Dir, filepath.Clean(b))
	}
	if got := skillsDirsOf(t, p); !equalLists(got, []string{a, c}) {
		t.Errorf("SkillsDirs() = %q, want [a c] in order", got)
	}
}

func TestRemoveUntrackedErrorsListingTracked(t *testing.T) {
	p := testPaths(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	trackDirs(t, p, a, b)

	_, err := Remove(p, filepath.Join(dir, "nope"))
	if err == nil {
		t.Fatal("Remove(untracked) should fail")
	}
	for _, want := range []string{a, b, "not tracked"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	if got := skillsDirsOf(t, p); !equalLists(got, []string{a, b}) {
		t.Errorf("refused Remove wrote SkillsDirs() = %q", got)
	}
}

func TestRemoveUntrackedWhenNothingTracked(t *testing.T) {
	p := testPaths(t)
	_, err := Remove(p, filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "no dirs are tracked") {
		t.Fatalf("Remove with nothing tracked error = %v, want none-tracked message", err)
	}
}

func TestRemoveMissingOnDiskStillUnlists(t *testing.T) {
	p := testPaths(t)
	missing := filepath.Join(t.TempDir(), "gone")
	trackDirs(t, p, missing)

	if _, err := Remove(p, missing); err != nil {
		t.Fatalf("Remove(dir missing from disk) error = %v", err)
	}
	if got := skillsDirsOf(t, p); len(got) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", got)
	}
}

func TestRemoveResolvesTildeAndRelative(t *testing.T) {
	p := testPaths(t)
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	tilded := filepath.Join(fakeHome, "skills")
	trackDirs(t, p, tilded)

	if _, err := Remove(p, "~/skills"); err != nil {
		t.Fatalf("Remove(~/skills) error = %v", err)
	}
	if got := skillsDirsOf(t, p); len(got) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", got)
	}

	parent := t.TempDir()
	relative := filepath.Join(parent, "customs")
	trackDirs(t, p, relative)
	t.Chdir(parent)
	if _, err := Remove(p, "customs"); err != nil {
		t.Fatalf("Remove(customs) error = %v", err)
	}
	if got := skillsDirsOf(t, p); len(got) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", got)
	}
}
