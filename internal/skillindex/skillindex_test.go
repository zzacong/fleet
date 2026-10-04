package skillindex

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// indexHome builds a fake home with the given collection dirs recorded in
// the config's skillsDirs list.
func indexHome(t *testing.T, collections ...string) *paths.Paths {
	t.Helper()
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	if len(collections) > 0 {
		writeSkillsDirs(t, p, collections...)
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
// home's config file.
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
	p := indexHome(t)
	collection := filepath.Join(t.TempDir(), "collection")
	// The tracked path is the collection: an immediate child with a
	// SKILL.md is a skill.
	writeIndexSkill(t, collection, "alpha", "alpha")
	// A `skills/` subdir is NOT derived: a skill nested under
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

func TestCustomHomesOrdersSkillsDirsThenFallback(t *testing.T) {
	dirA := filepath.Join(t.TempDir(), "collection-a")
	dirB := filepath.Join(t.TempDir(), "collection-b")
	p := indexHome(t, dirA, dirB)

	got, err := CustomHomes(p)
	if err != nil {
		t.Fatalf("CustomHomes() error = %v", err)
	}
	want := []string{dirA, dirB, p.FleetHomeSkills()}
	if !equalStrings(got, want) {
		t.Fatalf("CustomHomes() = %q, want %q", got, want)
	}
}

func TestCustomHomesFallbackOnlyWhenNothingTracked(t *testing.T) {
	p := indexHome(t)

	got, err := CustomHomes(p)
	if err != nil {
		t.Fatalf("CustomHomes() error = %v", err)
	}
	if len(got) != 1 || got[0] != p.FleetHomeSkills() {
		t.Fatalf("CustomHomes() = %q, want [%q]", got, p.FleetHomeSkills())
	}
}

func TestSkillsDirsPrecedeFallbackAndStoreOnNameCollision(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "collection")
	p := indexHome(t, collection)

	writeIndexSkill(t, p.SkillsStore(), "canon", "clash")
	writeIndexSkill(t, p.FleetHomeSkills(), "fleet", "clash")
	writeIndexSkill(t, collection, "explicit", "clash")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	hits := idx.Lookup("clash")
	if len(hits) != 3 {
		t.Fatalf("Lookup(clash) = %d copies, want 3: %+v", len(hits), hits)
	}
	wantHomes := []string{collection, p.FleetHomeSkills(), p.SkillsStore()}
	for i, want := range wantHomes {
		if hits[i].Home != want {
			t.Errorf("Lookup(clash)[%d].Home = %q, want %q", i, hits[i].Home, want)
		}
	}
	if hits[0].Skill.Dir != "explicit" {
		t.Errorf("winner dir = %q, want explicit (skillsDirs first)", hits[0].Skill.Dir)
	}
}

func TestMissingSkillsDirScansEmptyWithoutError(t *testing.T) {
	p := indexHome(t)
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

func TestLoadHomesEndsWithFallbackThenStore(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "team")
	p := indexHome(t, collection)
	writeIndexSkill(t, p.FleetHomeSkills(), "mine", "mine")

	idx, errs, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("Load() errs = %v, want none", errs)
	}
	want := []string{collection, p.FleetHomeSkills(), p.SkillsStore()}
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
	collection := filepath.Join(t.TempDir(), "team")
	p := indexHome(t, collection)
	writeIndexSkill(t, p.SkillsStore(), "notes", "notes")
	writeIndexSkill(t, collection, "notes", "notes")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := idx.Ordered("notes")
	if len(got) != 2 {
		t.Fatalf("Ordered() found %d copies, want 2", len(got))
	}
	if got[0].Home != collection || got[1].Home != p.SkillsStore() {
		t.Fatalf("Ordered() homes = [%q %q], want collection then store", got[0].Home, got[1].Home)
	}
}

func TestLookupMatchesDirBeforeName(t *testing.T) {
	p := indexHome(t)
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
	// A skillsDirs entry that is the fleet-home fallback itself is one
	// home, not two.
	p := indexHome(t)
	writeSkillsDirs(t, p, p.FleetHomeSkills())

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
	// A regular file where a collection dir should be: ReadDir fails.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := indexHome(t, blocker)

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
	p := indexHome(t)
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
	p := indexHome(t)
	writeIndexSkill(t, p.FleetHomeSkills(), "custom", "custom")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Complete() {
		t.Fatalf("Complete() = true, want false: the canonical store is absent")
	}
}

func TestPerHomeScanErrorMakesScanIncomplete(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := indexHome(t, blocker)
	writeIndexSkill(t, p.SkillsStore(), "alpha", "alpha")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Complete() {
		t.Fatalf("Complete() = true, want false: a home failed to scan")
	}
}

func TestInstalledNamesSpanEveryHome(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "collection")
	p := indexHome(t, collection)
	writeIndexSkill(t, p.SkillsStore(), "store-dir", "store-name")
	writeIndexSkill(t, collection, "explicit-dir", "explicit-name")
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
		"store-dir", "store-name",
	}
	if !equalStrings(idx.InstalledNames(), want) {
		t.Errorf("InstalledNames() = %q, want %q", idx.InstalledNames(), want)
	}
}

func TestBlockedHomesNameScanError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := indexHome(t, blocker)
	writeIndexSkill(t, p.SkillsStore(), "alpha", "alpha")

	idx, _, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Complete() {
		t.Fatal("Complete() = true, want false")
	}
	want := []string{blocker}
	sort.Strings(want)
	if !equalStrings(idx.BlockedHomes(), want) {
		t.Fatalf("BlockedHomes() = %q, want %q", idx.BlockedHomes(), want)
	}
}

func TestBlockedHomesEmptyWhenCompleteOrOnlyStoreMissing(t *testing.T) {
	// A complete scan has no blocker.
	p := indexHome(t)
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
	empty := indexHome(t)
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
