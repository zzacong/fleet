package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/state"
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

// fakeRepo records a fake tracked repo's skills/ collection in the home's
// config file and returns the repo root. The repo's skills/ dir is created
// so the entry scans as a collection.
func fakeRepo(t *testing.T, p *paths.Paths) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFleetConfig(t, p, []string{repo}, "")
	return repo
}

// repoSkill writes a skill into the fake tracked repo's skills/ dir,
// mirroring storeSkill.
func repoSkill(t *testing.T, p *paths.Paths, name string) {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	dirs := f.SkillsDirs()
	if len(dirs) == 0 {
		t.Fatal("repoSkill without a fakeRepo tracked entry")
	}
	collectionSkill(t, dirs[0], name)
}

// fleetSkill writes a skill into the fleet-home skills dir.
func fleetSkill(t *testing.T, p *paths.Paths, name string) {
	t.Helper()
	dir := filepath.Join(p.FleetHomeSkills(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: test skill " + name + "\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

// collectionSkill writes a skill into an arbitrary collection dir (a
// tracked dir or the fleet-home fallback).
func collectionSkill(t *testing.T, collection, name string) {
	t.Helper()
	dir := filepath.Join(collection, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: test skill " + name + "\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeFleetConfig records each root's skills/ collection dir in the
// fake home's skillsDirs list and the adopt target.
func writeFleetConfig(t *testing.T, p *paths.Paths, roots []string, target string) {
	t.Helper()
	cfg := map[string]any{}
	if roots != nil {
		collections := make([]string, len(roots))
		for i, root := range roots {
			collections[i] = filepath.Join(root, "skills")
		}
		cfg["skillsDirs"] = collections
	}
	if target != "" {
		cfg["adoptTarget"] = target
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p.FleetConfigFile(), string(body))
}

// writeSkillsDirs records explicit collection dirs verbatim, for fixtures
// that need a dir that is not a repo's skills/ subdir.
func writeSkillsDirs(t *testing.T, p *paths.Paths, dirs ...string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"skillsDirs": dirs})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p.FleetConfigFile(), string(body))
}

// writeLock writes a skills CLI lockfile with the given provenance
// entries, keyed by directory name.
func writeLock(t *testing.T, p *paths.Paths, lock map[string]scan.Provenance) {
	t.Helper()
	body, err := json.Marshal(struct {
		Skills map[string]scan.Provenance `json:"skills"`
	}{Skills: lock})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p.SkillLock(), string(body))
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
	if f.Cause != "scans the canonical store natively — this link double-covers the skill" {
		t.Errorf("cause = %q, want the harness-free explanation", f.Cause)
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
	if f.Cause != "discovers skills only through links, but this link's target is missing" {
		t.Errorf("cause = %q, want the harness-free explanation", f.Cause)
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
	wantCause := map[string]string{
		"hand-made": "a real directory, not a symlink — left alone",
		"mine":      "the symlink points outside the canonical store — fleet doesn't manage it",
	}
	for _, f := range rep.Findings {
		if f.Kind != KindUnknownEntry {
			t.Errorf("kind = %s, want unknown-entry", f.Kind)
		}
		if f.Cause != wantCause[f.Skill] {
			t.Errorf("cause for %q = %q, want %q", f.Skill, f.Cause, wantCause[f.Skill])
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
	if rep.Findings[0].Cause != "disabled by an entry fleet doesn't manage (a pattern or blanket rule) — edit the config by hand if that's wrong" {
		t.Errorf("cause = %q, want the harness-free explanation", rep.Findings[0].Cause)
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
	f := rep.Findings[0]
	if f.Reason != DriftDisabledUnlinked || f.Dir != p.ClaudeSkills() {
		t.Errorf("finding = %+v, want the disabled-unlinked reason and claude's skills dir", f)
	}
}

func TestAnalyzeEnabledCustomWithoutLinkIsDrift(t *testing.T) {
	// State leaves the custom enabled (no off entry), so Bob should reach
	// it through its managed link. Without one he cannot: report, don't
	// prompt — sync links it on the next command.
	p := fakeHome(t, "bob")
	storeSkill(t, p, "tdd")
	fakeRepo(t, p)
	repoSkill(t, p, "my-notes")

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := kinds(rep); !reflect.DeepEqual(got, []Kind{KindDrift}) {
		t.Fatalf("findings = %+v, want one drift finding", rep.Findings)
	}
	f := rep.Findings[0]
	if f.Harness != "bob" || f.Skill != "my-notes" {
		t.Errorf("finding = %+v, want bob/my-notes", f)
	}
	if f.Reason != DriftEnabledUnlinked || f.Dir != p.BobSkills() {
		t.Errorf("finding = %+v, want the enabled-unlinked reason and bob's skills dir", f)
	}
	if !strings.Contains(f.Message, "enabled in fleet's state") || !strings.Contains(f.Message, "cannot discover it") {
		t.Errorf("message must state the mismatch: %q", f.Message)
	}
}

func TestAnalyzeQuietWhenCustomLinkMatchesState(t *testing.T) {
	p := fakeHome(t, "bob")
	storeSkill(t, p, "tdd")
	repo := fakeRepo(t, p)
	repoSkill(t, p, "my-notes")
	symlink(t, filepath.Join(repo, "skills", "my-notes"), filepath.Join(p.BobSkills(), "my-notes"))

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 0 || len(rep.Conflicts) != 0 {
		t.Errorf("report = %+v, want empty when the link matches the enabled state", rep)
	}
}

func TestAnalyzeDisabledCustomWithLinkIsDrift(t *testing.T) {
	// The flip side: state hides the custom from Bob, but the managed link
	// is still there, so Bob can still discover it. Sync removes the link.
	p := fakeHome(t, "bob")
	storeSkill(t, p, "tdd")
	repo := fakeRepo(t, p)
	repoSkill(t, p, "my-notes")
	symlink(t, filepath.Join(repo, "skills", "my-notes"), filepath.Join(p.BobSkills(), "my-notes"))
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("my-notes", "bob")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := kinds(rep); !reflect.DeepEqual(got, []Kind{KindDrift}) {
		t.Fatalf("findings = %+v, want one drift finding", rep.Findings)
	}
	f := rep.Findings[0]
	if f.Harness != "bob" || f.Skill != "my-notes" || f.Path == "" {
		t.Errorf("finding = %+v, want bob/my-notes with the link path", f)
	}
	if f.Reason != DriftDisabledLinked || f.Dir != p.BobSkills() {
		t.Errorf("finding = %+v, want the disabled-linked reason and bob's skills dir", f)
	}
	if !strings.Contains(f.Message, "disabled in fleet's state") || !strings.Contains(f.Message, "still has a link") {
		t.Errorf("message must state the mismatch: %q", f.Message)
	}
}

func TestAnalyzeCustomLinkDriftOnEveryNativeScanner(t *testing.T) {
	// A custom skill's enablement is its managed link on every
	// native-scanning harness: opencode, pi, codex, cursor, and bob. A
	// missing link for an enabled custom is drift on each; a present link
	// for a disabled custom is drift too.
	p := fakeHome(t, "opencode", "pi", "codex", "cursor", "bob")
	storeSkill(t, p, "tdd")
	repo := fakeRepo(t, p)
	repoSkill(t, p, "my-notes")

	driftHarnesses := func(rep Report) map[string]bool {
		got := map[string]bool{}
		for _, f := range rep.Findings {
			if f.Kind == KindDrift && f.Skill == "my-notes" {
				got[f.Harness] = true
			}
		}
		return got
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	got := driftHarnesses(rep)
	for _, h := range []string{"opencode", "pi", "codex", "cursor", "bob"} {
		if !got[h] {
			t.Errorf("enabled custom: missing drift for %s: %+v", h, rep.Findings)
		}
	}

	// Flip it: disable the custom everywhere and leave the links in place.
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	linkDirs := map[string]string{
		"opencode": p.OpenCodeSkills(),
		"pi":       p.PiSkills(),
		"codex":    p.CodexSkills(),
		"cursor":   p.CursorSkills(),
		"bob":      p.BobSkills(),
	}
	for _, h := range []string{"opencode", "pi", "codex", "cursor", "bob"} {
		st.SetDisabled("my-notes", h)
		if err := os.MkdirAll(linkDirs[h], 0o755); err != nil {
			t.Fatal(err)
		}
		symlink(t, filepath.Join(repo, "skills", "my-notes"), filepath.Join(linkDirs[h], "my-notes"))
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	rep, err = Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	got = driftHarnesses(rep)
	for _, h := range []string{"opencode", "pi", "codex", "cursor", "bob"} {
		if !got[h] {
			t.Errorf("disabled custom: missing drift for %s: %+v", h, rep.Findings)
		}
	}
}

func TestAnalyzeCanonicalSkillIsNeverLinkDrift(t *testing.T) {
	// A canonical-store skill has no managed link by construction; its
	// absence on a native scanner is not drift.
	p := fakeHome(t, "opencode", "pi", "codex", "cursor", "bob")
	storeSkill(t, p, "tdd")

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 0 || len(rep.Conflicts) != 0 {
		t.Errorf("report = %+v, want clean for a canonical skill", rep)
	}
}

func TestAnalyzeDanglingCustomLinkIsBrokenNotDrift(t *testing.T) {
	// A custom skill's managed link whose target is gone surfaces through
	// the existing broken-link diagnostic, not as link drift.
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	repo := fakeRepo(t, p)
	repoSkill(t, p, "my-notes")
	target := filepath.Join(repo, "skills", "my-notes")
	symlink(t, target, filepath.Join(p.OpenCodeSkills(), "my-notes"))
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	f := findKind(rep, KindBrokenLink)
	if f == nil {
		t.Fatalf("findings = %+v, want a broken-link finding", rep.Findings)
	}
	if f.Harness != "opencode" || f.Skill != "my-notes" || f.Path == "" {
		t.Errorf("finding = %+v, want opencode/my-notes with the link path", *f)
	}
	// Removing the only skill left the tracked dir empty, which doctor also
	// reports; the dangling link itself must not be misread as drift.
	for _, finding := range rep.Findings {
		if finding.Kind == KindDrift {
			t.Errorf("dangling custom link reported as drift: %+v", finding)
		}
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

func TestAnalyzeFlagsSkillInBothStoreAndTracked(t *testing.T) {
	// The half-done adoption reversal: the store copy came back (or never
	// left) while the tracked copy stayed — the same name in both places.
	p := fakeHome(t)
	repo := fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	repoSkill(t, p, "tdd")

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	var found *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Kind == KindDoublePresence {
			found = &rep.Findings[i]
		}
	}
	if found == nil {
		t.Fatalf("findings = %+v, want one double-presence finding", rep.Findings)
	}
	if found.Skill != "tdd" || found.Harness != "" {
		t.Errorf("finding = %+v, want the skill name and no harness scope", *found)
	}
	if !strings.Contains(found.Message, filepath.Join(p.SkillsStore(), "tdd")) ||
		!strings.Contains(found.Message, filepath.Join(repo, "skills", "tdd")) {
		t.Errorf("message must name both copies: %q", found.Message)
	}
	if !strings.Contains(found.Message, "twice") || !strings.Contains(found.Message, "by hand") {
		t.Errorf("message must state the consequence and the manual resolution: %q", found.Message)
	}
}

func TestAnalyzeDoublePresenceFollowsTheNameNotTheDir(t *testing.T) {
	// ls dedupes on the frontmatter name; doctor flags the same way, so a
	// renamed directory still counts as the same skill in both places.
	p := fakeHome(t)
	repo := fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	dir := filepath.Join(repo, "skills", "my-tdd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: tdd\ndescription: the repo copy\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindDoublePresence || rep.Findings[0].Skill != "tdd" {
		t.Fatalf("findings = %+v, want one double-presence finding for tdd", rep.Findings)
	}
}

// TestAnalyzeDoublePresenceCarriesCopiesAndHarnesses checks the structured
// form the CLI groups on: every copy with its home, and the installed native
// scanners that would see the name twice. Claude is link-only, so it is not
// one of them.
func TestAnalyzeDoublePresenceCarriesCopiesAndHarnesses(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "codex", "claude", "cursor", "bob")
	repo := fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	repoSkill(t, p, "tdd")

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	var found *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Kind == KindDoublePresence {
			found = &rep.Findings[i]
		}
	}
	if found == nil {
		t.Fatalf("findings = %+v, want a double-presence finding", rep.Findings)
	}
	wantHomes := []string{p.SkillsStore(), filepath.Join(repo, "skills")}
	if !reflect.DeepEqual(found.Homes, wantHomes) {
		t.Errorf("homes = %+v, want %+v", found.Homes, wantHomes)
	}
	wantHarnesses := []string{"opencode", "pi", "codex", "cursor", "bob"}
	if !reflect.DeepEqual(found.Harnesses, wantHarnesses) {
		t.Errorf("harnesses = %+v, want %+v (claude excluded: it does not scan the store)", found.Harnesses, wantHarnesses)
	}
}

func TestAnalyzeQuietWhenTrackedSkillsAreUnique(t *testing.T) {
	// Distinct names on each side, no lockfile: no double presence, no
	// stale lock — the normal adopted-customs home.
	p := fakeHome(t)
	fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	repoSkill(t, p, "git-helper")

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("findings = %+v, want none", rep.Findings)
	}
}

func TestAnalyzeFlagsStaleLockEntryForAdoptedSkill(t *testing.T) {
	// The skill was adopted out of the store, but its lockfile entry
	// stayed: the skills CLI would keep trying to update a skill that now
	// lives in the tracked collection.
	p := fakeHome(t)
	repo := fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	repoSkill(t, p, "git-helper")
	writeLock(t, p, map[string]scan.Provenance{
		"git-helper": {Source: "mattpocock/skills", SourceType: "github", Hash: "abc123"},
	})

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the stale lock finding", rep.Findings)
	}
	f := rep.Findings[0]
	if f.Kind != KindStaleLock || f.Skill != "git-helper" || f.Harness != "" {
		t.Errorf("finding = %+v, want the stale lock for git-helper", f)
	}
	if f.Home != filepath.Join(repo, "skills") || f.HomeLabel != "the tracked dir" {
		t.Errorf("home = %q label = %q, want the tracked collection and its label", f.Home, f.HomeLabel)
	}
	if !strings.Contains(f.Message, p.SkillLock()) || !strings.Contains(f.Message, filepath.Join(repo, "skills", "git-helper")) {
		t.Errorf("message must name the lockfile and the tracked copy: %q", f.Message)
	}
	if !strings.Contains(f.Message, "skills CLI") || !strings.Contains(f.Message, "never writes the lockfile") {
		t.Errorf("message must state the consequence and that fleet never edits the lockfile: %q", f.Message)
	}
}

func TestAnalyzeQuietWhenLockEntriesMatchTheStore(t *testing.T) {
	// A lock entry for a store skill is normal provenance, and a repo
	// skill without one is a plain custom: neither is stale.
	p := fakeHome(t)
	fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	repoSkill(t, p, "git-helper")
	writeLock(t, p, map[string]scan.Provenance{
		"tdd": {Source: "mattpocock/skills", SourceType: "github", Hash: "abc123"},
	})

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("findings = %+v, want none", rep.Findings)
	}
}

func TestAnalyzePropagatesLockfileErrors(t *testing.T) {
	// Provenance must not be silently lost: a malformed lockfile fails the
	// checkup like any other unreadable input.
	p := fakeHome(t)
	fakeRepo(t, p)
	repoSkill(t, p, "git-helper")
	writeFile(t, p.SkillLock(), `{"skills": {`)

	if _, err := Analyze(p); err == nil || !strings.Contains(err.Error(), "read skills lockfile") {
		t.Fatalf("Analyze() error = %v, want the lockfile read error", err)
	}
}

func TestAnalyzeTouchesNothing(t *testing.T) {
	p := fakeHome(t, "opencode", "claude", "bob")
	repo := fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	repoSkill(t, p, "git-helper")
	writeLock(t, p, map[string]scan.Provenance{
		"tdd":        {Source: "mattpocock/skills", SourceType: "github", Hash: "abc123"},
		"git-helper": {Source: "mattpocock/skills", SourceType: "github", Hash: "def456"},
	})
	symlink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.OpenCodeSkills(), "tdd"))
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"manual": "deny"}}}`)
	if err := os.MkdirAll(p.ClaudeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, p.Home)
	for path, body := range snapshot(t, repo) {
		before["repo:"+path] = body
	}
	before["lock"] = readFileT(t, p.SkillLock())

	if _, err := Analyze(p); err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	after := snapshot(t, p.Home)
	for path, body := range snapshot(t, repo) {
		after["repo:"+path] = body
	}
	after["lock"] = readFileT(t, p.SkillLock())
	if !reflect.DeepEqual(before, after) {
		t.Error("Analyze() modified the home, the repo, or the lockfile")
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

func TestAnalyzeFlagsStaleLockEntryForFleetHomeSkill(t *testing.T) {
	p := fakeHome(t)
	fakeRepo(t, p)
	repoSkill(t, p, "repo-helper")
	storeSkill(t, p, "tdd")
	fleetSkill(t, p, "fleet-helper")
	writeLock(t, p, map[string]scan.Provenance{
		"fleet-helper": {Source: "mattpocock/skills", SourceType: "github", Hash: "abc123"},
	})

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the fleet stale lock finding", rep.Findings)
	}
	f := rep.Findings[0]
	if f.Kind != KindStaleLock || f.Skill != "fleet-helper" || f.Harness != "" {
		t.Errorf("finding = %+v, want the stale lock for fleet-helper", f)
	}
	if !strings.Contains(f.Message, p.SkillLock()) || !strings.Contains(f.Message, filepath.Join(p.FleetHomeSkills(), "fleet-helper")) {
		t.Errorf("message must name the lockfile and the fleet-home copy: %q", f.Message)
	}
	if !strings.Contains(f.Message, "skills CLI") || !strings.Contains(f.Message, "never writes the lockfile") {
		t.Errorf("message must state the consequence and that fleet never edits the lockfile: %q", f.Message)
	}
}

func TestAnalyzeFlagsStaleLockForBothCustomHomes(t *testing.T) {
	p := fakeHome(t)
	fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	fleetSkill(t, p, "fleet-helper")
	repoSkill(t, p, "repo-helper")
	writeLock(t, p, map[string]scan.Provenance{
		"fleet-helper": {Source: "a/b", SourceType: "github", Hash: "aaa"},
		"repo-helper":  {Source: "a/b", SourceType: "github", Hash: "bbb"},
	})

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	// Two stale-lock findings, one per custom home, sorted by skill.
	if len(rep.Findings) != 2 {
		t.Fatalf("findings = %+v, want two stale lock findings", rep.Findings)
	}
	found := map[string]bool{}
	for _, f := range rep.Findings {
		if f.Kind != KindStaleLock {
			t.Errorf("finding kind = %s, want stale-lock", f.Kind)
		}
		found[f.Skill] = true
		if f.Harness != "" {
			t.Errorf("stale lock should have no harness, got %q", f.Harness)
		}
	}
	if !found["fleet-helper"] || !found["repo-helper"] {
		t.Errorf("findings missing expected skills, got %+v", rep.Findings)
	}
}

func TestAnalyzeSuppressesManagedFleetHomeLinks(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "claude", "codex", "cursor", "bob")
	storeSkill(t, p, "tdd")
	fleetSkill(t, p, "fleet-helper")
	// Managed links into fleet-home must not be reported as unknown.
	for _, dir := range []string{p.OpenCodeSkills(), p.PiSkills(), p.ClaudeSkills(), p.CodexSkills(), p.CursorSkills(), p.BobSkills()} {
		symlink(t, filepath.Join(p.FleetHomeSkills(), "fleet-helper"), filepath.Join(dir, "fleet-helper"))
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	for _, f := range rep.Findings {
		if f.Kind == KindUnknownEntry && f.Skill == "fleet-helper" {
			t.Errorf("managed fleet-home link reported as unknown: %+v", f)
		}
	}
	// Also ensure no redundant/broken for these managed links.
	for _, f := range rep.Findings {
		if f.Skill == "fleet-helper" {
			t.Errorf("managed fleet link should be suppressed, got finding %+v", f)
		}
	}
}

func TestAnalyzeSuppressesManagedExplicitLinks(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "claude", "codex", "cursor", "bob")
	repo := fakeRepo(t, p)
	storeSkill(t, p, "tdd")
	repoSkill(t, p, "repo-helper")
	for _, dir := range []string{p.OpenCodeSkills(), p.PiSkills(), p.ClaudeSkills(), p.CodexSkills(), p.CursorSkills(), p.BobSkills()} {
		symlink(t, filepath.Join(repo, "skills", "repo-helper"), filepath.Join(dir, "repo-helper"))
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	for _, f := range rep.Findings {
		if f.Skill == "repo-helper" {
			t.Errorf("managed explicit link should be suppressed, got %+v", f)
		}
	}
}

func TestAnalyzeFlagsCollisionAcrossTrackedSet(t *testing.T) {
	// One name in every source: canonical, two tracked dirs, and
	// fallback — a single double-presence finding naming every copy.
	p := fakeHome(t)
	explicit := filepath.Join(t.TempDir(), "explicit")
	second := filepath.Join(t.TempDir(), "second")
	writeFleetConfig(t, p, []string{explicit, second}, "")
	storeSkill(t, p, "shared")
	fleetSkill(t, p, "shared")
	collectionSkill(t, filepath.Join(explicit, "skills"), "shared")
	collectionSkill(t, filepath.Join(second, "skills"), "shared")

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	var found []Finding
	for _, f := range rep.Findings {
		if f.Kind == KindDoublePresence {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		t.Fatalf("findings = %+v, want one double-presence finding for the four-way collision", rep.Findings)
	}
	f := found[0]
	if f.Skill != "shared" || f.Harness != "" {
		t.Errorf("finding = %+v, want the skill name and no harness scope", f)
	}
	for _, path := range []string{
		filepath.Join(p.SkillsStore(), "shared"),
		filepath.Join(p.FleetHomeSkills(), "shared"),
		filepath.Join(explicit, "skills", "shared"),
		filepath.Join(second, "skills", "shared"),
	} {
		if !strings.Contains(f.Message, path) {
			t.Errorf("message must name every copy, missing %q in %q", path, f.Message)
		}
	}
	if !strings.Contains(f.Message, "twice") || !strings.Contains(f.Message, "by hand") {
		t.Errorf("message must state the consequence and the manual resolution: %q", f.Message)
	}
}

func TestAnalyzeFlagsCollisionAcrossTrackedDirs(t *testing.T) {
	// A collision purely between tracked dirs still counts: every
	// cross-source name collision is drift.
	p := fakeHome(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	writeFleetConfig(t, p, []string{first, second}, "")
	collectionSkill(t, filepath.Join(first, "skills"), "dup")
	collectionSkill(t, filepath.Join(second, "skills"), "dup")

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	var found []Finding
	for _, f := range rep.Findings {
		if f.Kind == KindDoublePresence {
			found = append(found, f)
		}
	}
	if len(found) != 1 || found[0].Skill != "dup" {
		t.Fatalf("findings = %+v, want one double-presence finding for dup", rep.Findings)
	}
}

func TestAnalyzeWarnsOnUnscannedAdoptTarget(t *testing.T) {
	p := fakeHome(t)
	target := filepath.Join(t.TempDir(), "elsewhere", "skills")
	writeFleetConfig(t, p, nil, target)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	var found *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Kind == KindUnscannedAdoptTarget {
			found = &rep.Findings[i]
		}
	}
	if found == nil {
		t.Fatalf("findings = %+v, want an unscanned-adopt-target warning", rep.Findings)
	}
	if !strings.Contains(found.Message, target) {
		t.Errorf("message must name the adopt target: %q", found.Message)
	}
}

func TestAnalyzeQuietWhenAdoptTargetIsScanned(t *testing.T) {
	for _, target := range []string{"fallback", "tracked"} {
		t.Run(target, func(t *testing.T) {
			p := fakeHome(t)
			explicit := filepath.Join(t.TempDir(), "explicit")
			var want string
			if target == "fallback" {
				want = p.FleetHomeSkills()
			} else {
				want = filepath.Join(explicit, "skills")
			}
			writeFleetConfig(t, p, []string{explicit}, want)

			rep, err := Analyze(p)
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}
			for _, f := range rep.Findings {
				if f.Kind == KindUnscannedAdoptTarget {
					t.Errorf("scanned adopt target %q warned: %+v", want, f)
				}
			}
		})
	}
}

func TestAnalyzeSuppressesManagedTrackedLinks(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "claude", "codex")
	explicit := filepath.Join(t.TempDir(), "explicit")
	second := filepath.Join(t.TempDir(), "second")
	writeFleetConfig(t, p, []string{explicit, second}, "")
	collectionSkill(t, filepath.Join(explicit, "skills"), "tracked-helper")
	collectionSkill(t, filepath.Join(second, "skills"), "checkout-helper")
	storeSkill(t, p, "tdd")
	for _, dir := range []string{p.OpenCodeSkills(), p.PiSkills(), p.ClaudeSkills(), p.CodexSkills()} {
		symlink(t, filepath.Join(explicit, "skills", "tracked-helper"), filepath.Join(dir, "tracked-helper"))
		symlink(t, filepath.Join(second, "skills", "checkout-helper"), filepath.Join(dir, "checkout-helper"))
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	for _, f := range rep.Findings {
		if f.Skill == "tracked-helper" || f.Skill == "checkout-helper" {
			t.Errorf("managed tracked link should be suppressed, got finding %+v", f)
		}
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

func TestAnalyzeFlagsStaleConfigForUninstalledSkill(t *testing.T) {
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd") // the store exists and scans: the scan is complete
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	// The dormant state disable is what makes the leftover rule fleet's:
	// prune's config axis only covers names the state disables.
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("ghost", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
	before := readFileT(t, p.OpenCodeConfig())

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Conflicts) != 0 {
		t.Errorf("conflicts = %+v, want none for an uninstalled skill", rep.Conflicts)
	}
	var found *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Kind == KindStaleConfig {
			found = &rep.Findings[i]
		}
	}
	if found == nil {
		t.Fatalf("findings = %+v, want a stale-config finding", rep.Findings)
	}
	if found.Harness != "opencode" || found.Skill != "ghost" {
		t.Errorf("finding = %+v, want opencode/ghost", *found)
	}
	if !strings.Contains(found.Message, "fleet skill prune") {
		t.Errorf("message must point at prune: %q", found.Message)
	}
	if got := readFileT(t, p.OpenCodeConfig()); got != before {
		t.Errorf("Analyze modified the config:\n%s", got)
	}
}

func TestAnalyzeUntrackedConfigRuleForUninstalledSkillIsConflict(t *testing.T) {
	// A config rule for a skill the state does not track stays a manual
	// edit even when the skill is installed nowhere: prune's config axis
	// only covers state-disabled names, so doctor must not point at it.
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd") // complete scan
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Conflicts) != 1 || rep.Conflicts[0].Skill != "ghost" || !rep.Conflicts[0].ConfigDisables {
		t.Fatalf("conflicts = %+v, want ghost/config-disables", rep.Conflicts)
	}
	for _, f := range rep.Findings {
		if f.Kind == KindStaleConfig {
			t.Errorf("untracked config rule flagged stale: %+v", f)
		}
	}
}

func TestAnalyzeStaleConfigOnlyForFleetOwnedShapes(t *testing.T) {
	// A codex path selector disables an uninstalled, state-disabled skill,
	// but it is not fleet's shape: prune can't remove it, so doctor must
	// not report it as a stale config rule. The dormant state entry is
	// still stale and still points at prune.
	p := fakeHome(t, "codex")
	storeSkill(t, p, "tdd") // complete scan
	writeFile(t, p.CodexConfig(), "[[skills.config]]\npath = \"/agents/skills/ghost/SKILL.md\"\nenabled = false\n")
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("ghost", "codex")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	var staleState bool
	for _, f := range rep.Findings {
		if f.Kind == KindStaleConfig {
			t.Errorf("path selector flagged as a fleet-owned stale config rule: %+v", f)
		}
		if f.Kind == KindStaleState && f.Skill == "ghost" {
			staleState = true
		}
	}
	if !staleState {
		t.Errorf("findings = %+v, want a stale-state finding for ghost", rep.Findings)
	}
}

func TestAnalyzeFlagsStaleStateForUninstalledSkill(t *testing.T) {
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("ghost", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
	before := readFileT(t, p.FleetStateFile())

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	// The missing config rule is expected for a dormant disable, not drift.
	if len(rep.Conflicts) != 0 {
		t.Errorf("conflicts = %+v, want none", rep.Conflicts)
	}
	var found *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Kind == KindStaleState {
			found = &rep.Findings[i]
		}
	}
	if found == nil {
		t.Fatalf("findings = %+v, want a stale-state finding", rep.Findings)
	}
	if found.Harness != "opencode" || found.Skill != "ghost" {
		t.Errorf("finding = %+v, want opencode/ghost", *found)
	}
	if !strings.Contains(found.Message, "fleet skill prune") || !strings.Contains(found.Message, "reinstall") {
		t.Errorf("message must point at prune and warn about reinstall: %q", found.Message)
	}
	if got := readFileT(t, p.FleetStateFile()); got != before {
		t.Errorf("Analyze modified the state file:\n%s", got)
	}
}

func TestAnalyzeInstalledSkillMissingRuleStillDrifts(t *testing.T) {
	p := fakeHome(t, "opencode")
	storeSkill(t, p, "tdd")
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Conflicts) != 1 || rep.Conflicts[0].Skill != "tdd" {
		t.Fatalf("conflicts = %+v, want the existing opencode drift", rep.Conflicts)
	}
	for _, f := range rep.Findings {
		if f.Kind == KindStaleConfig || f.Kind == KindStaleState {
			t.Errorf("installed skill flagged stale: %+v", f)
		}
	}
}

func TestAnalyzeIncompleteScanSuppressesStaleFindings(t *testing.T) {
	// The canonical store is absent, so the scan can't be trusted: a name
	// that looks uninstalled might live in the store. Stale findings stay
	// suppressed.
	p := fakeHome(t, "opencode") // no canonical store
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("ghost", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	for _, f := range rep.Findings {
		if f.Kind == KindStaleConfig || f.Kind == KindStaleState {
			t.Errorf("incomplete scan still produced a stale finding: %+v", f)
		}
	}
}

// findKind returns the first finding of a kind, or nil.
func findKind(rep Report, kind Kind) *Finding {
	for i := range rep.Findings {
		if rep.Findings[i].Kind == kind {
			return &rep.Findings[i]
		}
	}
	return nil
}

func TestAnalyzeFlagsTrackedDirMissingFromDisk(t *testing.T) {
	// A tracked dir the user deleted (or never created) leaves an
	// incomplete index: report it, never create it.
	p := fakeHome(t)
	storeSkill(t, p, "tdd")
	missing := filepath.Join(t.TempDir(), "gone")
	writeSkillsDirs(t, p, missing)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	f := findKind(rep, KindTrackedDirMissing)
	if f == nil {
		t.Fatalf("findings = %+v, want a tracked-dir-missing finding", rep.Findings)
	}
	if f.Path != missing {
		t.Errorf("path = %q, want %q", f.Path, missing)
	}
	if !strings.Contains(f.Message, missing) || !strings.Contains(f.Message, "does not exist") {
		t.Errorf("message must name the missing tracked dir: %q", f.Message)
	}
}

func TestAnalyzeFlagsTrackedDirThatIsNotADirectory(t *testing.T) {
	// A file where a collection dir should be: the same "unusable tracked
	// dir" report, worded for the shape.
	p := fakeHome(t)
	storeSkill(t, p, "tdd")
	file := filepath.Join(t.TempDir(), "not-a-dir")
	writeFile(t, file, "i am a file")
	writeSkillsDirs(t, p, file)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	f := findKind(rep, KindTrackedDirMissing)
	if f == nil {
		t.Fatalf("findings = %+v, want a tracked-dir-missing finding", rep.Findings)
	}
	if f.Path != file || !strings.Contains(f.Message, "not a directory") {
		t.Errorf("finding = %+v, want the file path and a not-a-directory message", *f)
	}
}

func TestAnalyzeFlagsTrackedDirWithNoSkills(t *testing.T) {
	// An empty registration is a mistake worth surfacing: nothing will ever
	// reach a harness from it.
	p := fakeHome(t)
	storeSkill(t, p, "tdd")
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillsDirs(t, p, empty)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	f := findKind(rep, KindTrackedDirEmpty)
	if f == nil {
		t.Fatalf("findings = %+v, want a tracked-dir-empty finding", rep.Findings)
	}
	if f.Path != empty || !strings.Contains(f.Message, empty) || !strings.Contains(f.Message, "no skills") {
		t.Errorf("finding = %+v, want the empty dir named and the zero-skill reason", *f)
	}
}

func TestAnalyzeFlagsDuplicateTrackedDir(t *testing.T) {
	// A hand-edited config that lists one collection twice: the same dir is
	// tracked twice, which is always a mistake.
	p := fakeHome(t)
	storeSkill(t, p, "tdd")
	dir := filepath.Join(t.TempDir(), "dup")
	collectionSkill(t, dir, "helper")
	writeSkillsDirs(t, p, dir, dir)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	f := findKind(rep, KindTrackedSetOverlap)
	if f == nil {
		t.Fatalf("findings = %+v, want a tracked-set-overlap finding", rep.Findings)
	}
	if !strings.Contains(f.Message, dir) || !strings.Contains(f.Message, "more than once") {
		t.Errorf("message must name the duplicate and say so: %q", f.Message)
	}
}

func TestAnalyzeReportsDuplicateTrackedDirOnce(t *testing.T) {
	// The same dir listed three times is one mistake: doctor must not print
	// the identical finding once per pair.
	p := fakeHome(t)
	storeSkill(t, p, "tdd")
	dir := filepath.Join(t.TempDir(), "dup")
	collectionSkill(t, dir, "helper")
	writeSkillsDirs(t, p, dir, dir, dir)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	count := 0
	for _, f := range rep.Findings {
		if f.Kind == KindTrackedSetOverlap {
			count++
		}
	}
	if count != 1 {
		t.Errorf("KindTrackedSetOverlap findings = %d, want 1 (findings = %+v)", count, rep.Findings)
	}
}

func TestAnalyzeFlagsNestedTrackedDirs(t *testing.T) {
	// One tracked collection inside another makes precedence ambiguous;
	// report it for the hand-edit that caused it.
	p := fakeHome(t)
	storeSkill(t, p, "tdd")
	parent := filepath.Join(t.TempDir(), "parent")
	child := filepath.Join(parent, "child")
	collectionSkill(t, parent, "outer")
	collectionSkill(t, child, "inner")
	writeSkillsDirs(t, p, parent, child)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	f := findKind(rep, KindTrackedSetOverlap)
	if f == nil {
		t.Fatalf("findings = %+v, want a tracked-set-overlap finding", rep.Findings)
	}
	if !strings.Contains(f.Message, parent) || !strings.Contains(f.Message, child) || !strings.Contains(f.Message, "nested") {
		t.Errorf("message must name both dirs and the nesting: %q", f.Message)
	}
}

func TestAnalyzeQuietOnPlainNonGitTrackedDir(t *testing.T) {
	// The git model is gone: a tracked collection with no .git and a valid
	// skill is a normal home, not a warning.
	p := fakeHome(t)
	storeSkill(t, p, "tdd")
	dir := filepath.Join(t.TempDir(), "plain")
	collectionSkill(t, dir, "helper")
	writeSkillsDirs(t, p, dir)

	rep, err := Analyze(p)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(rep.Findings) != 0 || len(rep.Conflicts) != 0 {
		t.Fatalf("report = %+v, want quiet for a non-git tracked dir", rep)
	}
}
