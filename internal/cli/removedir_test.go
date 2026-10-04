package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// runRemoveDir runs `fleet skill remove-dir` against a fake home and returns
// stdout, stderr, and the error.
func runRemoveDir(t *testing.T, p *paths.Paths, args ...string) (string, string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill", "remove-dir"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestRemoveDirUnlistsRemovesLinksAndKeepsFiles(t *testing.T) {
	p := harnessHome(t)
	collection := filepath.Join(t.TempDir(), "customs")
	writeSkillDir(t, collection, "alpha", "Alpha.")
	writeSkillDir(t, collection, "beta", "Beta.")
	if _, _, err := runAddDir(t, p, collection); err != nil {
		t.Fatalf("add-dir: %v", err)
	}

	out, _, err := runRemoveDir(t, p, collection)
	if err != nil {
		t.Fatalf("remove-dir: %v\nout=%s", err, out)
	}

	// One headline, then one unlinked line per harness per skill.
	if !strings.Contains(out, `remove-dir: removed "`+collection+`"`) {
		t.Errorf("output missing headline:\n%s", out)
	}
	harnessDirs := map[string]string{
		"opencode": p.OpenCodeSkills(),
		"pi":       p.PiSkills(),
		"codex":    p.CodexSkills(),
		"claude":   p.ClaudeSkills(),
		"cursor":   p.CursorSkills(),
		"bob":      p.BobSkills(),
	}
	for h, dir := range harnessDirs {
		for _, skill := range []string{"alpha", "beta"} {
			want := h + `: unlinked "` + skill + `" → ` + filepath.Join(collection, skill)
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q:\n%s", want, out)
			}
			if _, rerr := os.Lstat(filepath.Join(dir, skill)); !os.IsNotExist(rerr) {
				t.Errorf("link %s/%s not removed: %v", dir, skill, rerr)
			}
		}
	}

	// The dir was unlisted; the files are untouched.
	if got := skillsDirsNow(t, p); len(got) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", got)
	}
	for _, skill := range []string{"alpha", "beta"} {
		if _, serr := os.Stat(filepath.Join(collection, skill, "SKILL.md")); serr != nil {
			t.Errorf("remove-dir deleted %s: %v", skill, serr)
		}
	}
}

func TestRemoveDirUntrackedErrorsListingTracked(t *testing.T) {
	p := harnessHome(t)
	tracked := filepath.Join(t.TempDir(), "tracked")
	writeSkillDir(t, tracked, "alpha", "Alpha.")
	if _, _, err := runAddDir(t, p, tracked); err != nil {
		t.Fatal(err)
	}

	_, _, err := runRemoveDir(t, p, filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), tracked) {
		t.Fatalf("remove-dir untracked error = %v, want listing %q", err, tracked)
	}
	if got := skillsDirsNow(t, p); len(got) != 1 || got[0] != filepath.Clean(tracked) {
		t.Errorf("refused remove-dir changed SkillsDirs() = %q", got)
	}
}

func TestRemoveDirMissingOnDiskStillUnlists(t *testing.T) {
	p := harnessHome(t)
	collection := filepath.Join(t.TempDir(), "customs")
	writeSkillDir(t, collection, "alpha", "Alpha.")
	if _, _, err := runAddDir(t, p, collection); err != nil {
		t.Fatal(err)
	}
	// Hand-delete the directory: the managed links go dangling but still
	// resolve under the tracked path, so remove-dir takes them with it.
	if err := os.RemoveAll(collection); err != nil {
		t.Fatal(err)
	}

	out, _, err := runRemoveDir(t, p, collection)
	if err != nil {
		t.Fatalf("remove-dir on a missing dir: %v\nout=%s", err, out)
	}
	if got := skillsDirsNow(t, p); len(got) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", got)
	}
	if _, rerr := os.Lstat(filepath.Join(p.OpenCodeSkills(), "alpha")); !os.IsNotExist(rerr) {
		t.Errorf("dangling link not removed: %v", rerr)
	}
}

func TestRemoveDirResolvesTildeAndRelative(t *testing.T) {
	p := harnessHome(t)
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	tilded := filepath.Join(fakeHome, "skills")
	writeSkillDir(t, tilded, "alpha", "Alpha.")
	if _, _, err := runAddDir(t, p, tilded); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRemoveDir(t, p, "~/skills"); err != nil {
		t.Fatalf("remove-dir ~/skills: %v", err)
	}
	if got := skillsDirsNow(t, p); len(got) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", got)
	}

	parent := t.TempDir()
	relative := filepath.Join(parent, "customs")
	writeSkillDir(t, relative, "beta", "Beta.")
	if _, _, err := runAddDir(t, p, relative); err != nil {
		t.Fatal(err)
	}
	t.Chdir(parent)
	if _, _, err := runRemoveDir(t, p, "customs"); err != nil {
		t.Fatalf("remove-dir customs: %v", err)
	}
	if got := skillsDirsNow(t, p); len(got) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", got)
	}
}
