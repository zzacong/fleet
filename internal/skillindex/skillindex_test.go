package skillindex

import (
	"os"
	"path/filepath"
	"sort"
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

// writeSkillsDirs records the explicit collection-dir list in the fake
// home's config file, preserving the legacy list when one is present.
func writeSkillsDirs(t *testing.T, p *paths.Paths, dirs ...string) {
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

func TestSkillsDirsScannedDirectlyNoSubdirDerivation(t *testing.T) {
	p := indexHome(t, nil, nil)
	collection := filepath.Join(t.TempDir(), "collection")
	// The tracked path is the collection: an immediate child with a
	// SKILL.md is a skill.
	writeIndexSkill(t, collection, "alpha", "alpha")
	// A legacy-style `skills/` subdir is NOT derived: a skill nested under
	// collection/skills is invisible.
	writeIndexSkill(t, filepath.Join(collection, "skills"), "nested", "nested")
	writeSkillsDirs(t, p, collection)

	idx, errs, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("Load() errs = %v, want none", errs)
	}
	if !idx.IsInstalled("alpha") {
		t.Errorf("IsInstalled(alpha) = false, want true: immediate child of the collection")
	}
	if idx.IsInstalled("nested") {
		t.Errorf("IsInstalled(nested) = true, want false: no `skills/` derivation")
	}
	if got := idx.Skills(collection); len(got) != 1 || got[0].Dir != "alpha" {
		t.Errorf("Skills(collection) = %+v, want just alpha", got)
	}
}

func TestCustomHomesOrdersSkillsDirsBeforeLegacySources(t *testing.T) {
	p := indexHome(t, []string{filepath.Join(t.TempDir(), "legacy")}, []string{"zeta"})
	dirA := filepath.Join(t.TempDir(), "collection-a")
	dirB := filepath.Join(t.TempDir(), "collection-b")
	writeSkillsDirs(t, p, dirA, dirB)

	got, err := CustomHomes(p)
	if err != nil {
		t.Fatalf("CustomHomes() error = %v", err)
	}
	// Explicit collections are scanned directly (no `skills/` join) and
	// come before the legacy repo-derived collection and the fallback.
	legacyRoot := mustLegacyRoot(t, p)
	checkout := filepath.Join(p.FleetReposDir(), "zeta", "skills")
	want := []string{dirA, dirB, filepath.Join(legacyRoot, "skills"), checkout, p.FleetHomeSkills()}
	if !equalStrings(got, want) {
		t.Fatalf("CustomHomes() = %q, want %q", got, want)
	}
}

// mustLegacyRoot returns the single legacy repo root recorded by an
// indexHome fixture (the explicit list's first entry).
func mustLegacyRoot(t *testing.T, p *paths.Paths) string {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	repos := f.SkillsRepos()
	if len(repos) != 1 {
		t.Fatalf("legacy repos = %q, want exactly one fixture root", repos)
	}
	return repos[0]
}

func TestSkillsDirsPrecedeLegacySourcesOnNameCollision(t *testing.T) {
	p := indexHome(t, []string{filepath.Join(t.TempDir(), "legacy")}, nil)
	legacyRoot := mustLegacyRoot(t, p)
	writeSkillsDirs(t, p, filepath.Join(t.TempDir(), "collection"))
	collection := mustSkillsDir(t, p, 0)

	writeIndexSkill(t, p.SkillsStore(), "canon", "clash")
	writeIndexSkill(t, p.FleetHomeSkills(), "fleet", "clash")
	writeIndexSkill(t, filepath.Join(legacyRoot, "skills"), "legacy", "clash")
	writeIndexSkill(t, collection, "explicit", "clash")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	hits := idx.Lookup("clash")
	if len(hits) != 4 {
		t.Fatalf("Lookup(clash) = %d copies, want 4: %+v", len(hits), hits)
	}
	wantHomes := []string{
		collection,
		filepath.Join(legacyRoot, "skills"),
		p.FleetHomeSkills(),
		p.SkillsStore(),
	}
	for i, want := range wantHomes {
		if hits[i].Home != want {
			t.Errorf("Lookup(clash)[%d].Home = %q, want %q", i, hits[i].Home, want)
		}
	}
	if hits[0].Skill.Dir != "explicit" {
		t.Errorf("winner dir = %q, want explicit (skillsDirs first)", hits[0].Skill.Dir)
	}
}

// mustSkillsDir returns the i-th explicit collection dir recorded in the
// fake home's config.
func mustSkillsDir(t *testing.T, p *paths.Paths, i int) string {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	dirs := f.SkillsDirs()
	if i >= len(dirs) {
		t.Fatalf("skillsDirs = %q, want index %d", dirs, i)
	}
	return dirs[i]
}

func TestMissingSkillsDirScansEmptyWithoutError(t *testing.T) {
	p := indexHome(t, nil, nil)
	writeIndexSkill(t, p.SkillsStore(), "canon", "canon")
	missing := filepath.Join(t.TempDir(), "gone")
	writeSkillsDirs(t, p, missing)

	idx, errs, err := Load(p)
	if err != nil {
		t.Fatalf("Load() with a missing skillsDirs entry should not error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("Load() errs = %v, want none for a missing collection", errs)
	}
	if got := idx.Skills(missing); len(got) != 0 {
		t.Errorf("Skills(missing) = %+v, want empty", got)
	}
	found := false
	for _, h := range idx.Homes() {
		if h == missing {
			found = true
		}
	}
	if !found {
		t.Errorf("Homes() = %q, want the missing collection still listed", idx.Homes())
	}
	if !idx.Complete() {
		t.Errorf("Complete() = false, want true: a missing optional skillsDirs entry is not a blocker")
	}
	if len(idx.BlockedHomes()) != 0 {
		t.Errorf("BlockedHomes() = %q, want none", idx.BlockedHomes())
	}
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

func TestCompleteWhenStoreScansAndFallbackMissing(t *testing.T) {
	p := indexHome(t, nil, nil)
	writeIndexSkill(t, p.SkillsStore(), "alpha", "alpha")

	idx, errs, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("Load() errs = %v, want none", errs)
	}
	if !idx.Complete() {
		t.Fatalf("Complete() = false, want true: the store scans and the absent fallback is optional")
	}
}

func TestMissingStoreMakesScanIncomplete(t *testing.T) {
	p := indexHome(t, nil, nil)
	writeIndexSkill(t, p.FleetHomeSkills(), "custom", "custom")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Complete() {
		t.Fatalf("Complete() = true, want false: the canonical store is absent")
	}
}

func TestMissingTrackedRepoRootMakesScanIncomplete(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "moved-repo")
	p := indexHome(t, []string{missing}, nil)
	writeIndexSkill(t, p.SkillsStore(), "alpha", "alpha")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Complete() {
		t.Fatalf("Complete() = true, want false: tracked repo root %q is missing", missing)
	}
}

func TestPerHomeScanErrorMakesScanIncomplete(t *testing.T) {
	p := indexHome(t, nil, []string{"team"})
	writeIndexSkill(t, p.SkillsStore(), "alpha", "alpha")
	blocker := filepath.Join(p.FleetReposDir(), "team", "skills")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Complete() {
		t.Fatalf("Complete() = true, want false: a home failed to scan")
	}
}

func TestInstalledNamesSpanEveryHome(t *testing.T) {
	explicitRoot := filepath.Join(t.TempDir(), "explicit-repo")
	p := indexHome(t, []string{explicitRoot}, []string{"team"})
	explicit := filepath.Join(explicitRoot, "skills")
	team := filepath.Join(p.FleetReposDir(), "team", "skills")
	writeIndexSkill(t, p.SkillsStore(), "store-dir", "store-name")
	writeIndexSkill(t, explicit, "explicit-dir", "explicit-name")
	writeIndexSkill(t, team, "repo-dir", "repo-name")
	writeIndexSkill(t, p.FleetHomeSkills(), "fb-dir", "fb-name")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !idx.Complete() {
		t.Fatalf("Complete() = false, want true: every home is present and scans")
	}
	for _, name := range []string{
		"store-dir", "store-name",
		"explicit-dir", "explicit-name",
		"repo-dir", "repo-name",
		"fb-dir", "fb-name",
	} {
		if !idx.IsInstalled(name) {
			t.Errorf("IsInstalled(%q) = false, want true", name)
		}
	}
	if idx.IsInstalled("absent") {
		t.Errorf("IsInstalled(absent) = true, want false")
	}
	want := []string{
		"explicit-dir", "explicit-name",
		"fb-dir", "fb-name",
		"repo-dir", "repo-name",
		"store-dir", "store-name",
	}
	if !equalStrings(idx.InstalledNames(), want) {
		t.Errorf("InstalledNames() = %q, want %q", idx.InstalledNames(), want)
	}
}

func TestBlockedHomesNameMissingRootAndScanError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "moved-repo")
	p := indexHome(t, []string{missing}, []string{"team"})
	writeIndexSkill(t, p.SkillsStore(), "alpha", "alpha")
	// team's collection is a regular file, not a dir: the scan fails.
	blocker := filepath.Join(p.FleetReposDir(), "team", "skills")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Complete() {
		t.Fatal("Complete() = true, want false")
	}
	want := []string{filepath.Join(missing, "skills"), blocker}
	sort.Strings(want)
	if !equalStrings(idx.BlockedHomes(), want) {
		t.Fatalf("BlockedHomes() = %q, want %q", idx.BlockedHomes(), want)
	}
}

func TestBlockedHomesEmptyWhenCompleteOrOnlyStoreMissing(t *testing.T) {
	// A complete scan has no blocker.
	p := indexHome(t, nil, nil)
	writeIndexSkill(t, p.SkillsStore(), "alpha", "alpha")
	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(idx.BlockedHomes()) != 0 {
		t.Fatalf("BlockedHomes() = %q, want none for a complete scan", idx.BlockedHomes())
	}

	// A missing canonical store makes the scan incomplete, but doctor
	// reports it as a missing directory, not as a blocked home.
	empty := indexHome(t, nil, nil)
	idx2, _, err := Load(empty)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx2.Complete() {
		t.Fatal("Complete() = true, want false for an absent store")
	}
	if len(idx2.BlockedHomes()) != 0 {
		t.Fatalf("BlockedHomes() = %q, want none: the store is reported separately", idx2.BlockedHomes())
	}
}
