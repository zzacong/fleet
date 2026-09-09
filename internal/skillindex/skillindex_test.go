package skillindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// indexHome builds a fake home with the given explicit repo roots in
// config and the given fleet-home checkout slots on disk.
func indexHome(t *testing.T, explicit []string, slots []string) *paths.Paths {
	t.Helper()
	t.Setenv("FLEET_REPO", "")
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	if err := os.MkdirAll(filepath.Dir(p.FleetConfigFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	if len(explicit) > 0 {
		quoted := make([]string, len(explicit))
		for i, r := range explicit {
			quoted[i] = `"` + r + `"`
		}
		body := `{"skillsRepos": [` + strings.Join(quoted, ", ") + `]}`
		if err := os.WriteFile(p.FleetConfigFile(), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range slots {
		if err := os.MkdirAll(filepath.Join(p.FleetReposDir(), name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func writeIndexSkill(t *testing.T, home, dir, name string) {
	t.Helper()
	path := filepath.Join(home, dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: Does things.\n---\n\n# docs\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func equalStrings(a, b []string) bool {
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

func TestCustomHomesOrdersTrackedThenFallback(t *testing.T) {
	home := t.TempDir()
	outsideB := filepath.Join(home, "explicit-b")
	outsideA := filepath.Join(home, "explicit-a")
	p := indexHome(t, []string{outsideB, outsideA}, []string{"zeta", "alpha"})

	got, err := CustomHomes(p)
	if err != nil {
		t.Fatalf("CustomHomes() error = %v", err)
	}
	want := []string{
		filepath.Join(outsideB, "skills"),
		filepath.Join(outsideA, "skills"),
		filepath.Join(p.FleetReposDir(), "alpha", "skills"),
		filepath.Join(p.FleetReposDir(), "zeta", "skills"),
		p.FleetHomeSkills(),
	}
	if !equalStrings(got, want) {
		t.Fatalf("CustomHomes() = %q, want %q", got, want)
	}
}

func TestCustomHomesFallbackOnlyWhenNothingTracked(t *testing.T) {
	p := indexHome(t, nil, nil)

	got, err := CustomHomes(p)
	if err != nil {
		t.Fatalf("CustomHomes() error = %v", err)
	}
	if len(got) != 1 || got[0] != p.FleetHomeSkills() {
		t.Fatalf("CustomHomes() = %q, want [%q]", got, p.FleetHomeSkills())
	}
}

func TestLoadHomesEndsWithFallbackThenStore(t *testing.T) {
	p := indexHome(t, nil, []string{"team"})
	writeIndexSkill(t, p.FleetHomeSkills(), "mine", "mine")

	idx, errs, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("Load() errs = %v, want none", errs)
	}
	team := filepath.Join(p.FleetReposDir(), "team", "skills")
	want := []string{team, p.FleetHomeSkills(), p.SkillsStore()}
	if !equalStrings(idx.Homes(), want) {
		t.Fatalf("Homes() = %q, want %q", idx.Homes(), want)
	}
	if idx.Store() != p.SkillsStore() {
		t.Errorf("Store() = %q, want %q", idx.Store(), p.SkillsStore())
	}
	if idx.Fallback() != p.FleetHomeSkills() {
		t.Errorf("Fallback() = %q, want %q", idx.Fallback(), p.FleetHomeSkills())
	}
}

func TestOrderedListsEveryCopyPrecedenceFirst(t *testing.T) {
	p := indexHome(t, nil, []string{"team"})
	team := filepath.Join(p.FleetReposDir(), "team", "skills")
	writeIndexSkill(t, p.SkillsStore(), "notes", "notes")
	writeIndexSkill(t, team, "notes", "notes")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := idx.Ordered("notes")
	if len(got) != 2 {
		t.Fatalf("Ordered() found %d copies, want 2", len(got))
	}
	if got[0].Home != team || got[1].Home != p.SkillsStore() {
		t.Fatalf("Ordered() homes = [%q %q], want team then store", got[0].Home, got[1].Home)
	}
}

func TestLookupMatchesDirBeforeName(t *testing.T) {
	p := indexHome(t, nil, nil)
	// Frontmatter name differs from directory: dir "renamed", name "notes".
	writeIndexSkill(t, p.SkillsStore(), "renamed", "notes")
	writeIndexSkill(t, p.FleetHomeSkills(), "notes", "other")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := idx.Lookup("notes")
	if len(got) != 2 {
		t.Fatalf("Lookup() found %d hits, want 2", len(got))
	}
	if got[0].Skill.Dir != "notes" {
		t.Errorf("Lookup()[0].Dir = %q, want the directory match first", got[0].Skill.Dir)
	}
	if got[1].Skill.Dir != "renamed" {
		t.Errorf("Lookup()[1].Dir = %q, want the frontmatter-name match second", got[1].Skill.Dir)
	}
}

func TestLoadDedupesOverlappingHomes(t *testing.T) {
	// An explicit repo root at the fleet home itself collects to the
	// fallback dir: one home, not two.
	p := indexHome(t, nil, nil)
	fleetRoot := filepath.Dir(p.FleetHomeSkills())
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos([]string{fleetRoot})
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{p.FleetHomeSkills(), p.SkillsStore()}
	if !equalStrings(idx.Homes(), want) {
		t.Fatalf("Homes() = %q, want %q", idx.Homes(), want)
	}
	if len(idx.CustomHomes()) != 1 {
		t.Fatalf("CustomHomes() = %q, want exactly the deduped fallback", idx.CustomHomes())
	}
}

func TestLoadReportsPerHomeErrors(t *testing.T) {
	p := indexHome(t, nil, []string{"team"})
	// A regular file where a collection dir should be: ReadDir fails.
	blocker := filepath.Join(p.FleetReposDir(), "team", "skills")
	if err := os.MkdirAll(filepath.Dir(blocker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, errs, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v, want only a per-home error", err)
	}
	if _, ok := errs[blocker]; !ok {
		t.Fatalf("errs = %v, want an entry for %q", errs, blocker)
	}
	// The failed home stays listed with no skills.
	found := false
	for _, h := range idx.Homes() {
		if h == blocker {
			found = true
		}
	}
	if !found {
		t.Errorf("Homes() = %q, want failed home %q still listed", idx.Homes(), blocker)
	}
	if len(idx.Skills(blocker)) != 0 {
		t.Errorf("Skills(failed) = %v, want empty", idx.Skills(blocker))
	}
}
