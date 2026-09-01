package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/state"
)

// fakeHome builds a home with the given harnesses' config directories
// present, returning the Paths and the harness names installed.
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
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
