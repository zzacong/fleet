package snapshot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/outdated"
	"github.com/zzacong/fleet/internal/paths"
)

func writeSkill(t *testing.T, store, dir, name, description string) {
	t.Helper()
	path := filepath.Join(store, dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\n"
	if name != "" {
		body += "name: " + name + "\n"
	}
	if description != "" {
		body += "description: " + description + "\n"
	}
	body += "---\n\n# " + dir + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fakeTrees struct {
	calls []string
	trees map[string]outdated.TreeResponse
	errs  map[string]error
}

func (f *fakeTrees) FetchTree(_ context.Context, owner, repo, ref, _ string) (outdated.TreeResponse, error) {
	key := owner + "/" + repo + "@" + ref
	f.calls = append(f.calls, key)
	if err := f.errs[key]; err != nil {
		return outdated.TreeResponse{}, err
	}
	return f.trees[key], nil
}

func (f *fakeTrees) scriptTree(key string, entries ...outdated.TreeEntry) {
	if f.trees == nil {
		f.trees = map[string]outdated.TreeResponse{}
	}
	f.trees[key] = outdated.TreeResponse{Entries: entries}
}

func TestSnapshotCanonicalOnly(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	writeSkill(t, p.SkillsStore(), "canon-skill", "canon-skill", "canonical description")
	// No lock entry => custom
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("skills = %d, want 1: %+v", len(report.Skills), report.Skills)
	}
	row := report.Skills[0]
	if row.Name != "canon-skill" {
		t.Errorf("Name = %q, want canon-skill", row.Name)
	}
	if !row.Custom {
		t.Errorf("canon-skill without lock should be custom")
	}
	if row.Description != "canonical description" {
		t.Errorf("Description = %q, want canonical description", row.Description)
	}
	if row.Outdated != nil {
		t.Errorf("Outdated = %v, want nil (unknown) for custom", *row.Outdated)
	}
	if len(trees.calls) != 0 {
		t.Errorf("custom should not hit API, calls = %v", trees.calls)
	}
	// JSON should carry custom and null outdated
	data, _ := json.Marshal(report.Skills[0])
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["outdated"]) != "null" {
		t.Errorf("outdated json = %s, want null", raw["outdated"])
	}
}

func TestSnapshotFleetHomeOnly(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	writeSkill(t, p.FleetHomeSkills(), "fleet-skill", "fleet-skill", "fleet description")
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(report.Skills))
	}
	row := report.Skills[0]
	if row.Name != "fleet-skill" {
		t.Errorf("Name = %q", row.Name)
	}
	if !row.Custom {
		t.Errorf("fleet-home skill should be custom")
	}
	if row.Description != "fleet description" {
		t.Errorf("Description = %q", row.Description)
	}
	if row.Outdated != nil {
		t.Errorf("fleet custom outdated should be nil")
	}
	if len(trees.calls) != 0 {
		t.Errorf("fleet custom should not hit API, calls = %v", trees.calls)
	}
}

func TestSnapshotFleetHomeFrontmatterFallback(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	// No frontmatter name, fallback to dir name
	writeSkill(t, p.FleetHomeSkills(), "fallback-dir", "", "from fleet home")
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 || report.Skills[0].Name != "fallback-dir" {
		t.Fatalf("frontmatter fallback failed: %+v", report.Skills)
	}
	if !report.Skills[0].Custom {
		t.Errorf("fallback skill should be custom")
	}
}

func TestSnapshotExplicitRepoOnly(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicit := t.TempDir()
	writeExplicitRepos(t, p, explicit)
	writeSkill(t, filepath.Join(explicit, "skills"), "repo-skill", "repo-skill", "repo description")
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(report.Skills))
	}
	row := report.Skills[0]
	if row.Name != "repo-skill" {
		t.Errorf("Name = %q", row.Name)
	}
	if !row.Custom {
		t.Errorf("repo skill should be custom")
	}
	if row.Outdated != nil {
		t.Errorf("repo custom outdated should be nil")
	}
}

func TestSnapshotMissingDirsNotError(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	// No dirs at all
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build with missing dirs should not error: %v", err)
	}
	if len(report.Skills) != 0 {
		t.Errorf("skills = %v, want empty", report.Skills)
	}
	// Also with an explicit entry whose collection dir is missing
	explicit := t.TempDir()
	writeExplicitRepos(t, p, explicit)
	report, _, err = Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build with missing explicit collection dir should not error: %v", err)
	}
	if len(report.Skills) != 0 {
		t.Errorf("skills = %v, want empty", report.Skills)
	}
}

func TestSnapshotTwoWayCanonicalFleetCollision(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	writeSkill(t, p.SkillsStore(), "shared-dir", "shared", "canonical desc")
	writeSkill(t, p.FleetHomeSkills(), "shared-fleet-dir", "shared", "fleet desc")
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("collision should dedupe to 1, got %d: %+v", len(report.Skills), report.Skills)
	}
	row := report.Skills[0]
	if row.Name != "shared" {
		t.Errorf("Name = %q", row.Name)
	}
	if row.Description != "fleet desc" {
		t.Errorf("Description = %q, want fleet desc (fleet > canonical)", row.Description)
	}
	if !row.Custom {
		t.Errorf("fleet winner should be custom")
	}
	if row.Outdated != nil {
		t.Errorf("fleet custom outdated should be nil")
	}
	if len(trees.calls) != 0 {
		t.Errorf("custom collision should not hit API")
	}
}

func TestSnapshotTwoWayCanonicalExplicitCollision(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicit := t.TempDir()
	writeExplicitRepos(t, p, explicit)
	writeSkill(t, p.SkillsStore(), "shared-dir", "shared", "canonical desc")
	writeSkill(t, filepath.Join(explicit, "skills"), "shared-repo-dir", "shared", "repo desc")
	// Add a lock entry for canonical that would otherwise make it installed
	lock := `{"version":3,"skills":{"shared-dir":{"source":"a/b","sourceType":"github","skillFolderHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","skillPath":"skills/shared/SKILL.md"}}}`
	if err := os.MkdirAll(p.AgentsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("collision dedupe failed: %+v", report.Skills)
	}
	row := report.Skills[0]
	if row.Description != "repo desc" {
		t.Errorf("Description = %q, want repo desc (repo > canonical)", row.Description)
	}
	if !row.Custom {
		t.Errorf("repo winner should be custom despite stale lock")
	}
	if row.Source != "" {
		t.Errorf("repo custom should have empty source, got %q", row.Source)
	}
	if row.Outdated != nil {
		t.Errorf("repo custom outdated should be nil")
	}
	if len(trees.calls) != 0 {
		t.Errorf("repo custom should not hit API, calls = %v", trees.calls)
	}
}

func TestSnapshotTwoWayFleetExplicitCollision(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicit := t.TempDir()
	writeExplicitRepos(t, p, explicit)
	writeSkill(t, p.FleetHomeSkills(), "shared-fleet-dir", "shared", "fleet desc")
	writeSkill(t, filepath.Join(explicit, "skills"), "shared-repo-dir", "shared", "repo desc")
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("fleet+repo collision dedupe failed: %+v", report.Skills)
	}
	row := report.Skills[0]
	if row.Description != "repo desc" {
		t.Errorf("Description = %q, want repo desc (repo > fleet)", row.Description)
	}
	if !row.Custom {
		t.Errorf("repo winner should be custom")
	}
}

func TestSnapshotThreeWayCollision(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicit := t.TempDir()
	writeExplicitRepos(t, p, explicit)
	writeSkill(t, p.SkillsStore(), "shared-canonical-dir", "shared", "canonical desc")
	writeSkill(t, p.FleetHomeSkills(), "shared-fleet-dir", "shared", "fleet desc")
	writeSkill(t, filepath.Join(explicit, "skills"), "shared-repo-dir", "shared", "repo desc")
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("three-way collision dedupe failed: %+v", report.Skills)
	}
	row := report.Skills[0]
	if row.Description != "repo desc" {
		t.Errorf("Description = %q, want repo desc (repo > fleet > canonical)", row.Description)
	}
	if !row.Custom {
		t.Errorf("repo winner should be custom")
	}
	if row.Outdated != nil {
		t.Errorf("custom outdated should be nil")
	}
}

func TestSnapshotWithoutRepoShowsCanonicalPlusFleet(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home) // no repo
	writeSkill(t, p.SkillsStore(), "canon-a", "canon-a", "canon")
	writeSkill(t, p.FleetHomeSkills(), "fleet-b", "fleet-b", "fleet")
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 2 {
		t.Fatalf("without repo should show canonical plus fleet, got %d: %+v", len(report.Skills), report.Skills)
	}
	byName := map[string]bool{}
	for _, r := range report.Skills {
		byName[r.Name] = true
		if r.Outdated != nil {
			t.Errorf("%s outdated should be nil (custom)", r.Name)
		}
		if !r.Custom {
			t.Errorf("%s should be custom (no lock)", r.Name)
		}
	}
	if !byName["canon-a"] || !byName["fleet-b"] {
		t.Errorf("missing skills: %+v", byName)
	}
}

func TestSnapshotInstalledVsCustomOutdated(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicit := t.TempDir()
	writeExplicitRepos(t, p, explicit)
	// installed skill in canonical with lock
	writeSkill(t, p.SkillsStore(), "installed", "installed", "installed desc")
	lock := `{"version":3,"skills":{"installed":{"source":"owner/repo","sourceType":"github","skillPath":"skills/installed/SKILL.md","skillFolderHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","ref":""}} }`
	if err := os.MkdirAll(p.AgentsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	// custom in fleet
	writeSkill(t, p.FleetHomeSkills(), "custom-fleet", "custom-fleet", "fleet custom")
	// custom in the explicit tracked repo
	writeSkill(t, filepath.Join(explicit, "skills"), "custom-repo", "custom-repo", "repo custom")
	// non-github installed
	writeSkill(t, p.SkillsStore(), "local-skill", "local-skill", "local")
	lock2 := `{"version":3,"skills":{
		"installed":{"source":"owner/repo","sourceType":"github","skillPath":"skills/installed/SKILL.md","skillFolderHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"local-skill":{"source":"my/local","sourceType":"local","skillPath":"skills/local/SKILL.md","skillFolderHash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock2), 0o644); err != nil {
		t.Fatal(err)
	}
	trees := &fakeTrees{}
	trees.scriptTree("owner/repo@",
		outdated.TreeEntry{Path: "skills/installed", Type: "tree", SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	)
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	byName := map[string]*bool{}
	for i := range report.Skills {
		byName[report.Skills[i].Name] = report.Skills[i].Outdated
	}
	// installed github current => false
	if v := byName["installed"]; v == nil || *v != false {
		t.Errorf("installed outdated = %v, want false (current)", v)
	}
	// customs => nil
	if v := byName["custom-fleet"]; v != nil {
		t.Errorf("custom-fleet outdated = %v, want nil", *v)
	}
	if v := byName["custom-repo"]; v != nil {
		t.Errorf("custom-repo outdated = %v, want nil", *v)
	}
	// non-github => nil
	if v := byName["local-skill"]; v != nil {
		t.Errorf("local-skill outdated = %v, want nil (non-github)", *v)
	}
	// customs should have zero API grouping (only installed counted)
	if len(trees.calls) != 1 {
		t.Errorf("API calls = %v, want 1 for owner/repo", trees.calls)
	}
	// rows should have custom true for fleet/repo and for canonical without lock? But installed has lock so custom false
	for _, r := range report.Skills {
		switch r.Name {
		case "installed":
			if r.Custom {
				t.Errorf("installed should not be custom")
			}
			if r.Source != "owner/repo" {
				t.Errorf("installed source = %q", r.Source)
			}
		case "custom-fleet", "custom-repo":
			if !r.Custom {
				t.Errorf("%s should be custom", r.Name)
			}
		case "local-skill":
			// local-skill has lock entry but non-github => still not custom? The row marks custom based on lock presence, not sourceType. So local-skill with lock is not custom, even though outdated unknown.
			// But spec: outdated unknown for non-Github sources, not custom flag.
			if r.Custom {
				t.Errorf("local-skill with lock should not be custom, outdated handles tri-state")
			}
		}
	}
}

func TestSnapshotSortingCustomFirst(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	writeSkill(t, p.SkillsStore(), "installed-a", "installed-a", "a")
	writeSkill(t, p.SkillsStore(), "installed-b", "installed-b", "b")
	writeSkill(t, p.FleetHomeSkills(), "custom-x", "custom-x", "x")
	lock := `{"version":3,"skills":{
		"installed-a":{"source":"owner/repo","sourceType":"github","skillFolderHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","skillPath":"skills/installed-a/SKILL.md"},
		"installed-b":{"source":"owner/repo","sourceType":"github","skillFolderHash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","skillPath":"skills/installed-b/SKILL.md"}
	}}`
	if err := os.MkdirAll(p.AgentsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 3 {
		t.Fatalf("skills = %d", len(report.Skills))
	}
	if !report.Skills[0].Custom {
		t.Errorf("first skill should be custom, got %q custom=%v", report.Skills[0].Name, report.Skills[0].Custom)
	}
	if report.Skills[0].Name != "custom-x" {
		t.Errorf("first skill = %q, want custom-x", report.Skills[0].Name)
	}
}

func TestSnapshotJSONCustomAndNullOutdated(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicit := t.TempDir()
	writeExplicitRepos(t, p, explicit)
	writeSkill(t, p.FleetHomeSkills(), "fleet-custom", "fleet-custom", "fleet")
	writeSkill(t, filepath.Join(explicit, "skills"), "repo-custom", "repo-custom", "repo")
	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Skills []map[string]json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	for _, s := range parsed.Skills {
		var name string
		if err := json.Unmarshal(s["name"], &name); err != nil {
			t.Fatal(err)
		}
		var custom bool
		if err := json.Unmarshal(s["custom"], &custom); err != nil {
			t.Errorf("%s missing custom", name)
		}
		if !custom {
			t.Errorf("%s custom = false, want true", name)
		}
		if got := string(s["outdated"]); got != "null" {
			t.Errorf("%s outdated = %s, want null", name, got)
		}
	}
}

// writeExplicitRepos records the explicit repo-root list in the fake home's
// config file, in precedence order.
func writeExplicitRepos(t *testing.T, p *paths.Paths, roots ...string) {
	t.Helper()
	t.Setenv("FLEET_REPO", "")
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos(roots)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
}

// checkoutSkills returns the skills/ collection dir of one auto-tracked
// fleet-home checkout slot.
func checkoutSkills(p *paths.Paths, name string) string {
	return filepath.Join(p.FleetReposDir(), name, "skills")
}

func TestSnapshotExplicitListOrderBeatsCheckout(t *testing.T) {
	// The first explicit entry wins over a later explicit entry, the
	// auto-tracked checkouts, the fallback, and the canonical store.
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicitA := t.TempDir()
	explicitB := t.TempDir()
	writeExplicitRepos(t, p, explicitA, explicitB)
	writeSkill(t, p.SkillsStore(), "canon-dir", "shared", "canonical desc")
	writeSkill(t, p.FleetHomeSkills(), "fallback-dir", "shared", "fallback desc")
	writeSkill(t, checkoutSkills(p, "alpha"), "checkout-dir", "shared", "checkout desc")
	writeSkill(t, filepath.Join(explicitB, "skills"), "explicit-dir", "shared", "explicit-b desc")
	writeSkill(t, filepath.Join(explicitA, "skills"), "repo-dir", "shared", "explicit-a desc")

	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("collision should dedupe to 1, got %d: %+v", len(report.Skills), report.Skills)
	}
	row := report.Skills[0]
	if row.Description != "explicit-a desc" {
		t.Errorf("Description = %q, want explicit-a desc (first explicit entry wins)", row.Description)
	}
	if !row.Custom {
		t.Errorf("explicit winner should be custom")
	}
	if row.Outdated != nil {
		t.Errorf("explicit custom outdated should be nil")
	}
	if len(trees.calls) != 0 {
		t.Errorf("custom collision should not hit API, calls = %v", trees.calls)
	}
}

func TestSnapshotTrackedSetUnionListsEverySource(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicitA := t.TempDir()
	explicitB := t.TempDir()
	writeExplicitRepos(t, p, explicitA, explicitB)
	writeSkill(t, p.SkillsStore(), "canon-only", "canon-only", "canonical")
	writeSkill(t, p.FleetHomeSkills(), "fallback-only", "fallback-only", "fallback")
	writeSkill(t, filepath.Join(explicitA, "skills"), "explicit-a-only", "explicit-a-only", "explicit a")
	writeSkill(t, filepath.Join(explicitB, "skills"), "explicit-b-only", "explicit-b-only", "explicit b")
	writeSkill(t, checkoutSkills(p, "zeta"), "checkout-only", "checkout-only", "checkout")

	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(report.Skills) != 5 {
		t.Fatalf("skills = %d, want one row per source: %+v", len(report.Skills), report.Skills)
	}
	for _, r := range report.Skills {
		if !r.Custom {
			t.Errorf("%s should be custom (no lockfile provenance for customs)", r.Name)
		}
		if r.Outdated != nil {
			t.Errorf("%s outdated should be nil (unknown by definition)", r.Name)
		}
		if r.Source != "" {
			t.Errorf("%s source = %q, want empty (customs carry no provenance)", r.Name, r.Source)
		}
	}
	if len(trees.calls) != 0 {
		t.Errorf("all-custom union should not hit API, calls = %v", trees.calls)
	}
}

func TestSnapshotTrackedSetPrecedenceWinnerOnly(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	explicitA := t.TempDir()
	explicitB := t.TempDir()
	writeExplicitRepos(t, p, explicitA, explicitB)
	expA := filepath.Join(explicitA, "skills")
	expB := filepath.Join(explicitB, "skills")
	alpha := checkoutSkills(p, "alpha")
	zeta := checkoutSkills(p, "zeta")

	// One name in every source: the first explicit entry wins.
	writeSkill(t, p.SkillsStore(), "canon-all", "all-four", "canonical desc")
	writeSkill(t, p.FleetHomeSkills(), "fallback-all", "all-four", "fallback desc")
	writeSkill(t, alpha, "alpha-all", "all-four", "alpha desc")
	writeSkill(t, zeta, "zeta-all", "all-four", "zeta desc")
	writeSkill(t, expB, "expb-all", "all-four", "explicit-b desc")
	writeSkill(t, expA, "expa-all", "all-four", "explicit-a desc")
	// Explicit list order beats everything below it.
	writeSkill(t, expB, "expb-order", "list-order", "explicit-b desc")
	writeSkill(t, expA, "expa-order", "list-order", "explicit-a desc")
	// Explicit beats checkouts, fallback, and canonical.
	writeSkill(t, p.SkillsStore(), "canon-exp", "explicit-wins", "canonical desc")
	writeSkill(t, p.FleetHomeSkills(), "fallback-exp", "explicit-wins", "fallback desc")
	writeSkill(t, alpha, "alpha-exp", "explicit-wins", "alpha desc")
	writeSkill(t, expB, "expb-exp", "explicit-wins", "explicit-b desc")
	// Alphabetical checkout beats fallback and canonical.
	writeSkill(t, p.SkillsStore(), "canon-co", "checkout-wins", "canonical desc")
	writeSkill(t, p.FleetHomeSkills(), "fallback-co", "checkout-wins", "fallback desc")
	writeSkill(t, zeta, "zeta-co", "checkout-wins", "zeta desc")
	writeSkill(t, alpha, "alpha-co", "checkout-wins", "alpha desc")
	// Fallback beats canonical.
	writeSkill(t, p.SkillsStore(), "canon-fb", "fallback-wins", "canonical desc")
	writeSkill(t, p.FleetHomeSkills(), "fallback-fb", "fallback-wins", "fallback desc")

	trees := &fakeTrees{}
	report, _, err := Build(context.Background(), p, trees)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	byDesc := map[string]string{}
	counts := map[string]int{}
	for _, r := range report.Skills {
		counts[r.Name]++
		byDesc[r.Name] = r.Description
		if !r.Custom {
			t.Errorf("%s should be custom", r.Name)
		}
		if r.Outdated != nil {
			t.Errorf("%s outdated should be nil", r.Name)
		}
	}
	want := map[string]string{
		"all-four":      "explicit-a desc",
		"list-order":    "explicit-a desc",
		"explicit-wins": "explicit-b desc",
		"checkout-wins": "alpha desc",
		"fallback-wins": "fallback desc",
	}
	if len(report.Skills) != len(want) {
		t.Fatalf("skills = %d, want winner-only rows %d: %+v", len(report.Skills), len(want), report.Skills)
	}
	for name, desc := range want {
		if counts[name] != 1 {
			t.Errorf("%s appears %d times, want winner-only", name, counts[name])
		}
		if byDesc[name] != desc {
			t.Errorf("%s description = %q, want %q", name, byDesc[name], desc)
		}
	}
	if len(trees.calls) != 0 {
		t.Errorf("all-custom collisions should not hit API, calls = %v", trees.calls)
	}
}
