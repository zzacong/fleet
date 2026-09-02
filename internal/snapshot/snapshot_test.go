package snapshot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

func TestSnapshotSkillsRepoOnly(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	p := paths.WithRepo(home, repo)
	writeSkill(t, p.RepoSkills(), "repo-skill", "repo-skill", "repo description")
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
	// Also with repo set but repo skills dir missing
	repo := t.TempDir()
	p2 := paths.WithRepo(home, repo)
	report, _, err = Build(context.Background(), p2, trees)
	if err != nil {
		t.Fatalf("Build with missing repo skills dir should not error: %v", err)
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

func TestSnapshotTwoWayCanonicalRepoCollision(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	p := paths.WithRepo(home, repo)
	writeSkill(t, p.SkillsStore(), "shared-dir", "shared", "canonical desc")
	writeSkill(t, p.RepoSkills(), "shared-repo-dir", "shared", "repo desc")
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

func TestSnapshotTwoWayFleetRepoCollision(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	p := paths.WithRepo(home, repo)
	writeSkill(t, p.FleetHomeSkills(), "shared-fleet-dir", "shared", "fleet desc")
	writeSkill(t, p.RepoSkills(), "shared-repo-dir", "shared", "repo desc")
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
	repo := t.TempDir()
	p := paths.WithRepo(home, repo)
	writeSkill(t, p.SkillsStore(), "shared-canonical-dir", "shared", "canonical desc")
	writeSkill(t, p.FleetHomeSkills(), "shared-fleet-dir", "shared", "fleet desc")
	writeSkill(t, p.RepoSkills(), "shared-repo-dir", "shared", "repo desc")
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
	repo := t.TempDir()
	p := paths.WithRepo(home, repo)
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
	// custom in repo
	writeSkill(t, p.RepoSkills(), "custom-repo", "custom-repo", "repo custom")
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
	repo := t.TempDir()
	p := paths.WithRepo(home, repo)
	writeSkill(t, p.FleetHomeSkills(), "fleet-custom", "fleet-custom", "fleet")
	writeSkill(t, p.RepoSkills(), "repo-custom", "repo-custom", "repo")
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
