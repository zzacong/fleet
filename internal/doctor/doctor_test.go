package doctor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/state"
)

// fakeHome builds a home with the given harnesses' config directories
// present. The canonical store is not created unless the test does it.
func fakeHome(t *testing.T, harnesses ...string) *paths.Paths {
	t.Helper()
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	dirs := map[string]string{
		"opencode": p.OpenCodeDir(),
		"pi":       p.PiDir(),
		"codex":    p.CodexDir(),
		"claude":   p.ClaudeDir(),
		"cursor":   p.CursorDir(),
		"bob":      p.BobDir(),
	}
	for _, h := range harnesses {
		if err := os.MkdirAll(dirs[h], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func storeSkill(t *testing.T, p *paths.Paths, name string) {
	t.Helper()
	dir := filepath.Join(p.SkillsStore(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: test skill " + name + "\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// snapshot records every file and symlink in the home so tests can assert
// Analyze touched nothing.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(path)
			files[path] = "→ " + target
			return readErr
		}
		if info.IsDir() {
			return nil
		}
		body, readErr := os.ReadFile(path)
		files[path] = string(body)
		return readErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func kinds(rep Report) []Kind {
	var out []Kind
	for _, f := range rep.Findings {
		out = append(out, f.Kind)
	}
	return out
}

func TestAnalyzeCleanHomeIsQuiet(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "codex", "claude", "cursor", "bob")
	storeSkill(t, p, "tdd")
	// claude's link dir exists and holds nothing unexpected.
	if err := os.MkdirAll(p.ClaudeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 0 || len(rep.Conflicts) != 0 {
		t.Errorf("report = %+v, want empty", rep)
	}
}

func TestAnalyzeReportsRedundantLinksWithWhatAndWhy(t *testing.T) {
	p := fakeHome(t, "opencode", "claude")
	storeSkill(t, p, "tdd")
	symlink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.OpenCodeSkills(), "tdd"))
	// Claude's identical link is load-bearing, not redundant.
	symlink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.ClaudeSkills(), "tdd"))

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the opencode link", rep.Findings)
	}
	f := rep.Findings[0]
	if f.Kind != KindRedundantLink || f.Harness != "opencode" || f.Skill != "tdd" {
		t.Errorf("finding = %+v, want the redundant opencode link", f)
	}
	if !strings.Contains(f.Message, "tdd") || !strings.Contains(f.Message, "natively") || !strings.Contains(f.Message, "sync removes it") {
		t.Errorf("message must say what and why: %q", f.Message)
	}
}

func TestAnalyzeReportsBrokenSymlinks(t *testing.T) {
	p := fakeHome(t, "claude")
	storeSkill(t, p, "tdd")
	// The skill was uninstalled after the skills CLI linked it.
	symlink(t, filepath.Join(p.SkillsStore(), "gone"), filepath.Join(p.ClaudeSkills(), "gone"))

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("findings = %+v, want one broken link", rep.Findings)
	}
	f := rep.Findings[0]
	if f.Kind != KindBrokenLink || f.Harness != "claude" || f.Skill != "gone" {
		t.Errorf("finding = %+v", f)
	}
}

func TestAnalyzeReportsUnknownEntries(t *testing.T) {
	p := fakeHome(t, "bob", "cursor")
	storeSkill(t, p, "tdd")
	if err := os.MkdirAll(filepath.Join(p.BobSkills(), "hand-made"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A repo-pointing link (ticket 03's managed links land here) whose
	// target exists.
	repo := filepath.Join(p.Home, "dev", "fleet", "skills", "mine")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, repo, filepath.Join(p.CursorSkills(), "mine"))

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := kinds(rep); !reflect.DeepEqual(got, []Kind{KindUnknownEntry, KindUnknownEntry}) {
		t.Fatalf("findings = %+v, want two unknown entries", rep.Findings)
	}
	for _, f := range rep.Findings {
		if f.Kind != KindUnknownEntry {
			t.Errorf("kind = %s, want unknown-entry", f.Kind)
		}
	}
}

func TestAnalyzeReportsMissingDirs(t *testing.T) {
	p := fakeHome(t, "claude") // no canonical store, no ~/.claude/skills

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := kinds(rep); !reflect.DeepEqual(got, []Kind{KindMissingDir, KindMissingDir}) {
		t.Fatalf("findings = %+v, want store and claude skills dir missing", rep.Findings)
	}
	for _, f := range rep.Findings {
		if !strings.Contains(f.Message, p.Home[:len(p.Home)]) && !strings.Contains(f.Message, "does not exist") {
			t.Errorf("message should name the missing dir: %q", f.Message)
		}
	}
}

func TestAnalyzeConflictsOnManualConfigEdits(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "codex", "claude")
	storeSkill(t, p, "tdd")
	storeSkill(t, p, "manual-skill")
	// claude can only see what it can discover.
	symlink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.ClaudeSkills(), "tdd"))
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"manual-skill": "deny"}}}`)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/manual-skill/SKILL.md"]}`)
	writeFile(t, p.CodexConfig(), "[[skills.config]]\nname = \"manual-skill\"\nenabled = false\n")
	writeFile(t, p.ClaudeSettings(), `{"skillOverrides": {"manual-skill": "off"}}`)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("findings = %+v, want none (exact disables are conflicts, not findings)", rep.Findings)
	}
	if len(rep.Conflicts) != 4 {
		t.Fatalf("conflicts = %+v, want one per config lever", rep.Conflicts)
	}
	for _, c := range rep.Conflicts {
		if c.Skill != "manual-skill" || !c.ConfigDisables {
			t.Errorf("conflict = %+v", c)
		}
		if !strings.Contains(c.Message, "manual-skill") {
			t.Errorf("message must name the skill: %q", c.Message)
		}
	}
}

func TestAnalyzeConflictIncludesSkillUnknownToTheStore(t *testing.T) {
	// A manual entry for a skill that isn't installed anymore is still a
	// disagreement the user must decide on.
	p := fakeHome(t, "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"vanished": "deny"}}}`)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Conflicts) != 1 || rep.Conflicts[0].Skill != "vanished" || !rep.Conflicts[0].ConfigDisables {
		t.Fatalf("conflicts = %+v, want vanished/config-disables", rep.Conflicts)
	}
}

func TestAnalyzePatternDisableIsAFindingNotAConflict(t *testing.T) {
	// The state can't record a pattern, and fleet can't remove a rule it
	// doesn't own — so it is reported, never prompted.
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"t*": "deny"}}}`)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Conflicts) != 0 {
		t.Errorf("conflicts = %+v, want none", rep.Conflicts)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindManualEdit {
		t.Fatalf("findings = %+v, want one manual-edit finding", rep.Findings)
	}
}

func TestAnalyzeDriftConflictWhenStateDisablesAndConfigDoesNot(t *testing.T) {
	p := fakeHome(t, "opencode", "claude")
	storeSkill(t, p, "tdd")
	symlink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.ClaudeSkills(), "tdd"))
	writeFile(t, p.ClaudeSettings(), `{"skillOverrides": {"tdd": "off"}}`)
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	st.SetDisabled("tdd", "claude")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
	// The opencode deny was deleted by hand; claude's is still in place.

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want exactly the opencode drift", rep.Conflicts)
	}
	c := rep.Conflicts[0]
	if c.Harness != "opencode" || c.Skill != "tdd" || c.ConfigDisables {
		t.Errorf("conflict = %+v, want opencode/tdd state-disables", c)
	}
	if !strings.Contains(c.Message, "sync re-disables") {
		t.Errorf("message should warn that sync repairs it: %q", c.Message)
	}
}

func TestAnalyzeClaudeDisableWithoutLinkIsAFinding(t *testing.T) {
	// State says claude shouldn't load the skill, but claude can't see it
	// at all (no link): nothing for sync to write, so report, don't prompt.
	p := fakeHome(t, "claude")
	storeSkill(t, p, "tdd")
	// The link dir exists but holds no link for tdd: the missing-dir
	// finding doesn't fire, only the moot disable does.
	if err := os.MkdirAll(p.ClaudeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "claude")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Conflicts) != 0 {
		t.Errorf("conflicts = %+v, want none", rep.Conflicts)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindDrift {
		t.Fatalf("findings = %+v, want one drift finding", rep.Findings)
	}
}

func TestAnalyzeUnreadableConfigIsAFinding(t *testing.T) {
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {`)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindBrokenConfig {
		t.Fatalf("findings = %+v, want one broken-config finding", rep.Findings)
	}
}

func TestAnalyzeTouchesNothing(t *testing.T) {
	p := fakeHome(t, "opencode", "claude", "bob")
	storeSkill(t, p, "tdd")
	symlink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.OpenCodeSkills(), "tdd"))
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"manual": "deny"}}}`)
	if err := os.MkdirAll(p.ClaudeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, p.Home)

	if _, err := Analyze(p); err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if after := snapshot(t, p.Home); !reflect.DeepEqual(before, after) {
		t.Error("Analyze() modified the home")
	}
}

func TestResolveKeepRecordsTheManualEdit(t *testing.T) {
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"tdd": "deny"}}}`)
	before := readFileT(t, p.OpenCodeConfig())

	c := Conflict{Harness: "opencode", Skill: "tdd", ConfigDisables: true}
	if _, err := Resolve(p, c, true); err != nil {
		t.Fatalf("Resolve(keep) error = %v", err)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("tdd", "opencode") {
		t.Error("state does not record the kept disable")
	}
	if got := readFileT(t, p.OpenCodeConfig()); got != before {
		t.Error("keep must not touch the config")
	}
}

func TestResolveKeepAdoptsADeletedDisable(t *testing.T) {
	// Drift, keep branch: the user deleted fleet's disable on purpose, so
	// the state entry goes away.
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	c := Conflict{Harness: "opencode", Skill: "tdd", ConfigDisables: false}
	if _, err := Resolve(p, c, true); err != nil {
		t.Fatalf("Resolve(keep) error = %v", err)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if st.IsDisabled("tdd", "opencode") {
		t.Error("state still records the disable the user deleted")
	}
}

func TestResolveRestoreSyncsTheStateBack(t *testing.T) {
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"tdd": "deny"}}}`)

	c := Conflict{Harness: "opencode", Skill: "tdd", ConfigDisables: true}
	rep, err := Resolve(p, c, false)
	if err != nil {
		t.Fatalf("Resolve(restore) error = %v", err)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != harness.StateOn {
		t.Errorf("report = %+v, want one flip to on", rep)
	}
	if got := readFileT(t, p.OpenCodeConfig()); strings.Contains(got, "tdd") {
		t.Errorf("config still holds the deny:\n%s", got)
	}
}

func TestResolveRestoreReDisablesDrift(t *testing.T) {
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	c := Conflict{Harness: "opencode", Skill: "tdd", ConfigDisables: false}
	if _, err := Resolve(p, c, false); err != nil {
		t.Fatalf("Resolve(restore) error = %v", err)
	}
	if got := readFileT(t, p.OpenCodeConfig()); !strings.Contains(got, `"tdd": "deny"`) {
		t.Errorf("config missing the restored deny:\n%s", got)
	}
}

func TestResolveRestoreRemovesStaleClaudeOverride(t *testing.T) {
	// A manual "off" for a skill claude can no longer discover: restore
	// must still converge the config, not silently do nothing.
	p := fakeHome(t, "claude")
	storeSkill(t, p, "tdd")
	writeFile(t, p.ClaudeSettings(), `{"skillOverrides": {"tdd": "off"}}`)

	c := Conflict{Harness: "claude", Skill: "tdd", ConfigDisables: true}
	if _, err := Resolve(p, c, false); err != nil {
		t.Fatalf("Resolve(restore) error = %v", err)
	}
	if got := readFileT(t, p.ClaudeSettings()); strings.Contains(got, "tdd") {
		t.Errorf("config still holds the stale override:\n%s", got)
	}
}

func TestResolveRefusesHarnessesWithoutALever(t *testing.T) {
	p := fakeHome(t, "cursor")
	c := Conflict{Harness: "cursor", Skill: "tdd", ConfigDisables: true}
	if _, err := Resolve(p, c, false); err == nil {
		t.Fatal("Resolve(restore) succeeded for cursor, want error")
	}
}

func readFileT(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
