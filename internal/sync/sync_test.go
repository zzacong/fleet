package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/state"
)

// fakeHome builds a home with the given harnesses' config directories
// present, returning the Paths and the harness names installed.
func fakeHome(t *testing.T, harnesses ...string) *paths.Paths {
	t.Helper()
	t.Setenv("FLEET_REPO", "")
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// writeExplicitRepos records the explicit repo-root list in the fake home's
// config file, in precedence order.
func writeExplicitRepos(t *testing.T, p *paths.Paths, roots ...string) {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos(roots)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
}

func TestRunProjectsDisabledSkillsIntoInstalledHarnesses(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "codex", "claude")
	writeFile(t, p.OpenCodeConfig(), "{\n  \"model\": \"gpt-5\"\n}\n")
	writeFile(t, p.PiSettings(), "{}\n")
	writeFile(t, p.CodexConfig(), "# codex\n")
	// claude can only disable a skill it can discover: give it a link.
	if err := os.MkdirAll(filepath.Join(p.ClaudeSkills(), "tdd"), 0o755); err != nil {
		t.Fatal(err)
	}

	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	st.SetDisabled("tdd", "pi")
	st.SetDisabled("tdd", "codex")
	st.SetDisabled("tdd", "claude")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var harnesses []string
	var totalChanges int
	for _, r := range reports {
		harnesses = append(harnesses, r.Harness)
		totalChanges += len(r.Changed)
	}
	want := []string{"opencode", "pi", "codex", "claude"}
	if !reflect.DeepEqual(harnesses, want) {
		t.Errorf("reports for %v, want %v", harnesses, want)
	}
	if totalChanges != 4 {
		t.Errorf("total changes = %d, want 4 (one per harness)", totalChanges)
	}

	// Each config carries the harness's own off mechanism now.
	if got := readFile(t, p.OpenCodeConfig()); !strings.Contains(got, `"tdd": "deny"`) {
		t.Errorf("opencode config missing the V1 deny:\n%s", got)
	}
	if got := readFile(t, p.PiSettings()); !strings.Contains(got, "-skills/tdd/SKILL.md") {
		t.Errorf("pi settings missing the exclusion:\n%s", got)
	}
	if got := readFile(t, p.CodexConfig()); !strings.Contains(got, "enabled = false") || !strings.Contains(got, `name = "tdd"`) {
		t.Errorf("codex config missing the disabled block:\n%s", got)
	}
	if got := readFile(t, p.ClaudeSettings()); !strings.Contains(got, `"tdd": "off"`) {
		t.Errorf("claude settings missing the override:\n%s", got)
	}

	// Sync is idempotent: a second run has nothing to say.
	reports, err = Run(p)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("second run reported %+v, want nothing", reports)
	}
}

func TestRunNeverWritesCustomOffEntriesOnNativeScanners(t *testing.T) {
	// State disables a custom skill on the native-scanning link harnesses.
	// Their lever is the managed link, so sync removes the link and writes
	// no config off-entry; a canonical skill disabled for the same
	// harnesses still projects to config.
	p := fakeHome(t, "opencode", "pi", "codex", "claude", "cursor", "bob")
	collection := filepath.Join(p.FleetReposDir(), "team", "skills")
	writeFile(t, filepath.Join(collection, "my-notes", "SKILL.md"), "---\nname: my-notes\ndescription: notes\n---\n")
	writeFile(t, filepath.Join(p.SkillsStore(), "tdd", "SKILL.md"), "---\nname: tdd\ndescription: tdd\n---\n")

	// The custom's managed links exist before the disable.
	linkDirs := []string{p.OpenCodeSkills(), p.PiSkills(), p.CodexSkills(), p.CursorSkills(), p.BobSkills()}
	for _, dir := range linkDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(collection, "my-notes"), filepath.Join(dir, "my-notes")); err != nil {
			t.Fatal(err)
		}
	}

	st, _ := state.Load(p.FleetStateFile())
	for _, h := range []string{"opencode", "pi", "codex", "cursor", "bob"} {
		st.SetDisabled("my-notes", h)
	}
	for _, h := range []string{"opencode", "pi", "codex"} {
		st.SetDisabled("tdd", h)
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(p); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// No custom off-entry lands in any writable config.
	for _, path := range []string{p.OpenCodeConfig(), p.PiSettings(), p.CodexConfig()} {
		if body := readFile(t, path); strings.Contains(body, "my-notes") {
			t.Errorf("%s wrote a custom off-entry:\n%s", filepath.Base(path), body)
		}
	}
	// The canonical disable still projects.
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"tdd": "deny"`) {
		t.Errorf("opencode config missing the canonical deny:\n%s", body)
	}
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi settings missing the canonical exclusion:\n%s", body)
	}
	if body := readFile(t, p.CodexConfig()); !strings.Contains(body, `name = "tdd"`) || !strings.Contains(body, "enabled = false") {
		t.Errorf("codex config missing the canonical disable:\n%s", body)
	}
	// The custom's link is gone from every native scanner.
	for _, dir := range linkDirs {
		if _, err := os.Lstat(filepath.Join(dir, "my-notes")); !os.IsNotExist(err) {
			t.Errorf("custom link in %s survived the disable", dir)
		}
	}
}

func TestRunRemovesLegacyCustomOffEntry(t *testing.T) {
	// A config off-entry recorded before the unified-link model is removed
	// by the one-time cleanup: the custom's lever is now its managed link,
	// so sync must not leave the stale deny behind or write a new one.
	p := fakeHome(t, "opencode")
	before := `{"permission": {"skill": {"my-notes": "deny"}}}`
	writeFile(t, p.OpenCodeConfig(), before)
	collection := filepath.Join(p.FleetReposDir(), "team", "skills")
	writeFile(t, filepath.Join(collection, "my-notes", "SKILL.md"), "---\nname: my-notes\ndescription: notes\n---\n")

	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("my-notes", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := readFile(t, p.OpenCodeConfig()); strings.Contains(got, `"my-notes"`) {
		t.Errorf("legacy custom off-entry survived:\n%s", got)
	}
	// The removal is reported once, and no new custom off-entry replaces it.
	var cleaned int
	for _, r := range reports {
		cleaned += len(r.Cleaned)
	}
	if cleaned != 1 {
		t.Errorf("cleaned = %d, want 1 (the legacy custom deny)", cleaned)
	}
	// Idempotent: a second run finds nothing left to remove.
	reports, err = Run(p)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	for _, r := range reports {
		if len(r.Cleaned) != 0 {
			t.Errorf("%s cleaned %v on the second run, want nothing", r.Harness, r.Cleaned)
		}
	}
}

func TestRunFlagsManualEditsWithoutState(t *testing.T) {
	// The manual-edit drift scenario: no state file, configs hold denies
	// fleet didn't write. Sync leaves every byte alone and flags them.
	p := fakeHome(t, "opencode", "pi")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"manual-skill": "deny"}}}`)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/manual-skill/SKILL.md"]}`)

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantFlags := map[string][]string{
		"opencode": {"disabled in config but not tracked by fleet's state — left alone"},
		"pi":       {"disabled in config but not tracked by fleet's state — left alone"},
	}
	for _, r := range reports {
		want, ok := wantFlags[r.Harness]
		if !ok {
			t.Errorf("unexpected report for %s: %+v", r.Harness, r)
			continue
		}
		var messages []string
		for _, f := range r.Flags {
			messages = append(messages, f.Message)
			if f.Skill != "manual-skill" {
				t.Errorf("%s flag skill = %q, want manual-skill", r.Harness, f.Skill)
			}
		}
		if !reflect.DeepEqual(messages, want) {
			t.Errorf("%s flags = %v, want %v", r.Harness, messages, want)
		}
		if len(r.Changed) != 0 {
			t.Errorf("%s changed = %v, want none (manual edits are untouched)", r.Harness, r.Changed)
		}
	}

	if got := readFile(t, p.OpenCodeConfig()); got != `{"permission": {"skill": {"manual-skill": "deny"}}}` {
		t.Errorf("opencode config was touched:\n%s", got)
	}
	if got := readFile(t, p.PiSettings()); got != `{"skills": ["-skills/manual-skill/SKILL.md"]}` {
		t.Errorf("pi settings were touched:\n%s", got)
	}
}

func TestRunSkipsUninstalledAndNonWritingHarnesses(t *testing.T) {
	// Only cursor is installed, and it has no write side: sync is a no-op.
	p := fakeHome(t, "cursor")
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode") // opencode is not installed
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("reports = %+v, want none", reports)
	}
}

func TestRunNeverEditsTheStateFile(t *testing.T) {
	p := fakeHome(t, "opencode")
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, p.FleetStateFile())

	if _, err := Run(p); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if after := readFile(t, p.FleetStateFile()); after != before {
		t.Errorf("state file changed:\n%s\nwas\n%s", after, before)
	}
}

func TestRunReportsErrorsFromBrokenConfigs(t *testing.T) {
	p := fakeHome(t, "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {`)
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(p); err == nil {
		t.Fatal("Run() succeeded on a broken config, want error")
	}
}

// linkSpam recreates the skills CLI's per-agent links by hand: every
// native-scan harness gets a link to the store skill, claude gets the one
// link it genuinely needs, and bob also gets a repo-style link and a real
// directory fleet must never touch.
func linkSpam(t *testing.T, p *paths.Paths, skill string) {
	t.Helper()
	store := filepath.Join(p.SkillsStore(), skill)
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		filepath.Join(p.OpenCodeSkills(), skill): store,
		filepath.Join(p.PiSkills(), skill):       store,
		filepath.Join(p.CodexSkills(), skill):    store,
		filepath.Join(p.CursorSkills(), skill):   store,
		filepath.Join(p.BobSkills(), skill):      store,
		// Claude is not a native canonical-store reader: load-bearing.
		filepath.Join(p.ClaudeSkills(), skill): store,
		// A repo-pointing custom-skill link (ticket 03's managed links)
		// and a real directory: never fleet's to remove.
		filepath.Join(p.BobSkills(), "my-custom"): filepath.Join(p.Home, "dev", "fleet", "skills", "my-custom"),
	}
	for link, target := range links {
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(p.BobSkills(), "hand-made"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRunRemovesRedundantLinksAndLeavesTheRest(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "codex", "claude", "cursor", "bob")
	linkSpam(t, p, "tdd")

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	removed := map[string][]string{}
	for _, r := range reports {
		for _, e := range r.Removed {
			removed[r.Harness] = append(removed[r.Harness], e.Name)
		}
	}
	want := map[string][]string{
		"opencode": {"tdd"},
		"pi":       {"tdd"},
		"codex":    {"tdd"},
		"cursor":   {"tdd"},
		"bob":      {"tdd"},
	}
	if !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}

	// The links are gone; claude's load-bearing link, the repo-pointing
	// custom link, and the real directory all stay.
	for _, gone := range []string{p.OpenCodeSkills(), p.PiSkills(), p.CodexSkills(), p.CursorSkills()} {
		if _, err := os.Lstat(filepath.Join(gone, "tdd")); !os.IsNotExist(err) {
			t.Errorf("redundant link in %s still there", gone)
		}
	}
	for _, kept := range []string{
		filepath.Join(p.ClaudeSkills(), "tdd"),
		filepath.Join(p.BobSkills(), "my-custom"),
		filepath.Join(p.BobSkills(), "hand-made"),
	} {
		if _, err := os.Lstat(kept); err != nil {
			t.Errorf("fleet removed %s, which it must never touch: %v", kept, err)
		}
	}
}

func TestRunMakesCustomHomesVisible(t *testing.T) {
	// A convention-tracked checkout and the fleet-home fallback each hold a
	// custom skill. Sync links both skills into every installed harness,
	// with no adopt or pull run.
	p := fakeHome(t, "opencode", "pi", "codex", "claude", "cursor", "bob")
	collection := filepath.Join(p.FleetReposDir(), "team", "skills")
	writeFile(t, filepath.Join(collection, "my-notes", "SKILL.md"), "---\nname: my-notes\ndescription: notes\n---\n")
	writeFile(t, filepath.Join(p.FleetHomeSkills(), "scratch", "SKILL.md"), "---\nname: scratch\ndescription: scratch\n---\n")

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	linked := map[string]string{}
	for _, r := range reports {
		for _, l := range r.Linked {
			linked[r.Harness+"/"+l.Name] = l.Target
		}
	}
	for _, h := range []string{"opencode", "pi", "codex", "claude", "cursor", "bob"} {
		if got := linked[h+"/my-notes"]; got != filepath.Join(collection, "my-notes") {
			t.Errorf("%s my-notes target = %q, want the checkout", h, got)
		}
		if got := linked[h+"/scratch"]; got != filepath.Join(p.FleetHomeSkills(), "scratch") {
			t.Errorf("%s scratch target = %q, want the fallback", h, got)
		}
	}
	// The links are on disk, not just in the report.
	if target, err := os.Readlink(filepath.Join(p.OpenCodeSkills(), "my-notes")); err != nil || target != filepath.Join(collection, "my-notes") {
		t.Errorf("opencode link = %q (err %v), want the checkout", target, err)
	}
	if target, err := os.Readlink(filepath.Join(p.BobSkills(), "my-notes")); err != nil || target != filepath.Join(collection, "my-notes") {
		t.Errorf("bob link = %q (err %v), want the checkout", target, err)
	}

	// Idempotent: a second run has nothing to say.
	reports, err = Run(p)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("second run reported %+v, want nothing", reports)
	}
}

func TestRunFirstTrackedHomeWinsCustomLinkCollision(t *testing.T) {
	// The same skill name in two explicit tracked homes: every harness's
	// managed link points at the first home — the listing's winner — not
	// the later, lower-precedence one, and a second sync has nothing left
	// to say.
	p := fakeHome(t, "opencode", "pi", "codex", "claude", "cursor", "bob")
	first, second := t.TempDir(), t.TempDir()
	writeExplicitRepos(t, p, first, second)
	writeFile(t, filepath.Join(first, "skills", "dup", "SKILL.md"), "---\nname: dup\ndescription: first\n---\n")
	writeFile(t, filepath.Join(second, "skills", "dup", "SKILL.md"), "---\nname: dup\ndescription: second\n---\n")

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := filepath.Join(first, "skills", "dup")
	for _, dir := range []string{p.OpenCodeSkills(), p.PiSkills(), p.CodexSkills(), p.ClaudeSkills(), p.CursorSkills(), p.BobSkills()} {
		if got, err := os.Readlink(filepath.Join(dir, "dup")); err != nil || got != want {
			t.Errorf("link in %s = %q, %v; want the first tracked home %q", dir, got, err, want)
		}
	}
	for _, r := range reports {
		for _, l := range r.Linked {
			if l.Name == "dup" && l.Target != want {
				t.Errorf("%s reported dup target %q, want %q", r.Harness, l.Target, want)
			}
		}
	}

	// Idempotent: a second sync reports no link changes.
	reports, err = Run(p)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	for _, r := range reports {
		if len(r.Linked) != 0 {
			t.Errorf("second sync linked %v, want none", r.Linked)
		}
	}
}

func TestRunLeavesUserSkillSourceEntriesAlone(t *testing.T) {
	// A user's own discovery-source entries are not fleet's to touch: no
	// collection path is ever written, and a hand-added entry survives a
	// sync byte for byte while the custom home is linked.
	p := fakeHome(t, "opencode", "pi", "codex")
	ocBefore := `{"skills": {"paths": ["~/.claude/skills"]}}`
	piBefore := `{"skills": ["~/.claude/skills"]}`
	writeFile(t, p.OpenCodeConfig(), ocBefore)
	writeFile(t, p.PiSettings(), piBefore)
	writeFile(t, filepath.Join(p.FleetHomeSkills(), "scratch", "SKILL.md"), "---\nname: scratch\ndescription: scratch\n---\n")

	if _, err := Run(p); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := readFile(t, p.OpenCodeConfig()); got != ocBefore {
		t.Errorf("opencode config changed:\n%s\nwas\n%s", got, ocBefore)
	}
	if got := readFile(t, p.PiSettings()); got != piBefore {
		t.Errorf("pi settings changed:\n%s\nwas\n%s", got, piBefore)
	}
	for _, dir := range []string{p.OpenCodeSkills(), p.PiSkills(), p.CodexSkills()} {
		if got, err := os.Readlink(filepath.Join(dir, "scratch")); err != nil || got != filepath.Join(p.FleetHomeSkills(), "scratch") {
			t.Errorf("link in %s = %q, %v; want the fallback skill", dir, got, err)
		}
	}
}

func TestRunRemovesRedundantLinksWithMissingTargets(t *testing.T) {
	// The skill was uninstalled after the skills CLI linked it: cleanup is
	// still safe and still idempotent. linkSpam's directories make every
	// harness installed, so every native scanner's link goes; claude's
	// stays (broken, and doctor's business — never sync's to delete).
	p := fakeHome(t, "opencode", "bob")
	linkSpam(t, p, "tdd")
	if err := os.RemoveAll(p.SkillsStore()); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	var removed int
	for _, r := range reports {
		removed += len(r.Removed)
	}
	if removed != 5 {
		t.Errorf("removed %d links, want 5 (every native scanner)", removed)
	}
	if _, err := os.Lstat(filepath.Join(p.OpenCodeSkills(), "tdd")); !os.IsNotExist(err) {
		t.Error("opencode link with missing target not removed")
	}
	if _, err := os.Lstat(filepath.Join(p.ClaudeSkills(), "tdd")); err != nil {
		t.Errorf("claude's load-bearing link was removed: %v", err)
	}
}

func TestRunLinkCleanupIsIdempotent(t *testing.T) {
	p := fakeHome(t, "opencode", "bob")
	linkSpam(t, p, "tdd")

	if _, err := Run(p); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	reports, err := Run(p)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	for _, r := range reports {
		if len(r.Removed) != 0 {
			t.Errorf("%s removed %v on the second run, want nothing", r.Harness, r.Removed)
		}
	}
}

// scanComplete creates the canonical store so the skill index reports a
// complete scan: an empty store scans cleanly, and the absent fleet-home
// fallback is optional. Without it, "installed nowhere" cannot be trusted.
func scanComplete(t *testing.T, p *paths.Paths) {
	t.Helper()
	if err := os.MkdirAll(p.SkillsStore(), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRunLeavesDormantDisableUnprojected(t *testing.T) {
	// A state disable whose skill is installed nowhere is dormant: the
	// state keeps the intent, but sync creates no new off entry in any
	// config lever. The store exists, so the scan is complete and the
	// "installed nowhere" answer is trusted.
	p := fakeHome(t, "opencode", "pi", "codex", "claude")
	scanComplete(t, p)
	writeFile(t, p.OpenCodeConfig(), "{\n  \"model\": \"gpt-5\"\n}\n")
	writeFile(t, p.PiSettings(), "{}\n")
	writeFile(t, p.CodexConfig(), "# codex\n")
	// Claude can only disable a skill it discovers, so give it a directory
	// for ghost. Without dormancy it would write a skillOverrides entry.
	if err := os.MkdirAll(filepath.Join(p.ClaudeSkills(), "ghost"), 0o755); err != nil {
		t.Fatal(err)
	}

	before := map[string]string{
		p.OpenCodeConfig(): readFile(t, p.OpenCodeConfig()),
		p.PiSettings():     readFile(t, p.PiSettings()),
		p.CodexConfig():    readFile(t, p.CodexConfig()),
	}

	st, _ := state.Load(p.FleetStateFile())
	for _, h := range []string{"opencode", "pi", "codex", "claude"} {
		st.SetDisabled("ghost", h)
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
	stateBefore := readFile(t, p.FleetStateFile())

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("reports = %+v, want none for a dormant disable", reports)
	}
	for path, want := range before {
		if got := readFile(t, path); got != want {
			t.Errorf("%s changed:\n%s\nwas\n%s", path, got, want)
		}
	}
	if _, err := os.Stat(p.ClaudeSettings()); !os.IsNotExist(err) {
		t.Errorf("claude settings created for a dormant disable: %v", err)
	}
	if got := readFile(t, p.FleetStateFile()); got != stateBefore {
		t.Errorf("state file changed:\n%s\nwas\n%s", got, stateBefore)
	}
}

func TestRunLeavesDormantDisableUnprojectedInOpenCodeV2(t *testing.T) {
	// The same dormancy in opencode's V2 dialect: the permissions array
	// gains no deny rule.
	p := fakeHome(t, "opencode")
	scanComplete(t, p)
	writeFile(t, p.OpenCodeConfig(), "{\n  \"permissions\": []\n}\n")
	before := readFile(t, p.OpenCodeConfig())

	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("ghost", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("reports = %+v, want none for a dormant disable", reports)
	}
	if got := readFile(t, p.OpenCodeConfig()); got != before {
		t.Errorf("opencode v2 config changed:\n%s\nwas\n%s", got, before)
	}
}

func TestRunLeavesExistingDisableForUninstalledSkillAlone(t *testing.T) {
	// The state still owns the name, so a rule already written into a
	// config is left byte-for-byte and not flagged as untracked.
	p := fakeHome(t, "opencode", "pi", "codex", "claude")
	scanComplete(t, p)
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md"]}`)
	writeFile(t, p.CodexConfig(), "[[skills.config]]\nname = \"ghost\"\nenabled = false\n")
	writeFile(t, p.ClaudeSettings(), `{"skillOverrides": {"ghost": "off"}}`)

	before := map[string]string{
		p.OpenCodeConfig(): readFile(t, p.OpenCodeConfig()),
		p.PiSettings():     readFile(t, p.PiSettings()),
		p.CodexConfig():    readFile(t, p.CodexConfig()),
		p.ClaudeSettings(): readFile(t, p.ClaudeSettings()),
	}

	st, _ := state.Load(p.FleetStateFile())
	for _, h := range []string{"opencode", "pi", "codex", "claude"} {
		st.SetDisabled("ghost", h)
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("reports = %+v, want none: an existing rule the state owns stays put", reports)
	}
	for path, want := range before {
		if got := readFile(t, path); got != want {
			t.Errorf("%s changed:\n%s\nwas\n%s", path, got, want)
		}
	}
}

func TestRunStillFlagsUntrackedDisableAlongsideDormant(t *testing.T) {
	// A dormant disable must not silence a rule the state does not track:
	// only the state-owned name is exempt from the sweep.
	p := fakeHome(t, "pi")
	scanComplete(t, p)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md", "-skills/manual/SKILL.md"]}`)
	before := readFile(t, p.PiSettings())

	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("ghost", "pi")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	var flagged []string
	for _, r := range reports {
		if len(r.Changed) != 0 {
			t.Errorf("%s changed %v, want none", r.Harness, r.Changed)
		}
		for _, f := range r.Flags {
			flagged = append(flagged, f.Skill)
		}
	}
	if !reflect.DeepEqual(flagged, []string{"manual"}) {
		t.Errorf("flagged = %v, want only the untracked manual disable", flagged)
	}
	if got := readFile(t, p.PiSettings()); got != before {
		t.Errorf("pi settings changed:\n%s\nwas\n%s", got, before)
	}
}

func TestRunProjectsDisableWhenScanIncomplete(t *testing.T) {
	// The store is absent, so the scan is incomplete and "installed
	// nowhere" cannot be trusted: sync projects every disable rather than
	// risk leaving an installed skill enabled.
	p := fakeHome(t, "pi")
	writeFile(t, p.PiSettings(), "{}\n")
	// no canonical store

	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("ghost", "pi")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(p); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := readFile(t, p.PiSettings()); !strings.Contains(got, "-skills/ghost/SKILL.md") {
		t.Errorf("pi settings missing the fallback projection:\n%s", got)
	}
}

func TestRunProjectsDisableWhenTrackedRepoIsMissing(t *testing.T) {
	// A tracked repo root recorded in config but missing from disk makes
	// the scan incomplete even though the store exists.
	p := fakeHome(t, "pi")
	scanComplete(t, p)
	writeFile(t, p.FleetConfigFile(), `{"skillsRepos": ["`+filepath.Join(p.Home, "gone")+`"]}`)
	writeFile(t, p.PiSettings(), "{}\n")

	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("ghost", "pi")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(p); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := readFile(t, p.PiSettings()); !strings.Contains(got, "-skills/ghost/SKILL.md") {
		t.Errorf("pi settings missing the fallback projection:\n%s", got)
	}
}
