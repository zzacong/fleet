package trackedset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

func testPaths(t *testing.T) *paths.Paths {
	t.Helper()
	t.Setenv("FLEET_REPO", "")
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

func trackExplicit(t *testing.T, p *paths.Paths, repos ...string) {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos(repos)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
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

func TestListIgnoresUnknownFileKeys(t *testing.T) {
	p := testPaths(t)
	// A config carrying only unknown keys (including the retired
	// single-pointer key) behaves as unset.
	writeTrackedConfig(t, p, `{"future": 123, "skillsRepo": "/old/pointer"}`)
	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List() = %q, want empty (unknown keys behave as unset)", got)
	}
}

func TestListKeepsExplicitOrderThenScansFleetHomeAlphabetically(t *testing.T) {
	p := testPaths(t)
	outsideB := filepath.Join(t.TempDir(), "explicit-b")
	outsideA := filepath.Join(t.TempDir(), "explicit-a")
	writeTrackedConfig(t, p, `{"skillsRepos": ["`+outsideB+`", "`+outsideA+`"]}`)
	for _, name := range []string{"zeta", "alpha", "mid"} {
		if err := os.MkdirAll(filepath.Join(p.FleetReposDir(), name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A stray file in the checkout parent is not a slot.
	if err := os.WriteFile(filepath.Join(p.FleetReposDir(), "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []string{
		outsideB, outsideA,
		filepath.Join(p.FleetReposDir(), "alpha"),
		filepath.Join(p.FleetReposDir(), "mid"),
		filepath.Join(p.FleetReposDir(), "zeta"),
	}
	if !equalLists(got, want) {
		t.Errorf("List() = %q, want %q", got, want)
	}
}

func TestListPrependsEnvAndDedupes(t *testing.T) {
	p := testPaths(t)
	envRepo := filepath.Join(t.TempDir(), "env-repo")
	explicit := filepath.Join(t.TempDir(), "explicit")
	writeTrackedConfig(t, p, `{"skillsRepos": ["`+envRepo+`", "`+explicit+`"]}`)
	if err := os.MkdirAll(filepath.Join(p.FleetReposDir(), "auto"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_REPO", envRepo)
	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []string{envRepo, explicit, filepath.Join(p.FleetReposDir(), "auto")}
	if !equalLists(got, want) {
		t.Errorf("List() = %q, want %q", got, want)
	}
}

func TestListDedupesUncleanedVariants(t *testing.T) {
	p := testPaths(t)
	repo := filepath.Join(t.TempDir(), "explicit")
	// Trailing separator, dot segment, and parent segment all clean to
	// the same identity: first occurrence wins.
	writeTrackedConfig(t, p, `{"skillsRepos": ["`+repo+string(filepath.Separator)+`", "`+repo+`", "`+
		filepath.Join(repo, "sub", "..")+`"]}`)
	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !equalLists(got, []string{filepath.Clean(repo)}) {
		t.Errorf("List() = %q, want [%q]", got, filepath.Clean(repo))
	}
}

func TestListResolvesRelativeEnvToAbsolute(t *testing.T) {
	p := testPaths(t)
	repoDir := filepath.Join(t.TempDir(), "myrepo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	wd := t.TempDir()
	t.Chdir(wd)
	rel, err := filepath.Rel(wd, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(rel) {
		t.Fatalf("expected relative path, got %q", rel)
	}
	t.Setenv("FLEET_REPO", rel)

	got, err := List(p)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	absWant, _ := filepath.Abs(rel)
	if len(got) != 1 || got[0] != absWant {
		t.Errorf("List() = %q, want absolute [%q]", got, absWant)
	}
	if !filepath.IsAbs(got[0]) {
		t.Errorf("List()[0] %q is not absolute", got[0])
	}
}

func TestCollectionDirsFollowsListOrder(t *testing.T) {
	p := testPaths(t)
	outside := filepath.Join(t.TempDir(), "explicit")
	trackExplicit(t, p, outside)
	for _, name := range []string{"zeta", "alpha"} {
		if err := os.MkdirAll(filepath.Join(p.FleetReposDir(), name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := List(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := CollectionDirs(p)
	if err != nil {
		t.Fatalf("CollectionDirs() error = %v", err)
	}
	want := make([]string, len(roots))
	for i, root := range roots {
		want[i] = filepath.Join(root, "skills")
	}
	if !equalLists(got, want) {
		t.Errorf("CollectionDirs() = %q, want %q", got, want)
	}
}

func TestCollectionDirsEmptyWhenNothingTracked(t *testing.T) {
	p := testPaths(t)
	got, err := CollectionDirs(p)
	if err != nil {
		t.Fatalf("CollectionDirs() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("CollectionDirs() = %q, want empty", got)
	}
}

func TestRememberInsideFleetHomeWritesNoConfig(t *testing.T) {
	p := testPaths(t)
	inside := filepath.Join(p.FleetReposDir(), "customs")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	wrote, err := Remember(p, inside)
	if err != nil {
		t.Fatalf("Remember() error = %v", err)
	}
	if wrote {
		t.Error("Remember inside fleet home should not write")
	}
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("config file should not exist, stat err = %v", err)
	}
}

func TestRememberOutsideAppendsOnceWithoutReordering(t *testing.T) {
	p := testPaths(t)
	outsideA := filepath.Join(t.TempDir(), "a")
	outsideB := filepath.Join(t.TempDir(), "b")
	for _, dir := range []string{outsideA, outsideB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Remember(p, outsideA); err != nil {
		t.Fatal(err)
	}
	if _, err := Remember(p, outsideB); err != nil {
		t.Fatal(err)
	}
	// Re-remembering an existing entry never reorders.
	if _, err := Remember(p, outsideA); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	got := f.SkillsRepos()
	if len(got) != 2 || got[0] != outsideA || got[1] != outsideB {
		t.Errorf("SkillsRepos() = %q, want [%q %q] in order", got, outsideA, outsideB)
	}
}

func TestForgetUnlistsPreservingOrder(t *testing.T) {
	p := testPaths(t)
	dir := t.TempDir()
	repoA := filepath.Join(dir, "a")
	repoB := filepath.Join(dir, "b")
	repoC := filepath.Join(dir, "c")
	trackExplicit(t, p, repoA, repoB, repoC)

	wrote, err := Forget(p, repoB)
	if err != nil {
		t.Fatalf("Forget() error = %v", err)
	}
	if !wrote {
		t.Error("Forget of a listed repo should write")
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if got := f.SkillsRepos(); !equalLists(got, []string{repoA, repoC}) {
		t.Errorf("SkillsRepos() = %q, want [%q %q] in order", got, repoA, repoC)
	}
}

func TestForgetAbsentEntryWritesNothing(t *testing.T) {
	p := testPaths(t)
	trackExplicit(t, p, filepath.Join(t.TempDir(), "a"))

	wrote, err := Forget(p, filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("Forget() error = %v", err)
	}
	if wrote {
		t.Error("Forget of an unlisted repo should not write")
	}
}

func TestForgetConventionTrackedWritesNoConfig(t *testing.T) {
	p := testPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}
	wrote, err := Forget(p, slot)
	if err != nil {
		t.Fatalf("Forget() error = %v", err)
	}
	if wrote {
		t.Error("Forget of a convention-tracked checkout should not write")
	}
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("config file should not exist, stat err = %v", err)
	}
}

func TestResolveExactPath(t *testing.T) {
	p := testPaths(t)
	outside := filepath.Join(t.TempDir(), "customs")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, outside)

	got, err := Resolve(p, outside)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != filepath.Clean(outside) {
		t.Errorf("Resolve() = %q, want %q", got, outside)
	}
}

func TestResolveSlotName(t *testing.T) {
	p := testPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team-customs")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(p, "team-customs")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != filepath.Clean(slot) {
		t.Errorf("Resolve() = %q, want %q", got, slot)
	}
}

func TestResolveUnknownListsTracked(t *testing.T) {
	p := testPaths(t)
	outside := filepath.Join(t.TempDir(), "customs")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, outside)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(p, filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("Resolve(unknown) should fail")
	}
	for _, want := range []string{filepath.Clean(outside), filepath.Clean(slot)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list tracked repo %q, got: %v", want, err)
		}
	}
}

func TestResolveRelativePath(t *testing.T) {
	p := testPaths(t)
	parent := t.TempDir()
	outside := filepath.Join(parent, "customs")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, outside)
	t.Chdir(parent)

	got, err := Resolve(p, "customs")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != filepath.Clean(outside) {
		t.Errorf("Resolve() = %q, want %q", got, outside)
	}
}

func TestResolveEmptySetReportsNoneTracked(t *testing.T) {
	p := testPaths(t)
	_, err := Resolve(p, "anything")
	if err == nil {
		t.Fatal("Resolve with nothing tracked should fail")
	}
	if !strings.Contains(err.Error(), "no skills repos are tracked") {
		t.Errorf("error should report nothing tracked, got: %v", err)
	}
}
