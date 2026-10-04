package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// harnessHome builds a fake home with every harness's config dir present,
// and no tracked dirs or skills.
func harnessHome(t *testing.T) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// runAddDir runs `fleet skill add-dir` against a fake home and returns
// stdout, stderr, and the error.
func runAddDir(t *testing.T, p *paths.Paths, args ...string) (string, string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill", "add-dir"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func skillsDirsNow(t *testing.T, p *paths.Paths) []string {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	return f.SkillsDirs()
}

func TestAddDirLinksEveryHarnessPerSkillAndSyncs(t *testing.T) {
	p := harnessHome(t)
	collection := filepath.Join(t.TempDir(), "customs")
	writeSkillDir(t, collection, "alpha", "Alpha.")
	writeSkillDir(t, collection, "beta", "Beta.")

	// A redundant canonical-store link for the sync half of the run to clean.
	writeSkillDir(t, p.SkillsStore(), "tdd", "TDD.")
	if err := os.MkdirAll(p.OpenCodeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.OpenCodeSkills(), "tdd")); err != nil {
		t.Fatal(err)
	}

	out, _, err := runAddDir(t, p, collection)
	if err != nil {
		t.Fatalf("add-dir: %v\nout=%s", err, out)
	}

	// One headline, then one linked line per harness per skill.
	if !strings.Contains(out, `add-dir: added "`+collection+`"`) {
		t.Errorf("output missing headline:\n%s", out)
	}
	harnesses := []string{"opencode", "pi", "codex", "claude", "cursor", "bob"}
	for _, h := range harnesses {
		for _, skill := range []string{"alpha", "beta"} {
			want := h + `: linked "` + skill + `" → ` + filepath.Join(collection, skill)
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q:\n%s", want, out)
			}
		}
	}

	// Every harness's skills dir holds a managed link to each skill.
	dirs := []string{p.OpenCodeSkills(), p.PiSkills(), p.CodexSkills(), p.ClaudeSkills(), p.CursorSkills(), p.BobSkills()}
	for _, dir := range dirs {
		for _, skill := range []string{"alpha", "beta"} {
			got, rerr := os.Readlink(filepath.Join(dir, skill))
			if rerr != nil || got != filepath.Join(collection, skill) {
				t.Errorf("link %s/%s = %q, %v; want %q", dir, skill, got, rerr, filepath.Join(collection, skill))
			}
		}
	}

	// The dir was appended, and sync ran (the redundant link is gone).
	if got := skillsDirsNow(t, p); len(got) != 1 || got[0] != filepath.Clean(collection) {
		t.Errorf("SkillsDirs() = %q, want [%q]", got, filepath.Clean(collection))
	}
	if _, rerr := os.Lstat(filepath.Join(p.OpenCodeSkills(), "tdd")); !os.IsNotExist(rerr) {
		t.Errorf("sync did not remove the redundant link: %v", rerr)
	}
	if !strings.Contains(out, `sync: opencode: removed redundant link "tdd"`) {
		t.Errorf("output missing the sync report:\n%s", out)
	}
}

func TestAddDirResolvesTildeAndRelative(t *testing.T) {
	p := harnessHome(t)
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	tilded := filepath.Join(fakeHome, "skills")
	writeSkillDir(t, tilded, "alpha", "Alpha.")

	if _, _, err := runAddDir(t, p, "~/skills"); err != nil {
		t.Fatalf("add-dir ~/skills: %v", err)
	}
	if got := skillsDirsNow(t, p); len(got) != 1 || got[0] != filepath.Clean(tilded) {
		t.Errorf("SkillsDirs() = %q, want [%q]", got, filepath.Clean(tilded))
	}

	parent := t.TempDir()
	relative := filepath.Join(parent, "customs")
	writeSkillDir(t, relative, "beta", "Beta.")
	t.Chdir(parent)
	if _, _, err := runAddDir(t, p, "customs"); err != nil {
		t.Fatalf("add-dir customs: %v", err)
	}
	absWant, _ := filepath.Abs("customs")
	if got := skillsDirsNow(t, p); len(got) != 2 || got[1] != absWant {
		t.Errorf("SkillsDirs() = %q, want relative added as %q", got, absWant)
	}
}

func TestAddDirAlreadyTrackedIsNoOp(t *testing.T) {
	p := harnessHome(t)
	collection := filepath.Join(t.TempDir(), "customs")
	writeSkillDir(t, collection, "alpha", "Alpha.")
	if _, _, err := runAddDir(t, p, collection); err != nil {
		t.Fatal(err)
	}

	out, _, err := runAddDir(t, p, collection)
	if err != nil {
		t.Fatalf("re-run add-dir: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "already tracked") {
		t.Errorf("re-run output missing already-tracked message:\n%s", out)
	}
	if got := skillsDirsNow(t, p); len(got) != 1 {
		t.Errorf("re-run duplicated the entry: %q", got)
	}
}

func TestAddDirRefusalsLeaveConfigUntouched(t *testing.T) {
	p := harnessHome(t)
	file := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	inside := filepath.Join(p.FleetConfigDir(), "collections")
	writeSkillDir(t, inside, "beta", "Beta.")
	writeSkillDir(t, p.FleetHomeSkills(), "gamma", "Gamma.")
	writeSkillDir(t, p.SkillsStore(), "delta", "Delta.")

	cases := []struct {
		name string
		arg  string
		want string
	}{
		{"missing", filepath.Join(t.TempDir(), "nope"), "does not exist"},
		{"file", file, "not a directory"},
		{"empty", empty, "holds no skills"},
		{"canonical store", p.SkillsStore(), "canonical store"},
		{"fallback", p.FleetHomeSkills(), "fallback"},
		{"inside fleet home", inside, "fleet home"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runAddDir(t, p, tc.arg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("add-dir %q error = %v, want %q", tc.arg, err, tc.want)
			}
			if got := skillsDirsNow(t, p); len(got) != 0 {
				t.Errorf("refused add-dir wrote SkillsDirs() = %q", got)
			}
		})
	}
}

func TestAddDirRefusesNestedAndNameCollision(t *testing.T) {
	p := harnessHome(t)
	first := filepath.Join(t.TempDir(), "first")
	writeSkillDir(t, first, "shared", "Shared.")
	if _, _, err := runAddDir(t, p, first); err != nil {
		t.Fatal(err)
	}

	// A nested dir inside the tracked one.
	child := filepath.Join(first, "child")
	writeSkillDir(t, child, "child", "Child.")
	if _, _, err := runAddDir(t, p, child); err == nil || !strings.Contains(err.Error(), "inside tracked") {
		t.Fatalf("nested add-dir error = %v, want inside-tracked refusal", err)
	}

	// A name collision with the tracked dir.
	second := filepath.Join(t.TempDir(), "second")
	writeSkillDir(t, second, "shared", "Shared again.")
	if _, _, err := runAddDir(t, p, second); err == nil || !strings.Contains(err.Error(), "shared") {
		t.Fatalf("collision add-dir error = %v, want name collision with home", err)
	}
	if got := skillsDirsNow(t, p); len(got) != 1 || got[0] != filepath.Clean(first) {
		t.Errorf("refused add-dir changed SkillsDirs() = %q", got)
	}
}
