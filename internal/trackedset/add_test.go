package trackedset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// writeSkill writes a minimal skill directory (<collection>/<name>/SKILL.md)
// so a collection dir scans as one holding skills.
func writeSkill(t *testing.T, collection, name string) {
	t.Helper()
	dir := filepath.Join(collection, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + name + ".\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// trackDirs seeds the explicit collection-dir list in the config file.
func trackDirs(t *testing.T, p *paths.Paths, dirs ...string) {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsDirs(dirs)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
}

func skillsDirsOf(t *testing.T, p *paths.Paths) []string {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	return f.SkillsDirs()
}

func TestAddAppendsCleanedAbsoluteWithoutReordering(t *testing.T) {
	p := testPaths(t)
	a := filepath.Join(t.TempDir(), "a")
	b := filepath.Join(t.TempDir(), "b")
	writeSkill(t, a, "alpha")
	writeSkill(t, b, "beta")

	if res, err := Add(p, a); err != nil || !res.Added || res.Dir != filepath.Clean(a) {
		t.Fatalf("Add(a) = %+v, %v; want added %q", res, err, filepath.Clean(a))
	}
	if res, err := Add(p, b); err != nil || !res.Added {
		t.Fatalf("Add(b) = %+v, %v; want added", res, err)
	}
	if got := skillsDirsOf(t, p); !equalLists(got, []string{filepath.Clean(a), filepath.Clean(b)}) {
		t.Errorf("SkillsDirs() = %q, want [a b] in order", got)
	}

	// A path that cleans to an existing entry is a no-op: no duplicate, no
	// reorder.
	noisy := filepath.Join(a, ".", "..", filepath.Base(a))
	res, err := Add(p, noisy)
	if err != nil {
		t.Fatalf("Add(existing) error = %v", err)
	}
	if res.Added {
		t.Errorf("Add(existing) Added = true, want no-op")
	}
	if res.Dir != filepath.Clean(a) {
		t.Errorf("Add(existing) Dir = %q, want %q", res.Dir, filepath.Clean(a))
	}
	if got := skillsDirsOf(t, p); !equalLists(got, []string{filepath.Clean(a), filepath.Clean(b)}) {
		t.Errorf("SkillsDirs() = %q, want unchanged [a b]", got)
	}
}

func TestAddResolvesTildeAndRelativePaths(t *testing.T) {
	p := testPaths(t)
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	tilded := filepath.Join(fakeHome, "skills")
	writeSkill(t, tilded, "alpha")

	if _, err := Add(p, "~/skills"); err != nil {
		t.Fatalf("Add(~/skills) error = %v", err)
	}
	if got := skillsDirsOf(t, p); !equalLists(got, []string{filepath.Clean(tilded)}) {
		t.Errorf("SkillsDirs() = %q, want [%q]", got, filepath.Clean(tilded))
	}

	// Relative paths resolve against the working directory.
	parent := t.TempDir()
	relative := filepath.Join(parent, "customs")
	writeSkill(t, relative, "beta")
	t.Chdir(parent)
	if _, err := Add(p, "customs"); err != nil {
		t.Fatalf("Add(customs) error = %v", err)
	}
	absWant, _ := filepath.Abs("customs")
	if got := skillsDirsOf(t, p); len(got) != 2 || got[1] != absWant {
		t.Errorf("SkillsDirs() = %q, want relative added as %q", got, absWant)
	}
}

func TestAddRefusesUnusablePaths(t *testing.T) {
	p := testPaths(t)
	file := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()

	cases := []struct {
		name string
		arg  string
		want string
	}{
		{"missing", filepath.Join(t.TempDir(), "nope"), "does not exist"},
		{"file", file, "not a directory"},
		{"empty", empty, "holds no skills"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Add(p, tc.arg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Add(%q) error = %v, want %q", tc.arg, err, tc.want)
			}
			if got := skillsDirsOf(t, p); len(got) != 0 {
				t.Errorf("refused Add wrote SkillsDirs() = %q", got)
			}
		})
	}
}

func TestAddRefusesCanonicalStoreFallbackAndFleetHome(t *testing.T) {
	p := testPaths(t)
	// Give each refused path a skill so only the location rule can refuse.
	store := p.SkillsStore()
	writeSkill(t, store, "alpha")
	fallback := p.FleetHomeSkills()
	writeSkill(t, fallback, "beta")
	inside := filepath.Join(p.FleetConfigDir(), "collections")
	writeSkill(t, inside, "gamma")

	cases := []struct {
		name string
		arg  string
		want string
	}{
		{"canonical store", store, "canonical store"},
		{"fallback", fallback, "fallback"},
		{"inside fleet home", inside, "fleet home"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Add(p, tc.arg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Add(%q) error = %v, want %q", tc.arg, err, tc.want)
			}
		})
	}
}

func TestAddRefusesNestedTrackedDirsBothDirections(t *testing.T) {
	p := testPaths(t)
	parent := filepath.Join(t.TempDir(), "parent")
	writeSkill(t, parent, "alpha")
	child := filepath.Join(parent, "child")
	writeSkill(t, child, "beta")

	// child inside a tracked parent
	trackDirs(t, p, parent)
	if _, err := Add(p, child); err == nil || !strings.Contains(err.Error(), "inside tracked") {
		t.Fatalf("Add(child) error = %v, want nested-inside refusal", err)
	}

	// parent containing a tracked child
	trackDirs(t, p, child)
	if _, err := Add(p, parent); err == nil || !strings.Contains(err.Error(), "contains tracked") {
		t.Fatalf("Add(parent) error = %v, want contains-tracked refusal", err)
	}
}

func TestAddRefusesNameCollisionWithTrackedDirAndFallback(t *testing.T) {
	p := testPaths(t)
	first := filepath.Join(t.TempDir(), "first")
	writeSkill(t, first, "shared")
	trackDirs(t, p, first)

	second := filepath.Join(t.TempDir(), "second")
	writeSkill(t, second, "shared")
	_, err := Add(p, second)
	if err == nil {
		t.Fatal("Add(second) should refuse the shared skill name")
	}
	for _, want := range []string{"shared", filepath.Clean(first)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("collision error missing %q: %v", want, err)
		}
	}

	// The fallback is a custom home too.
	fallback := p.FleetHomeSkills()
	writeSkill(t, fallback, "dup")
	third := filepath.Join(t.TempDir(), "third")
	writeSkill(t, third, "dup")
	_, err = Add(p, third)
	if err == nil || !strings.Contains(err.Error(), "dup") || !strings.Contains(err.Error(), fallback) {
		t.Fatalf("Add(third) error = %v, want fallback collision naming dup and %q", err, fallback)
	}
}

func TestAddAllowsCanonicalStoreNameCollision(t *testing.T) {
	p := testPaths(t)
	writeSkill(t, p.SkillsStore(), "shared")
	incoming := filepath.Join(t.TempDir(), "customs")
	writeSkill(t, incoming, "shared")

	res, err := Add(p, incoming)
	if err != nil {
		t.Fatalf("Add(incoming) error = %v, want canonical collision allowed", err)
	}
	if !res.Added {
		t.Errorf("Add(incoming) Added = false, want true")
	}
	if got := skillsDirsOf(t, p); !equalLists(got, []string{filepath.Clean(incoming)}) {
		t.Errorf("SkillsDirs() = %q, want [%q]", got, filepath.Clean(incoming))
	}
}

func TestAddRefusesUnreadableDir(t *testing.T) {
	p := testPaths(t)
	dir := filepath.Join(t.TempDir(), "locked")
	writeSkill(t, dir, "alpha")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	// Root and some filesystems ignore the permission bits; the rule only
	// applies when a read is actually denied.
	if _, rerr := os.ReadDir(dir); rerr == nil {
		t.Skip("directory stays readable after chmod 000")
	}

	if _, err := Add(p, dir); err == nil {
		t.Fatal("Add(unreadable) should fail")
	}
}
